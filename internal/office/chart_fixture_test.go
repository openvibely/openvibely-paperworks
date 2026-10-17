package office

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// Original openpyxl packages, deduplicated by uncompressed member SHA. The
// original method fixtures include serialization/order details used by cache
// preservation checks; they must not be regenerated with Excelize defaults.
func referenceChartCacheFixture(t *testing.T, name string) string {
	t.Helper()
	path := referenceOriginalWorkbookFixture(t, "chart-cache-fixtures.zip", name)
	// These callers consume packages immediately after DocumentGapChecks.rewrite.
	// The blocked array input uses the ordinary loader after its later openpyxl save.
	// Preserve that same-path ZIP Store boundary, not the loader's Deflate default.
	if err := referenceRewriteZIP(path, func(*referenceOrderedZIP) error { return nil }); err != nil {
		t.Fatal(err)
	}
	return path
}

func referenceOriginalWorkbookFixture(t *testing.T, archive, name string) string {
	t.Helper()
	bundle := referencePreviewZIP(t, "../../testdata/reference-mirror/"+archive)
	var manifest struct {
		Fixtures map[string][]struct {
			Name string
			SHA  string `json:"sha256"`
		}
	}
	if err := json.Unmarshal(referencePreviewZIPRead(t, bundle, "manifest.json"), &manifest); err != nil {
		t.Fatal(err)
	}
	parts, ok := manifest.Fixtures[name]
	if !ok || len(parts) == 0 {
		t.Fatal("unknown chart fixture", name)
	}
	var raw bytes.Buffer
	z := zip.NewWriter(&raw)
	seen := map[string]bool{}
	for _, p := range parts {
		if seen[p.Name] {
			t.Fatal("duplicate fixture member", p.Name)
		}
		seen[p.Name] = true
		data := referencePreviewZIPRead(t, bundle, "parts/"+p.SHA)
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
	out := filepath.Join(t.TempDir(), name+".xlsx")
	if err := os.WriteFile(out, raw.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	return out
}
