package testsupport

import (
	"bytes"
	"encoding/binary"
	"io"
	"os"
)

// Reference walks the directory by byte length, ignoring end-record entry counts.
// Give archive/zip a corrected count only after walking that complete directory.
// The source stays open and unmodified. ZIP64 normalization is bounded to
// fixed, single-disk end records and fewer than 65535 directory entries.
type mutationDirectorySource interface {
	io.ReaderAt
	Stat() (os.FileInfo, error)
}

type mutationDirectoryView struct {
	mutationDirectorySource
	at  int64
	end []byte
}

func (v *mutationDirectoryView) ReadAt(p []byte, off int64) (int, error) {
	n, err := v.mutationDirectorySource.ReadAt(p, off)
	for i := 0; i < n; i++ {
		if a := off + int64(i) - v.at; a >= 0 && a < int64(len(v.end)) {
			p[i] = v.end[a]
		}
	}
	return n, err
}
func mutationClassicDirectory(source mutationDirectorySource, size int64) io.ReaderAt {
	n := min(size, int64(65557))
	if n < 22 {
		return source
	}
	tail := make([]byte, n)
	if _, err := source.ReadAt(tail, size-n); err != nil {
		return source
	}
	e := len(tail) - 22
	if !bytes.Equal(tail[e:e+4], []byte("PK\x05\x06")) || binary.LittleEndian.Uint16(tail[e+20:]) != 0 {
		e = bytes.LastIndex(tail, []byte("PK\x05\x06"))
	}
	if e < 0 || e+22 > len(tail) {
		return source
	}
	end := append([]byte(nil), tail[e:e+22]...)
	at := size - n + int64(e)
	// A fixed, single-disk ZIP64 end record carries counts too, but Reference
	// still walks by directory byte length. Normalize only the reader view.
	length, offset := uint64(binary.LittleEndian.Uint32(end[12:])), uint64(binary.LittleEndian.Uint32(end[16:]))
	boundary := at
	zip64 := false
	var locator [20]byte
	if at >= 20 {
		source.ReadAt(locator[:], at-20)
		if bytes.Equal(locator[:4], []byte("PK\x06\x07")) {
			if at < 76 || binary.LittleEndian.Uint32(locator[4:]) != 0 || binary.LittleEndian.Uint32(locator[16:]) > 1 {
				return source
			}
			record := make([]byte, 56)
			if _, err := source.ReadAt(record, at-76); err != nil || !bytes.Equal(record[:4], []byte("PK\x06\x06")) || binary.LittleEndian.Uint64(record[4:]) != 44 || binary.LittleEndian.Uint32(record[16:]) != 0 || binary.LittleEndian.Uint32(record[20:]) != 0 {
				return source
			}
			length, offset = binary.LittleEndian.Uint64(record[40:]), binary.LittleEndian.Uint64(record[48:])
			boundary = at - 76
			if offset > uint64(boundary) || length > uint64(boundary)-offset || binary.LittleEndian.Uint64(locator[8:]) != offset+length {
				return source
			}
			end, at, zip64 = record, boundary, true
		}
	}
	if (!zip64 && (length == 0xffffffff || offset == 0xffffffff)) || offset > uint64(boundary) || length > uint64(boundary)-offset {
		return source
	}
	pos := boundary - int64(length)
	count := 0
	for pos < at {
		var h [46]byte
		if at-pos < 46 {
			return source
		}
		if _, err := source.ReadAt(h[:], pos); err != nil || !bytes.Equal(h[:4], []byte("PK\x01\x02")) {
			return source
		}
		pos += 46 + int64(binary.LittleEndian.Uint16(h[28:])) + int64(binary.LittleEndian.Uint16(h[30:])) + int64(binary.LittleEndian.Uint16(h[32:]))
		count++
		if pos > at || count >= 65535 {
			return source
		}
	}
	if zip64 {
		binary.LittleEndian.PutUint64(end[24:], uint64(count))
		binary.LittleEndian.PutUint64(end[32:], uint64(count))
	} else {
		if int(binary.LittleEndian.Uint16(end[10:])) == count {
			return source
		}
		binary.LittleEndian.PutUint16(end[8:], uint16(count))
		binary.LittleEndian.PutUint16(end[10:], uint16(count))
	}
	return &mutationDirectoryView{source, at, end}
}
