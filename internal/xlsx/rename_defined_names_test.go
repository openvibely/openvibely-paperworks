package xlsx

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/xuri/excelize/v2"
)

func TestRenameSheetDefinedNamesWithCellAddressSheet(t *testing.T) {
	for _, source := range []string{"Q1", "FY2024"} {
		t.Run(source, func(t *testing.T) {
			f := excelize.NewFile()
			defer f.Close()
			if err := f.SetSheetName("Sheet1", "Summary"); err != nil {
				t.Fatal(err)
			}
			for _, sheet := range []string{source, "Q4"} {
				if _, err := f.NewSheet(sheet); err != nil {
					t.Fatal(err)
				}
			}
			target := "Renamed " + source
			if source == "Q1" {
				target = "Quarter 1"
			}
			quoted := "'" + target + "'"
			cases := []struct{ name, scope, formula, want string }{
				{"RowOne", "", source + "!A1:" + source, quoted + "!A1:" + source},
				{"Band", "", "Summary!B2:" + source, "Summary!B2:" + source},
				{"Cell", "", "Summary!" + source, "Summary!" + source},
				{"Absolute", "", source + "!$A$1:$Q$1", quoted + "!$A$1:$Q$1"},
				{"OtherAbsolute", "", "Summary!$Q$1", "Summary!$Q$1"},
				{"Span", "", source + ":Q4!A1", "'" + target + ":Q4'!A1"},
				{"SpanEnd", "", "Q4:" + source + "!A1", "'Q4:" + target + "'!A1"},
				{"Quoted", "", "'" + source + "'!A1:" + source, quoted + "!A1:" + source},
				{"Local", source, source + "!A1:" + source, quoted + "!A1:" + source},
				{"Local", "Summary", "Summary!" + source, "Summary!" + source},
				{"_xlnm.Print_Area", source, source + "!A1:" + source, quoted + "!A1:" + source},
				{"_xlnm._FilterDatabase", "Summary", "Summary!B2:" + source, "Summary!B2:" + source},
			}
			for _, tc := range cases {
				if err := f.SetDefinedName(&excelize.DefinedName{Name: tc.name, Scope: tc.scope, RefersTo: tc.formula, Comment: "preserve comment"}); err != nil {
					t.Fatal(err)
				}
			}
			g, err := renameSheet(f, source, target)
			if err != nil {
				t.Fatal(err)
			}
			defer g.Close()
			names := g.GetDefinedName()
			if len(names) != len(cases) {
				t.Fatalf("got %d names, want %d", len(names), len(cases))
			}
			for i, tc := range cases {
				scope := tc.scope
				if scope == "" {
					scope = "Workbook"
				}
				if scope == source {
					scope = target
				}
				got := names[i]
				if got.Name != tc.name || got.Scope != scope || got.RefersTo != tc.want || got.Comment != "preserve comment" {
					t.Errorf("%s (%s): got %+v, want scope %q formula %q with original comment", tc.name, tc.scope, got, scope, tc.want)
				}
			}
		})
	}
}

func TestEditRenameSheetDefinedNameCalculation(t *testing.T) {
	dir := t.TempDir()
	in, out := filepath.Join(dir, "in.xlsx"), filepath.Join(dir, "out.xlsx")
	f := excelize.NewFile()
	defer f.Close()
	if err := f.SetSheetName("Sheet1", "Summary"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.NewSheet("Q1"); err != nil {
		t.Fatal(err)
	}
	for cell, value := range map[string]int64{"A1": 7, "Q1": 11} {
		if err := f.SetCellInt("Q1", cell, value); err != nil {
			t.Fatal(err)
		}
	}
	if err := f.SetDefinedName(&excelize.DefinedName{Name: "Row1", RefersTo: "Q1!A1:Q1"}); err != nil {
		t.Fatal(err)
	}
	if err := f.SetCellFormula("Summary", "A1", "=SUM(Row1)"); err != nil {
		t.Fatal(err)
	}
	if err := f.SaveAs(in); err != nil {
		t.Fatal(err)
	}
	if err := EditFile(context.Background(), in, out, dir, EditSpec{Edits: []Edit{{Operation: "rename-sheet", Sheet: "Q1", Name: "Quarter 1"}}}); err != nil {
		t.Fatal(err)
	}
	g, err := excelize.OpenFile(out)
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	names := g.GetDefinedName()
	if len(names) != 1 || names[0].Name != "Row1" || names[0].RefersTo != "'Quarter 1'!A1:Q1" {
		t.Fatalf("unexpected defined names: %+v", names)
	}
	if got, err := g.GetCellFormula("Summary", "A1"); err != nil || got != "=SUM(Row1)" {
		t.Fatalf("formula = %q, %v", got, err)
	}
	if got, err := g.CalcCellValue("Summary", "A1"); err != nil || got != "18" {
		t.Fatalf("calculated value = %q, %v; want 18", got, err)
	}
}
