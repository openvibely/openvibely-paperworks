package testsupport

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// Truncated extra fields fail before callback and destination publication.
func TestMutationTruncatedExtraField(t *testing.T) {
	for _, tc := range []struct {
		name string
	}{
		{"truncated-extra-field"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var buffer bytes.Buffer
			writer := zip.NewWriter(&buffer)
			member, err := writer.CreateHeader(&zip.FileHeader{Name: "original", Method: zip.Store, Extra: []byte{0xfe, 0xca, 8, 0, 'x'}})
			if err != nil {
				t.Fatal(err)
			}
			if _, err = member.Write([]byte("keep")); err != nil {
				t.Fatal(err)
			}
			if err = writer.Close(); err != nil {
				t.Fatal(err)
			}
			raw := buffer.Bytes()

			root := t.TempDir()
			src, dst := filepath.Join(root, "source.zip"), filepath.Join(root, "2.pptx")
			if err := os.WriteFile(src, raw, 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(dst, []byte("sentinel"), 0600); err != nil {
				t.Fatal(err)
			}
			callbacks := 0
			mutate := func() error {
				z, err := zip.OpenReader(src)
				if err != nil {
					return err
				}
				defer z.Close()
				parts, err := ReadOrderedZIP(z)
				if err != nil {
					return err
				}
				callbacks++
				if len(parts.Parts) != 1 || parts.Parts[0] != (ZIPPart{"original", "keep"}) {
					t.Fatalf("callback parts: %#v", parts.Parts)
				}
				parts.Set("added", []byte("value"))
				return SaveStoredZIP(dst, parts)
			}
			if err := mutate(); err == nil {
				t.Fatal("accepted truncated extra field")
			}
			if callbacks != 0 {
				t.Fatalf("callbacks=%d", callbacks)
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
			if !bytes.Equal(after, []byte("sentinel")) {
				t.Fatal("destination changed")
			}
			entries, err := os.ReadDir(root)
			if err != nil {
				t.Fatal(err)
			}
			if len(entries) != 2 || entries[0].Name() != "2.pptx" || entries[1].Name() != "source.zip" {
				t.Fatal("directory entries changed")
			}

		})
	}
}
