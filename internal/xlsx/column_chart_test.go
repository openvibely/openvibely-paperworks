package xlsx

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"github.com/xuri/excelize/v2"
)

func inheritedTestChart() ColumnChart {
	return ColumnChart{Cell: "F2", Title: "Sales & costs <2026>", ValueTitle: "USD", WidthEMU: 5400000, HeightEMU: 2700000, Series: []ColumnSeries{{Name: "'Sheet1'!D1", Categories: "'Sheet1'!$A$2:$A$3", Values: "'Sheet1'!$D$2:$D$3"}}}
}
func TestInheritedColumnChartsAndTheme(t *testing.T) {
	root := t.TempDir()
	theme := []byte(`<a:theme xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" name="User theme"><a:themeElements/></a:theme>`)
	if e := os.WriteFile(filepath.Join(root, "theme.xml"), theme, 0600); e != nil {
		t.Fatal(e)
	}
	c := inheritedTestChart()
	c.Series = append(c.Series, c.Series[0])
	out := filepath.Join(root, "new.xlsx")
	base := uint8(8)
	height := 15.0
	left := 0.75
	s := Spec{ThemeFile: "theme.xml", Sheets: []Sheet{{Name: "Sheet1", Rows: [][]any{{"Label", "Value"}, {"A", 120}}, ColumnCharts: []ColumnChart{c, c}, SheetProps: &excelize.SheetPropsOptions{BaseColWidth: &base, DefaultRowHeight: &height}, PageMargins: &excelize.PageLayoutMarginsOptions{Left: &left}}, {Name: "Second", ColumnCharts: []ColumnChart{c}}}}
	if e := Create(context.Background(), s, root, out); e != nil {
		t.Fatal(e)
	}
	p, e := opc.Open(out)
	if e != nil {
		t.Fatal(e)
	}
	if string(p.Get("xl/theme/theme1.xml")) != string(theme) {
		t.Fatal("theme rewritten")
	}
	const nc = "http://schemas.openxmlformats.org/drawingml/2006/chart"
	for _, name := range []string{"xl/charts/chart1.xml", "xl/charts/chart2.xml", "xl/charts/chart3.xml"} {
		d, e := p.XML(name)
		if e != nil {
			t.Fatal(e)
		}
		if got := office.All(d.Root(), nc, "ser"); len(got) != 2 {
			t.Fatal("series", len(got))
		}
		if got := office.All(d.Root(), nc, "f"); len(got) != 6 || got[0].Text() != c.Series[0].Name {
			t.Fatal("references")
		}
		if got := office.All(d.Root(), "http://schemas.openxmlformats.org/drawingml/2006/main", "t"); len(got) != 2 || got[0].Text() != c.Title {
			t.Fatal("escaped title")
		}
		if len(office.All(d.Root(), nc, "numCache")) != 0 {
			t.Fatal("frozen cache")
		}
	}
	for _, name := range []string{"xl/drawings/drawing1.xml", "xl/drawings/drawing2.xml"} {
		d, e := p.XML(name)
		if e != nil {
			t.Fatal(e)
		}
		n := "http://schemas.openxmlformats.org/drawingml/2006/spreadsheetDrawing"
		a := office.All(d.Root(), n, "oneCellAnchor")
		want := 1
		if name == "xl/drawings/drawing1.xml" {
			want = 2
		}
		if len(a) != want {
			t.Fatal("anchors", len(a))
		}
		for _, x := range a {
			ext := office.Child(x, n, "ext")
			if ext.SelectAttrValue("cx", "") != "5400000" || ext.SelectAttrValue("cy", "") != "2700000" {
				t.Fatal("rounded dimensions")
			}
		}
	}
	report, e := office.PackageReport(out)
	if e != nil {
		t.Fatal(e)
	}
	_ = report
	// Check the published relationship graph, not just orphan chart members.
	for _, name := range []string{"xl/worksheets/sheet1.xml", "xl/worksheets/sheet2.xml", "xl/drawings/drawing1.xml", "xl/drawings/drawing2.xml"} {
		rels, err := p.Rels(name)
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, rel := range rels {
			if rel.Type == opc.NSOffR+"/drawing" || rel.Type == opc.NSOffR+"/chart" {
				found = true
				if len(p.Get(opc.Resolve(name, rel.Target))) == 0 {
					t.Fatal("missing target", rel)
				}
			}
		}
		if !found {
			t.Fatal("missing relationships", name)
		}
	}
	f, e := excelize.OpenFile(out)
	if e != nil {
		t.Fatal(e)
	}
	defer f.Close()
	props, e := f.GetSheetProps("Sheet1")
	if e != nil || props.BaseColWidth == nil || *props.BaseColWidth != 8 {
		t.Fatal("props", props, e)
	}
	margins, e := f.GetPageMargins("Sheet1")
	if e != nil || *margins.Left != left {
		t.Fatal("margins", e)
	}
}
func TestInheritedColumnChartRejectsWithoutPublishing(t *testing.T) {
	for _, name := range []string{"empty", "refs", "cell", "dimensions", "overflow", "mixed", "theme"} {
		t.Run(name, func(t *testing.T) {
			c := inheritedTestChart()
			s := Spec{Sheets: []Sheet{{Name: "Sheet1", ColumnCharts: []ColumnChart{c}}}}
			switch name {
			case "empty":
				s.Sheets[0].ColumnCharts[0].Series = nil
			case "refs":
				s.Sheets[0].ColumnCharts[0].Series[0].Values = ""
			case "cell":
				s.Sheets[0].ColumnCharts[0].Cell = "not-a-cell"
			case "dimensions":
				s.Sheets[0].ColumnCharts[0].WidthEMU = 0
			case "overflow":
				s.Sheets[0].ColumnCharts[0].HeightEMU = 2147483648
			case "mixed":
				s.Sheets[0].Charts = []Chart{{Type: "pie", Cell: "H2", Series: []excelize.ChartSeries{{Categories: "Sheet1!A1", Values: "Sheet1!B1"}}}}
			case "theme":
				s.ThemeFile = "wrong.xml"
			}
			root := t.TempDir()
			if e := os.WriteFile(filepath.Join(root, "wrong.xml"), []byte(`<theme/>`), 0600); e != nil {
				t.Fatal(e)
			}
			out := filepath.Join(root, "no.xlsx")
			if e := Create(context.Background(), s, root, out); e == nil {
				t.Fatal("accepted invalid input")
			}
			if _, e := os.Stat(out); !os.IsNotExist(e) {
				t.Fatal("published", e)
			}
		})
	}
}

