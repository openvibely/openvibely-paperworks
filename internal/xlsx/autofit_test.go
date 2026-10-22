package xlsx

import (
	"context"
	"github.com/xuri/excelize/v2"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
