package testsupport

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// Invalid flagged names must fail before callback or destination publication.
func TestMutationInvalidUTF8Name(t *testing.T) {
	raw, err := StoredZIPBytes(&OrderedZIP{Parts: []ZIPPart{{"original", "keep"}}})
	if err != nil {
		t.Fatal(err)
	}
	central := bytes.Index(raw, []byte("PK\x01\x02"))
	if central < 0 {
		t.Fatal("central header missing")
	}
	binary.LittleEndian.PutUint16(raw[6:8], 0x800)
	binary.LittleEndian.PutUint16(raw[central+8:central+10], 0x800)
	raw[30], raw[central+46] = 255, 255

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
		parts.Set("added", []byte("value"))
		return SaveStoredZIP(dst, parts)
	}
	if err := mutate(); !errors.Is(err, zip.ErrFormat) {
		t.Fatalf("want invalid-name acquisition error, got %v", err)
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
	t.Log("Reference UnicodeDecodeError / Go wrapped zip.ErrFormat: callback absent, source and destination preserved")
}
