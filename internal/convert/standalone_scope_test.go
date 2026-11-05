package convert

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openvibely/openvibely-paperworks/internal/pdf"
	"github.com/xuri/excelize/v2"
)

// Original runtime lines 785-835: whole-sheet preview sizing disregards print
// areas, while hidden/empty sheets must not shift selected sheet indexes.
func TestReferenceStandaloneRuntimeWholeSheetScope(t *testing.T) {
	referenceRuntimeOffice(t)
	src := referenceRuntimeOriginalWorkbookFixture(t, "runtime-workflow-fixtures.zip", "whole-sheet")
	before := contents(t, src)
	for _, c := range []struct{ name, sheet, area string }{{"selected", "Report", ""}, {"all", "", ""}, {"range", "Report", "A1:D20"}} {
		t.Run(c.name, func(t *testing.T) {
			out := filepath.Join(t.TempDir(), "render")
			_, e := RenderFile(context.Background(), src, out, c.sheet, c.area, pdf.RenderOptions{})
			if e != nil {
				t.Fatal(e)
			}
			pages := referenceRuntimePDFTexts(t, contents(t, filepath.Join(out, "whole-sheet.pdf")))
			text := strings.Join(pages, "\n")
			t.Run("printed-and-outside", func(t *testing.T) {
				if !strings.Contains(text, "PRINTED") || !strings.Contains(text, "OUTSIDE_PRINT_AREA") {
					t.Error(text)
				}
			})
			t.Run("other-sheet-scope", func(t *testing.T) {
				if c.sheet != "" {
					if strings.Contains(text, "OTHER_SHEET") {
						t.Error(text)
					}
				} else if !strings.Contains(text, "OTHER_SHEET") || !strings.Contains(text, "OTHER_OUTSIDE") {
					t.Error(text)
				}
			})
			t.Run("source-preserved", func(t *testing.T) {
				if !bytes.Equal(before, contents(t, src)) {
					t.Error("source changed")
				}
			})
		})
	}
}

func TestReferenceStandaloneRuntimeConvertPreservesPrintAreaSelections(t *testing.T) {
	referenceRuntimeOffice(t)
	source := referenceRuntimeOriginalWorkbookFixture(t, "runtime-workflow-fixtures.zip", "whole-sheet")
	before := contents(t, source)
	for _, tc := range []struct {
		name, sheet, cellRange string
	}{
		{name: "default"},
		{name: "sheet-only", sheet: "Report"},
		{name: "sheet-range-override", sheet: "Report", cellRange: "A1:D20"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out := filepath.Join(t.TempDir(), "pdf-output")
			result, err := Convert(context.Background(), source, out, Options{Sheet: tc.sheet, CellRange: tc.cellRange})
			if err != nil {
				t.Fatal(err)
			}
			pages := referenceRuntimePDFTexts(t, contents(t, result["output"].(string)))
			text := strings.Join(pages, "\n")
			if !strings.Contains(text, "PRINTED") {
				t.Errorf("saved print area content missing: %s", text)
			}
			if tc.cellRange == "" {
				if strings.Contains(text, "OUTSIDE_PRINT_AREA") || strings.Contains(text, "OTHER_OUTSIDE") {
					t.Errorf("content outside saved print areas was exported: %s", text)
				}
			} else if !strings.Contains(text, "OUTSIDE_PRINT_AREA") {
				t.Errorf("explicit range did not override saved print area: %s", text)
			}
			if tc.sheet != "" {
				if strings.Contains(text, "OTHER_SHEET") || strings.Contains(text, "OTHER_OUTSIDE") {
					t.Errorf("another worksheet leaked into selected conversion: %s", text)
				}
			} else if !strings.Contains(text, "OTHER_SHEET") {
				t.Errorf("second visible worksheet missing: %s", text)
			}
			if !bytes.Equal(before, contents(t, source)) {
				t.Error("source workbook changed")
			}
		})
	}
}

