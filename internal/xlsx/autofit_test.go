package xlsx

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"github.com/xuri/excelize/v2"
)

func TestAutofitRowsSavesWrappedAndRichTextHeights(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "input.xlsx")
	out := filepath.Join(dir, "out.xlsx")
	f := excelize.NewFile()
	f.SetDefaultFont("Go")
	f.SetColWidth("Sheet1", "A", "B", 12)
	id, _ := f.NewStyle(&excelize.Style{Font: &excelize.Font{Family: "Go", Size: 11}, Alignment: &excelize.Alignment{WrapText: true}})
	f.SetCellStyle("Sheet1", "A1", "B4", id)
	f.SetCellValue("Sheet1", "A1", "Short")
	f.SetCellValue("Sheet1", "A2", "one two three four five six seven eight nine ten eleven twelve")
	f.SetCellValue("Sheet1", "A3", "first\nsecond\nthird")
	f.SetCellRichText("Sheet1", "A4", []excelize.RichTextRun{{Text: "Big", Font: &excelize.Font{Family: "Go", Size: 30, Bold: true}}})
	f.SetRowHeight("Sheet1", 5, 42)
	f.SetCellValue("Sheet1", "A5", "Untouched")
	if err := f.SaveAs(in); err != nil {
		t.Fatal(err)
	}
	f.Close()
	original, _ := os.ReadFile(in)
	if err := EditFile(context.Background(), in, out, dir, EditSpec{Edits: []Edit{{Operation: "autofit-rows", Sheet: "Sheet1", Range: "A1:B4"}}}); err != nil {
		t.Fatal(err)
	}
	g, err := excelize.OpenFile(out)
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	h1, _ := g.GetRowHeight("Sheet1", 1)
	h2, _ := g.GetRowHeight("Sheet1", 2)
	h3, _ := g.GetRowHeight("Sheet1", 3)
	h4, _ := g.GetRowHeight("Sheet1", 4)
	h5, _ := g.GetRowHeight("Sheet1", 5)
	if h2 <= h1*2 || h3 <= h1*2 || h4 < 36 || h5 != 42 {
		t.Fatal(h1, h2, h3, h4, h5)
	}
	if v, _ := g.GetCellValue("Sheet1", "A5"); v != "Untouched" {
		t.Fatal(v)
	}
	after, _ := os.ReadFile(in)
	if string(original) != string(after) {
		t.Fatal("source changed")
	}
}

func TestAutofitRowsAcceptsCachedEmptyStringFormulaWithOtherContent(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "input.xlsx")
	out := filepath.Join(dir, "out.xlsx")
	f := excelize.NewFile()
	if err := f.SetDefaultFont("Go"); err != nil {
		t.Fatal(err)
	}
	if err := f.SetColWidth("Sheet1", "B", "B", 9); err != nil {
		t.Fatal(err)
	}
	style, err := f.NewStyle(&excelize.Style{Alignment: &excelize.Alignment{WrapText: true}})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.SetCellStyle("Sheet1", "B1", "B1", style); err != nil {
		t.Fatal(err)
	}
	if err := f.SetCellFormula("Sheet1", "A1", `""`); err != nil {
		t.Fatal(err)
	}
	if err := f.SetCellValue("Sheet1", "B1", "A long visible value that wraps onto several lines in this narrow column"); err != nil {
		t.Fatal(err)
	}
	if err := f.SaveAs(in); err != nil {
		t.Fatal(err)
	}
	f.Close()
	setStringFormulaCache(t, in, "Sheet1", "A1", true)

	if err := EditFile(context.Background(), in, out, dir, EditSpec{Edits: []Edit{{Operation: "autofit-rows", Sheet: "Sheet1", Range: "A1:B1"}}}); err != nil {
		t.Fatal(err)
	}
	got, err := excelize.OpenFile(out)
	if err != nil {
		t.Fatal(err)
	}
	defer got.Close()
	height, err := got.GetRowHeight("Sheet1", 1)
	if err != nil {
		t.Fatal(err)
	}
	if height <= 20 {
		t.Fatalf("row height %v did not account for wrapped visible content", height)
	}
}

