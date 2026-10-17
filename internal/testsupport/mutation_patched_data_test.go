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

// Documents malformed plaintext ZIPs advertising patched-data, not support for
// compressed patches. Reference rejects the flags before invoking the callback.
func TestMutationPatchedDataFlag(t *testing.T) {
	for _, tc := range []struct {
		name string
		flag uint16
	}{
		{"compressed-patched-data", 32},
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
			for _, offset := range []int{6, central + 8} {
				binary.LittleEndian.PutUint16(raw[offset:offset+2], binary.LittleEndian.Uint16(raw[offset:offset+2])|tc.flag)
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
			if err := mutate(); !errors.Is(err, zip.ErrAlgorithm) {
				t.Fatalf("want patched-data rejection, got %v", err)
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
				t.Fatal("destination changed before patched-data rejection")
			}
			entries, err := os.ReadDir(root)
			if err != nil {
				t.Fatal(err)
			}
			if len(entries) != 2 || entries[0].Name() != "2.pptx" || entries[1].Name() != "source.zip" {
				t.Fatal("directory entries changed")
			}
			t.Log("The reference and Go reject patched-data flags before callback; source, sentinel and entries preserved; language-specific errors retained")
		})
	}
}

// Patched-data rejection belongs to selected payload acquisition, not indexing.
func TestMutationShadowedPatchedData(t *testing.T) {
	var b bytes.Buffer
	w := zip.NewWriter(&b)
	for _, data := range []string{"hidden", "visible"} {
		f, err := w.CreateHeader(&zip.FileHeader{Name: "original", Method: zip.Store})
		if err != nil {
			t.Fatal(err)
		}
		if _, err = f.Write([]byte(data)); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	raw := b.Bytes()
	central := bytes.Index(raw, []byte{'P', 'K', 1, 2})
	for _, offset := range []int{6, central + 8} {
		binary.LittleEndian.PutUint16(raw[offset:], binary.LittleEndian.Uint16(raw[offset:])|32)
	}
	root := t.TempDir()
	src, dst := filepath.Join(root, "source.zip"), filepath.Join(root, "2.pptx")
	if err := os.WriteFile(src, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, []byte("sentinel"), 0600); err != nil {
		t.Fatal(err)
	}
	z, err := zip.OpenReader(src)
	if err != nil {
		t.Fatal(err)
	}
	defer z.Close()
	calls := 0
	out, err := EditStoredZIP(z, func(p *OrderedZIP) error {
		calls++
		if len(p.Parts) != 1 || p.Parts[0] != (ZIPPart{"original", "visible"}) {
			t.Fatalf("callback: %#v", p.Parts)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatal(calls)
	}
	if err = os.WriteFile(dst, out, 0600); err != nil {
		t.Fatal(err)
	}
	saved, err := zip.OpenReader(dst)
	if err != nil {
		t.Fatal(err)
	}
	defer saved.Close()
	parts, err := ReadOrderedZIP(saved)
	if err != nil {
		t.Fatal(err)
	}
	if len(parts.Parts) != 1 || parts.Parts[0] != (ZIPPart{"original", "visible"}) {
		t.Fatal(parts)
	}
	after, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, raw) {
		t.Fatal("source changed")
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || entries[0].Name() != "2.pptx" || entries[1].Name() != "source.zip" {
		t.Fatal(entries)
	}
}
