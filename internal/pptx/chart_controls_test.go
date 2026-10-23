package pptx

import (
	"bytes"
	"context"
	"github.com/beevik/etree"
	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"github.com/xuri/excelize/v2"
	"path/filepath"
	"testing"
)

func chartControlFixture(t *testing.T, kind string) (string, *etree.Document) {
	t.Helper()
	dir := t.TempDir()
	in := filepath.Join(dir, "in.pptx")
	c := &Chart{Type: kind, Categories: []string{"A", "B", "C"}, Series: []Series{{Name: "Sales", Values: []float64{10, 20, 30}}}}
	if err := Create(context.Background(), Spec{Slides: []Slide{{Elements: []Element{{Type: "chart", W: 8, H: 5, Chart: c}}}}}, dir, in); err != nil {
		t.Fatal(err)
	}
	p, err := opc.Open(in)
	if err != nil {
		t.Fatal(err)
	}
	d, err := p.XML("ppt/charts/chart1.xml")
	if err != nil {
		t.Fatal(err)
	}
	return in, d
}
func editChartControl(t *testing.T, in string, f ChartFormat) *etree.Document {
	t.Helper()
	out := filepath.Join(t.TempDir(), "out.pptx")
	if _, err := EditFile(context.Background(), in, out, EditSpec{Edits: []Edit{{Slide: 1, ShapeID: "2", Operation: "chart-format", ChartFormat: &f}}}); err != nil {
		t.Fatal(err)
	}
	p, _ := opc.Open(in)
	q, err := opc.Open(out)
	if err != nil {
		t.Fatal(err)
	}
	for _, part := range p.Parts {
		if part.Name != "ppt/charts/chart1.xml" && !bytes.Equal(part.Data, q.Get(part.Name)) {
			t.Fatal("changed unrelated part", part.Name)
		}
	}
	before, _ := p.XML("ppt/charts/chart1.xml")
	after, err := q.XML("ppt/charts/chart1.xml")
	if err != nil {
		t.Fatal(err)
	}
	for _, tag := range []string{"cat", "val", "xVal", "yVal", "externalData"} {
		a := office.All(before.Root(), cns, tag)
		b := office.All(after.Root(), cns, tag)
		if len(a) != len(b) {
			t.Fatal("changed data count", tag)
		}
		for i := range a {
			da := etree.NewDocument()
			da.SetRoot(a[i].Copy())
			db := etree.NewDocument()
			db.SetRoot(b[i].Copy())
			aa, _ := da.WriteToBytes()
			bb, _ := db.WriteToBytes()
			if !bytes.Equal(aa, bb) {
				t.Fatal("changed data", tag)
			}
		}
	}
	if _, err := office.VerifyChartData(q, "powerpoint"); err != nil {
		t.Fatal(err)
	}
	return after
}
func TestIndividualChartLabel(t *testing.T) {
	in, _ := chartControlFixture(t, "bar")
	text := "Special & <label>"
	size := 20.0
	yes := true
	f := ChartFormat{Series: []ChartSeriesFormat{{Index: 0, LabelOverrides: []ChartLabelOverride{{Index: 1, Text: &text, TextStyle: &ChartTextStyle{FontSize: &size, Bold: &yes}, Fill: &DrawingStyle{Fill: "FFFF00"}, Position: "outEnd"}}}}}
	d := editChartControl(t, in, f)
	labels := office.All(d.Root(), cns, "dLbl")
	if len(labels) != 1 {
		t.Fatal("wrong label count")
	}
	l := labels[0]
	if office.Child(l, cns, "idx").SelectAttrValue("val", "") != "1" || office.All(l, ans, "t")[0].Text() != text {
		t.Fatal("label text/index lost")
	}
	rich := office.Child(office.Child(l, cns, "tx"), cns, "rich")
	pr := office.All(rich, ans, "defRPr")[0]
	if pr.SelectAttrValue("sz", "") != "2000" || pr.SelectAttrValue("b", "") != "1" {
		t.Fatal("rich formatting missing")
	}
	if office.All(office.Child(l, cns, "spPr"), ans, "srgbClr")[0].SelectAttrValue("val", "") != "FFFF00" {
		t.Fatal("label fill missing")
	}
	f.Series[0].LabelOverrides[0].Index = 3
	if _, err := EditFile(context.Background(), in, filepath.Join(t.TempDir(), "bad.pptx"), EditSpec{Edits: []Edit{{Slide: 1, ShapeID: "2", Operation: "chart-format", ChartFormat: &f}}}); err == nil {
		t.Fatal("invalid label index accepted")
	}
}