func TestAutofitRowsAcceptsCachedEmptyStringFormulaInEmptyRow(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "input.xlsx")
	out := filepath.Join(dir, "out.xlsx")
	f := excelize.NewFile()
	if err := f.SetDefaultFont("Go"); err != nil {
		t.Fatal(err)
	}
	if err := f.SetCellFormula("Sheet1", "A1", `""`); err != nil {
		t.Fatal(err)
	}
	if err := f.SaveAs(in); err != nil {
		t.Fatal(err)
	}
	f.Close()
	setStringFormulaCache(t, in, "Sheet1", "A1", true)

	if err := EditFile(context.Background(), in, out, dir, EditSpec{Edits: []Edit{{Operation: "autofit-rows", Sheet: "Sheet1", Range: "A1:A1"}}}); err != nil {
		t.Fatal(err)
	}
	got, err := excelize.OpenFile(out)
	if err != nil {
		t.Fatal(err)
	}
	defer got.Close()
	height, err := got.GetRowHeight("Sheet1", 1)
	if err != nil {
		t.Fatal(err)
	}
	if height != 16.2 {
		t.Fatalf("row height %v, want baseline 16.2", height)
	}
}

func TestAutofitRowsAcceptsCachedEmptyStringSharedFormulaFollower(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "input.xlsx")
	out := filepath.Join(dir, "out.xlsx")
	f := excelize.NewFile()
	if err := f.SetDefaultFont("Go"); err != nil {
		t.Fatal(err)
	}
	formulaType := excelize.STCellFormulaTypeShared
	ref := "A1:A2"
	if err := f.SetCellFormula("Sheet1", "A1", `IF(B1="","",B1)`, excelize.FormulaOpts{Type: &formulaType, Ref: &ref}); err != nil {
		t.Fatal(err)
	}
	if err := f.SaveAs(in); err != nil {
		t.Fatal(err)
	}
	f.Close()
	setStringFormulaCache(t, in, "Sheet1", "A2", true)

	if err := EditFile(context.Background(), in, out, dir, EditSpec{Edits: []Edit{{Operation: "autofit-rows", Sheet: "Sheet1", Range: "A2:A2"}}}); err != nil {
		t.Fatal(err)
	}
}

func TestAutofitRejectsFormulaWithoutCacheOnRecreatedSheet(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "input.xlsx")
	out := filepath.Join(dir, "out.xlsx")
	f := excelize.NewFile()
	if err := f.SetDefaultFont("Go"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.NewSheet("Other"); err != nil {
		t.Fatal(err)
	}
	if err := f.SetCellFormula("Sheet1", "A1", `""`); err != nil {
		t.Fatal(err)
	}
	if err := f.SaveAs(in); err != nil {
		t.Fatal(err)
	}
	f.Close()
	setStringFormulaCache(t, in, "Sheet1", "A1", true)

	spec := EditSpec{Edits: []Edit{
		{Operation: "delete-sheet", Sheet: "Sheet1"},
		{Operation: "add-sheet", Name: "Sheet1"},
		{Operation: "formula", Sheet: "Sheet1", Cell: "A1", Formula: `""`},
		{Operation: "autofit-rows", Sheet: "Sheet1", Range: "A1:A1"},
	}}
	err := EditFile(context.Background(), in, out, dir, spec)
	if err == nil || !strings.Contains(err.Error(), "A1 needs recalculation") {
		t.Fatalf("got %v, want recalculation error", err)
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Fatalf("published failed edit: %v", err)
	}
}

