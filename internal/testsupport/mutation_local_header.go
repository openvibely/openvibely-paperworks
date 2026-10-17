package testsupport

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"sort"
	"unicode/utf8"

	"golang.org/x/text/encoding/charmap"
)

type mutationLocalRange struct{ offset, end int64 }

// mutationLocalNames checks selected local names in bounded single-disk ZIP layouts.
// OpenRaw's SectionReader exposes the same open ReaderAt via Outer; no path is
// reopened. Classic and ZIP64 directories are cross-checked against the entries
// already parsed by archive/zip. Unrecognized layouts retain the prior path;
// this is not a general ZIP validation implementation.
func mutationLocalNames(z *zip.Reader, selected *zip.File) (map[*zip.File]mutationLocalRange, io.ReaderAt, error) {
	raw, err := selected.OpenRaw()
	if err != nil {
		return nil, nil, err
	}
	section, ok := raw.(*io.SectionReader)
	if !ok {
		return nil, nil, nil
	}
	source, _, _ := section.Outer()
	statter, ok := source.(interface{ Stat() (os.FileInfo, error) })
	if !ok {
		return nil, nil, nil
	}
	info, err := statter.Stat()
	if err != nil {
		return nil, nil, err
	}
	size := info.Size()
	n := min(size, int64(65557))
	if n < 22 {
		return nil, nil, nil
	}
	tail := make([]byte, n)
	if _, err = source.ReadAt(tail, size-n); err != nil {
		return nil, nil, err
	}
	// Match pinned zipfile._EndRecData: a no-comment final record is a fast
	// path; otherwise use the last signature in the bounded tail. The claimed
	// comment need not consume the rest of the file (trailing bytes are allowed).
	e := len(tail) - 22
	if !bytes.Equal(tail[e:e+4], []byte("PK\x05\x06")) || binary.LittleEndian.Uint16(tail[e+20:]) != 0 {
		e = bytes.LastIndex(tail, []byte("PK\x05\x06"))
	}
	if e < 0 || e+22 > len(tail) {
		return nil, nil, nil
	}
	end := tail[e:]
	u16 := binary.LittleEndian.Uint16
	u32 := binary.LittleEndian.Uint32
	u64 := binary.LittleEndian.Uint64
	length := int64(u32(end[12:]))
	start := int64(u32(end[16:]))
	endOffset := size - n + int64(e)
	var locator [20]byte
	if endOffset >= 20 {
		if _, err = source.ReadAt(locator[:], endOffset-20); err != nil {
			return nil, nil, err
		}
	}
	if bytes.Equal(locator[:4], []byte("PK\x06\x07")) {
		// _EndRecData64 first tries the locator's offset, then the fixed
		// record immediately before it for archives with prepended bytes.
		limit := endOffset - 20 - 56
		rel := u64(locator[8:])
		if limit < 0 || rel > uint64(limit) || u32(locator[4:]) != 0 || u32(locator[16:]) > 1 {
			return nil, nil, nil
		}
		at := int64(rel)
		var z64 [56]byte
		if _, err = source.ReadAt(z64[:], at); err != nil {
			return nil, nil, err
		}
		if !bytes.Equal(z64[:4], []byte("PK\x06\x06")) && at != limit {
			at = limit
			if _, err = source.ReadAt(z64[:], at); err != nil {
				return nil, nil, err
			}
		}
		if !bytes.Equal(z64[:4], []byte("PK\x06\x06")) || u64(z64[4:]) != uint64(endOffset-20-at-12) || u32(z64[16:]) != 0 || u32(z64[20:]) != 0 {
			return nil, nil, nil
		}
		// Bound before conversion/addition, including malicious 64-bit fields.
		l, s := u64(z64[40:]), u64(z64[48:])
		if l > rel || s != rel-l {
			return nil, nil, nil
		}
		length, start = int64(l), int64(s)
		endOffset = at
	} else if length == 0xffffffff || start == 0xffffffff {
		return nil, nil, nil
	}
	base := endOffset - length - start
	if base < 0 {
		return nil, nil, nil
	}
	offsets := make(map[*zip.File]mutationLocalRange, len(z.File))
	pos := start + base
	for _, f := range z.File {
		var h [46]byte
		if _, err = source.ReadAt(h[:], pos); err != nil {
			return nil, nil, err
		}
		if !bytes.Equal(h[:4], []byte("PK\x01\x02")) {
			return nil, nil, nil
		}
		nameLen, extraLen, commentLen := int64(u16(h[28:])), int64(u16(h[30:])), int64(u16(h[32:]))
		offset := uint64(u32(h[42:]))
		if offset == 0xffffffff {
			// ZIP64 extra values occur only for corresponding sentinel fields,
			// in uncompressed-size, compressed-size, local-offset order.
			extra := make([]byte, extraLen)
			if _, err = source.ReadAt(extra, pos+46+nameLen); err != nil {
				return nil, nil, err
			}
			found := false
			for len(extra) >= 4 {
				tag, n := u16(extra), int(u16(extra[2:]))
				if n > len(extra)-4 {
					return nil, nil, nil
				}
				if tag == 1 {
					skip := 0
					if u32(h[24:]) == 0xffffffff {
						skip += 8
					}
					if u32(h[20:]) == 0xffffffff {
						skip += 8
					}
					if n < skip+8 {
						return nil, nil, nil
					}
					offset = u64(extra[4+skip:])
					found = true
					break
				}
				extra = extra[4+n:]
			}
			if !found {
				return nil, nil, nil
			}
		}
		if offset > uint64(size-base) {
			return nil, nil, nil
		}
		name := make([]byte, nameLen)
		if _, err = source.ReadAt(name, pos+46); err != nil {
			return nil, nil, err
		}
		if string(name) != f.Name || u16(h[8:]) != f.Flags || u16(h[10:]) != f.Method || u32(h[16:]) != f.CRC32 {
			return nil, nil, nil
		}
		offsets[f] = mutationLocalRange{offset: base + int64(offset)}
		pos += 46 + nameLen + extraLen + commentLen
	}
	if pos != endOffset {
		return nil, nil, nil
	}
	// ZipFile._RealGetContents stably sorts physical offsets, then assigns
	// each entry the next local header (or central directory) as its boundary.
	// Include shadowed members: their data is not read but their offsets count.
	physical := append([]*zip.File(nil), z.File...)
	sort.SliceStable(physical, func(i, j int) bool { return offsets[physical[i]].offset < offsets[physical[j]].offset })
	boundary := start + base
	for i := len(physical) - 1; i >= 0; i-- {
		f := physical[i]
		local := offsets[f]
		local.end = boundary
		offsets[f] = local
		boundary = local.offset
	}
	return offsets, source, nil
}

