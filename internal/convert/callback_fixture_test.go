package convert

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"
)

// Exact original openpyxl/ReportLab callback inputs; mock PDF geometry and font
// encoding must not be substituted by the production Go PDF authoring engine.
func referenceCallbackFixture(t *testing.T, name string) []byte {
	t.Helper()
	z, err := zip.OpenReader("../../testdata/reference-mirror/callback-preview-fixtures.zip")
	if err != nil {
		t.Fatal(err)
	}
	defer z.Close()
	read := func(name string) []byte {
		t.Helper()
		r, err := z.Open(name)
		if err != nil {
			t.Fatal(err)
		}
		defer r.Close()
		b, err := io.ReadAll(r)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	var hashes map[string]string
	if err := json.Unmarshal(read("manifest.json"), &hashes); err != nil {
		t.Fatal(err)
	}
	b := read(name)
	sum := sha256.Sum256(b)
	if hex.EncodeToString(sum[:]) != hashes[name] {
		t.Fatal("original callback fixture hash mismatch", name)
	}
	return b
}
func referenceCallbackInput(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, referenceCallbackFixture(t, name), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}
