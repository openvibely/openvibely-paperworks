package testsupport

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

type mutationReadFault struct {
	*bytes.Reader
	target int64
	armed  bool
	events *[]string
	err    error
}

func (r *mutationReadFault) ReadAt(p []byte, off int64) (int, error) {
	if r.armed && off <= r.target && off+int64(len(p)) > r.target {
		*r.events = append(*r.events, "read-error")
		return 0, r.err
	}
	return r.Reader.ReadAt(p, off)
}

// Reference captures inject after indexing, inside the unchanged helper's
// source context. They do not claim concurrent file mutation or syscall equivalence.
func TestMutationReadFaultPrecedence(t *testing.T) {
	var fixture struct {
		Cases []struct {
			Method     int
			Target     int64
			CloseError bool `json:"close_error"`
			Input      string
			Events     []string
			Error      string
		}
	}
	raw, err := os.ReadFile("testdata/mutation-read-faults.json")
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	for _, c := range fixture.Cases {
		t.Run(fmt.Sprintf("codec%d-offset%d-close%v", c.Method, c.Target, c.CloseError), func(t *testing.T) {
			data, e := base64.StdEncoding.DecodeString(c.Input)
			if e != nil {
				t.Fatal(e)
			}
			events := []string{}
			readErr := errors.New("injected read")
			closeErr := errors.New("injected close")
			source := &mutationReadFault{Reader: bytes.NewReader(data), target: c.Target, events: &events, err: readErr}
			z, e := zip.NewReader(source, int64(len(data)))
			if e != nil {
				t.Fatal(e)
			}
			source.armed = true
			dir := t.TempDir()
			closed := false
			out, e := mutateOrderedZIP(z, func() error {
				closed = true
				events = append(events, "close")
				if c.CloseError {
					return closeErr
				}
				return nil
			}, func(*OrderedZIP) error { events = append(events, "callback"); return nil }, func() string { events = append(events, "output"); return filepath.Join(dir, "result.zip") })
			want := readErr
			if c.CloseError {
				want = closeErr
			}
			if !errors.Is(e, want) || e.Error() != c.Error || out != "" || !closed || !reflect.DeepEqual(events, c.Events) {
				t.Fatalf("out=%q err=%v events=%v closed=%v", out, e, events, closed)
			}
			entries, e := os.ReadDir(dir)
			if e != nil || len(entries) != 0 {
				t.Fatalf("output entries %v: %v", entries, e)
			}
		})
	}
}
