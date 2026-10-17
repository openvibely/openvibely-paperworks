package testsupport

import (
	"archive/zip"
	"bytes"
	"compress/flate"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/text/encoding/charmap"
)

// Dict assignment keeps a key's position; deleting then assigning appends it.
// Reading duplicate ZIP names uses the last value at the first key position.
type ZIPPart struct{ Name, Data string }

type OrderedZIP struct{ Parts []ZIPPart }

func (p *OrderedZIP) Set(name string, data []byte) {
	for i := range p.Parts {
		if p.Parts[i].Name == name {
			p.Parts[i].Data = string(data)
			return
		}
	}
	p.Parts = append(p.Parts, ZIPPart{name, string(data)})
}
func (p *OrderedZIP) Remove(name string) {
	for i := range p.Parts {
		if p.Parts[i].Name == name {
			p.Parts = append(p.Parts[:i], p.Parts[i+1:]...)
			return
		}
	}
}

// MutateStoredZIP takes ownership of z, following PresentationNative.mutate:
// acquire all parts, close the source, run the callback, allocate the destination,
// then open and stream the archive. Acquisition, close and callback failures do
// not allocate an output. ReadOrderedZIP and EditStoredZIP retain caller ownership.
func MutateStoredZIP(z *zip.ReadCloser, transform func(*OrderedZIP) error, output func() string) (string, error) {
	return mutateOrderedZIP(&z.Reader, z.Close, transform, output)
}

func mutateOrderedZIP(z *zip.Reader, closeSource func() error, transform func(*OrderedZIP) error, output func() string) (string, error) {
	return mutateOrderedZIPWithWarnings(z, closeSource, transform, output, nil)
}

func mutateOrderedZIPWithWarnings(z *zip.Reader, closeSource func() error, transform func(*OrderedZIP) error, output func() string, warn func(string) error) (string, error) {
	p, err := readOrderedZIPWithWarnings(z, warn)
	// Reference leaves the source ZipFile context before edit(parts), even when
	// dictionary acquisition raises. A close failure takes precedence.
	if closeErr := closeSource(); closeErr != nil {
		return "", closeErr
	}
	if err != nil {
		return "", err
	}
	if err := transform(p); err != nil {
		return "", err
	}
	out := output()
	if err := saveStoredZIPWithWarnings(out, p, warn); err != nil {
		return "", err
	}
	return out, nil
}

func EditStoredZIP(z *zip.ReadCloser, transform func(*OrderedZIP) error) ([]byte, error) {
	p, err := ReadOrderedZIP(z)
	if err != nil {
		return nil, err
	}
	if err := transform(p); err != nil {
		return nil, err
	}
	return StoredZIPBytes(p)
}

func ReadOrderedZIP(z *zip.ReadCloser) (*OrderedZIP, error) {
	return readOrderedZIP(&z.Reader)
}

func readOrderedZIP(z *zip.Reader) (*OrderedZIP, error) {
	return readOrderedZIPWithWarnings(z, nil)
}

