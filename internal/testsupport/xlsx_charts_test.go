package testsupport

import (
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"github.com/xuri/excelize/v2"
	"testing"
)

func TestActiveWorksheetChartsIgnoresOrphans(t *testing.T) {
	f := excelize.NewFile()
	defer f.Close()
	must := func(e error) {
		t.Helper()
		if e != nil {
			t.Fatal(e)
		}
	}
	_, e := f.NewSheet("Other")
	must(e)
	must(f.SetCellValue("Sheet1", "A1", 7))
	must(f.AddChart("Sheet1", "D2", &excelize.Chart{Type: excelize.Col, Series: []excelize.ChartSeries{{Name: "Values", Values: "Sheet1!$A$1"}}}))
	raw, e := f.WriteToBuffer()
	must(e)
	for _, tc := range []struct {
		name string
		want int
	}{{"active", 1}, {"inactive", 0}, {"orphan-only", 0}, {"unused-chart-rel", 1}, {"no-drawing-element", 1}, {"unanchored", 0}, {"duplicate-anchor", 2}} {
		t.Run(tc.name, func(t *testing.T) {
			p, e := opc.Read(raw.Bytes())
			if e != nil {
				t.Fatal(e)
			}
			switch tc.name {
			case "inactive":
				d, e := p.XML("xl/workbook.xml")
				must(e)
				view := d.Root().FindElement("bookViews/workbookView")
				if view == nil {
					t.Fatal("missing view")
				}
				view.CreateAttr("activeTab", "1")
				must(p.SetXML("xl/workbook.xml", d))
			case "orphan-only":
				p.Remove("xl/worksheets/_rels/sheet1.xml.rels")
			case "unused-chart-rel":
				p.Set("xl/charts/orphan.xml", p.Get("xl/charts/chart1.xml"))
				_, e = p.AddRel("xl/drawings/drawing1.xml", "http://schemas.openxmlformats.org/officeDocument/2006/relationships/chart", "../charts/orphan.xml", false)
				must(e)
			case "no-drawing-element":
				d, e := p.XML("xl/worksheets/sheet1.xml")
				must(e)
				for _, v := range d.Root().ChildElements() {
					if v.Tag == "drawing" {
						d.Root().RemoveChild(v)
					}
				}
				must(p.SetXML("xl/worksheets/sheet1.xml", d))
			case "unanchored", "duplicate-anchor":
				d, e := p.XML("xl/drawings/drawing1.xml")
				must(e)
				anchors := d.Root().ChildElements()
				if len(anchors) != 1 {
					t.Fatal("expected one anchor")
				}
				if tc.name == "duplicate-anchor" {
					d.Root().AddChild(anchors[0].Copy())
				} else {
					for _, v := range anchors[0].ChildElements() {
						if v.Tag == "graphicFrame" {
							anchors[0].RemoveChild(v)
						}
					}
				}
				must(p.SetXML("xl/drawings/drawing1.xml", d))
			}
			data, e := p.Bytes()
			must(e)
			charts, e := ActiveWorksheetCharts(data)
			if e != nil || len(charts) != tc.want {
				t.Fatalf("%v, %v; want %d attached active-sheet charts", charts, e, tc.want)
			}
		})
	}
}
