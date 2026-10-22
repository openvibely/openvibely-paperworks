package xlsx

import (
	"context"
	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"github.com/xuri/excelize/v2"
	"os"
	"path/filepath"
	"testing"
)

func TestExistingWorkbookEdits(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "in.xlsx")
	out := filepath.Join(dir, "out.xlsx")
	f := excelize.NewFile()
	f.SetCellValue("Sheet1", "A1", 2)
	f.SetCellFormula("Sheet1", "B1", "A1*2")
	f.NewSheet("Untouched")
	f.SetCellValue("Untouched", "A1", "keep")
	if err := f.SaveAs(in); err != nil {
		t.Fatal(err)
	}
	f.Close()
	original, _ := os.ReadFile(in)
	spec := EditSpec{Edits: []Edit{{Operation: "formula", Sheet: "Sheet1", Cell: "B1", Formula: "A1*3"}, {Operation: "style", Sheet: "Sheet1", Range: "B1", Style: &excelize.Style{Font: &excelize.Font{Bold: true}}}, {Operation: "note", Sheet: "Sheet1", Note: &excelize.Comment{Cell: "B1", Author: "Reviewer", Text: "Check formula"}}, {Operation: "add-sheet", Name: "New"}, {Operation: "value", Sheet: "New", Cell: "A1", Value: "=literal"}}}
	if err := EditFile(context.Background(), in, out, dir, spec); err != nil {
		t.Fatal(err)
	}
	g, err := excelize.OpenFile(out)
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	if v, _ := g.GetCellFormula("Sheet1", "B1"); v != "A1*3" {
		t.Fatal(v)
	}
	if v, _ := g.CalcCellValue("Sheet1", "B1"); v != "6" {
		t.Fatal(v)
	}
	if v, _ := g.GetCellValue("Untouched", "A1"); v != "keep" {
		t.Fatal(v)
	}
	if v, _ := g.GetCellFormula("New", "A1"); v != "" {
		t.Fatal(v)
	}
	if v, _ := g.GetCellValue("New", "A1"); v != "=literal" {
		t.Fatal(v)
	}
	notes, err := g.GetComments("Sheet1")
	if err != nil || len(notes) != 1 || notes[0].Text != "Check formula" {
		t.Fatal(notes, err)
	}
	id, _ := g.GetCellStyle("Sheet1", "B1")
	style, _ := g.GetStyle(id)
	if style.Font == nil || !style.Font.Bold {
		t.Fatal(style)
	}
	current, _ := os.ReadFile(in)
	if string(current) != string(original) {
		t.Fatal("source changed")
	}
}
func TestEditBatchFailureDoesNotPublish(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "in.xlsx")
	out := filepath.Join(dir, "out.xlsx")
	f := excelize.NewFile()
	f.SaveAs(in)
	f.Close()
	err := EditFile(context.Background(), in, out, dir, EditSpec{Edits: []Edit{{Operation: "value", Sheet: "Sheet1", Cell: "A1", Value: 5}, {Operation: "value", Sheet: "Missing", Cell: "A1", Value: 6}}})
	if err == nil {
		t.Fatal("expected failure")
	}
	if _, err = os.Stat(out); !os.IsNotExist(err) {
		t.Fatal("partial output", err)
	}
}
func TestCSVImportLiteralFields(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "in.csv")
	out := filepath.Join(dir, "out.xlsx")
	os.WriteFile(in, []byte("code,expression\n001,=1+2\n"), 0600)
	if err := ImportCSV(context.Background(), in, out, "Data", ','); err != nil {
		t.Fatal(err)
	}
	f, err := excelize.OpenFile(out)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if v, _ := f.GetCellValue("Data", "A2"); v != "001" {
		t.Fatal(v)
	}
	if v, _ := f.GetCellFormula("Data", "B2"); v != "" {
		t.Fatal(v)
	}
}
func TestRangeCopyAndFillTranslateFormulas(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "in.xlsx")
	out := filepath.Join(dir, "out.xlsx")
	f := excelize.NewFile()
	f.SetCellFormula("Sheet1", "B2", `A2+$A$1+LEN("A2")`)
	f.SetCellInt("Sheet1", "A2", 4)
	f.SaveAs(in)
	f.Close()
	err := EditFile(context.Background(), in, out, dir, EditSpec{Edits: []Edit{{Operation: "copy", Sheet: "Sheet1", Range: "A2:B2", Target: "A3"}, {Operation: "fill-formula", Sheet: "Sheet1", Range: "C2:C4", Formula: "A2+$A$1"}}})
	if err != nil {
		t.Fatal(err)
	}
	g, err := excelize.OpenFile(out)
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	if v, _ := g.GetCellFormula("Sheet1", "B3"); v != `A3+$A$1+LEN("A2")` {
		t.Fatal(v)
	}
	if v, _ := g.GetCellFormula("Sheet1", "C4"); v != "A4+$A$1" {
		t.Fatal(v)
	}
	if v, _ := g.GetCellValue("Sheet1", "A3"); v != "4" {
		t.Fatal(v)
	}
}
func TestR1C1FormulaTranslation(t *testing.T) {
	v, err := r1c1Formula(`SUM(R[-1]C:R[-1]C[2])+R1C1+LEN("R1C1")`, "B3")
	if err != nil || v != `SUM(B2:D2)+$A$1+LEN("R1C1")` {
		t.Fatal(v, err)
	}
}
func TestFormulaCopyPreservesQuotedAndStructuredReferences(t *testing.T) {
	v, err := TranslateFormula(`SUM(A:A,$B:$C,1:3)+T1[A1]+日本A1+'A1 sheet'!A1+LEN("A1")`, 1, 1)
	if err != nil || v != `SUM(B:B,$B:$C,2:4)+T1[A1]+日本A1+'A1 sheet'!B2+LEN("A1")` {
		t.Fatal(v, err)
	}
}

