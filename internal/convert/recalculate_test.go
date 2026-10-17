package convert

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/openvibely/openvibely-paperworks/internal/lo"
	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"github.com/openvibely/openvibely-paperworks/internal/xlsx"
	"github.com/xuri/excelize/v2"
)

func TestLibreOfficeExplicitRecalculationAndPreservation(t *testing.T) {
	if _, e := lo.Find(); e != nil {
		t.Skip("LibreOffice unavailable; explicit recalculation acceptance not run")
	}
	ctx := context.Background()
	dir := t.TempDir()
	initial := filepath.Join(dir, "initial.xlsx")
	spec := xlsx.Spec{Sheets: []xlsx.Sheet{{Name: "Model", Rows: [][]any{{"Period", "Input", "Result"}, {"FY2028", 12, nil}, {"FY2029", 15, nil}}, Cells: map[string]xlsx.Cell{"C2": {Formula: "B2*2"}, "C3": {Formula: "B3*2"}, "C4": {Formula: "SUM(C2:C3)"}}, Charts: []xlsx.Chart{{Type: "column", Cell: "E2", Title: "Projection", Series: []excelize.ChartSeries{{Name: "Model!$C$1", Categories: "Model!$A$2:$A$3", Values: "Model!$C$2:$C$3"}}}}}}}
	if e := xlsx.Create(ctx, spec, ".", initial); e != nil {
		t.Fatal(e)
	}
	p, e := opc.Open(initial)
	if e != nil {
		t.Fatal(e)
	}
	// Save deliberately wrong caches and disable automatic calculation. The
	// calculateAll bridge must still replace them, rather than merely re-save.
	d, e := p.XML("xl/worksheets/sheet1.xml")
	if e != nil {
		t.Fatal(e)
	}
	for _, c := range office.All(d.Root(), office.NSS, "c") {
		if office.Child(c, office.NSS, "f") != nil {
			c.CreateAttr("t", "n")
			v := office.Child(c, office.NSS, "v")
			if v == nil {
				v = c.CreateElement("v")
			}
			v.SetText("999")
		}
	}
	if e = p.SetXML("xl/worksheets/sheet1.xml", d); e != nil {
		t.Fatal(e)
	}
	wb, e := p.XML("xl/workbook.xml")
	if e != nil {
		t.Fatal(e)
	}
	calc := office.Child(wb.Root(), office.NSS, "calcPr")
	if calc == nil {
		calc = wb.Root().CreateElement("calcPr")
	}
	calc.CreateAttr("calcMode", "manual")
	calc.CreateAttr("fullCalcOnLoad", "0")
	calc.CreateAttr("forceFullCalc", "0")
	p.SetXML("xl/workbook.xml", wb)
	p.Set("customXml/preservation.xml", []byte(`<lineage xmlns="urn:paperworks:test">retain me exactly</lineage>`))
	p.EnsureContentType("customXml/preservation.xml", "application/xml")
	if e = office.RefreshWorkbookChartCaches(p); e != nil {
		t.Fatal(e)
	}
	source := filepath.Join(dir, "stale.xlsx")
	if e = p.SaveNew(source); e != nil {
		t.Fatal(e)
	}
	before, _ := os.ReadFile(source)
	result, e := Convert(ctx, source, filepath.Join(dir, "calculated"), Options{Recalculate: true})
	if e != nil {
		t.Fatal(e)
	}
	out := result["output"].(string)
	f, e := excelize.OpenFile(out)
	if e != nil {
		t.Fatal(e)
	}
	defer f.Close()
	for cell, want := range map[string]string{"C2": "24", "C3": "30", "C4": "54"} {
		got, e := f.GetCellValue("Model", cell)
		if e != nil || got != want {
			t.Fatal(cell, got, want, e)
		}
	}
	q, e := opc.Open(out)
	if e != nil {
		t.Fatal(e)
	}
	if e = office.RefreshWorkbookChartCaches(q); e != nil {
		t.Fatal(e)
	}
	// The saved chart must already match a fresh cache refresh.
	saved, e := opc.Open(out)
	if e != nil {
		t.Fatal(e)
	}
	if !bytes.Equal(saved.Get("xl/charts/chart1.xml"), q.Get("xl/charts/chart1.xml")) {
		t.Fatal("saved chart cache stale")
	}
	for _, part := range p.Parts {
		if part.Name == "xl/worksheets/sheet1.xml" || part.Name == "xl/charts/chart1.xml" {
			continue
		}
		if !bytes.Equal(part.Data, saved.Get(part.Name)) {
			t.Fatal("unrelated part changed", part.Name)
		}
	}
	for cell, want := range map[string]string{"C2": "B2*2", "C3": "B3*2", "C4": "SUM(C2:C3)"} {
		got, e := f.GetCellFormula("Model", cell)
		if e != nil || got != want {
			t.Fatal("formula changed", cell, got, e)
		}
	}
	after, _ := os.ReadFile(source)
	if !bytes.Equal(before, after) {
		t.Fatal("source modified")
	}
	if result["original_package_preserved_except_calculated_results"] != true {
		t.Fatal(result)
	}
}
