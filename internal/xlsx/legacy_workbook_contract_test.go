package xlsx

import (
	"crypto/sha256"
	"os"
	"path/filepath"
	"testing"
)

// Exact read-only legacy workbook contract from test_document_libraries.py
// lines 248-258. No conversion or preview may hide late sheets/cells.
// The pinned BIFF fixture is necessary: generating XLSX would not test XLS.
func TestReferenceStandaloneLibrariesLegacyXLS(t *testing.T) {
	raw, err := os.ReadFile("../../testdata/reference-mirror/full.xls")
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(t.TempDir(), "full.xls")
	if err := os.WriteFile(source, raw, 0600); err != nil {
		t.Fatal(err)
	}
	before := sha256.Sum256(raw)
	// Keep each original assertion independently visible if reading is unsupported.
	for _, check := range []string{"sheet-count", "outside-preview", "last-sheet"} {
		t.Run(check, func(t *testing.T) {
			book, err := ReadLegacy(source)
			if err != nil {
				t.Fatalf("legacy XLS read capability: %v", err)
			}
			sheets := book.Sheets
			switch check {
			case "sheet-count":
				if len(sheets) != 101 {
					t.Errorf("sheets=%d, want 101", len(sheets))
				}
			case "outside-preview", "last-sheet":
				index, row, col, want := 0, 10051, 220, "OUTSIDE_PREVIEW"
				if check == "last-sheet" {
					index, row, col, want = 100, 1, 1, "LAST_SHEET_VALUE"
				}
				if len(sheets) <= index {
					t.Fatalf("missing sheet index %d", index)
				}
				cell := legacyAddress(row-1, col-1)
				got, err := book.CellValue(index, row-1, col-1)
				if err != nil {
					t.Fatal(err)
				}
				if got != want {
					t.Errorf("sheet[%d] %s=%q, want %q", index, cell, got, want)
				}
			}
		})
	}
	t.Run("source-digest", func(t *testing.T) {
		after, err := os.ReadFile(source)
		if err != nil {
			t.Fatal(err)
		}
		if sha256.Sum256(after) != before {
			t.Error("legacy source digest changed")
		}
	})
}
