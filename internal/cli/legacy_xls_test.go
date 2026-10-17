package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestInspectLegacyXLS(t *testing.T) {
	raw, err := os.ReadFile("../../testdata/reference-mirror/full.xls")
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(t.TempDir(), "book.xls")
	if err = os.WriteFile(source, raw, 0600); err != nil {
		t.Fatal(err)
	}
	result := runCLI(t, "inspect", source)
	sheets := result["sheets"].([]any)
	if len(sheets) != 101 {
		t.Fatalf("sheets=%d", len(sheets))
	}
	for _, c := range []struct {
		sheet       int
		cell, value string
	}{{0, "HL10051", "OUTSIDE_PREVIEW"}, {100, "A1", "LAST_SHEET_VALUE"}} {
		if got := sheets[c.sheet].(map[string]any)["cells"].(map[string]any)[c.cell]; got != c.value {
			t.Fatalf("%d/%s=%v", c.sheet, c.cell, got)
		}
	}
	after, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(raw, after) {
		t.Fatal("inspect modified XLS source")
	}
}
