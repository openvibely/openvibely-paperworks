package testsupport

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"testing"
	"time"
)

type flushFaultFile struct {
	*os.File
	stage              string
	failed, closeFails bool
	events             []string
}

func (f *flushFaultFile) Write(b []byte) (int, error) {
	match := f.stage == "header" && bytes.HasPrefix(b, []byte("PK\x03\x04")) && bytes.HasSuffix(b, []byte("second")) || f.stage == "payload" && string(b) == "second payload" || f.stage == "directory" && bytes.HasPrefix(b, []byte("PK\x01\x02"))
	if match && !f.failed {
		f.failed = true
		f.events = append(f.events, "write-error")
		n, e := f.File.Write(b[:3])
		if e != nil {
			return n, e
		}
		return n, errors.New("injected write")
	}
	return f.File.Write(b)
}
func (f *flushFaultFile) Flush() error {
	f.events = append(f.events, "flush")
	if f.stage == "flush" {
		return errors.New("injected flush")
	}
	return nil
}
func (f *flushFaultFile) Close() error {
	f.events = append(f.events, "close")
	e := f.File.Close()
	if f.closeFails {
		return errors.New("injected close")
	}
	return e
}
func TestMutationFlushAndAdvancingFailures(t *testing.T) {
	var fixture struct {
		Cases []struct {
			Stage      string
			Persistent bool
			Events     []string
			Output     []byte
			Error      string
		}
	}
	raw, e := os.ReadFile("testdata/mutation-flush.json")
	if e != nil {
		t.Fatal(e)
	}
	if e = json.Unmarshal(raw, &fixture); e != nil {
		t.Fatal(e)
	}
	parts := &OrderedZIP{Parts: []ZIPPart{{"first", "first payload"}, {"second", "second payload"}}}
	for _, c := range fixture.Cases {
		t.Run(c.Stage+map[bool]string{true: "-close", false: ""}[c.Persistent], func(t *testing.T) {
			f, e := os.CreateTemp(t.TempDir(), "out")
			if e != nil {
				t.Fatal(e)
			}
			dst := &flushFaultFile{File: f, stage: c.Stage, closeFails: c.Persistent}
			e = writeReferenceStoredZIP(dst, parts, func() time.Time { return time.Date(2024, 2, 3, 4, 5, 6, 0, time.Local) })
			want := "injected write"
			if c.Stage == "flush" {
				want = "injected flush"
			}
			if e == nil || e.Error() != want {
				t.Fatalf("writer error %v", e)
			}
			// Fixed-clock serialization is checked independently of the ownership wrapper.
			_ = dst.Close()
			got, e := os.ReadFile(f.Name())
			if e != nil {
				t.Fatal(e)
			}
			if !bytes.Equal(got, c.Output) {
				t.Fatalf("partial archive: %d bytes want %d", len(got), len(c.Output))
			}
			if !reflect.DeepEqual(dst.events, c.Events[2:]) {
				t.Fatalf("events %v want %v", dst.events, c.Events)
			}
			// Exercise actual ownership and close precedence using the normal clock.
			f, e = os.CreateTemp(t.TempDir(), "owned")
			if e != nil {
				t.Fatal(e)
			}
			dst = &flushFaultFile{File: f, stage: c.Stage, closeFails: c.Persistent}
			e = saveStoredZIPTo(dst, parts)
			if e == nil || e.Error() != c.Error {
				t.Fatalf("owned error %v want %s", e, c.Error)
			}
			if !reflect.DeepEqual(dst.events, c.Events[2:]) {
				t.Fatalf("owned events %v", dst.events)
			}
		})
	}
}
