package testsupport

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// Same acquisition boundary as unchanged PresentationNative.mutate: neither
// callback nor destination allocation/save is reached after a source error.
func TestMutationSourceErrors(t *testing.T) {
	for _, name := range []string{"unsupported-compression", "missing-central-directory", "not-zip"} {
		t.Run(name, func(t *testing.T) {
			raw, err := StoredZIPBytes(&OrderedZIP{Parts: []ZIPPart{{"original", "keep"}}})
			if err != nil {
				t.Fatal(err)
			}
			central := bytes.Index(raw, []byte("PK\x01\x02"))
			if central < 0 {
				t.Fatal("missing fixture central directory")
			}
			want := zip.ErrFormat
			switch name {
			case "unsupported-compression":
				binary.LittleEndian.PutUint16(raw[8:], 99)
				binary.LittleEndian.PutUint16(raw[central+10:], 99)
				want = zip.ErrAlgorithm
			case "missing-central-directory":
				raw = raw[:central]
			case "not-zip":
				raw = []byte("not a zip")
			}
			dir := t.TempDir()
			src := filepath.Join(dir, "source.zip")
			dst := filepath.Join(dir, "2.pptx")
			if err := os.WriteFile(src, raw, 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(dst, []byte("sentinel"), 0600); err != nil {
				t.Fatal(err)
			}
			calls, saves := 0, 0
			mutate := func() error {
				z, err := zip.OpenReader(src)
				if err != nil {
					return err
				}
				defer z.Close()
				p, err := ReadOrderedZIP(z)
				if err != nil {
					return err
				}
				calls++
				p.Set("added", []byte("value"))
				saves++
				return SaveStoredZIP(dst, p)
			}
			if err := mutate(); !errors.Is(err, want) {
				t.Fatalf("got %v, want %v", err, want)
			}
			if calls != 0 || saves != 0 {
				t.Fatalf("callbacks=%d saves=%d", calls, saves)
			}
			after, err := os.ReadFile(src)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(after, raw) {
				t.Fatal("source changed")
			}
			after, err = os.ReadFile(dst)
			if err != nil {
				t.Fatal(err)
			}
			if string(after) != "sentinel" {
				t.Fatal("destination changed")
			}
			entries, err := os.ReadDir(dir)
			if err != nil {
				t.Fatal(err)
			}
			names := []string{}
			for _, e := range entries {
				names = append(names, e.Name())
			}
			if !reflect.DeepEqual(names, []string{"2.pptx", "source.zip"}) {
				t.Fatalf("entries %v", names)
			}
		})
	}
}
