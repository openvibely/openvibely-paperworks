package testsupport

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"reflect"
	"testing"
	"time"
)

type positionFaultFile struct {
	*os.File
	stage        string
	tells, seeks int
	closeError   bool
	events       []string
}

func (f *positionFaultFile) Seek(offset int64, whence int) (int64, error) {
	fail := false
	if whence == io.SeekCurrent {
		f.tells++
		fail = f.stage == "tell-header" && f.tells == 4 || f.stage == "tell-end" && f.tells == 5
	} else {
		f.seeks++
		target := map[string]int{"seek-header": 5, "seek-rewrite": 6, "seek-return": 7, "seek-directory": 8}[f.stage]
		fail = f.seeks == target
	}
	if fail {
		f.events = append(f.events, "position-error")
		return 0, errors.New("injected position")
	}
	return f.File.Seek(offset, whence)
}
func (f *positionFaultFile) Flush() error { f.events = append(f.events, "flush"); return nil }
func (f *positionFaultFile) Close() error {
	f.events = append(f.events, "close")
	e := f.File.Close()
	if f.closeError {
		return errors.New("injected close")
	}
	return e
}
func TestMutationPositionFaults(t *testing.T) {
	var data struct {
		Cases []struct {
			Stage      string
			Persistent bool
			Events     []string
			Error      string
			Output     []byte
		}
	}
	raw, e := os.ReadFile("testdata/mutation-position.json")
	if e != nil {
		t.Fatal(e)
	}
	if e = json.Unmarshal(raw, &data); e != nil {
		t.Fatal(e)
	}
	parts := &OrderedZIP{Parts: []ZIPPart{{"first", "first payload"}, {"second", "second payload"}}}
	for _, c := range data.Cases {
		t.Run(c.Stage+map[bool]string{true: "-close"}[c.Persistent], func(t *testing.T) {
			for _, fixed := range []bool{true, false} {
				fp, e := os.CreateTemp(t.TempDir(), "out")
				if e != nil {
					t.Fatal(e)
				}
				f := &positionFaultFile{File: fp, stage: c.Stage, closeError: c.Persistent}
				if fixed {
					e = writeReferenceStoredZIP(f, parts, func() time.Time { return time.Date(2024, 2, 3, 4, 5, 6, 0, time.Local) })
					if ce := f.Close(); ce != nil {
						e = ce
					}
				} else {
					e = saveStoredZIPTo(f, parts)
				}
				if e == nil || e.Error() != c.Error {
					t.Fatalf("fixed=%v: error %v want %s", fixed, e, c.Error)
				}
				if !reflect.DeepEqual(f.events, c.Events[2:]) {
					t.Fatalf("events %v want %v", f.events, c.Events[2:])
				}
				b, e := os.ReadFile(fp.Name())
				if e != nil {
					t.Fatal(e)
				}
				if fixed && !bytes.Equal(b, c.Output) {
					t.Fatalf("partial archive differs: %d vs %d bytes", len(b), len(c.Output))
				}
			}
		})
	}
}
