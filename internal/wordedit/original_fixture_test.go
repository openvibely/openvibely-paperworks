package wordedit

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

// Exact original ordered members; unchanged template payloads reuse the retained
// runtime Word bundle. ZIP container metadata is not a equivalence claim.
func referenceOriginalWordFixture(t *testing.T, name string) string {
	t.Helper()
	if name != "base" && !forReferenceOriginals(t, func(referenceOriginalSnapshot) {}) {
		referenceOriginalWordFixture(t, "base")
	}
	bundle, err := zip.OpenReader("../../testdata/reference-mirror/word-method-fixtures.zip")
	if err != nil {
		t.Fatal(err)
	}
	defer bundle.Close()
	base, err := zip.OpenReader("../../testdata/reference-mirror/runtime-word-fixtures.zip")
	if err != nil {
		t.Fatal(err)
	}
	defer base.Close()
	read := func(name string) []byte {
		t.Helper()
		r, e := bundle.Open(name)
		if e != nil {
			r, e = base.Open(name)
		}
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
	out := filepath.Join(t.TempDir(), name+".docx")
	if err := os.WriteFile(out, raw.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	registerReferenceOriginal(t, out)
	return out
}
