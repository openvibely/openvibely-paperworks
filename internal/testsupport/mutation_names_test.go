package testsupport

import (
	"archive/zip"
	"io"
	"path/filepath"
	"testing"
)

// Expectations come from the unchanged Reference mutation helper and ZipInfo.
func TestMutationMemberNames(t *testing.T) {
	for _, tc := range []struct{ input, saved string }{
		{"plain", "plain"}, {`a\b`, `a\b`}, {"é.xml", "é.xml"},
		{"prefix\x00suffix", "prefix"}, {"\x00suffix", ""}, {"", ""},
	} {
		t.Run(tc.input, func(t *testing.T) {
			p := &OrderedZIP{Parts: []ZIPPart{{"original", "keep"}, {tc.input, "value"}}}
			dst := filepath.Join(t.TempDir(), "output.zip")
			if err := SaveStoredZIP(dst, p); err != nil {
				t.Fatal(err)
			}
			z, err := zip.OpenReader(dst)
			if err != nil {
				t.Fatal(err)
			}
			defer z.Close()
			if len(z.File) != 2 {
				t.Fatalf("members: %d", len(z.File))
			}
			for i, expected := range []ZIPPart{{"original", "keep"}, {tc.saved, "value"}} {
				f := z.File[i]
				if f.Name != expected.Name || f.Method != zip.Store {
					t.Fatalf("member %d: %q, method %d", i, f.Name, f.Method)
				}
				r, err := f.Open()
				if err != nil {
					t.Fatal(err)
				}
				data, err := io.ReadAll(r)
				r.Close()
				if err != nil || string(data) != expected.Data {
					t.Fatalf("payload %q: %q, %v", f.Name, data, err)
				}
			}
			if p.Parts[1].Name != tc.input {
				t.Fatal("serialization changed callback dictionary")
			}
		})
	}
}
