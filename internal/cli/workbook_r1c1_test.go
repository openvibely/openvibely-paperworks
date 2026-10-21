package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/xuri/excelize/v2"
)

func TestR1C1WholeAxisCalculationsThroughCLI(t *testing.T) {
	dir := t.TempDir()
	in, out := filepath.Join(dir, "input.xlsx"), filepath.Join(dir, "edited.xlsx")
	f := excelize.NewFile()
	defer f.Close()
	if err := f.SetSheetName("Sheet1", "Data"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.NewSheet("Report"); err != nil {
		t.Fatal(err)
	}
	for cell, value := range map[string]int64{"A1": 1, "A2": 2, "B1": 10, "B2": 20, "C1": 99} {
		if err := f.SetCellInt("Data", cell, value); err != nil {
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
	var edits []any
	for cell, formula := range map[string]string{
		"A1": "SUM(Data!C1)", "A2": "SUM(Data!R1)",
		"A3": "SUM(Data!C1:C2)", "A4": "SUM(Data!R1:R2)",
		"D5": "SUM(Data!C[-3])", "E5": "SUM(Data!R[-4])",
	} {
		edits = append(edits, map[string]any{"operation": "formula", "sheet": "Report", "cell": cell, "formula_style": "r1c1", "formula": formula})
	}
	edits = append(edits,
		map[string]any{"operation": "fill-formula", "sheet": "Report", "range": "D8:E8", "formula_style": "r1c1", "formula": "SUM(Data!C[-3])"},
		map[string]any{"operation": "fill-formula", "sheet": "Report", "range": "G8:G9", "formula_style": "r1c1", "formula": "SUM(Data!R[-7])"},
	)
	spec := writeSpec(t, dir, "edits.json", map[string]any{"edits": edits})
	runCLI(t, "workbook-edit", in, "--spec", spec, "--output", out)
	result := runCLI(t, "recalculate", out, "--output", filepath.Join(dir, "calculated"))
	calculated, err := excelize.OpenFile(result["output"].(string))
	if err != nil {
		t.Fatal(err)
	}
	defer calculated.Close()
	for cell, want := range map[string]string{
		"A1": "3", "A2": "110", "A3": "33", "A4": "132", "D5": "3", "E5": "110",
		"D8": "3", "E8": "30", "G8": "110", "G9": "22",
	} {
		got, err := calculated.GetCellValue("Report", cell)
		if err != nil || got != want {
			t.Errorf("%s: got %q (%v), want %q", cell, got, err, want)
		}
	}
	after, err := os.ReadFile(in)
	if err != nil || !bytes.Equal(original, after) {
		t.Fatal("input changed", err)
	}
}
