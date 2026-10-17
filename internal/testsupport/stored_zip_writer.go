package testsupport

import (
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"io"
	"strings"
	"time"
	"unicode/utf8"
)

// Reference's writestr Store path uses known sizes, Unix permissions, local DOS
// timestamps and no data descriptors. archive/zip's normal writer instead uses
// descriptors and refuses data for directory names. Keep this test oracle
// adapter separate from production OPC serialization.
const referenceZIP64Limit = uint64(1<<31 - 1)

type referenceStoreRecord struct {
	name                        string
	flags, stamp, date, version uint16
	crc                         uint32
	size, offset                uint64
}

func writeReferenceStoredZIP(dst io.Writer, parts *OrderedZIP, now func() time.Time) (err error) {
	return writeReferenceStoredZIPWithWarnings(dst, parts, now, nil)
}

func writeReferenceStoredZIPWithWarnings(dst io.Writer, parts *OrderedZIP, now func() time.Time, warn func(string) error) (err error) {
	var position uint64
	var startDir uint64
	seeker, seekable := dst.(io.Seeker)
	seek := func(offset uint64) error {
		p, e := seeker.Seek(int64(offset), io.SeekStart)
		if e == nil {
			position = uint64(p)
		}
		return e
	}
	// Match the seekable writer's initial tell/seek and preserve a nonzero origin.
	// Failure to establish seekability is explicit here; nonseekable descriptor
	// fallback is a separate, still-unproven contract.
	if seekable {
		p, e := seeker.Seek(0, io.SeekCurrent)
		if e != nil {
			return e
		}
		if p < 0 {
			return fmt.Errorf("negative ZIP output position")
		}
		position, startDir = uint64(p), uint64(p)
		if e := seek(startDir); e != nil {
			return e
		}
	}
	failed := false
	write := func(b []byte) error {
		n, e := dst.Write(b)
		position += uint64(n)
		if e == nil && n != len(b) {
			e = io.ErrShortWrite
		}
		if e != nil {
			failed = true
		}
		return e
	}
	records := []referenceStoreRecord{}
	seen := map[string]bool{}
	// Seekable destinations follow ZipFile.__exit__ recovery at start_dir.
	// Streaming destinations retain their bounded stop-on-write-error contract.
	// Go short-write errors remain explicit; Reference may silently accept them.
	defer func() {
		if seekable {
			if e := seek(startDir); e != nil {
				err = e
				return
			}
		} else if failed {
			return
		}
		if e := finishReferenceStoredZIP(write, &position, records); e != nil {
			err = e
			return
		}
		// ZipFile flushes only after completing the directory. This also runs
		// after recoverable member faults; a flush error supersedes that fault.
		// os.File writes are unbuffered here; Sync would change the contract.
		if f, ok := dst.(interface{ Flush() error }); ok {
			if e := f.Flush(); e != nil {
				err = e
			}
		}
	}()
	for _, part := range parts.Parts {
		name := referenceZIPMemberName(part.Name)
		if !utf8.ValidString(name) {
			return fmt.Errorf("invalid UTF-8 ZIP member name")
		}
		if len(name) > 65535 {
			return fmt.Errorf("ZIP member name exceeds uint16 length")
		}
		date := now().Local()
		if date.Year() < 1980 || date.Year() > 2107 {
			return fmt.Errorf("ZIP timestamp year outside DOS range")
		}
		r := referenceStoreRecord{name: name, version: 20, size: uint64(len(part.Data)), offset: position, crc: crc32.ChecksumIEEE([]byte(part.Data)), stamp: uint16(date.Hour()<<11 | date.Minute()<<5 | date.Second()/2), date: uint16((date.Year()-1980)<<9 | int(date.Month())<<5 | date.Day())}
		for _, b := range []byte(name) {
			if b >= 128 {
				r.flags = 0x800
				break
			}
		}
		// ZipFile._open_to_write reserves ZIP64 when the known size plus its
		// compression allowance exceeds ZIP64_LIMIT, even with ZIP_STORED.
		zip64 := float64(r.size)*1.05 > float64(referenceZIP64Limit)
		extra := []byte{}
		size := uint32(r.size)
		if zip64 {
			r.version = 45
			size = 0xffffffff
			extra = zip64Extra(r.size, r.size)
		}
		if seekable {
			if e := seek(startDir); e != nil {
				return e
			}
			// ZipFile obtains header_offset with tell(), independently of seek().
			p, e := seeker.Seek(0, io.SeekCurrent)
			if e != nil {
				return e
			}
			if p < 0 {
				return fmt.Errorf("negative ZIP output position")
			}
			position, r.offset = uint64(p), uint64(p)
		}
		if seen[name] && warn != nil {
			if e := warn("Duplicate name: " + mutationNameRepr(name)); e != nil {
				return e
			}
		}
		seen[name] = true
		h := make([]byte, 30)
		binary.LittleEndian.PutUint32(h, 0x04034b50)
		binary.LittleEndian.PutUint16(h[4:], r.version)
		binary.LittleEndian.PutUint16(h[6:], r.flags)
		binary.LittleEndian.PutUint16(h[10:], r.stamp)
		binary.LittleEndian.PutUint16(h[12:], r.date)
		binary.LittleEndian.PutUint32(h[14:], r.crc)
		binary.LittleEndian.PutUint32(h[18:], size)
		binary.LittleEndian.PutUint32(h[22:], size)
		binary.LittleEndian.PutUint16(h[26:], uint16(len(name)))
		binary.LittleEndian.PutUint16(h[28:], uint16(len(extra)))
		if seekable {
			// ZipFile writes a provisional header, then rewrites it even when
			// writing the payload raised. Archive close uses the last start_dir.
			header := append(append(h, []byte(name)...), extra...)
			provisional := append([]byte(nil), header...)
			binary.LittleEndian.PutUint32(provisional[14:], 0)
			if !zip64 {
				binary.LittleEndian.PutUint32(provisional[18:], 0)
			} else {
				binary.LittleEndian.PutUint64(provisional[30+len(name)+12:], 0)
			}
			if e := write(provisional); e != nil {
				return e
			}
			payloadErr := write([]byte(part.Data))
			// tell() matters after a writer advances its cursor before failing.
			p, e := seeker.Seek(0, io.SeekCurrent)
			if e != nil {
				return e
			}
			startDir = uint64(p)
			if e := seek(r.offset); e != nil {
				return e
			}
			if e := write(header); e != nil {
				return e
			}
			if e := seek(startDir); e != nil {
				return e
			}
			records = append(records, r)
			if payloadErr != nil {
				return payloadErr
			}
			continue
		}
		for _, b := range [][]byte{h, []byte(name), extra, []byte(part.Data)} {
			if e := write(b); e != nil {
				return e
			}
		}
		records = append(records, r)
	}
	return nil
}

