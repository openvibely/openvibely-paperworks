package testsupport

import (
	"archive/zip"
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// Pinned PresentationNative.mutate reads z.read(name), which resolves the last
// duplicate, while its dictionary retains the first occurrence's position.
func TestMutationDuplicateAcquisition(t *testing.T) {
	for _, bad := range []int{-1, 0, 2} {
		name := map[int]string{-1: "valid", 0: "shadowed-corrupt", 2: "winning-corrupt"}[bad]
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer
			w := zip.NewWriter(&buf)
			for _, p := range []ZIPPart{{"duplicate", "old"}, {"other", "middle"}, {"duplicate", "new"}} {
				f, err := w.CreateHeader(&zip.FileHeader{Name: p.Name, Method: zip.Store})
				if err != nil {
					t.Fatal(err)
				}
				if _, err = f.Write([]byte(p.Data)); err != nil {
					t.Fatal(err)
				}
			}
			if err := w.Close(); err != nil {
				t.Fatal(err)
			}
			raw := append([]byte(nil), buf.Bytes()...)
			if bad >= 0 {
				z, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
				if err != nil {
					t.Fatal(err)
				}
				offset, err := z.File[bad].DataOffset()
				if err != nil {
					t.Fatal(err)
				}
				raw[offset] ^= 1
			}
			src := filepath.Join(t.TempDir(), "input.zip")
			if err := os.WriteFile(src, raw, 0600); err != nil {
				t.Fatal(err)
			}
			z, err := zip.OpenReader(src)
			if err != nil {
				t.Fatal(err)
			}
			calls := 0
			out, err := EditStoredZIP(z, func(p *OrderedZIP) error {
				calls++
				if !reflect.DeepEqual(p.Parts, []ZIPPart{{"duplicate", "new"}, {"other", "middle"}}) {
					t.Fatalf("parts: %#v", p.Parts)
				}
				return nil
			})
			z.Close()
			if bad == 2 {
				if !errors.Is(err, zip.ErrChecksum) || calls != 0 || out != nil {
					t.Fatalf("error=%v callbacks=%d bytes=%d", err, calls, len(out))
				}
			} else {
				if err != nil || calls != 1 {
					t.Fatalf("error=%v callbacks=%d", err, calls)
				}
				saved, err := zip.NewReader(bytes.NewReader(out), int64(len(out)))
				if err != nil {
					t.Fatal(err)
				}
				if len(saved.File) != 2 || saved.File[0].Name != "duplicate" || saved.File[1].Name != "other" {
					t.Fatal("saved member order")
				}
			}
			after, err := os.ReadFile(src)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(after, raw) {
				t.Fatal("source changed")
			}
		})
	}
}
