package xlsx

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"github.com/xuri/excelize/v2"
)

func example(t *testing.T) Spec {
	t.Helper()
	data, e := os.ReadFile("../../testdata/workflows/revenue.json")
	if e != nil {
		t.Fatal(e)
	}
	var spec Spec
	if e = json.Unmarshal(data, &spec); e != nil {
		t.Fatal(e)
	}
	return spec
}
func TestRevenueSavedNativeObjects(t *testing.T) {
	spec := example(t)
	p := filepath.Join(t.TempDir(), "revenue.xlsx")
	if e := Create(context.Background(), spec, ".", p); e != nil {
		t.Fatal(e)
	}
	f, e := excelize.OpenFile(p)
	if e != nil {
		t.Fatal(e)
	}
	defer f.Close()
	for cell, want := range map[string]string{"D2": "B2*C2", "D3": "B3*C3", "D4": "SUM(D2:D3)"} {
		got, e := f.GetCellFormula("Revenue", cell)
		if e != nil || got != want {
			t.Fatalf("%s: %q %v", cell, got, e)
		}
	}
	// Independent arithmetic verifies the example's intended total, without
	// claiming a saved cache or successful external-engine recalculation.
	q1, _ := f.GetCellValue("Revenue", "B2")
	p1, _ := f.GetCellValue("Revenue", "C2", excelize.Options{RawCellValue: true})
	if q1 != "3" || p1 != "40" {
		t.Fatal(q1, p1)
	}
	got, e := f.CalcCellValue("Revenue", "D4", excelize.Options{RawCellValue: true})
	if e != nil || got != "200" {
		t.Fatalf("example calculation: %q %v", got, e)
	}
	pkg, e := opc.Open(p)
	if e != nil {
		t.Fatal(e)
	}
	chart, e := pkg.XML("xl/charts/chart1.xml")
	if e != nil {
		t.Fatal(e)
	}
	refs := chart.FindElements("//f")
	foundSeries := false
	for _, ref := range refs {
		if ref.Text() == "'Revenue'!$D$2:$D$3" {
			foundSeries = true
		}
	}
	if !foundSeries {
		t.Fatal("missing editable chart series binding")
	}
	validations, e := f.GetDataValidations("Revenue")
	if e != nil || len(validations) != 1 || validations[0].Sqref != "B2:B3" || validations[0].Formula1 != "0" {
		t.Fatalf("validations: %+v %v", validations, e)
	}
	cf, e := f.GetConditionalFormats("Revenue")
	if e != nil || len(cf["D2:D3"]) != 1 {
		t.Fatalf("conditional: %+v %v", cf, e)
	}
	id, e := f.GetCellStyle("Revenue", "A1")
	if e != nil {
		t.Fatal(e)
	}
	style, e := f.GetStyle(id)
	if e != nil || style.Font == nil || !style.Font.Bold || style.Font.Color != "FFFFFF" {
		t.Fatalf("header: %+v %v", style, e)
	}
	panes, e := f.GetPanes("Revenue")
	if e != nil || !panes.Freeze || panes.TopLeftCell != "B2" {
		t.Fatalf("panes: %+v %v", panes, e)
	}
	names := f.GetDefinedName()
	found := false
	for _, n := range names {
		if n.Name == "_xlnm.Print_Area" && strings.Contains(n.RefersTo, "A1:Q23") {
			found = true
		}
	}
	if !found {
		t.Fatal(names)
	}
	before, e := os.ReadFile(p)
	if e != nil {
		t.Fatal(e)
	}
	if e = Create(context.Background(), spec, ".", p); e == nil {
		t.Fatal("overwrote existing workbook")
	}
	after, _ := os.ReadFile(p)
	if string(before) != string(after) {
		t.Fatal("existing workbook changed")
	}
}
func TestLiteralFormulaMarkersAndDates(t *testing.T) {
	p := filepath.Join(t.TempDir(), "typed.xlsx")
	spec := Spec{Sheets: []Sheet{{Name: "Data", Rows: [][]any{{"=1+1", "00123", true}}, Cells: map[string]Cell{"D1": {Date: "2025-01-02T00:00:00Z"}}}}}
	if e := Create(context.Background(), spec, ".", p); e != nil {
		t.Fatal(e)
	}
	f, e := excelize.OpenFile(p)
	if e != nil {
		t.Fatal(e)
	}
	defer f.Close()
	formula, _ := f.GetCellFormula("Data", "A1")
	value, _ := f.GetCellValue("Data", "A1")
	id, _ := f.GetCellValue("Data", "B1")
	if formula != "" || value != "=1+1" || id != "00123" {
		t.Fatal(formula, value, id)
	}
	typ, e := f.GetCellType("Data", "D1")
	if e != nil || typ == excelize.CellTypeSharedString {
		t.Fatalf("date was text: %v %v", typ, e)
	}
}
func TestInvalidAndCancelledNoOutput(t *testing.T) {
	for _, spec := range []Spec{{}, {Sheets: []Sheet{{Name: "A"}, {Name: "a"}}}, {Sheets: []Sheet{{Name: "A", Hidden: true}}}, {Sheets: []Sheet{{Name: "A", Cells: map[string]Cell{"A1": {Formula: "1+1", Value: 2}}}}}, {Sheets: []Sheet{{Name: "A", Charts: []Chart{{Type: "unknown", Cell: "A1"}}}}}} {
		p := filepath.Join(t.TempDir(), "bad.xlsx")
		if e := Create(context.Background(), spec, ".", p); e == nil {
			t.Fatal("accepted invalid input")
		}
		if _, e := os.Stat(p); !os.IsNotExist(e) {
			t.Fatal("left output", e)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	p := filepath.Join(t.TempDir(), "cancelled.xlsx")
	if e := Create(ctx, example(t), ".", p); e != context.Canceled {
		t.Fatal(e)
	}
}

// Explicit false must survive decoding; omission retains Excelize's default.
func TestChartSeriesColorPolicy(t *testing.T) {
	for _, tc := range []struct{ name, field, want string }{
		{"omitted", "", "1"},
		{"series", `,"vary_colors":false`, "0"},
		{"categories", `,"vary_colors":true`, "1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var spec Spec
			data := `{"sheets":[{"name":"S","rows":[["Item","Value"],["A",1],["B",2]],"charts":[{"type":"column","cell":"D1"` + tc.field + `,"series":[{"Name":"S!$B$1","Categories":"S!$A$2:$A$3","Values":"S!$B$2:$B$3"}]}]}]}`
			if err := json.Unmarshal([]byte(data), &spec); err != nil {
				t.Fatal(err)
			}
			output := filepath.Join(t.TempDir(), "chart.xlsx")
			if err := Create(context.Background(), spec, "", output); err != nil {
				t.Fatal(err)
			}
			pkg, err := opc.Open(output)
			if err != nil {
				t.Fatal(err)
			}
			doc, err := pkg.XML("xl/charts/chart1.xml")
			if err != nil {
				t.Fatal(err)
			}
			nodes := doc.FindElements("//varyColors")
			if len(nodes) != 1 || nodes[0].SelectAttrValue("val", "") != tc.want {
				t.Fatalf("varyColors: %v; want %s", nodes, tc.want)
			}
		})
	}
}
