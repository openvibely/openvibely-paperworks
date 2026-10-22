package xlsx

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"github.com/xuri/excelize/v2"
)

func TestCopyValuesPreservesCachedResultTypes(t *testing.T) {
	dir := t.TempDir()
	in, out := filepath.Join(dir, "cached.xlsx"), filepath.Join(dir, "edited.xlsx")
	f := excelize.NewFile()
	defer f.Close()
	results := []struct{ formula, kind, value string }{
		{`"00123"`, "str", "00123"}, {`""`, "str", ""},
		{"NA()", "e", "#N/A"}, {"TRUE()", "b", "1"}, {"42", "n", "42"},
		{"DATE(2026,1,2)", "d", "2026-01-02T03:04:05Z"},
	}
	for i, result := range results {
		cell, _ := excelize.CoordinatesToCellName(1, i+1)
		if err := f.SetCellFormula("Sheet1", cell, result.formula); err != nil {
			t.Fatal(err)
		}
	}
	raw, err := f.WriteToBuffer()
	if err != nil {
		t.Fatal(err)
	}
	pkg, err := opc.Read(raw.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	wb, err := office.OpenWorkbook(pkg)
	if err != nil {
		t.Fatal(err)
	}
	sh := wb.Sheets["Sheet1"]
	for i, result := range results {
		address, _ := excelize.CoordinatesToCellName(1, i+1)
		cell := sh.Cells[address]
		cell.CreateAttr("t", result.kind)
		office.RemoveAll(cell, office.NSS, "v")
		office.NewChild(cell, office.NSS, "v", nil).SetText(result.value)
	}
	if err := pkg.SetXML(sh.Part, sh.Doc); err != nil {
		t.Fatal(err)
	}
	if err := pkg.SaveNew(in); err != nil {
		t.Fatal(err)
	}
	if err := EditFile(context.Background(), in, out, dir, EditSpec{Edits: []Edit{
		{Operation: "copy-values", Sheet: "Sheet1", Range: "A1:A6", Target: "B1"},
		{Operation: "copy", Sheet: "Sheet1", Range: "B1:B6", Target: "C1"},
		{Operation: "rename-sheet", Sheet: "Sheet1", Name: "Result"},
		{Operation: "value", Sheet: "Result", Cell: "B3", Value: "recovered"},
		{Operation: "add-sheet", Name: "Other"},
		{Operation: "copy-values", Sheet: "Result", Range: "C3", Name: "other", Target: "D1"},
	}}); err != nil {
		t.Fatal(err)
	}
	got, err := excelize.OpenFile(out)
	if err != nil {
		t.Fatal(err)
	}
	defer got.Close()
	for _, col := range []int{2, 3} {
		for i, result := range results {
			address, _ := excelize.CoordinatesToCellName(col, i+1)
			wantValue := result.value
			wantType := []excelize.CellType{excelize.CellTypeSharedString, excelize.CellTypeSharedString, excelize.CellTypeError, excelize.CellTypeBool, excelize.CellTypeNumber, excelize.CellTypeDate}[i]
			if address == "B3" {
				wantValue, wantType = "recovered", excelize.CellTypeSharedString
			}
			value, err := got.GetCellValue("Result", address, excelize.Options{RawCellValue: true})
			if err != nil || value != wantValue {
				t.Fatalf("%s value=%q err=%v want=%q", address, value, err, wantValue)
			}
			kind, err := got.GetCellType("Result", address)
			if wantType == excelize.CellTypeNumber && kind == excelize.CellTypeUnset {
				kind = excelize.CellTypeNumber
			}
			if err != nil || kind != wantType {
				t.Fatalf("%s type=%v err=%v want=%v", address, kind, err, wantType)
			}
			if formula, err := got.GetCellFormula("Result", address); err != nil || formula != "" {
				t.Fatalf("%s formula=%q err=%v", address, formula, err)
			}
		}
	}
	if kind, err := got.GetCellType("Other", "D1"); err != nil || kind != excelize.CellTypeError {
		t.Fatalf("cross-sheet error type=%v err=%v", kind, err)
	}
	for i, result := range results {
		address, _ := excelize.CoordinatesToCellName(1, i+1)
		if formula, err := got.GetCellFormula("Result", address); err != nil || formula != result.formula {
			t.Fatalf("source formula changed: %s=%q err=%v", address, formula, err)
		}
	}
}
