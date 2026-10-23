package pptx

import (
	"bytes"
	"context"
	"github.com/beevik/etree"
	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"os"
	"path/filepath"
	"testing"
)

func TestChartDetailedFormatting(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "in.pptx")
	out := filepath.Join(dir, "out.pptx")
	if err := Create(context.Background(), Spec{Slides: []Slide{{Elements: []Element{{Type: "chart", W: 8, H: 5, Chart: &Chart{Type: "bar", Categories: []string{"A", "B", "C"}, Series: []Series{{Name: "Sales", Values: []float64{1, 3, 6}}}}}}}}}, dir, in); err != nil {
		t.Fatal(err)
	}
	p, _ := opc.Open(in)
	before, _ := p.XML("ppt/charts/chart1.xml")
	axis := office.All(before.Root(), cns, "valAx")[0]
	id := office.Child(axis, cns, "axId").SelectAttrValue("val", "")
	size, minor, value := 14.0, 0.5, 5.0
	yes := true
	style := &ChartTextStyle{Font: "Arial", FontSize: &size, Color: "123456", Bold: &yes}
	trends := []ChartTrendline{{Type: "linear", DisplayEquation: &yes, DisplayRSquared: &yes}}
	f := ChartFormat{Axes: []ChartAxisFormat{{ID: id, MinorUnit: &minor, TextStyle: style}}, Legend: &ChartLegendFormat{TextStyle: style, Fill: &DrawingStyle{Fill: "FFFFFF"}}, Series: []ChartSeriesFormat{{Index: 0, Points: []ChartPointFormat{{Index: 1, Fill: &DrawingStyle{Fill: "FF0000"}}}, Labels: &ChartLabelsFormat{TextStyle: style, ShowValue: &yes}, Trendlines: &trends, ErrorBars: &ChartErrorBars{Type: "percentage", Value: &value}}}}
	edit := func(dest string) error {
		_, e := EditFile(context.Background(), in, dest, EditSpec{Edits: []Edit{{Slide: 1, ShapeID: "2", Operation: "chart-format", ChartFormat: &f}}})
		return e
	}
	if err := edit(out); err != nil {
		t.Fatal(err)
	}
	q, _ := opc.Open(out)
	after, err := q.XML("ppt/charts/chart1.xml")
	if err != nil {
		t.Fatal(err)
	}
	for _, part := range p.Parts {
		if part.Name != "ppt/charts/chart1.xml" && !bytes.Equal(part.Data, q.Get(part.Name)) {
			t.Fatal("changed unrelated part", part.Name)
		}
	}
	for _, tag := range []string{"cat", "val", "externalData"} {
		a := office.All(before.Root(), cns, tag)
		b := office.All(after.Root(), cns, tag)
		if tag == "val" {
			b = b[:len(a)]
		}
		for i := range a {
			if a[i].Text() != b[i].Text() {
				t.Fatal("changed data", tag)
			}
		}
	}
	if _, err := office.VerifyChartData(q, "powerpoint"); err != nil {
		t.Fatal(err)
	}
	for _, tag := range []string{"minorUnit", "dPt", "trendline", "errBars"} {
		if len(office.All(after.Root(), cns, tag)) != 1 {
			t.Fatal("missing", tag)
		}
	}
	for _, tag := range []string{"legend", "dLbls", "valAx"} {
		n := office.All(after.Root(), cns, tag)[0]
		if len(office.All(office.Child(n, cns, "txPr"), ans, "defRPr")) == 0 {
			t.Fatal("missing text style", tag)
		}
	}
	tr := office.All(after.Root(), cns, "trendline")[0]
	if tr.ChildElements()[0].Tag != "trendlineType" {
		t.Fatal("trendline order", tr.ChildElements()[0].Tag)
	}
	// Invalid requests must not publish a partially modified deck.
	f.Series[0].Points[0].Index = 99
	bad := filepath.Join(dir, "bad.pptx")
	if edit(bad) == nil {
		t.Fatal("accepted invalid point")
	}
	if _, err := os.Stat(bad); !os.IsNotExist(err) {
		t.Fatal("wrote rejected edit")
	}
	f.Series[0].Points = nil
	trends[0].Type = "bogus"
	if edit(bad) == nil {
		t.Fatal("accepted invalid trend")
	}
	trends[0].Type = "linear"
	f.Series[0].ErrorBars.Type = "standardError"
	if edit(bad) == nil {
		t.Fatal("accepted stdErr value")
	}
}

func TestChartTrendlineOrder(t *testing.T) {
	d := etree.NewDocument()
	if err := d.ReadFromString(`<c:lineChart xmlns:c="` + cns + `"><c:ser><c:idx val="0"/><c:order val="0"/><c:val/></c:ser></c:lineChart>`); err != nil {
		t.Fatal(err)
	}
	trends := []ChartTrendline{{Type: "linear", Name: "first"}, {Type: "power", Name: "second"}, {Type: "logarithmic", Name: "third"}}
	ser := office.Child(d.Root(), cns, "ser")
	if err := formatSeriesExtras(ser, ChartSeriesFormat{Trendlines: &trends}); err != nil {
		t.Fatal(err)
	}
	nodes := office.Children(ser, cns, "trendline")
	for i, n := range nodes {
		if office.Child(n, cns, "name").Text() != trends[i].Name {
			t.Fatal("trendlines reordered")
		}
	}
	if ser.ChildElements()[len(ser.ChildElements())-1].Tag != "val" {
		t.Fatal("trendlines follow data")
	}
}

func TestPointFormattingPreservesMarkerSymbol(t *testing.T) {
	d := etree.NewDocument()
	if err := d.ReadFromString(`<c:lineChart xmlns:c="` + cns + `"><c:ser><c:idx val="0"/><c:order val="0"/><c:marker><c:symbol val="diamond"/></c:marker><c:val><c:numLit><c:ptCount val="1"/><c:pt idx="0"><c:v>3</c:v></c:pt></c:numLit></c:val></c:ser></c:lineChart>`); err != nil {
		t.Fatal(err)
	}
	ser := office.Child(d.Root(), cns, "ser")
	if err := formatSeriesExtras(ser, ChartSeriesFormat{Points: []ChartPointFormat{{Index: 0, Fill: &DrawingStyle{Fill: "FF0000"}}}}); err != nil {
		t.Fatal(err)
	}
	point := office.Child(ser, cns, "dPt")
	marker := office.Child(point, cns, "marker")
	if office.Child(marker, cns, "symbol").SelectAttrValue("val", "") != "diamond" {
		t.Fatal("marker shape lost")
	}
	if len(office.All(marker, ans, "srgbClr")) != 1 {
		t.Fatal("missing marker paint")
	}
}
