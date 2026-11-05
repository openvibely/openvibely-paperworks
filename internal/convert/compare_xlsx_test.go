package convert

import (
	"context"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/beevik/etree"
	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"github.com/openvibely/openvibely-paperworks/internal/pdf"
	"github.com/xuri/excelize/v2"
)

type compareTestCell struct {
	value   any
	formula string
}

type compareTestSheet struct {
	name  string
	cells map[string]compareTestCell
	bold  bool
}

func writeCompareTestWorkbook(t *testing.T, path string, sheets ...compareTestSheet) {
	t.Helper()
	book := excelize.NewFile()
	for i, sheet := range sheets {
		if i == 0 {
			if err := book.SetSheetName("Sheet1", sheet.name); err != nil {
				t.Fatal(err)
			}
		} else if _, err := book.NewSheet(sheet.name); err != nil {
			t.Fatal(err)
		}
		for address, cell := range sheet.cells {
			var err error
			if cell.formula != "" {
				err = book.SetCellFormula(sheet.name, address, cell.formula)
			} else {
				err = book.SetCellValue(sheet.name, address, cell.value)
			}
			if err != nil {
				t.Fatal(err)
			}
			if sheet.bold {
				style, err := book.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true}})
				if err != nil {
					t.Fatal(err)
				}
				if err := book.SetCellStyle(sheet.name, address, address, style); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
	if err := book.SaveAs(path); err != nil {
		t.Fatal(err)
	}
	if err := book.Close(); err != nil {
		t.Fatal(err)
	}
}

// SetCellFormula creates an empty cached result. Write the same nonempty cache
// into both test inputs to prove formula text is compared independently.
func setCompareTestFormulaCache(t *testing.T, path, address, value string) {
	t.Helper()
	pkg, err := opc.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := pkg.XML("xl/worksheets/sheet1.xml")
	if err != nil {
		t.Fatal(err)
	}
	var target *etree.Element
	for _, cell := range office.All(doc.Root(), office.NSS, "c") {
		if cell.SelectAttrValue("r", "") == address {
			target = cell
			break
		}
	}
	if target == nil {
		t.Fatalf("formula cell %s not found", address)
	}
	cached := office.Child(target, office.NSS, "v")
	if cached == nil {
		cached = target.CreateElement("v")
	}
	cached.SetText(value)
	if err := pkg.SetXML("xl/worksheets/sheet1.xml", doc); err != nil {
		t.Fatal(err)
	}
	patched := path + ".patched"
	if err := pkg.SaveNew(patched); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(patched, path); err != nil {
		t.Fatal(err)
	}
}

func findCellChange(changes []CellChange, sheet, address string) (CellChange, bool) {
	for _, change := range changes {
		if change.Sheet == sheet && change.Address == address {
			return change, true
		}
	}
	return CellChange{}, false
}

func TestCompareXLSXContentsLiteralAndFormulaEdits(t *testing.T) {
	before, after := filepath.Join(t.TempDir(), "before.xlsx"), filepath.Join(t.TempDir(), "after.xlsx")
	writeCompareTestWorkbook(t, before,
		compareTestSheet{name: "Model", cells: map[string]compareTestCell{"F2": {formula: "1+2"}}},
		compareTestSheet{name: "Inputs", cells: map[string]compareTestCell{"A1": {value: 12}}},
	)
	writeCompareTestWorkbook(t, after,
		compareTestSheet{name: "Model", cells: map[string]compareTestCell{"F2": {formula: "2+1"}}},
		compareTestSheet{name: "Inputs", cells: map[string]compareTestCell{"A1": {value: 15}}},
	)
	setCompareTestFormulaCache(t, before, "F2", "3")
	setCompareTestFormulaCache(t, after, "F2", "3")

	changes, err := compareXLSXContents(before, after)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 2 {
		t.Fatalf("got %d cell changes: %#v", len(changes), changes)
	}
	literal, ok := findCellChange(changes, "Inputs", "A1")
	if !ok || literal.Status != "changed" || literal.Before == nil || literal.After == nil || literal.Before.Value == nil || literal.After.Value == nil || *literal.Before.Value != "12" || *literal.After.Value != "15" {
		t.Fatalf("literal change = %#v", literal)
	}
	formula, ok := findCellChange(changes, "Model", "F2")
	if !ok || formula.Status != "changed" || formula.Before == nil || formula.After == nil || formula.Before.Formula == nil || formula.After.Formula == nil || *formula.Before.Formula != "1+2" || *formula.After.Formula != "2+1" {
		t.Fatalf("formula change = %#v", formula)
	}
	if formula.Before.CachedValue == nil || formula.After.CachedValue == nil || *formula.Before.CachedValue != "3" || *formula.After.CachedValue != "3" {
		t.Fatalf("cached formula values were not retained separately: %#v", formula)
	}
}