func TestFillRejectsUnknownFormulaNotation(t *testing.T) {
	f := excelize.NewFile()
	defer f.Close()
	err := applyEdit(f, "", Edit{Operation: "fill-formula", Sheet: "Sheet1", Range: "A1:A2", Formula: "R[1]C", FormulaStyle: "typo"})
	if err == nil {
		t.Fatal("unknown formula notation accepted")
	}
	if v, _ := f.GetCellFormula("Sheet1", "A1"); v != "" {
		t.Fatal("modified cell before rejecting notation")
	}
}

func TestRenameSheetUpdatesFormulasWithoutChangingLiterals(t *testing.T) {
	dir := t.TempDir()
	in, out := filepath.Join(dir, "in.xlsx"), filepath.Join(dir, "out.xlsx")
	f := excelize.NewFile()
	f.SetSheetName("Sheet1", "Data")
	f.SetCellValue("Data", "A1", 7)
	f.NewSheet("Report")
	f.SetCellFormula("Report", "A1", "Data!A1*2")
	f.SetCellValue("Report", "B1", "Data!A1")
	f.AddChart("Report", "D1", &excelize.Chart{Type: excelize.Col, Series: []excelize.ChartSeries{{Name: "Data!$A$1", Categories: "Data!$A$1", Values: "Data!$A$1"}}})
	if err := f.SaveAs(in); err != nil {
		t.Fatal(err)
	}
	f.Close()
	if err := EditFile(context.Background(), in, out, dir, EditSpec{Edits: []Edit{{Operation: "rename-sheet", Sheet: "Data", Name: "New Data"}, {Operation: "formula", Sheet: "Report", Cell: "A2", Formula: "'New Data'!A1*3"}}}); err != nil {
		t.Fatal(err)
	}
	g, err := excelize.OpenFile(out)
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	if formula, _ := g.GetCellFormula("Report", "A1"); formula != "'New Data'!A1*2" {
		t.Fatal(formula)
	}
	if v, err := g.CalcCellValue("Report", "A1"); err != nil || v != "14" {
		t.Fatal(v, err)
	}
	if v, _ := g.GetCellValue("Report", "B1"); v != "Data!A1" {
		t.Fatal("literal changed", v)
	}

	pkg, err := opc.Open(out)
	if err != nil {
		t.Fatal(err)
	}
	chart, err := pkg.XML("xl/charts/chart1.xml")
	if err != nil {
		t.Fatal(err)
	}
	for _, formula := range office.All(chart.Root(), office.NSC, "f") {
		if formula.Text() != "'New Data'!$A$1" {
			t.Fatal("chart reference not updated", formula.Text())
		}
	}

	for _, s := range []string{`"Data!A1"`, `'[Other.xlsx]Data'!A1`, `[Other.xlsx]Data!A1`} {
		if got := renameFormulaSheet(s, "Data", "New Data"); got != s {
			t.Fatal("rewrote string/external link", got)
		}
	}
}

func TestRenameSheetThreeDimensionalReferences(t *testing.T) {
	for input, want := range map[string]string{"SUM(Data:End!A1)": "SUM('New Data:End'!A1)", "SUM(Start:Data!A1)": "SUM('Start:New Data'!A1)", "SUM('Data:End'!A1)": "SUM('New Data:End'!A1)"} {
		if got := renameFormulaSheet(input, "Data", "New Data"); got != want {
			t.Fatal(input, got, want)
		}
	}
}

func TestR1C1SheetQualifierSurvivesWorkbookEdit(t *testing.T) {
	dir := t.TempDir()
	in, out := filepath.Join(dir, "in.xlsx"), filepath.Join(dir, "out.xlsx")
	f := excelize.NewFile()
	if _, err := f.NewSheet("R1C1"); err != nil {
		t.Fatal(err)
	}
	if err := f.SetCellInt("R1C1", "A1", 17); err != nil {
		t.Fatal(err)
	}
	if err := f.SaveAs(in); err != nil {
		t.Fatal(err)
	}
	f.Close()
	if err := EditFile(context.Background(), in, out, dir, EditSpec{Edits: []Edit{{Operation: "formula", Sheet: "Sheet1", Cell: "B2", FormulaStyle: "r1c1", Formula: "R1C1!R1C1"}}}); err != nil {
		t.Fatal(err)
	}
	g, err := excelize.OpenFile(out)
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	if got, err := g.GetCellFormula("Sheet1", "B2"); err != nil || got != "R1C1!$A$1" {
		t.Fatal("sheet qualifier changed", got, err)
	}
	if got, err := g.CalcCellValue("Sheet1", "B2"); err != nil || got != "17" {
		t.Fatal("wrong formula result", got, err)
	}
}

