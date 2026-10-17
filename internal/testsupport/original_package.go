package testsupport

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"
)

// OriginalPackageFixture reconstructs pinned ordered members, checking every payload hash.
func OriginalPackageFixture(t *testing.T, archive, name, extension string) string {
	t.Helper()
	bundle, err := zip.OpenReader("../../testdata/reference-mirror/" + archive)
	if err != nil {
		t.Fatal(err)
	}
	defer bundle.Close()
	read := func(name string) []byte {
		t.Helper()
		r, e := bundle.Open(name)
		if e != nil {
			t.Fatal(e)
		}
		defer r.Close()
		b, e := io.ReadAll(r)
		if e != nil {
			t.Fatal(e)
		}
		return b
	}
	var manifest struct {
		Fixtures map[string][]struct {
			Name string
			SHA  string `json:"sha256"`
		}
	}
	if err := json.Unmarshal(read("manifest.json"), &manifest); err != nil {
		t.Fatal(err)
	}
	parts, ok := manifest.Fixtures[name]
	if !ok || len(parts) == 0 {
		t.Fatal("unknown original package fixture", name)
	}
	var raw bytes.Buffer
	z := zip.NewWriter(&raw)
	seen := map[string]bool{}
	for _, p := range parts {
		if seen[p.Name] {
			t.Fatal("duplicate fixture member", p.Name)
		}
		seen[p.Name] = true
		data := read("parts/" + p.SHA)
		sum := sha256.Sum256(data)
		if hex.EncodeToString(sum[:]) != p.SHA {
			t.Fatal("fixture member hash mismatch", p.Name)
		}
		w, err := z.Create(p.Name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = w.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), name+extension)
	if err := os.WriteFile(out, raw.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	return out
}