func TestChartAxisPlacement(t *testing.T) {
	in, before := chartControlFixture(t, "line")
	axis := office.All(before.Root(), cns, "valAx")[0]
	id := office.Child(axis, cns, "axId").SelectAttrValue("val", "")
	f := ChartFormat{Axes: []ChartAxisFormat{{ID: id, Position: "right", TickLabelPosition: "high"}}}
	d := editChartControl(t, in, f)
	axis = office.All(d.Root(), cns, "valAx")[0]
	if office.Child(axis, cns, "axPos").SelectAttrValue("val", "") != "r" || office.Child(axis, cns, "tickLblPos").SelectAttrValue("val", "") != "high" {
		t.Fatal("axis placement not saved")
	}
	for _, bad := range []ChartAxisFormat{{ID: id, Position: "bogus"}, {ID: id, TickLabelPosition: "bogus"}} {
		f.Axes = []ChartAxisFormat{bad}
		if _, err := EditFile(context.Background(), in, filepath.Join(t.TempDir(), "bad.pptx"), EditSpec{Edits: []Edit{{Slide: 1, ShapeID: "2", Operation: "chart-format", ChartFormat: &f}}}); err == nil {
			t.Fatal("accepted invalid axis placement")
		}
	}
}

func TestChartMarkerEditing(t *testing.T) {
	in, _ := chartControlFixture(t, "line")
	size := 12
	f := ChartFormat{Series: []ChartSeriesFormat{{Index: 0, Marker: &ChartMarkerFormat{Symbol: "diamond", Size: &size, Fill: &DrawingStyle{Fill: "FF0000"}, Outline: &TextOutline{Width: 1, Fill: &DrawingStyle{Fill: "000000"}}}}}}
	d := editChartControl(t, in, f)
	m := office.Child(office.All(d.Root(), cns, "ser")[0], cns, "marker")
	if office.Child(m, cns, "symbol").SelectAttrValue("val", "") != "diamond" || office.Child(m, cns, "size").SelectAttrValue("val", "") != "12" {
		t.Fatal("marker not saved")
	}
	if office.All(m, ans, "srgbClr")[0].SelectAttrValue("val", "") != "FF0000" {
		t.Fatal("marker color missing")
	}
	size = 73
	if _, err := EditFile(context.Background(), in, filepath.Join(t.TempDir(), "bad.pptx"), EditSpec{Edits: []Edit{{Slide: 1, ShapeID: "2", Operation: "chart-format", ChartFormat: &f}}}); err == nil {
		t.Fatal("invalid marker size accepted")
	}
}
func TestChartCreationPreservesSeriesFormatting(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "created.pptx")
	opts := &excelize.Chart{Series: []excelize.ChartSeries{{Name: "Wrong!A1", Categories: "Wrong!A2:A3", Values: "Wrong!B2:B3", Sizes: "Wrong!C2:C3", Marker: excelize.ChartMarker{Symbol: "diamond", Size: 12, Fill: excelize.Fill{Color: []string{"FF0000"}}}, Line: excelize.LineOptions{Width: 3}}}}
	c := &Chart{Type: "line", Categories: []string{"A", "B"}, Series: []Series{{Name: "Sales", Values: []float64{10, 20}}}, Options: opts}
	if err := Create(context.Background(), Spec{Slides: []Slide{{Elements: []Element{{Type: "chart", W: 8, H: 5, Chart: c}}}}}, dir, out); err != nil {
		t.Fatal(err)
	}
	pkg, err := opc.Open(out)
	if err != nil {
		t.Fatal(err)
	}
	d, err := pkg.XML("ppt/charts/chart1.xml")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := office.VerifyChartData(pkg, "powerpoint"); err != nil {
		t.Fatal(err)
	}
	ser := office.All(d.Root(), cns, "ser")[0]
	m := office.Child(ser, cns, "marker")
	if office.Child(m, cns, "symbol").SelectAttrValue("val", "") != "diamond" || office.Child(m, cns, "size").SelectAttrValue("val", "") != "12" {
		t.Fatal("discarded creation marker options")
	}
	for _, f := range office.All(ser, cns, "f") {
		if bytes.Contains([]byte(f.Text()), []byte("Wrong")) {
			t.Fatal("retained supplied data reference")
		}
	}
	if opts.Series[0].Name != "Wrong!A1" {
		t.Fatal("mutated caller options")
	}
}
