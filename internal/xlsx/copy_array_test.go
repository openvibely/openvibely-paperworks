package xlsx

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/beevik/etree"
	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"github.com/xuri/excelize/v2"
)

func arrayWorkbook(t *testing.T, dir string, formulas map[string][2]string) string {
	t.Helper()
	f := excelize.NewFile()
	defer f.Close()
	for i := 1; i <= 3; i++ {
		a, _ := excelize.CoordinatesToCellName(1, i)
		b, _ := excelize.CoordinatesToCellName(2, i)
		_ = f.SetCellValue("Sheet1", a, i)
		_ = f.SetCellValue("Sheet1", b, i*10)
	}
	for cell, spec := range formulas {
		var err error
		if spec[1] == "" {
			err = f.SetCellFormula("Sheet1", cell, spec[0])
		} else {
			kind, ref := excelize.STCellFormulaTypeArray, spec[1]
			err = f.SetCellFormula("Sheet1", cell, spec[0], excelize.FormulaOpts{Type: &kind, Ref: &ref})
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	path := filepath.Join(dir, "array.xlsx")
	if err := f.SaveAs(path); err != nil {
		t.Fatal(err)
	}
	return path
}

func sheetCells(t *testing.T, path string) map[string]*etree.Element {
	t.Helper()
	pkg, err := opc.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	wb, err := office.OpenWorkbook(pkg)
	if err != nil {
		t.Fatal(err)
	}
	return wb.Sheets["Sheet1"].Cells
}

func formulaOf(cells map[string]*etree.Element, address string) *etree.Element {
	if c := cells[address]; c != nil {
		return office.Child(c, office.NSS, "f")
	}
	return nil
}

func TestCopySingleCellArrayFormula(t *testing.T) {
	for _, operation := range []string{"copy", "copy-formulas"} {
		t.Run(operation, func(t *testing.T) {
			dir := t.TempDir()
			in := arrayWorkbook(t, dir, map[string][2]string{"C1": {"SUM(A1:A3*B1:B3)", "C1"}})
			out := filepath.Join(dir, "out.xlsx")
			if err := EditFile(context.Background(), in, out, dir, EditSpec{Edits: []Edit{{Operation: operation, Sheet: "Sheet1", Range: "C1", Target: "D1"}}}); err != nil {
				t.Fatal(err)
			}
			fe := formulaOf(sheetCells(t, out), "D1")
			if fe == nil || fe.SelectAttrValue("t", "") != "array" || fe.SelectAttrValue("ref", "") != "D1" || fe.Text() != "SUM(B1:B3*C1:C3)" {
				t.Fatalf("D1 formula = %#v", fe)
			}
		})
	}
}

func TestCopyWholeArrayBlock(t *testing.T) {
	dir := t.TempDir()
	in := arrayWorkbook(t, dir, map[string][2]string{"C1": {"A1:A3*B1:B3", "C1:C3"}})
	out := filepath.Join(dir, "out.xlsx")
	if err := EditFile(context.Background(), in, out, dir, EditSpec{Edits: []Edit{{Operation: "copy", Sheet: "Sheet1", Range: "C1:C3", Target: "D1"}}}); err != nil {
		t.Fatal(err)
	}
	cells := sheetCells(t, out)
	fe := formulaOf(cells, "D1")
	if fe == nil || fe.SelectAttrValue("t", "") != "array" || fe.SelectAttrValue("ref", "") != "D1:D3" || fe.Text() != "B1:B3*C1:C3" {
		t.Fatalf("D1 formula = %#v", fe)
	}
	for _, address := range []string{"D2", "D3"} {
		c := cells[address]
		if c == nil {
			continue
		}
		if formulaOf(cells, address) != nil || c.SelectAttrValue("t", "") != "" || office.Child(c, office.NSS, "v") != nil {
			t.Fatalf("%s has stray content: t=%q", address, c.SelectAttrValue("t", ""))
		}
	}
}

func TestCopyPartialArrayBlockFails(t *testing.T) {
	for _, source := range []string{"C1:C2", "C2", "B2:C3"} {
		dir := t.TempDir()
		in := arrayWorkbook(t, dir, map[string][2]string{"C1": {"A1:A3*B1:B3", "C1:C3"}})
		out := filepath.Join(dir, "out.xlsx")
		err := EditFile(context.Background(), in, out, dir, EditSpec{Edits: []Edit{{Operation: "copy", Sheet: "Sheet1", Range: source, Target: "E1"}}})
		if err == nil || !strings.Contains(err.Error(), "part of array formula") {
			t.Fatalf("%s: err = %v", source, err)
		}
	}
}

func TestCopyArrayBlockOutOfBounds(t *testing.T) {
	for _, target := range []string{"D1048575", "XFD1"} {
		dir := t.TempDir()
		in := arrayWorkbook(t, dir, map[string][2]string{"C1": {"A1:A3*B1:B3", "C1:D3"}})
		out := filepath.Join(dir, "out.xlsx")
		err := EditFile(context.Background(), in, out, dir, EditSpec{Edits: []Edit{{Operation: "copy-formulas", Sheet: "Sheet1", Range: "C1:D3", Target: target}}})
		if err == nil || !strings.Contains(err.Error(), "exceeds worksheet bounds") {
			t.Fatalf("%s: err = %v", target, err)
		}
	}
}

func TestCopyNormalFormulaStaysPlain(t *testing.T) {
	dir := t.TempDir()
	in := arrayWorkbook(t, dir, map[string][2]string{"C1": {"A1*B1", ""}})
	out := filepath.Join(dir, "out.xlsx")
	if err := EditFile(context.Background(), in, out, dir, EditSpec{Edits: []Edit{{Operation: "copy", Sheet: "Sheet1", Range: "C1", Target: "D2"}}}); err != nil {
		t.Fatal(err)
	}
	fe := formulaOf(sheetCells(t, out), "D2")
	if fe == nil || fe.SelectAttrValue("t", "") != "" || fe.SelectAttrValue("ref", "") != "" || fe.Text() != "B2*C2" {
		t.Fatalf("D2 formula = %#v", fe)
	}
}