func TestAutofitRowsAcceptsCachedEmptyStringFormulaAfterSheetRename(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "input.xlsx")
	out := filepath.Join(dir, "out.xlsx")
	f := excelize.NewFile()
	if err := f.SetDefaultFont("Go"); err != nil {
		t.Fatal(err)
	}
	if err := f.SetCellFormula("Sheet1", "A1", `IF(Sheet1!B1="","",Sheet1!B1)`); err != nil {
		t.Fatal(err)
	}
	if err := f.SaveAs(in); err != nil {
		t.Fatal(err)
	}
	f.Close()
	setStringFormulaCache(t, in, "Sheet1", "A1", true)

	spec := EditSpec{Edits: []Edit{
		{Operation: "rename-sheet", Sheet: "Sheet1", Name: "Renamed"},
		{Operation: "autofit-rows", Sheet: "Renamed", Range: "A1:A1"},
	}}
	if err := EditFile(context.Background(), in, out, dir, spec); err != nil {
		t.Fatal(err)
	}
	got, err := excelize.OpenFile(out)
	if err != nil {
		t.Fatal(err)
	}
	defer got.Close()
	height, err := got.GetRowHeight("Renamed", 1)
	if err != nil {
		t.Fatal(err)
	}
	if height != 16.2 {
		t.Fatalf("row height %v, want baseline 16.2", height)
	}
}

func TestAutofitRowsRejectsFormulaWithoutCachedResultAndDoesNotPublish(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "input.xlsx")
	out := filepath.Join(dir, "out.xlsx")
	f := excelize.NewFile()
	if err := f.SetDefaultFont("Go"); err != nil {
		t.Fatal(err)
	}
	if err := f.SetCellFormula("Sheet1", "A1", `""`); err != nil {
		t.Fatal(err)
	}
	if err := f.SaveAs(in); err != nil {
		t.Fatal(err)
	}
	f.Close()
	setStringFormulaCache(t, in, "Sheet1", "A1", false)

	err := EditFile(context.Background(), in, out, dir, EditSpec{Edits: []Edit{{Operation: "autofit-rows", Sheet: "Sheet1", Range: "A1:A1"}}})
	if err == nil || !strings.Contains(err.Error(), "A1 needs recalculation") {
		t.Fatalf("got %v, want recalculation error", err)
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Fatalf("published failed edit: %v", err)
	}
}

func setStringFormulaCache(t *testing.T, path, sheetName, cellName string, hasEmptyResult bool) {
	t.Helper()
	pkg, err := opc.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	wb, err := office.OpenWorkbook(pkg)
	if err != nil {
		t.Fatal(err)
	}
	sheet := wb.Sheets[sheetName]
	if sheet == nil || sheet.Cells[cellName] == nil {
		t.Fatalf("missing test cell %s!%s", sheetName, cellName)
	}
	cell := sheet.Cells[cellName]
	cell.CreateAttr("t", "str")
	office.RemoveAll(cell, office.NSS, "v")
	if hasEmptyResult {
		office.NewChild(cell, office.NSS, "v", nil)
	}
	if err := pkg.SetXML(sheet.Part, sheet.Doc); err != nil {
		t.Fatal(err)
	}
	data, err := pkg.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
}

func TestAutofitRejectsVerticalMergeWithoutOutput(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "input.xlsx")
	out := filepath.Join(dir, "out.xlsx")
	f := excelize.NewFile()
	f.SetDefaultFont("Go")
	f.MergeCell("Sheet1", "A1", "A2")
	f.SetCellValue("Sheet1", "A1", "Merged")
	f.SaveAs(in)
	f.Close()
	err := EditFile(context.Background(), in, out, dir, EditSpec{Edits: []Edit{{Operation: "autofit-rows", Sheet: "Sheet1", Range: "A1:A2"}}})
	if err == nil {
		t.Fatal("accepted vertical merge")
	}
	if _, err = os.Stat(out); !os.IsNotExist(err) {
		t.Fatal("published failed edit", err)
	}
}

