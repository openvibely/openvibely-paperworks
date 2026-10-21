package xlsx

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/xuri/excelize/v2"
)

func TestCopyAndFillPreserveSheetQualifiers(t *testing.T) {
	for _, tc := range []struct{ formula, want string }{
		{"SUM(A:C!A1)", "SUM(A:C!B2)"},
		{"SUM(S1:S2!A1)", "SUM(S1:S2!B2)"},
		{"SUM('A:C'!$A1)", "SUM('A:C'!$A2)"},
		{"SUM(A:C)+SUM(1:3)+A1", "SUM(B:D)+SUM(2:4)+B2"},
		{"SUM([1]A:C!A1)", "SUM([1]A:C!B2)"},
		{"SUM(Sheet1!A1:Sheet1!A2)", "SUM(Sheet1!B2:Sheet1!B3)"},
		{"SUM('Sheet1'!A1:'Sheet1'!A2)", "SUM('Sheet1'!B2:'Sheet1'!B3)"},
		{"SUM(Sheet1!$A1:Sheet1!B$2)", "SUM(Sheet1!$A2:Sheet1!C$2)"},
		{"SUM(S1:S2!A1:B2)", "SUM(S1:S2!B2:C3)"},
	} {
		t.Run(tc.formula, func(t *testing.T) {
			dir := t.TempDir()
			in, out := filepath.Join(dir, "in.xlsx"), filepath.Join(dir, "out.xlsx")
			f := excelize.NewFile()
			defer f.Close()
			for _, name := range []string{"A", "B", "C", "S1", "S2"} {
				if _, err := f.NewSheet(name); err != nil {
					t.Fatal(err)
				}
			}
			if err := f.SetCellFormula("Sheet1", "D1", tc.formula); err != nil {
				t.Fatal(err)
			}
			if err := f.SaveAs(in); err != nil {
				t.Fatal(err)
			}
			if err := EditFile(context.Background(), in, out, dir, EditSpec{Edits: []Edit{
				{Operation: "copy-formulas", Sheet: "Sheet1", Range: "D1", Target: "E2"},
				{Operation: "fill-formula", Sheet: "Sheet1", Range: "G1:H2", Formula: tc.formula},
			}}); err != nil {
				t.Fatal(err)
			}
			got, err := excelize.OpenFile(out)
			if err != nil {
				t.Fatal(err)
			}
			defer got.Close()
			for _, cell := range []string{"E2", "H2"} {
				formula, err := got.GetCellFormula("Sheet1", cell)
				if err != nil || formula != tc.want {
					t.Fatalf("%s: %q, %v; want %q", cell, formula, err, tc.want)
				}
			}
			if original, _ := got.GetCellFormula("Sheet1", "D1"); original != tc.formula {
				t.Fatal("source formula changed", original)
			}
		})
	}
}

func TestRenameSheetPreservesStructuredReferences(t *testing.T) {
	dir := t.TempDir()
	in, out := filepath.Join(dir, "in.xlsx"), filepath.Join(dir, "out.xlsx")
	f := excelize.NewFile()
	defer f.Close()
	if err := f.SetSheetName("Sheet1", "Data"); err != nil {
		t.Fatal(err)
	}
	for cell, value := range map[string]any{"A1": "Data!", "A2": 4, "B1": "Other", "B2": 7} {
		if err := f.SetCellValue("Data", cell, value); err != nil {
			t.Fatal(err)
		}
	}
	if err := f.AddTable("Data", &excelize.Table{Range: "A1:B2", Name: "Table1"}); err != nil {
		t.Fatal(err)
	}
	formulas := map[string]string{
		"D1": "SUM(Table1[Data!])+Data!A2",
		"D2": "SUM(Table1[[#Data],[Data!]])",
		"D3": `SUM([1]Data!A2)+LEN("Data!A2")`,
		"D4": "SUM('Data'!A2)",
	}
	wants := map[string]string{
		"D1": "SUM(Table1[Data!])+'New Data'!A2",
		"D2": formulas["D2"], "D3": formulas["D3"], "D4": "SUM('New Data'!A2)",
	}
	for cell, formula := range formulas {
		if err := f.SetCellFormula("Data", cell, formula); err != nil {
			t.Fatal(err)
		}
	}
	if err := f.SaveAs(in); err != nil {
		t.Fatal(err)
	}
	if err := EditFile(context.Background(), in, out, dir, EditSpec{Edits: []Edit{{Operation: "rename-sheet", Sheet: "Data", Name: "New Data"}}}); err != nil {
		t.Fatal(err)
	}
	got, err := excelize.OpenFile(out)
	if err != nil {
		t.Fatal(err)
	}
	defer got.Close()
	for cell, want := range wants {
		formula, err := got.GetCellFormula("New Data", cell)
		if err != nil || formula != want {
			t.Fatalf("%s: %q, %v; want %q", cell, formula, err, want)
		}
	}
	if header, _ := got.GetCellValue("New Data", "A1"); header != "Data!" {
		t.Fatal("table header changed", header)
	}
}

func TestRenameIgnoresEscapedStructuredHeaders(t *testing.T) {
	for _, formula := range []string{"Table1[Data!]", "Table1[[#Data],[x']Data!]]", "Table1[x'[Data!]", "Table1[O''Data!]"} {
		if got := renameFormulaSheet(formula+"+Data!A1", "Data", "Renamed"); got != formula+"+'Renamed'!A1" {
			t.Fatalf("%q: %q", formula, got)
		}
	}
}

func TestCopyRepeatedSheetRangeCalculatesCorrectTotal(t *testing.T) {
	dir := t.TempDir()
	in, out := filepath.Join(dir, "in.xlsx"), filepath.Join(dir, "out.xlsx")
	f := excelize.NewFile()
	defer f.Close()
	for cell, value := range map[string]int{"A1": 1, "A2": 2, "B1": 10, "B2": 20} {
		if err := f.SetCellInt("Sheet1", cell, int64(value)); err != nil {
			t.Fatal(err)
		}
	}
	if err := f.SetCellFormula("Sheet1", "D1", "SUM(Sheet1!A1:Sheet1!A2)"); err != nil {
		t.Fatal(err)
	}
	if err := f.SaveAs(in); err != nil {
		t.Fatal(err)
	}
	if err := EditFile(context.Background(), in, out, dir, EditSpec{Edits: []Edit{
		{Operation: "copy-formulas", Sheet: "Sheet1", Range: "D1", Target: "E1"},
		{Operation: "fill-formula", Sheet: "Sheet1", Range: "G1:H1", Formula: "SUM(Sheet1!A1:Sheet1!A2)"},
	}}); err != nil {
		t.Fatal(err)
	}
	got, err := excelize.OpenFile(out)
	if err != nil {
		t.Fatal(err)
	}
	defer got.Close()
	for cell, want := range map[string]string{"D1": "3", "E1": "30", "G1": "3", "H1": "30"} {
		value, err := got.CalcCellValue("Sheet1", cell)
		if err != nil || value != want {
			t.Fatalf("%s: got %q (%v), want %q", cell, value, err, want)
		}
	}
}