func readOrderedZIPWithWarnings(z *zip.Reader, warn func(string) error) (*OrderedZIP, error) {
	p := &OrderedZIP{}
	// ZipFile.read(name) resolves the last member with that name. Do not read
	// shadowed payloads: they can have invalid CRCs without affecting Reference's
	// dictionary acquisition. Iterating the original list retains key order.
	last := make(map[string]*zip.File, len(z.File))
	names := make(map[*zip.File]string, len(z.File))
	for _, f := range z.File {
		// ZipInfo validates the low-byte extraction version for every member,
		// including duplicates whose payloads will never be read.
		if f.ReaderVersion&0xff > 63 {
			return nil, fmt.Errorf("unsupported ZIP extraction version %d: %w", f.ReaderVersion&0xff, zip.ErrFormat)
		}
		// ZipInfo rejects an extra field whose declared payload is truncated,
		// including entries shadowed by a later duplicate. Short trailing bytes
		// without a complete field header are ignored by Reference.
		for extra := f.Extra; len(extra) >= 4; {
			n := int(binary.LittleEndian.Uint16(extra[2:4]))
			if n > len(extra)-4 {
				return nil, fmt.Errorf("truncated ZIP extra field: %w", zip.ErrFormat)
			}
			extra = extra[4+n:]
		}
		// Reference decodes every central-directory name before dictionary acquisition,
		// including entries later shadowed by duplicate names.
		if f.Flags&0x800 != 0 && !utf8.ValidString(f.Name) {
			return nil, fmt.Errorf("invalid UTF-8 ZIP member name: %w", zip.ErrFormat)
		}
		name, err := mutationExtraName(f, warn)
		if err != nil {
			return nil, err
		}
		names[f] = name
		last[name] = f
	}
	var localOffsets map[*zip.File]mutationLocalRange
	var localSource io.ReaderAt
	checkedLocal := false
	seen := make(map[string]bool, len(last))
	for _, entry := range z.File {
		name := names[entry]
		if seen[name] {
			continue
		}
		seen[name] = true
		f := last[name]
		if !checkedLocal {
			var err error
			localOffsets, localSource, err = mutationLocalNames(z, f)
			if err != nil {
				return nil, err
			}
			checkedLocal = true
		}
		if offset, ok := localOffsets[f]; ok {
			if err := validateMutationLocalHeader(f, localSource, offset.offset); err != nil {
				return nil, err
			}
		} else if err := validateMutationFlags(f); err != nil {
			// Unrecognized layouts retain the preceding central-flag guard;
			// local-header/error-order equivalence is not established there.
			return nil, err
		}
		if local, ok := localOffsets[f]; ok {
			if err := validateMutationOverlapWithWarnings(f, localSource, local, warn); err != nil {
				return nil, err
			}
		}
		if f.Flags&1 != 0 {
			return nil, fmt.Errorf("ZIP member requires a password: %w", zip.ErrAlgorithm)
		}
		// ZipFile.read allows data on names ending in '/'. Go's directory
		// shortcut rejects it; a value copy bypasses only that shortcut while
		// retaining offsets, decompression and CRC validation.
		payload := *f
		payload.Name = strings.TrimSuffix(payload.Name, "/")
		var data []byte
		if f.Method == zip.Store {
			// ZipExtFile slices stored bytes to _left, rather than requiring
			// equality between compressed and uncompressed lengths. CRC is on
			// that prefix, including when a long declaration ends early.
			raw, err := payload.OpenRaw()
			if err != nil {
				return nil, err
			}
			data, err = io.ReadAll(raw)
			if err != nil {
				return nil, err
			}
			if uint64(len(data)) < f.CompressedSize64 && (len(data) == 0 || uint64(len(data)) < f.UncompressedSize64) {
				return nil, io.ErrUnexpectedEOF
			}
			if uint64(len(data)) > f.UncompressedSize64 {
				data = data[:f.UncompressedSize64]
			}
		} else if f.Method == zip.Deflate {
			// ZipExtFile decodes before slicing to _left. It accepts a stream
			// ending without a final block if the resulting prefix passes CRC,
			// and uses central metadata rather than the trailing descriptor.
			// This materializes the stream; Reference's 1GiB read-chunk boundary
			// and concurrent source I/O remain separate unproven contracts.
			raw, err := payload.OpenRaw()
			if err != nil {
				return nil, err
			}
			decoder := flate.NewReader(raw)
			data, err = io.ReadAll(decoder)
			decoder.Close()
			if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) {
				return nil, err
			}
			if uint64(len(data)) > f.UncompressedSize64 {
				data = data[:f.UncompressedSize64]
			}
		} else if f.Method == 12 {
			// BZ2Decompressor finishes decoding before ZipExtFile slices to
			// _left and checks the central CRC. Keep stream/block corruption
			// errors even when the requested prefix is short or empty.
			raw, err := payload.OpenRaw()
			if err != nil {
				return nil, err
			}
			decoder := newMutationBZIP2Reader(raw)
			data, err = io.ReadAll(decoder)
			decoder.Close()
			if err != nil {
				return nil, err
			}
			if uint64(len(data)) > f.UncompressedSize64 {
				data = data[:f.UncompressedSize64]
			}
		} else if f.Method == 14 {
			// Reference decodes before slicing to the declared length and checks
			// CRC on the resulting prefix. Its raw LZMA decoder has no expected
			// output size; it also permits an early EOF with a matching CRC.
			raw, err := payload.OpenRaw()
			if err != nil {
				return nil, err
			}
			data, err = readMutationLZMA(raw, f.UncompressedSize64)
			if err != nil {
				return nil, err
			}
		} else {
			r, err := payload.Open()
			if err != nil {
				return nil, err
			}
			data, err = io.ReadAll(r)
			closeErr := r.Close()
			if err != nil {
				return nil, err
			}
			if closeErr != nil {
				return nil, closeErr
			}
		}
		if (f.Method == zip.Store || f.Method == zip.Deflate || f.Method == 12 || f.Method == 14) && crc32.ChecksumIEEE(data) != f.CRC32 {
			return nil, zip.ErrChecksum
		}
		p.Set(name, data)
	}
	return p, nil
}

// Reference zipfile uses CP437 unless the member declares UTF-8. Decode before
// dictionary lookup, so differently encoded names can identify the same key.
func mutationZIPName(f *zip.File) string {
	if f.Flags&0x800 != 0 {
		return referenceZIPMemberName(f.Name)
	}
	decoded, _ := charmap.CodePage437.NewDecoder().String(f.Name)
	return referenceZIPMemberName(decoded)
}

// StoredZIPBytes writes ordered dictionary entries without an OPC round trip.
func StoredZIPBytes(p *OrderedZIP) ([]byte, error) {
	var b bytes.Buffer
	if err := writeReferenceStoredZIP(&b, p, time.Now); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}
func RewriteStoredZIP(path string, transform func(*OrderedZIP) error) error {
	z, err := zip.OpenReader(path)
	if err != nil {
		return err
	}
	raw, err := EditStoredZIP(z, transform)
	closeErr := z.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	// Reference invokes the callback before opening its destination for truncation.
	return os.WriteFile(path, raw, 0600)
}

// SaveStoredZIP opens the destination before member serialization, matching the
// original mutation helper. Failure can leave a partial archive; do not delete it.
func SaveStoredZIP(path string, p *OrderedZIP) (err error) {
	return saveStoredZIPWithWarnings(path, p, nil)
}

func saveStoredZIPWithWarnings(path string, p *OrderedZIP, warn func(string) error) (err error) {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() {
		if e := f.Close(); e != nil {
			err = e
		}
	}()
	return writeReferenceStoredZIPWithWarnings(f, p, time.Now, warn)
}

// Keep ownership and error handling identical for files and injected I/O faults.
func saveStoredZIPTo(f io.WriteCloser, p *OrderedZIP) (err error) {
	defer func() {
		if e := f.Close(); e != nil {
			err = e
		}
	}()
	return writeReferenceStoredZIP(f, p, time.Now)
}

// ZipInfo truncates a member name at the first NUL when serializing. Keep the
// callback dictionary key intact; only the archived name is changed.
func referenceZIPMemberName(name string) string {
	name, _, _ = strings.Cut(name, "\x00")
	return name
}