func TestAutofitHorizontalMergeUsesCombinedWidth(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "input.xlsx")
	f := excelize.NewFile()
	f.SetDefaultFont("Go")
	f.SetColWidth("Sheet1", "A", "B", 12)
	id, _ := f.NewStyle(&excelize.Style{Alignment: &excelize.Alignment{WrapText: true}})
	f.SetCellStyle("Sheet1", "A1", "B2", id)
	text := "Long text that needs multiple lines in a narrow cell"
	f.SetCellValue("Sheet1", "A1", text)
	f.SetCellValue("Sheet1", "A2", text)
	f.MergeCell("Sheet1", "A2", "B2")
	if err := f.SaveAs(in); err != nil {
		t.Fatal(err)
	}
	f.Close()
	out := filepath.Join(dir, "out.xlsx")
	if err := EditFile(context.Background(), in, out, dir, EditSpec{Edits: []Edit{{Operation: "autofit-rows", Sheet: "Sheet1", Range: "A1:B2"}}}); err != nil {
		t.Fatal(err)
	}
	g, err := excelize.OpenFile(out)
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	narrow, _ := g.GetRowHeight("Sheet1", 1)
	wide, _ := g.GetRowHeight("Sheet1", 2)
	if narrow <= wide {
		t.Fatal("combined merge width was ignored", narrow, wide)
	}
	partial := filepath.Join(dir, "partial.xlsx")
	if err := EditFile(context.Background(), in, partial, dir, EditSpec{Edits: []Edit{{Operation: "autofit-rows", Sheet: "Sheet1", Range: "A2"}}}); err == nil {
		t.Fatal("accepted partial merged range")
	}
	if _, err := os.Stat(partial); !os.IsNotExist(err) {
		t.Fatal("published failed edit", err)
	}
}

func TestAutofitRowsPreservesBaselineHeightsForCoveredCells(t *testing.T) {
	f := excelize.NewFile()
	defer f.Close()
	if err := f.SetDefaultFont("Go"); err != nil {
		t.Fatal(err)
	}
	if err := f.SetColWidth("Sheet1", "A", "J", 12); err != nil {
		t.Fatal(err)
	}
	style, err := f.NewStyle(&excelize.Style{Font: &excelize.Font{Family: "Go", Size: 11}, Alignment: &excelize.Alignment{WrapText: true}})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.SetCellStyle("Sheet1", "A1", "J4", style); err != nil {
		t.Fatal(err)
	}
	if err := f.SetCellValue("Sheet1", "A1", "This unmerged row wraps across its narrow column width."); err != nil {
		t.Fatal(err)
	}
	for _, merge := range []struct{ start, end, value string }{
		{"A2", "C2", "This horizontal merge has enough text to wrap onto another line."},
		{"D3", "F3", "A second merged row with wrapped text."},
		{"A4", "J4", "short"},
	} {
		if err := f.MergeCell("Sheet1", merge.start, merge.end); err != nil {
			t.Fatal(err)
		}
		if err := f.SetCellValue("Sheet1", merge.start, merge.value); err != nil {
			t.Fatal(err)
		}
	}
	coveredStyle, err := f.NewStyle(&excelize.Style{Font: &excelize.Font{Family: "Go", Size: 72}, Alignment: &excelize.Alignment{WrapText: true}})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.SetCellStyle("Sheet1", "E3", "E3", coveredStyle); err != nil {
		t.Fatal(err)
	}
	if err := f.SetCellValue("Sheet1", "E3", "Covered cell content must not change the merge height."); err != nil {
		t.Fatal(err)
	}
	if err := f.SetRowHeight("Sheet1", 5, 42); err != nil {
		t.Fatal(err)
	}
	if err := autofitRows(f, Edit{Sheet: "Sheet1", Range: "A1:J4"}); err != nil {
		t.Fatal(err)
	}
	want := []float64{69, 29.4, 29.4, 16.2, 42}
	for row, expected := range want {
		got, err := f.GetRowHeight("Sheet1", row+1)
		if err != nil {
			t.Fatal(err)
		}
		if got < expected-0.01 || got > expected+0.01 {
			t.Fatalf("row %d height = %.2f, want baseline %.2f", row+1, got, expected)
		}
	}
}

