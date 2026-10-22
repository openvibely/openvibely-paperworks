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

func TestCopyRejectsPartialDestinationArray(t *testing.T) {
	cases := []struct {
		name, operation, source, sheet, target string
		formulas                               map[string][2]string
	}{
		{"same-sheet overlap with source array", "copy", "C1:C3", "", "C2", map[string][2]string{"C1": {"A1:A3*B1:B3", "C1:C3"}}},
		{"existing array at destination", "copy", "C1:C3", "", "E2", map[string][2]string{"C1": {"A1:A3*B1:B3", "C1:C3"}, "E1": {"A1:A3*2", "E1:E3"}}},
		{"existing array under plain copy", "copy-formulas", "A1", "", "E2", map[string][2]string{"E1": {"A1:A3*2", "E1:E3"}}},
		{"existing array under values copy", "copy-values", "A1:A2", "", "E2", map[string][2]string{"E1": {"A1:A3*2", "E1:E3"}}},
		{"existing array on other sheet", "copy", "C1:C3", "Other", "E2", map[string][2]string{"C1": {"A1:A3*B1:B3", "C1:C3"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			in := arrayWorkbook(t, dir, tc.formulas)
			out := filepath.Join(dir, "out.xlsx")
			edits := []Edit{}
			if tc.sheet != "" {
				edits = append(edits,
					Edit{Operation: "add-sheet", Name: tc.sheet},
					Edit{Operation: "copy", Sheet: "Sheet1", Range: "C1:C3", Name: tc.sheet, Target: "E1"})
			}
			edits = append(edits, Edit{Operation: tc.operation, Sheet: "Sheet1", Range: tc.source, Name: tc.sheet, Target: tc.target})
			err := EditFile(context.Background(), in, out, dir, EditSpec{Edits: edits})
			if err == nil || !strings.Contains(err.Error(), "cannot change part of array formula") {
				t.Fatalf("err = %v", err)
			}
		})
	}
}

func TestCopyReplacesWhollyCoveredDestinationArray(t *testing.T) {
	for _, operation := range []string{"copy", "copy-values"} {
		t.Run(operation, func(t *testing.T) {
			dir := t.TempDir()
			in := arrayWorkbook(t, dir, map[string][2]string{"C1": {"A1:A3*B1:B3", "C1:C3"}, "E1": {"A1:A3*2", "E1:E3"}})
			out := filepath.Join(dir, "out.xlsx")
			if err := EditFile(context.Background(), in, out, dir, EditSpec{Edits: []Edit{{Operation: operation, Sheet: "Sheet1", Range: "B1:C3", Target: "D1"}}}); err != nil {
				t.Fatal(err)
			}
			cells := sheetCells(t, out)
			masters := 0
			for address := range cells {
				if fe := formulaOf(cells, address); fe != nil && fe.SelectAttrValue("t", "") == "array" {
					masters++
					if address != "C1" && !(operation == "copy" && address == "E1" && fe.SelectAttrValue("ref", "") == "E1:E3" && fe.Text() == "C1:C3*D1:D3") {
						t.Fatalf("unexpected array %s ref=%q %q", address, fe.SelectAttrValue("ref", ""), fe.Text())
					}
				}
			}
			if want := map[string]int{"copy": 2, "copy-values": 1}[operation]; masters != want {
				t.Fatalf("array masters = %d, want %d", masters, want)
			}
			for _, address := range []string{"D1", "D2", "D3"} {
				if formulaOf(cells, address) != nil {
					t.Fatalf("%s retained a formula", address)
				}
			}
		})
	}
}

func TestCopyArrayBlockOntoItself(t *testing.T) {
	dir := t.TempDir()
	in := arrayWorkbook(t, dir, map[string][2]string{"C1": {"A1:A3*B1:B3", "C1:C3"}})
	out := filepath.Join(dir, "out.xlsx")
	if err := EditFile(context.Background(), in, out, dir, EditSpec{Edits: []Edit{{Operation: "copy", Sheet: "Sheet1", Range: "C1:C3", Target: "C1"}}}); err != nil {
		t.Fatal(err)
	}
	fe := formulaOf(sheetCells(t, out), "C1")
	if fe == nil || fe.SelectAttrValue("t", "") != "array" || fe.SelectAttrValue("ref", "") != "C1:C3" || fe.Text() != "A1:A3*B1:B3" {
		t.Fatalf("C1 formula = %#v", fe)
	}
}
