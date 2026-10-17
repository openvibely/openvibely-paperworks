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

// Faults occur at semantic boundaries, independent of write call counts.
type recoveryFile struct {
	*os.File
	stage              string
	persistent, failed bool
	events             []string
}

func (s *recoveryFile) Write(b []byte) (int, error) {
	match := s.stage == "header" && bytes.HasPrefix(b, []byte("PK\x03\x04")) && bytes.HasSuffix(b, []byte("second")) || s.stage == "payload" && string(b) == "second payload" || s.stage == "directory" && bytes.HasPrefix(b, []byte("PK\x01\x02"))
	if (!s.failed && match) || (s.failed && s.persistent) {
		s.failed = true
		s.events = append(s.events, "write-error")
		return 0, errors.New("injected write")
	}
	return s.File.Write(b)
}
func TestMutationSeekableOutputRecovery(t *testing.T) {
	var fixture struct {
		Cases []struct {
			Stage      string
			Persistent bool
			Events     []string
			Output     []byte
		}
	}
	raw, err := os.ReadFile("testdata/mutation-output-recovery.json")
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	for _, c := range fixture.Cases {
		label := c.Stage
		if c.Persistent {
			label += "-persistent"
		}
		t.Run(label, func(t *testing.T) {
			f, e := os.CreateTemp(t.TempDir(), "output")
			if e != nil {
				t.Fatal(e)
			}
			s := &recoveryFile{File: f, stage: c.Stage, persistent: c.Persistent}
			e = writeReferenceStoredZIP(s, &OrderedZIP{Parts: []ZIPPart{{"first", "first payload"}, {"second", "second payload"}}}, func() time.Time { return time.Date(2024, 2, 3, 4, 5, 6, 0, time.Local) })
			if e == nil || e.Error() != "injected write" {
				t.Fatalf("error: %v", e)
			}
			if e = f.Close(); e != nil {
				t.Fatal(e)
			}
			got, e := os.ReadFile(f.Name())
			if e != nil {
				t.Fatal(e)
			}
			if !bytes.Equal(got, c.Output) {
				t.Fatalf("partial archive differs: got %d bytes want %d", len(got), len(c.Output))
			}
			if !reflect.DeepEqual(s.events, c.Events[2:len(c.Events)-1]) {
				t.Fatalf("events %v want %v", s.events, c.Events)
			}
		})
	}
}

// Explicit Go short-write errors remain required even though Reference ignores
// a short return. This checks safety and ownership, not Reference equivalence.
type shortRecoveryFile struct {
	*os.File
	closeErr error
	closed   int
}

func (s *shortRecoveryFile) Write(b []byte) (int, error) {
	if string(b) == "second payload" {
		return s.File.Write(b[:3])
	}
	return s.File.Write(b)
}
func (s *shortRecoveryFile) Close() error {
	s.closed++
	e := s.File.Close()
	if s.closeErr != nil {
		return s.closeErr
	}
	return e
}
func TestMutationSeekableShortWriteRemainsError(t *testing.T) {
	for _, failClose := range []bool{false, true} {
		f, e := os.CreateTemp(t.TempDir(), "output")
		if e != nil {
			t.Fatal(e)
		}
		s := &shortRecoveryFile{File: f}
		want := error(io.ErrShortWrite)
		if failClose {
			s.closeErr = errors.New("close failed")
			want = s.closeErr
		}
		e = saveStoredZIPTo(s, &OrderedZIP{Parts: []ZIPPart{{"first", "first payload"}, {"second", "second payload"}}})
		if !errors.Is(e, want) || s.closed != 1 {
			t.Fatalf("error=%v closes=%d", e, s.closed)
		}
		raw, e := os.ReadFile(f.Name())
		if e != nil {
			t.Fatal(e)
		}
		if !bytes.Contains(raw, []byte("PK\x05\x06")) {
			t.Fatal("missing recovered directory")
		}
	}
}