func TestAutofitRowsHandlesManyHorizontalMerges(t *testing.T) {
	f := excelize.NewFile()
	defer f.Close()
	if err := f.SetDefaultFont("Go"); err != nil {
		t.Fatal(err)
	}
	style, err := f.NewStyle(&excelize.Style{Font: &excelize.Font{Family: "Go", Size: 11}, Alignment: &excelize.Alignment{WrapText: true}})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.SetCellStyle("Sheet1", "A1", "J1000", style); err != nil {
		t.Fatal(err)
	}
	for row := 1; row <= 1000; row++ {
		start, _ := excelize.CoordinatesToCellName(1, row)
		end, _ := excelize.CoordinatesToCellName(10, row)
		if err := f.MergeCell("Sheet1", start, end); err != nil {
			t.Fatal(err)
		}
		if err := f.SetCellValue("Sheet1", start, strings.Repeat("wrapped merge text ", 8)); err != nil {
			t.Fatal(err)
		}
	}
	if err := f.SetRowHeight("Sheet1", 1001, 42); err != nil {
		t.Fatal(err)
	}
	if err := autofitRows(f, Edit{Sheet: "Sheet1", Range: "A1:J1000"}); err != nil {
		t.Fatal(err)
	}
	first, err := f.GetRowHeight("Sheet1", 1)
	if err != nil {
		t.Fatal(err)
	}
	last, err := f.GetRowHeight("Sheet1", 1000)
	if err != nil {
		t.Fatal(err)
	}
	if first < 16.2 || last != first {
		t.Fatalf("merged row heights = %.2f and %.2f, want consistent fitted heights", first, last)
	}
	untouched, err := f.GetRowHeight("Sheet1", 1001)
	if err != nil {
		t.Fatal(err)
	}
	if untouched != 42 {
		t.Fatalf("untouched row height = %.2f, want 42", untouched)
	}
}

func TestAutofitVisitsSingleCellAndAllRectangleCorners(t *testing.T) {
	cases := []struct {
		name, r, cell string
	}{
		{name: "single", r: "A1", cell: "A1"},
		{name: "top-left", r: "B2:C3", cell: "B2"},
		{name: "top-right", r: "B2:C3", cell: "C2"},
		{name: "bottom-left", r: "B2:C3", cell: "B3"},
		{name: "bottom-right", r: "B2:C3", cell: "C3"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := excelize.NewFile()
			defer f.Close()
			if err := f.SetDefaultFont("Go"); err != nil {
				t.Fatal(err)
			}
			if err := f.SetCellFormula("Sheet1", tc.cell, "1+1"); err != nil {
				t.Fatal(err)
			}
			err := autofitRows(f, Edit{Sheet: "Sheet1", Range: tc.r})
			if err == nil || !strings.Contains(err.Error(), tc.cell+" needs recalculation") {
				t.Fatalf("got %v, want recalculation error at %s", err, tc.cell)
			}
		})
	}
}

func TestAutofitRejectsRangesOverItsCellLimit(t *testing.T) {
	f := excelize.NewFile()
	defer f.Close()
	err := autofitRows(f, Edit{Sheet: "Sheet1", Range: "A1:J1001"})
	if err == nil || !strings.Contains(err.Error(), "autofit range exceeds 10000 cells") {
		t.Fatalf("got %v, want autofit range limit error", err)
	}
}

func BenchmarkAutofitRowsMergeLookup(b *testing.B) {
	for _, tc := range []struct {
		name   string
		merges int
	}{
		{name: "none"},
		{name: "small", merges: 10},
		{name: "heavy_10k_cells_1k_merges", merges: 1000},
	} {
		b.Run(tc.name, func(b *testing.B) {
			f := excelize.NewFile()
			defer f.Close()
			if err := f.SetDefaultFont("Go"); err != nil {
				b.Fatal(err)
			}
			for i := 0; i < tc.merges; i++ {
				row := i + 1
				start, _ := excelize.CoordinatesToCellName(1, row)
				end, _ := excelize.CoordinatesToCellName(10, row)
				if err := f.MergeCell("Sheet1", start, end); err != nil {
					b.Fatal(err)
				}
				if err := f.SetCellValue("Sheet1", start, "A long value in a merged row"); err != nil {
					b.Fatal(err)
				}
			}
			edit := Edit{Sheet: "Sheet1", Range: "A1:J1000"}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if err := autofitRows(f, edit); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