func TestR1C1PreservesSheetQualifiers(t *testing.T) {
	for _, tc := range []struct{ formula, want string }{
		{`R1C1!R[-1]C`, `R1C1!B2`},
		{`SUM(R1C1:R2C2!R1C1)`, `SUM(R1C1:R2C2!$A$1)`},
		{`'R1C1'!R1C1+LEN("R1C1!R1C1")`, `'R1C1'!$A$1+LEN("R1C1!R1C1")`},
		{`[Book.xlsx]R1C1!R1C1`, `[Book.xlsx]R1C1!$A$1`},
		{`R1C1!R1C1:R1C1!R2C2`, `R1C1!$A$1:R1C1!$B$2`},
		{`SUM(R1C1:R2C2)`, `SUM($A$1:$B$2)`},
	} {
		t.Run(tc.formula, func(t *testing.T) {
			got, err := r1c1Formula(tc.formula, "B3")
			if err != nil || got != tc.want {
				t.Fatalf("got %q, %v; want %q", got, err, tc.want)
			}
		})
	}
}

func TestCSVImportStripsLeadingBOM(t *testing.T) {
	const bom = "\xEF\xBB\xBF"
	cases := []struct {
		name, data string
		delim      rune
		a1, b1     string
	}{
		{"unquoted", bom + "Name,Qty\r\nWidget,5\r\n", ',', "Name", "Qty"},
		{"quoted delimiter", bom + "\"Name, full\",Qty\r\nWidget,5\r\n", ',', "Name, full", "Qty"},
		{"quoted", bom + "\"Name\",\"Qty\"\r\n", ',', "Name", "Qty"},
		{"tab", bom + "Name\tQty\n", '\t', "Name", "Qty"},
		{"custom", bom + "Name;Qty\n", ';', "Name", "Qty"},
		{"custom quoted", bom + "\"Name; full\";Qty\n", ';', "Name; full", "Qty"},
		{"literal zero", bom + "001,=1+2\n", ',', "001", "=1+2"},
		{"literal formula", bom + "=1+2,Qty\n", ',', "=1+2", "Qty"},
		{"double bom", bom + bom + "Name,Qty\n", ',', "\ufeffName", "Qty"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			in := filepath.Join(dir, "in.csv")
			out := filepath.Join(dir, "out.xlsx")
			os.WriteFile(in, []byte(tc.data), 0600)
			if err := ImportCSV(context.Background(), in, out, "Data", tc.delim); err != nil {
				t.Fatal(err)
			}
			f, err := excelize.OpenFile(out)
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			if v, _ := f.GetCellValue("Data", "A1"); v != tc.a1 {
				t.Fatalf("A1 = %q", v)
			}
			if v, _ := f.GetCellValue("Data", "B1"); v != tc.b1 {
				t.Fatalf("B1 = %q", v)
			}
			for _, c := range []string{"A1", "B1"} {
				if v, _ := f.GetCellFormula("Data", c); v != "" {
					t.Fatalf("%s formula %q", c, v)
				}
			}
		})
	}
}

func TestCSVImportBOMOnlyMatchesEmpty(t *testing.T) {
	dump := func(data string) [][]string {
		dir := t.TempDir()
		in := filepath.Join(dir, "in.csv")
		out := filepath.Join(dir, "out.xlsx")
		os.WriteFile(in, []byte(data), 0600)
		if err := ImportCSV(context.Background(), in, out, "Data", ','); err != nil {
			t.Fatal(err)
		}
		f, err := excelize.OpenFile(out)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		rows, err := f.GetRows("Data")
		if err != nil {
			t.Fatal(err)
		}
		return rows
	}
	want := len(dump(""))
	for _, data := range []string{"\xEF\xBB\xBF", "\xEF\xBB\xBF\n", "\xEF\xBB\xBF\r\n"} {
		if got := dump(data); len(got) != want {
			t.Fatalf("%q: rows %q", data, got)
		}
	}
}

func TestCSVImportPreservesInnerBOM(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "in.csv")
	out := filepath.Join(dir, "out.xlsx")
	os.WriteFile(in, []byte("Name,Qty\nWidget,\xEF\xBB\xBF5\n"), 0600)
	if err := ImportCSV(context.Background(), in, out, "Data", ','); err != nil {
		t.Fatal(err)
	}
	f, err := excelize.OpenFile(out)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if v, _ := f.GetCellValue("Data", "B2"); v != "\ufeff5" {
		t.Fatalf("B2 = %q", v)
	}
	if v, _ := f.GetCellValue("Data", "A1"); v != "Name" {
		t.Fatalf("A1 = %q", v)
	}
}
