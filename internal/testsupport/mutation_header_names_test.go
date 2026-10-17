package testsupport

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// Reference rejects a local-header name mismatch before callbacks or publication.
// This formerly recorded the adapter divergence; it now guards the correction.
func TestMutationLocalHeaderNameMismatch(t *testing.T) {
	raw, err := StoredZIPBytes(&OrderedZIP{Parts: []ZIPPart{{"original", "keep"}}})
	if err != nil {
		t.Fatal(err)
	}
	copy(raw[30:38], []byte("changed!"))
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
		t.Fatal("local-header name mismatch accepted")
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
		t.Fatal("destination changed after acquisition failure")
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || entries[0].Name() != "2.pptx" || entries[1].Name() != "source.zip" {
		t.Fatal("directory entries changed")
	}
	t.Log("Python and Go reject before callback and preserve the destination sentinel")
}