func validateMutationLocalHeader(f *zip.File, source io.ReaderAt, offset int64) error {
	var h [30]byte
	if _, err := source.ReadAt(h[:], offset); err != nil {
		return err
	}
	if !bytes.Equal(h[:4], []byte("PK\x03\x04")) {
		return zip.ErrFormat
	}
	name := make([]byte, int(binary.LittleEndian.Uint16(h[26:])))
	if _, err := source.ReadAt(name, offset+30); err != nil {
		return err
	}
	// ZipFile.open checks patch/strong-encryption flags after acquiring the
	// local header/name bytes, but before decoding or comparing that name.
	if err := validateMutationFlags(f); err != nil {
		return err
	}
	decode := func(raw string, flags uint16) (string, error) {
		if flags&0x800 != 0 {
			if !utf8.ValidString(raw) {
				return "", fmt.Errorf("invalid local UTF-8 name: %w", zip.ErrFormat)
			}
			return raw, nil
		}
		return charmap.CodePage437.NewDecoder().String(raw)
	}
	local, err := decode(string(name), binary.LittleEndian.Uint16(h[6:]))
	if err != nil {
		return err
	}
	central, err := decode(f.Name, f.Flags)
	if err != nil {
		return err
	}
	// ZipFile.open compares orig_filename, before NUL truncation of the key.
	if local != central {
		return fmt.Errorf("local and central ZIP names differ: %w", zip.ErrFormat)
	}
	return nil
}

// validateMutationOverlap uses the declared compressed size before decoding,
// even if a decoder would stop early or slice to a shorter uncompressed size.
func validateMutationOverlap(f *zip.File, source io.ReaderAt, local mutationLocalRange) error {
	return validateMutationOverlapWithWarnings(f, source, local, nil)
}
func validateMutationOverlapWithWarnings(f *zip.File, source io.ReaderAt, local mutationLocalRange, warn func(string) error) error {
	var h [30]byte
	if _, err := source.ReadAt(h[:], local.offset); err != nil {
		return err
	}
	start := local.offset + 30 + int64(binary.LittleEndian.Uint16(h[26:])) + int64(binary.LittleEndian.Uint16(h[28:]))
	// Pinned Reference warns rather than raising for an identical-offset entry.
	// Delivery and promotion follow the call-local policy.
	if local.end == local.offset {
		if warn != nil {
			name := f.Name
			if f.Flags&0x800 == 0 {
				name, _ = charmap.CodePage437.NewDecoder().String(name)
			}
			return warn("Overlapped entries: " + mutationNameRepr(name) + " (possible zip bomb)")
		}
		return nil
	}
	if start > local.end || f.CompressedSize64 > uint64(local.end-start) {
		return fmt.Errorf("overlapped ZIP member %q: %w", f.Name, zip.ErrFormat)
	}
	return nil
}

// Selected central flags control acquisition. Local copies of these flags are
// ignored, and shadowed duplicate entries never reach this validation.
func validateMutationFlags(f *zip.File) error {
	if f.Flags&0x20 != 0 {
		return fmt.Errorf("compressed ZIP patches unsupported: %w", zip.ErrAlgorithm)
	}
	if f.Flags&0x40 != 0 {
		return fmt.Errorf("strong ZIP encryption unsupported: %w", zip.ErrAlgorithm)
	}
	return nil
}
