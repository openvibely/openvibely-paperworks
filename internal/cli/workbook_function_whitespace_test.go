package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/xuri/excelize/v2"
)

func TestFunctionWhitespaceCopiesAndFillsThroughCLI(t *testing.T) {
	dir := t.TempDir()
	in, out := filepath.Join(dir, "input.xlsx"), filepath.Join(dir, "edited.xlsx")
	f := excelize.NewFile()
	defer f.Close()
	for cell, value := range map[string]int64{"A1": 100, "A2": 1000} {
		if err := f.SetCellInt("Sheet1", cell, value); err != nil {
			t.Fatal(err)
		}
	}
	for _, cell := range []string{"B1", "C1"} {
		if err := f.SetCellFormula("Sheet1", cell, "LOG10 (A1)"); err != nil {
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
		map[string]any{"operation": "copy-formulas", "sheet": "Sheet1", "range": "B1", "target": "B2"},
		map[string]any{"operation": "copy", "sheet": "Sheet1", "range": "C1", "target": "C2"},
		map[string]any{"operation": "fill-formula", "sheet": "Sheet1", "range": "D1:D2", "formula": "LOG10 (A1)"},
	}})
	runCLI(t, "workbook-edit", in, "--spec", spec, "--output", out)
	edited, err := excelize.OpenFile(out)
	if err != nil {
		t.Fatal(err)
	}
	defer edited.Close()
	for _, cell := range []string{"B2", "C2", "D2"} {
		formula, err := edited.GetCellFormula("Sheet1", cell)
		if err != nil || formula != "LOG10 (A2)" {
			t.Errorf("%s: got formula %q (%v), want LOG10 (A2)", cell, formula, err)
		}
	}
	result := runCLI(t, "recalculate", out, "--output", filepath.Join(dir, "calculated"))
	got, err := excelize.OpenFile(result["output"].(string))
	if err != nil {
		t.Fatal(err)
	}
	defer got.Close()
	for cell, want := range map[string]string{"B1": "2", "B2": "3", "C1": "2", "C2": "3", "D1": "2", "D2": "3"} {
		value, err := got.GetCellValue("Sheet1", cell)
		if err != nil || value != want {
			t.Errorf("%s: got %q (%v), want %q", cell, value, err, want)
		}
	}
	after, err := os.ReadFile(in)
	if err != nil || !bytes.Equal(original, after) {
		t.Fatal("input changed", err)
	}
}