func zip64Extra(values ...uint64) []byte {
	if len(values) == 0 {
		return nil
	}
	b := make([]byte, 4+8*len(values))
	binary.LittleEndian.PutUint16(b, 1)
	binary.LittleEndian.PutUint16(b[2:], uint16(8*len(values)))
	for i, v := range values {
		binary.LittleEndian.PutUint64(b[4+i*8:], v)
	}
	return b
}

func finishReferenceStoredZIP(write func([]byte) error, position *uint64, records []referenceStoreRecord) error {
	start := *position
	for _, r := range records {
		size, offset := uint32(r.size), uint32(r.offset)
		values := []uint64{}
		version := r.version
		if r.size > referenceZIP64Limit {
			size = 0xffffffff
			values = append(values, r.size, r.size)
		}
		if r.offset > referenceZIP64Limit {
			offset = 0xffffffff
			values = append(values, r.offset)
		}
		extra := zip64Extra(values...)
		if len(extra) > 0 {
			version = 45
		}
		h := make([]byte, 46)
		binary.LittleEndian.PutUint32(h, 0x02014b50)
		binary.LittleEndian.PutUint16(h[4:], 3<<8|version)
		binary.LittleEndian.PutUint16(h[6:], version)
		binary.LittleEndian.PutUint16(h[8:], r.flags)
		binary.LittleEndian.PutUint16(h[12:], r.stamp)
		binary.LittleEndian.PutUint16(h[14:], r.date)
		binary.LittleEndian.PutUint32(h[16:], r.crc)
		binary.LittleEndian.PutUint32(h[20:], size)
		binary.LittleEndian.PutUint32(h[24:], size)
		binary.LittleEndian.PutUint16(h[28:], uint16(len(r.name)))
		binary.LittleEndian.PutUint16(h[30:], uint16(len(extra)))
		permissions := uint32(0600 << 16)
		if strings.HasSuffix(r.name, "/") {
			permissions = 040775<<16 | 0x10
		}
		binary.LittleEndian.PutUint32(h[38:], permissions)
		binary.LittleEndian.PutUint32(h[42:], offset)
		for _, b := range [][]byte{h, []byte(r.name), extra} {
			if err := write(b); err != nil {
				return err
			}
		}
	}
	size := *position - start
	count := uint64(len(records))
	if count > 65535 || start > referenceZIP64Limit || size > referenceZIP64Limit {
		endOffset := *position
		h := make([]byte, 56)
		binary.LittleEndian.PutUint32(h, 0x06064b50)
		binary.LittleEndian.PutUint64(h[4:], 44)
		binary.LittleEndian.PutUint16(h[12:], 45)
		binary.LittleEndian.PutUint16(h[14:], 45)
		binary.LittleEndian.PutUint64(h[24:], count)
		binary.LittleEndian.PutUint64(h[32:], count)
		binary.LittleEndian.PutUint64(h[40:], size)
		binary.LittleEndian.PutUint64(h[48:], start)
		if err := write(h); err != nil {
			return err
		}
		locator := make([]byte, 20)
		binary.LittleEndian.PutUint32(locator, 0x07064b50)
		binary.LittleEndian.PutUint64(locator[8:], endOffset)
		binary.LittleEndian.PutUint32(locator[16:], 1)
		if err := write(locator); err != nil {
			return err
		}
	}
	end := make([]byte, 22)
	binary.LittleEndian.PutUint32(end, 0x06054b50)
	binary.LittleEndian.PutUint16(end[8:], uint16(min(count, 65535)))
	binary.LittleEndian.PutUint16(end[10:], uint16(min(count, 65535)))
	binary.LittleEndian.PutUint32(end[12:], uint32(min(size, 0xffffffff)))
	binary.LittleEndian.PutUint32(end[16:], uint32(min(start, 0xffffffff)))
	return write(end)
}
