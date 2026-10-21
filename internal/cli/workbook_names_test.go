package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/xuri/excelize/v2"
)

func TestNamedRangeFormulaCopiesAndFillsThroughCLI(t *testing.T) {
	dir := t.TempDir()
	in, out := filepath.Join(dir, "input.xlsx"), filepath.Join(dir, "edited.xlsx")
	f := excelize.NewFile()
	defer f.Close()
	for cell, value := range map[string]int64{"A1": 7, "A2": 13} {
		if err := f.SetCellInt("Sheet1", cell, value); err != nil {
			t.Fatal(err)
		}
	}
	for name, ref := range map[string]string{"A1048577": "Sheet1!$A$1", "XFE1": "Sheet1!$A$2"} {
		if err := f.SetDefinedName(&excelize.DefinedName{Name: name, RefersTo: ref}); err != nil {
			t.Fatal(err)
		}
	}
	for cell, formula := range map[string]string{"B2": "A1048577", "C1": "XFE1"} {
		if err := f.SetCellFormula("Sheet1", cell, formula); err != nil {
			t.Fatal(err)
		}
	}
	if err := f.SaveAs(in); err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile(in)
	if err != nil {
		t.Fatal(err)
	}
	spec := writeSpec(t, dir, "edits.json", map[string]any{"edits": []any{
		map[string]any{"operation": "copy-formulas", "sheet": "Sheet1", "range": "B2", "target": "B1"},
		map[string]any{"operation": "copy", "sheet": "Sheet1", "range": "C1", "target": "C2"},
		map[string]any{"operation": "fill-formula", "sheet": "Sheet1", "range": "D1:D2", "formula": "A1048577+XFE1+A1"},
	}})
	runCLI(t, "workbook-edit", in, "--spec", spec, "--output", out)
	result := runCLI(t, "recalculate", out, "--output", filepath.Join(dir, "calculated"))
	got, err := excelize.OpenFile(result["output"].(string))
	if err != nil {
		t.Fatal(err)
	}
	defer got.Close()
	for cell, want := range map[string]string{"B1": "7", "B2": "7", "C1": "13", "C2": "13", "D1": "27", "D2": "33"} {
		value, err := got.GetCellValue("Sheet1", cell)
		if err != nil || value != want {
			t.Errorf("%s: got %q (%v), want %q", cell, value, err, want)
		}
	}
	for cell, want := range map[string]string{"B1": "A1048577", "C2": "XFE1", "D2": "A1048577+XFE1+A2"} {
		formula, err := got.GetCellFormula("Sheet1", cell)
		if err != nil || formula != want {
			t.Errorf("%s: got formula %q (%v), want %q", cell, formula, err, want)
		}
	}
	after, err := os.ReadFile(in)
	if err != nil || !bytes.Equal(original, after) {
		t.Fatal("input changed", err)
	}
}
