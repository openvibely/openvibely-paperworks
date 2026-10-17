package opc

import (
	"archive/zip"
	"bytes"
	"fmt"
	"hash/crc32"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/openvibely/openvibely-paperworks/internal/zlibcompat"
)

// DocumentBytes serializes a newly authored DOCX like the pinned reference DOCX library
// seekable writer. Its exact compression path supports members up to64 MiB.
// Larger parts retain the existing OPC writer, without an exact-byte claim.
// Callers supply a clock; production uses time.Now and captures can fix it.
func (p *Package) DocumentBytes(clock func() time.Time) ([]byte, error) {
	for _, part := range p.Parts {
		if len(part.Data) > 64<<20 {
			return p.Bytes()
		}
	}
	return ReferenceZIPBytes(p, clock, zlibcompat.DeflateZIPLevel6)
}

// ReferenceZIPBytes writes classic seekable ZIP metadata using the supplied
// compressor and wall clock. It is shared by new DOCX authoring and the bounded
// reference fixture adapters; existing-package mutation retains Package.Bytes.
func ReferenceZIPBytes(p *Package, clock func() time.Time, compress func([]byte) ([]byte, error)) ([]byte, error) {
	return ReferenceZIPBytesWithAttrs(p, func() time.Time { return clock().In(time.Local) }, compress, nil)
}

// ReferenceZIPBytesWithAttrs also supports ZipFile.write's file-mode attributes.
func ReferenceZIPBytesWithAttrs(p *Package, clock func() time.Time, compress func([]byte) ([]byte, error), attrs func(string) uint32) ([]byte, error) {
	var out bytes.Buffer
	zw := zip.NewWriter(&out)
	// Reference starts ZIP64 well below uint32 overflow. This adapter deliberately
	// rejects those archives instead of claiming that CreateRaw matches that path.
	if len(p.Parts) >= 65535 {
		return nil, fmt.Errorf("Python ZIP requires classic ZIP member count")
	}
	for _, part := range p.Parts {
		if len(part.Name) == 0 || len(part.Name) > 65535 || !utf8.ValidString(part.Name) || strings.ContainsRune(part.Name, 0) || strings.HasSuffix(part.Name, "/") || len(part.Data) > 1<<30 {
			return nil, fmt.Errorf("unsupported Python ZIP member: %q", part.Name)
		}
		now := clock()
		if now.Year() < 1980 || now.Year() > 2107 {
			return nil, fmt.Errorf("Python ZIP timestamp outside DOS range")
		}
		compressed, err := compress(part.Data)
		if err != nil {
			return nil, fmt.Errorf("Python ZIP member %q: %w", part.Name, err)
		}

		h := zip.FileHeader{Name: part.Name, Method: zip.Deflate, CreatorVersion: 3<<8 | 20, ReaderVersion: 20, ExternalAttrs: 0600 << 16, CRC32: crc32.ChecksumIEEE(part.Data), UncompressedSize64: uint64(len(part.Data)), CompressedSize64: uint64(len(compressed))}
		if attrs != nil {
			h.ExternalAttrs = attrs(part.Name)
		}
		for _, b := range []byte(part.Name) {
			if b >= 128 {
				h.Flags = 0x800
				break
			}
		}
		h.ModifiedDate = uint16((now.Year()-1980)<<9 | int(now.Month())<<5 | now.Day())
		h.ModifiedTime = uint16(now.Hour()<<11 | now.Minute()<<5 | now.Second()/2)
		w, err := zw.CreateRaw(&h)
		if err != nil {
			return nil, err
		}
		if _, err = w.Write(compressed); err != nil {
			return nil, err
		}
		if out.Len() > 1<<30 {
			return nil, fmt.Errorf("Python ZIP exceeds bounded archive size")
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}
