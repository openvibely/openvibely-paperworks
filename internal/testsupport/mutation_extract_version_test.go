package testsupport

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

// Reference rejects versions above 63 before callback or destination truncation.
func TestMutationExtractVersion(t *testing.T) {
	for _, tc := range []struct {
		name string
		flag uint16
	}{
		{"version-64", 64}, {"version-255", 255},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw, err := StoredZIPBytes(&OrderedZIP{Parts: []ZIPPart{{"original", "keep"}}})
			if err != nil {
				t.Fatal(err)
			}
			central := bytes.Index(raw, []byte{'P', 'K', 1, 2})
			if central < 0 {
				t.Fatal("missing central directory")
			}
			for _, offset := range []int{4, central + 6} {
				binary.LittleEndian.PutUint16(raw[offset:offset+2], tc.flag)
			}
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
				t.Fatal("unsupported extraction version accepted")
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
				t.Fatal("destination changed before acquisition completed")
			}
			entries, err := os.ReadDir(root)
			if err != nil {
				t.Fatal(err)
			}
			if len(entries) != 2 || entries[0].Name() != "2.pptx" || entries[1].Name() != "source.zip" {
				t.Fatal("directory entries changed")
			}
			t.Log("unsupported version rejected before callback; source and destination preserved")
		})
	}
}
