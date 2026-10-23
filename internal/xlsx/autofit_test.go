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
