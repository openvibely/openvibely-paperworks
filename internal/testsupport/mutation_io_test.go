package testsupport

import (
	"archive/zip"
	"bytes"
	"errors"
	"io"
	"syscall"
	"testing"
)

type mutationFaultSink struct {
	bytes.Buffer
	kind   string
	closes int
	writes int
}

func (f *mutationFaultSink) Write(p []byte) (int, error) {
	f.writes++
	switch f.kind {
	case "disk-full", "write-and-close":
		return 0, syscall.ENOSPC
	case "short-write":
		return f.Buffer.Write(p[:len(p)/2])
	case "zero-write":
		return 0, nil
	case "short-once":
		if f.writes == 1 {
			return f.Buffer.Write(p[:len(p)/2])
		}
	}
	return f.Buffer.Write(p)
}
func (f *mutationFaultSink) Close() error {
	f.closes++
	if f.kind == "close-error" || f.kind == "write-and-close" {
		return syscall.EIO
	}
	return nil
}

// Reference's original mutate helper was exercised with these injected file
// failures, including zero progress and a single transient short write. Short writes deliberately remain a documented non-equivalence:
// zipfile ignores the short count; Go reports it instead of claiming success.
func TestMutationInjectedIOFailures(t *testing.T) {
	for _, kind := range []string{"disk-full", "short-write", "zero-write", "short-once", "close-error", "write-and-close"} {
		t.Run(kind, func(t *testing.T) {
			f := &mutationFaultSink{kind: kind}
			err := saveStoredZIPTo(f, &OrderedZIP{Parts: []ZIPPart{{"first", "saved"}, {"second", "data"}}})
			want := error(syscall.ENOSPC)
			if kind == "short-write" || kind == "zero-write" || kind == "short-once" {
				want = io.ErrShortWrite
			}
			if kind == "close-error" || kind == "write-and-close" {
				want = syscall.EIO
			}
			if !errors.Is(err, want) {
				t.Fatalf("error %v, want %v", err, want)
			}
			if f.closes != 1 {
				t.Fatalf("close count %d", f.closes)
			}
			valid := false
			if z, e := zip.NewReader(bytes.NewReader(f.Bytes()), int64(f.Len())); e == nil {
				valid = len(z.File) == 2
				for i, expected := range []string{"saved", "data"} {
					if i >= len(z.File) {
						valid = false
						break
					}
					r, e := z.File[i].Open()
					if e != nil {
						valid = false
						break
					}
					b, e := io.ReadAll(r)
					r.Close()
					valid = valid && e == nil && string(b) == expected
				}
			}
			if valid != (kind == "close-error") {
				t.Fatalf("unexpected archive validity %v", valid)
			}
			t.Logf("case=%s error=%v bytes=%d valid=%v writes=%d closes=%d", kind, err, f.Len(), valid, f.writes, f.closes)
		})
	}
}