func TestReferenceStandaloneRuntimeWorkbookWithoutPrintAreasUsesUsedRange(t *testing.T) {
	referenceRuntimeOffice(t)
	dir := t.TempDir()
	f := excelize.NewFile()
	t.Cleanup(func() { _ = f.Close() })
	if err := f.SetCellValue("Sheet1", "A1", "USED_START"); err != nil {
		t.Fatal(err)
	}
	if err := f.SetCellValue("Sheet1", "C8", "USED_END"); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(dir, "no-print-area.xlsx")
	if err := f.SaveAs(source); err != nil {
		t.Fatal(err)
	}
	for _, name := range f.GetDefinedName() {
		if name.Name == "_xlnm.Print_Area" {
			t.Fatal("fixture unexpectedly has a saved print area")
		}
	}
	before := contents(t, source)
	out := filepath.Join(dir, "pdf-output")
	result, err := Convert(context.Background(), source, out, Options{})
	if err != nil {
		t.Fatal(err)
	}
	pages := referenceRuntimePDFTexts(t, contents(t, result["output"].(string)))
	text := strings.Join(pages, "\n")
	if len(pages) == 0 || !strings.Contains(text, "USED_START") || !strings.Contains(text, "USED_END") {
		t.Fatalf("used range was not exported: %q", text)
	}
	if !bytes.Equal(before, contents(t, source)) {
		t.Fatal("source workbook changed")
	}
}

func TestReferenceStandaloneRuntimeAutocrop(t *testing.T) {
	referenceRuntimeOffice(t)
	src := referenceRuntimeOriginalWorkbookFixture(t, "runtime-workflow-fixtures.zip", "autocrop")
	before := contents(t, src)
	for _, sheet := range []string{"Wide", "Tall", ""} {
		name := sheet
		if name == "" {
			name = "all"
		}
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			out := filepath.Join(t.TempDir(), "render")
			_, e := RenderFile(ctx, src, out, sheet, "", pdf.RenderOptions{})
			if e != nil {
				t.Fatal(e)
			}
			data := contents(t, filepath.Join(out, "autocrop.pdf"))
			pages := referenceRuntimePDFTexts(t, data)
			dims := referenceRuntimePDFDimensions(t, data)
			text := strings.Join(pages, "\n")
			t.Run("hidden-dependency", func(t *testing.T) {
				if strings.Contains(text, "HIDDEN_DEPENDENCY") {
					t.Error(text)
				}
			})
			if sheet != "" {
				t.Run("page-count", func(t *testing.T) {
					if len(pages) != 1 {
						t.Errorf("pages %d, want 1", len(pages))
					}
				})
			}
			if sheet == "Wide" {
				t.Run("dimensions", func(t *testing.T) {
					if len(dims) == 0 || dims[0].Width <= 1000 || dims[0].Height >= 100 {
						t.Error(dims)
					}
				})
				t.Run("all-columns", func(t *testing.T) {
					for c := 1; c <= 20; c++ {
						if !strings.Contains(text, fmt.Sprintf("Metric%02d", c)) {
							t.Errorf("missing column %d: %s", c, text)
						}
					}
				})
				t.Run("value-and-scope", func(t *testing.T) {
					if !strings.Contains(text, "2000") || strings.Contains(text, "TALL_START") {
						t.Error(text)
					}
				})
			} else if sheet == "Tall" {
				t.Run("text-and-scope", func(t *testing.T) {
					if !strings.Contains(text, "TALL_START") || !strings.Contains(text, "TALL_END") || strings.Contains(text, "Metric01") {
						t.Error(text)
					}
				})
				t.Run("dimensions", func(t *testing.T) {
					if len(dims) == 0 || dims[0].Height <= 1000 || dims[0].Width >= 200 {
						t.Error(dims)
					}
				})
			} else {
				t.Run("wide-page", func(t *testing.T) {
					for _, p := range pages {
						if strings.Contains(p, "Metric01") && strings.Contains(p, "Metric20") && strings.Contains(p, "2000") {
							return
						}
					}
					t.Error(pages)
				})
				t.Run("tall-page", func(t *testing.T) {
					for _, p := range pages {
						if strings.Contains(p, "TALL_START") && strings.Contains(p, "TALL_END") {
							return
						}
					}
					t.Error(pages)
				})
			}
			t.Run("source-preserved", func(t *testing.T) {
				if !bytes.Equal(before, contents(t, src)) {
					t.Error("source changed")
				}
			})
		})
	}
}