func TestCompareXLSXContentsKeepsSheetIdentityAndReportsAddedRemoved(t *testing.T) {
	before, after := filepath.Join(t.TempDir(), "before.xlsx"), filepath.Join(t.TempDir(), "after.xlsx")
	writeCompareTestWorkbook(t, before,
		compareTestSheet{name: "First", cells: map[string]compareTestCell{"A1": {value: "old first"}, "C1": {value: "remove me"}}},
		compareTestSheet{name: "Second", cells: map[string]compareTestCell{"A1": {value: "old second"}}},
	)
	writeCompareTestWorkbook(t, after,
		compareTestSheet{name: "First", cells: map[string]compareTestCell{"A1": {value: "new first"}, "B1": {value: "add me"}}},
		compareTestSheet{name: "Second", cells: map[string]compareTestCell{"A1": {value: "new second"}}},
	)

	changes, err := compareXLSXContents(before, after)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 4 {
		t.Fatalf("got %d cell changes: %#v", len(changes), changes)
	}
	for sheet, want := range map[string]string{"First": "new first", "Second": "new second"} {
		change, ok := findCellChange(changes, sheet, "A1")
		if !ok || change.Status != "changed" || change.After == nil || change.After.Value == nil || *change.After.Value != want {
			t.Fatalf("%s!A1 change = %#v", sheet, change)
		}
	}
	added, ok := findCellChange(changes, "First", "B1")
	if !ok || added.Status != "added" || added.Before != nil || added.After == nil || added.After.Value == nil || *added.After.Value != "add me" {
		t.Fatalf("added cell = %#v", added)
	}
	removed, ok := findCellChange(changes, "First", "C1")
	if !ok || removed.Status != "removed" || removed.Before == nil || removed.Before.Value == nil || *removed.Before.Value != "remove me" || removed.After != nil {
		t.Fatalf("removed cell = %#v", removed)
	}
}

func TestCompareXLSXContentsIgnoresFormattingAndIdenticalCells(t *testing.T) {
	before, after := filepath.Join(t.TempDir(), "before.xlsx"), filepath.Join(t.TempDir(), "after.xlsx")
	writeCompareTestWorkbook(t, before,
		compareTestSheet{name: "Data", cells: map[string]compareTestCell{"A1": {value: "same"}, "B2": {formula: "1+2"}}},
	)
	writeCompareTestWorkbook(t, after,
		compareTestSheet{name: "Data", cells: map[string]compareTestCell{"A1": {value: "same"}, "B2": {formula: "1+2"}}, bold: true},
	)
	setCompareTestFormulaCache(t, before, "B2", "3")
	setCompareTestFormulaCache(t, after, "B2", "3")

	changes, err := compareXLSXContents(before, after)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 0 {
		t.Fatalf("formatting-only or identical content generated cell changes: %#v", changes)
	}
	identical, err := compareXLSXContents(before, before)
	if err != nil {
		t.Fatal(err)
	}
	if len(identical) != 0 {
		t.Fatalf("identical workbook generated cell changes: %#v", identical)
	}
}

func TestCompareXLSXKeepsFormattingDifferencesInVisualPages(t *testing.T) {
	before, after := filepath.Join(t.TempDir(), "before.xlsx"), filepath.Join(t.TempDir(), "after.xlsx")
	writeCompareTestWorkbook(t, before, compareTestSheet{name: "Data", cells: map[string]compareTestCell{"A1": {value: "same"}}})
	writeCompareTestWorkbook(t, after, compareTestSheet{name: "Data", cells: map[string]compareTestCell{"A1": {value: "same"}}, bold: true})
	beforeImage, afterImage := filepath.Join(t.TempDir(), "before.png"), filepath.Join(t.TempDir(), "after.png")
	writeCompareTestPNG(t, beforeImage, 1)
	writeCompareTestPNG(t, afterImage, 2)

	ctx := withDeps(dependencies{
		convert: func(_ context.Context, input, _ string, options Options) (map[string]any, error) {
			if options.Recalculate {
				t.Fatalf("unexpected explicit recalculation for %s", input)
			}
			return map[string]any{"output": input}, nil
		},
		render: func(_ context.Context, input, _ string, _ pdf.RenderOptions) ([]string, error) {
			if input == before {
				return []string{beforeImage}, nil
			}
			return []string{afterImage}, nil
		},
	})
	result, err := Compare(ctx, before, after, filepath.Join(t.TempDir(), "compare"), pdf.RenderOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got := result["cell_changes"].([]CellChange); len(got) != 0 {
		t.Fatalf("formatting-only difference produced cell changes: %#v", got)
	}
	pages := result["pages"].([]PageChange)
	if len(pages) != 1 || pages[0].Status != "changed" {
		t.Fatalf("formatting difference was not retained as a visual page change: %#v", pages)
	}
}

func writeCompareTestPNG(t *testing.T, path string, red uint8) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	for y := 0; y < 2; y++ {
		for x := 0; x < 2; x++ {
			img.SetRGBA(x, y, color.RGBA{R: red, A: 255})
		}
	}
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(file, img); err != nil {
		file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}
