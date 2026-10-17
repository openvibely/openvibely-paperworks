package office

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/openvibely/openvibely-paperworks/internal/opc"
)

// Exact original reference presentation library notes fixture, stored as a small delta against the
// retained default template. Every reconstructed member is independently hashed.
func referenceNotesFixture(t *testing.T) string {
	t.Helper()
	base, err := os.ReadFile("../../testdata/reference-mirror/literal-chart.pptx")
	if err != nil {
		t.Fatal(err)
	}
	delta, err := opc.Open("../../testdata/reference-mirror/notes-master-delta.zip")
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		BaseSHA        string `json:"base_sha256"`
		Names, Removed []string
		Hashes         map[string]string `json:"parts_sha256"`
	}
	if err = json.Unmarshal(delta.Get("fixture.json"), &manifest); err != nil {
		t.Fatal(err)
	}
	hash := func(b []byte) string { v := sha256.Sum256(b); return hex.EncodeToString(v[:]) }
	if hash(base) != manifest.BaseSHA {
		t.Fatal("notes fixture base changed")
	}
	p, err := opc.Read(base)
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range manifest.Removed {
		p.Remove(n)
	}
	for _, part := range delta.Parts {
		if part.Name != "fixture.json" {
			p.Set(part.Name, part.Data)
		}
	}
	byName := map[string]*opc.Part{}
	for _, part := range p.Parts {
		byName[part.Name] = part
	}
	if len(byName) != len(manifest.Names) {
		t.Fatal("notes fixture member count")
	}
	p.Parts = nil
	for _, n := range manifest.Names {
		part := byName[n]
		if part == nil || hash(part.Data) != manifest.Hashes[n] {
			t.Fatal("notes fixture member mismatch", n)
		}
		p.Parts = append(p.Parts, part)
	}
	out := filepath.Join(t.TempDir(), "notes-source.pptx")
	if err = p.SaveNew(out); err != nil {
		t.Fatal(err)
	}
	return out
}
