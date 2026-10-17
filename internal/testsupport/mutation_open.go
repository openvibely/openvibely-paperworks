package testsupport

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"os"
)

// MutateStoredZIPPath includes source acquisition, unlike MutateStoredZIP, whose
// caller has already constructed a zip.ReadCloser. No source bytes are rewritten.
func MutateStoredZIPPath(path string, transform func(*OrderedZIP) error, output func() string) (string, error) {
	return MutateStoredZIPPathWithWarnings(path, transform, output, nil)
}

// MutateStoredZIPPathWithWarnings delivers supported ZIP diagnostics to
// warn. A returned error aborts before callback/output allocation and the source
// is closed. Nil ignores diagnostics. Policies are local to this call, not a
// process-global Reference warnings registry. Unicode-path and equal-offset overlap warnings occur during acquisition;
// duplicate output-name warnings occur during serialization.
func MutateStoredZIPPathWithWarnings(path string, transform func(*OrderedZIP) error, output func() string, warn func(string) error) (string, error) {
	z, source, err := openMutationZIP(path)
	if err != nil {
		return "", err
	}
	return mutateOrderedZIPWithWarnings(z, source.Close, transform, output, warn)
}

func openMutationZIP(path string) (*zip.Reader, *os.File, error) {
	source, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	fail := func(err error) (*zip.Reader, *os.File, error) { source.Close(); return nil, nil, err }
	info, err := source.Stat()
	if err != nil {
		return fail(err)
	}
	z, err := zip.NewReader(mutationClassicDirectory(source, info.Size()), info.Size())
	if err == nil {
		return z, source, nil
	}
	if !errors.Is(err, zip.ErrFormat) {
		return fail(err)
	}
	// Pinned zipfile._EndRecData64 retries the fixed record before the locator
	// for prepended archives. Restrict recovery to that single-disk layout with
	// consistent record lengths and offsets, then let archive/zip parse the rest.
	base, ok := mutationZIP64Prefix(source, info.Size())
	if !ok {
		return fail(err)
	}
	view := &mutationZIPView{SectionReader: io.NewSectionReader(source, base, info.Size()-base), info: mutationZIPInfo{info, info.Size() - base}}
	z, err = zip.NewReader(mutationClassicDirectory(view, view.Size()), view.Size())
	if err != nil {
		return fail(err)
	}
	return z, source, nil
}

// Stat exposes the virtual archive size to local-header validation. OpenRaw's
// ReaderAt remains this view of the same file; neither reopening nor copying is
// necessary, and offsets below are relative to the recovered archive start.
type mutationZIPView struct {
	*io.SectionReader
	info mutationZIPInfo
}

func (v *mutationZIPView) Stat() (os.FileInfo, error) { return v.info, nil }

type mutationZIPInfo struct {
	os.FileInfo
	size int64
}

func (i mutationZIPInfo) Size() int64 { return i.size }

func mutationZIP64Prefix(source io.ReaderAt, size int64) (int64, bool) {
	n := min(size, int64(65557))
	if n < 98 {
		return 0, false
	}
	tail := make([]byte, n)
	if _, err := source.ReadAt(tail, size-n); err != nil {
		return 0, false
	}
	e := len(tail) - 22
	u16, u32, u64 := binary.LittleEndian.Uint16, binary.LittleEndian.Uint32, binary.LittleEndian.Uint64
	if !bytes.Equal(tail[e:e+4], []byte("PK\x05\x06")) || u16(tail[e+20:]) != 0 {
		e = bytes.LastIndex(tail, []byte("PK\x05\x06"))
	}
	if e < 76 || e+22 > len(tail) {
		return 0, false
	}
	end := tail[e:]
	if u16(end[4:]) != 0 || u16(end[6:]) != 0 {
		return 0, false
	}
	locator, record := tail[e-20:e], tail[e-76:e-20]
	if !bytes.Equal(locator[:4], []byte("PK\x06\x07")) || u32(locator[4:]) != 0 || u32(locator[16:]) > 1 {
		return 0, false
	}
	at := size - n + int64(e) - 76
	rel := u64(locator[8:])
	if rel >= uint64(at) {
		return 0, false
	}
	// Recovery applies only when the original locator does not identify a record.
	var signature [4]byte
	if _, err := source.ReadAt(signature[:], int64(rel)); err != nil || bytes.Equal(signature[:], []byte("PK\x06\x06")) {
		return 0, false
	}
	if !bytes.Equal(record[:4], []byte("PK\x06\x06")) || u64(record[4:]) != 44 || u32(record[16:]) != 0 || u32(record[20:]) != 0 {
		return 0, false
	}
	length, start := u64(record[40:]), u64(record[48:])
	if length > rel || start != rel-length {
		return 0, false
	}
	return at - int64(rel), true
}
