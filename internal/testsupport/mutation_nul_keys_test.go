package testsupport

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// ZipInfo truncates decoded names at NUL before namelist/dictionary lookup.
// Last duplicate payload wins, while the first key position remains stable.
func TestMutationNULKeys(t *testing.T) {
	for _, reinsert := range []bool{false, true} {
		name := "observe"
		if reinsert {
			name = "reinsert"
		}
		t.Run(name, func(t *testing.T) {
			var b bytes.Buffer
			w := zip.NewWriter(&b)
			for _, p := range []ZIPPart{{"key!a", "first"}, {"other", "middle"}, {"key!b", "last"}} {
				f, e := w.CreateHeader(&zip.FileHeader{Name: p.Name, Method: zip.Store})
				if e != nil {
					t.Fatal(e)
				}
				if _, e = f.Write([]byte(p.Data)); e != nil {
					t.Fatal(e)
				}
			}
			if e := w.Close(); e != nil {
				t.Fatal(e)
			}
			raw := bytes.ReplaceAll(b.Bytes(), []byte("key!a"), []byte("key\x00a"))
			raw = bytes.ReplaceAll(raw, []byte("key!b"), []byte("key\x00b"))
			root := t.TempDir()
			src, dst := filepath.Join(root, "source.zip"), filepath.Join(root, "2.pptx")
			if e := os.WriteFile(src, raw, 0600); e != nil {
				t.Fatal(e)
			}
			if e := os.WriteFile(dst, []byte("sentinel"), 0600); e != nil {
				t.Fatal(e)
			}
			calls := 0
			err := func() error {
				z, e := zip.OpenReader(src)
				if e != nil {
					return e
				}
				defer z.Close()
				p, e := ReadOrderedZIP(z)
				if e != nil {
					return e
				}
				calls++
				if !reflect.DeepEqual(p.Parts, []ZIPPart{{"key", "last"}, {"other", "middle"}}) {
					t.Fatalf("callback: %#v", p.Parts)
				}
				if reinsert {
					p.Remove("key")
					p.Set("key", []byte("changed"))
				}
				return SaveStoredZIP(dst, p)
			}()
			if err != nil || calls != 1 {
				t.Fatalf("error=%v callbacks=%d", err, calls)
			}
			after, e := os.ReadFile(src)
			if e != nil || !bytes.Equal(after, raw) {
				t.Fatal("source changed", e)
			}
			after, e = os.ReadFile(dst)
			if e != nil || bytes.Equal(after, []byte("sentinel")) {
				t.Fatal("destination not replaced", e)
			}
			z, e := zip.OpenReader(dst)
			if e != nil {
				t.Fatal(e)
			}
			defer z.Close()
			p, e := ReadOrderedZIP(z)
			if e != nil {
				t.Fatal(e)
			}
			want := []ZIPPart{{"key", "last"}, {"other", "middle"}}
			if reinsert {
				want = []ZIPPart{{"other", "middle"}, {"key", "changed"}}
			}
			if !reflect.DeepEqual(p.Parts, want) {
				t.Fatalf("saved=%#v want=%#v", p.Parts, want)
			}
			entries, e := os.ReadDir(root)
			if e != nil || len(entries) != 2 || entries[0].Name() != "2.pptx" || entries[1].Name() != "source.zip" {
				t.Fatal("directory changed", e)
			}
		})
	}
}
