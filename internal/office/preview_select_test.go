package office

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/xuri/excelize/v2"
)

func TestPreviewRejectsInvalidSelectionWithoutOutput(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source.xlsx")
	f := excelize.NewFile()
	if err := f.SaveAs(source); err != nil {
		t.Fatal(err)
	}
	f.Close()
	before, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	// First seven cases map test_workbook_preview_rejects_invalid_selection.
	for _, tc := range []struct{ sheet, area string }{
		{"Missing", "A1:B2"}, {"Sheet1", "A:A"}, {"Sheet1", "A0:B2"},
		{"Sheet1", "B2:A1"}, {"Sheet1", "A1:XFE2"}, {"Sheet1", "A1:B20000"},
		{"Sheet1", "A1,B2"}, {"Sheet1", "B1:A2"}, {"Sheet1", "A2:B1"},
		{"Sheet1", "A999999999999999999999999:B2"}, {"Sheet1", "A1:B999999999999999999999999"},
	} {
		t.Run(tc.sheet+"/"+tc.area, func(t *testing.T) {
			output := filepath.Join(t.TempDir(), "preview.xlsx")
			if err := WorkbookPreviewCopy(source, output, tc.sheet, tc.area); err == nil {
				t.Fatal("accepted invalid selection")
			}
			if _, err := os.Stat(output); !os.IsNotExist(err) {
				t.Fatalf("output exists or stat failed: %v", err)
			}
			after, err := os.ReadFile(source)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatal("source changed", err)
			}
		})
	}
	// General range users retain normalization; preview alone rejects reversed endpoints.
	l, top, r, b, err := ParseRange("B2:A1")
	if err != nil || l != 1 || top != 1 || r != 2 || b != 2 {
		t.Fatal(l, top, r, b, err)
	}
	for _, area := range []string{"A1", "$A$1:$B$2", "A1:J1000", "XFD1048576"} {
		if err := WorkbookPreviewCopy(source, filepath.Join(t.TempDir(), "preview.xlsx"), "Sheet1", area); err != nil {
			t.Fatal(area, err)
		}
	}
}