func TestThemeResourcePathsAndValidation(t *testing.T) {
	const drawingML = "http://schemas.openxmlformats.org/drawingml/2006/main"
	theme := []byte(`<a:theme xmlns:a="` + drawingML + `" name="Theme bytes"><a:themeElements/></a:theme>`)

	for _, tc := range []struct {
		name   string
		useAbs bool
	}{
		{name: "relative"},
		{name: "absolute", useAbs: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			if err := os.WriteFile(filepath.Join(root, "theme.xml"), theme, 0600); err != nil {
				t.Fatal(err)
			}
			name := "theme.xml"
			if tc.useAbs {
				name = filepath.Join(root, name)
			}
			out := filepath.Join(root, "theme.xlsx")
			if err := Create(context.Background(), Spec{ThemeFile: name, Sheets: []Sheet{{Name: "Sheet1"}}}, root, out); err != nil {
				t.Fatal(err)
			}
			pkg, err := opc.Open(out)
			if err != nil {
				t.Fatal(err)
			}
			if got := pkg.Get("xl/theme/theme1.xml"); string(got) != string(theme) {
				t.Fatal("theme resource bytes changed")
			}
		})
	}

	for _, tc := range []struct {
		name string
		raw  string
	}{
		{name: "missing"},
		{name: "malformed", raw: "<a:theme"},
		{name: "wrong-root", raw: `<a:clrScheme xmlns:a="` + drawingML + `"/>`},
		{name: "wrong-namespace", raw: `<theme/>`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			if tc.raw != "" {
				if err := os.WriteFile(filepath.Join(root, "theme.xml"), []byte(tc.raw), 0600); err != nil {
					t.Fatal(err)
				}
			}
			out := filepath.Join(root, "invalid.xlsx")
			err := Create(context.Background(), Spec{ThemeFile: "theme.xml", Sheets: []Sheet{{Name: "Sheet1"}}}, root, out)
			if err == nil {
				t.Fatal("invalid theme accepted")
			}
			if _, err = os.Stat(out); !os.IsNotExist(err) {
				t.Fatal("published invalid theme", err)
			}
		})
	}
}

func TestColumnChartPartsAvoidExistingCharts(t *testing.T) {
	out := filepath.Join(t.TempDir(), "mixed.xlsx")
	spec := Spec{Sheets: []Sheet{
		{Name: "Sheet1", Rows: [][]any{{"A", 1}, {"B", 2}}, Charts: []Chart{{Type: "column", Cell: "F2", Series: []excelize.ChartSeries{{Name: "Sheet1!$A$1", Categories: "Sheet1!$A$1:$A$2", Values: "Sheet1!$B$1:$B$2"}}}}},
		{Name: "Second", ColumnCharts: []ColumnChart{inheritedTestChart()}},
	}}
	if err := Create(context.Background(), spec, "", out); err != nil {
		t.Fatal(err)
	}
	p, err := opc.Open(out)
	if err != nil {
		t.Fatal(err)
	}
	for _, sheet := range []string{"xl/worksheets/sheet1.xml", "xl/worksheets/sheet2.xml"} {
		rels, err := p.Rels(sheet)
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range rels {
			if r.Type != opc.NSOffR+"/drawing" {
				continue
			}
			drawing := opc.Resolve(sheet, r.Target)
			links, err := p.Rels(drawing)
			if err != nil {
				t.Fatal(err)
			}
			for _, link := range links {
				if !p.Has(opc.Resolve(drawing, link.Target)) {
					t.Fatal("dangling chart", link)
				}
			}
		}
	}
	for _, name := range []string{"xl/charts/chart1.xml", "xl/charts/chart2.xml", "xl/drawings/drawing1.xml", "xl/drawings/drawing2.xml"} {
		if !p.Has(name) {
			t.Fatal("missing", name)
		}
	}
}
