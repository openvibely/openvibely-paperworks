package convert

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/pdf"
	"github.com/xuri/excelize/v2"
)

// UNRUN: complete original test_workbook_preview_rejects_invalid_selection.
// Keep the seven preview failures and three render failures in one sequence,
// sharing the saved source and output paths as the Reference method does.
func TestReferenceWorkbookPreviewRejectsInvalidSelection(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source.xlsx")
	f := excelize.NewFile()
	if err := f.SetSheetName("Sheet1", "Sheet"); err != nil {
		t.Fatal(err)
	}
	if err := f.SaveAs(source); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(dir, "bad.xlsx")
	for _, c := range []struct{ sheet, area, reason string }{
		{"Missing", "A1:B2", "unknown preview worksheet"}, {"Sheet", "A:A", "single finite A1"}, {"Sheet", "A0:B2", "single finite A1"},
		{"Sheet", "B2:A1", "top-left to bottom-right"}, {"Sheet", "A1:XFE2", "worksheet bounds"}, {"Sheet", "A1:B20000", "at most 10000 cells"}, {"Sheet", "A1,B2", "single finite A1"},
	} {
		if err := office.WorkbookPreviewCopy(source, output, c.sheet, c.area); err == nil || !strings.Contains(err.Error(), c.reason) {
			t.Fatal("expected selection validation error", c, err)
		}
		if _, err := os.Stat(output); !os.IsNotExist(err) {
			t.Fatal("failed preview output exists or cannot be inspected", err)
		}
	}
	for _, name := range []string{"source.xlsx", "source.pdf", "source.pptx"} {
		_, err := RenderFile(context.Background(), filepath.Join(dir, name), filepath.Join(dir, "render"), "", "A1:B2", pdf.RenderOptions{})
		if err == nil {
			t.Fatal("range without sheet accepted", name)
		}
		// Additional diagnostic guard: a missing-file or renderer failure must not
		// accidentally satisfy the original validation-error assertion.
		if !strings.Contains(err.Error(), "requires XLSX and --sheet") {
			t.Fatal("wrong failure", name, err)
		}
	}
}
