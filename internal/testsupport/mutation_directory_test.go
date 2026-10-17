package testsupport

import (
	"archive/zip"
	"io"
	"path/filepath"
	"testing"
)

// Reference writestr retains the payload even for slash-terminated member names.
func TestMutationDirectoryPayloadBoundary(t *testing.T) {
	for _, name := range []string{"folder/", "folder/child", "folder/\x00ignored"} {
		t.Run(name, func(t *testing.T) {
			p := &OrderedZIP{Parts: []ZIPPart{{"original", "keep"}, {name, "value"}}}
			dst := filepath.Join(t.TempDir(), "out.zip")
			err := SaveStoredZIP(dst, p)
			if err != nil {
				t.Fatal(err)
			}
			z, err := zip.OpenReader(dst)
			if err != nil {
				t.Fatal(err)
			}
			defer z.Close()
			if len(z.File) != 2 {
				t.Fatalf("members=%d", len(z.File))
			}
			for i, want := range []string{"keep", "value"} {
				f := z.File[i]
				r, e := f.OpenRaw()
				if e != nil {
					t.Fatal(e)
				}
				data, e := io.ReadAll(r)
				if e != nil || string(data) != want {
					t.Fatalf("member %d payload=%q error=%v", i, data, e)
				}
				if f.Method != zip.Store {
					t.Fatal("non-Store member")
				}
			}
			readback, e := ReadOrderedZIP(z)
			if e != nil || len(readback.Parts) != 2 || readback.Parts[1].Data != "value" {
				t.Fatal(readback, e)
			}
			if z.File[1].Name != referenceZIPMemberName(name) || p.Parts[1].Name != name || p.Parts[1].Data != "value" {
				t.Fatal("name or callback dictionary changed")
			}
		})
	}
}
