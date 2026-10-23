package pptx

import (
	"bytes"
	"context"
	"github.com/beevik/etree"
	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"github.com/xuri/excelize/v2"
	"path/filepath"
	"strings"
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

func TestChartBackgroundFills(t *testing.T) {
	in, _ := chartControlFixture(t, "bar")
	d := editChartControl(t, in, ChartFormat{ChartFill: &DrawingStyle{Fill: "EEEEEE"}, PlotAreaFill: &DrawingStyle{Fill: "DDDDDD"}})
	plot := office.All(d.Root(), cns, "plotArea")[0]
	for _, v := range []struct {
		node  *etree.Element
		color string
	}{{d.Root(), "EEEEEE"}, {plot, "DDDDDD"}} {
		sp := office.Child(v.node, cns, "spPr")
		if sp == nil {
			t.Fatal("missing background properties")
		}
		if office.All(sp, ans, "srgbClr")[0].SelectAttrValue("val", "") != v.color {
			t.Fatal("wrong background color")
		}
	}
	children := plot.ChildElements()
	if children[len(children)-1].Tag != "spPr" {
		t.Fatal("plot fill placed before chart/axes")
	}
	root := d.Root().ChildElements()
	index := 0
	for i, n := range root {
		if n.Tag == "chart" {
			index = i
		}
	}
	if root[index+1].Tag != "spPr" {
		t.Fatal("chart fill order invalid")
	}
}

func TestIndividualChartLabelInheritedNamespaces(t *testing.T) {
	for _, prefix := range []string{"a", "z", ""} {
		t.Run("prefix="+prefix, func(t *testing.T) {
			binding, qualified := "xmlns", ""
			if prefix != "" {
				binding += ":" + prefix
				qualified = prefix + ":"
			}
			xml := `<c:chartSpace xmlns:c="` + cns + `" ` + binding + `="` + ans + `"><c:chart><c:plotArea><c:lineChart><c:ser><c:idx val="0"/><c:order val="0"/><c:dLbls><c:dLbl><c:idx val="0"/><c:tx><c:rich><DRAW:bodyPr/><DRAW:lstStyle/><DRAW:p><DRAW:r><DRAW:rPr sz="1200" i="1"/><DRAW:t>Original</DRAW:t></DRAW:r></DRAW:p></c:rich></c:tx></c:dLbl></c:dLbls><c:val><c:numLit><c:ptCount val="1"/><c:pt idx="0"><c:v>1</c:v></c:pt></c:numLit></c:val></c:ser></c:lineChart></c:plotArea></c:chart></c:chartSpace>`
			d := etree.NewDocument()
			if err := d.ReadFromString(strings.ReplaceAll(xml, "DRAW:", qualified)); err != nil {
				t.Fatal(err)
			}
			size := 20.0
			if err := formatLabelOverrides(office.All(d.Root(), cns, "ser")[0], []ChartLabelOverride{{Index: 0, TextStyle: &ChartTextStyle{FontSize: &size}}}); err != nil {
				t.Fatal(err)
			}
			raw, err := d.WriteToBytes()
			if err != nil {
				t.Fatal(err)
			}
			saved := etree.NewDocument()
			if err := saved.ReadFromBytes(raw); err != nil {
				t.Fatal(err)
			}
			rich := office.All(saved.Root(), cns, "rich")[0]
			for _, tag := range []string{"bodyPr", "lstStyle", "p"} {
				if got := len(office.Children(rich, ans, tag)); got != 1 {
					t.Fatalf("%s count = %d, want 1", tag, got)
				}
			}
			runs := office.All(rich, ans, "rPr")
			if len(runs) != 1 || runs[0].SelectAttrValue("sz", "") != "2000" || runs[0].SelectAttrValue("i", "") != "1" {
				t.Fatalf("run style not updated/preserved: %s", raw)
			}
			texts := office.All(rich, ans, "t")
			if len(texts) != 1 || texts[0].Text() != "Original" {
				t.Fatalf("label text changed: %s", raw)
			}
		})
	}
}

func TestChartTitleEdit(t *testing.T) {
	in, _ := chartControlFixture(t, "line")
	for _, title := range []string{"Revenue & costs", ""} {
		d := editChartControl(t, in, ChartFormat{Title: &title})
		chart := office.Child(d.Root(), cns, "chart")
		got := office.Child(chart, cns, "title")
		if title == "" {
			if got != nil || office.Child(chart, cns, "autoTitleDeleted").SelectAttrValue("val", "") != "1" {
				t.Fatal("title not removed")
			}
		} else {
			if got == nil || office.All(got, ans, "t")[0].Text() != title {
				t.Fatal("title not set")
			}
			if office.Child(chart, cns, "autoTitleDeleted").SelectAttrValue("val", "") != "0" {
				t.Fatal("title hidden")
			}
		}
	}
}

func TestChartAreaOutlines(t *testing.T) {
	in, _ := chartControlFixture(t, "line")
	d := editChartControl(t, in, ChartFormat{ChartFill: &DrawingStyle{Fill: "EEEEEE"}, ChartOutline: &TextOutline{Width: 2, Fill: &DrawingStyle{Fill: "FF0000"}}, PlotAreaOutline: &TextOutline{Width: 0}})
	sp := office.Child(d.Root(), cns, "spPr")
	if office.Child(sp, ans, "solidFill") == nil {
		t.Fatal("background missing")
	}
	line := office.Child(sp, ans, "ln")
	if line == nil || line.SelectAttrValue("w", "") != "25400" || office.All(line, ans, "srgbClr")[0].SelectAttrValue("val", "") != "FF0000" {
		t.Fatal("outer outline missing")
	}
	plot := office.All(d.Root(), cns, "plotArea")[0]
	if office.All(office.Child(plot, cns, "spPr"), ans, "noFill") == nil {
		t.Fatal("plot outline not cleared")
	}
}

func TestChartAxisCrossingAndOutline(t *testing.T) {
	in, before := chartControlFixture(t, "line")
	valID := office.Child(office.All(before.Root(), cns, "valAx")[0], cns, "axId").SelectAttrValue("val", "")
	catID := office.Child(office.All(before.Root(), cns, "catAx")[0], cns, "axId").SelectAttrValue("val", "")
	cross := 25.0
	offset := 200
	d := editChartControl(t, in, ChartFormat{Axes: []ChartAxisFormat{{ID: valID, CrossesAt: &cross, Outline: &TextOutline{Width: 2, Fill: &DrawingStyle{Fill: "FF0000"}}}, {ID: catID, LabelOffsetPercent: &offset}}})
	val := office.All(d.Root(), cns, "valAx")[0]
	if office.Child(val, cns, "crosses") != nil || office.Child(val, cns, "crossesAt").SelectAttrValue("val", "") != "25" {
		t.Fatal("crossing not replaced")
	}
	if office.All(office.Child(val, cns, "spPr"), ans, "ln")[0].SelectAttrValue("w", "") != "25400" {
		t.Fatal("axis outline missing")
	}
	cat := office.All(d.Root(), cns, "catAx")[0]
	if office.Child(cat, cns, "lblOffset").SelectAttrValue("val", "") != "200" {
		t.Fatal("offset missing")
	}
	for _, bad := range []ChartAxisFormat{{ID: valID, LabelOffsetPercent: &offset}, {ID: catID, LabelOffsetPercent: func() *int { x := 1001; return &x }()}} {
		f := ChartFormat{Axes: []ChartAxisFormat{bad}}
		if _, err := EditFile(context.Background(), in, filepath.Join(t.TempDir(), "bad.pptx"), EditSpec{Edits: []Edit{{Slide: 1, ShapeID: "2", Operation: "chart-format", ChartFormat: &f}}}); err == nil {
			t.Fatal("invalid offset accepted")
		}
	}
}

func TestChartAdditionalLabelControls(t *testing.T) {
	in, _ := chartControlFixture(t, "line")
	yes, no := true, false
	sep := " / "
	empty := ""
	d := editChartControl(t, in, ChartFormat{Series: []ChartSeriesFormat{{Index: 0, Labels: &ChartLabelsFormat{Separator: &sep, ShowLeaderLines: &yes, ShowLegendKey: &yes}, LabelOverrides: []ChartLabelOverride{{Index: 1, Separator: &empty, ShowValue: &no, ShowCategory: &yes, ShowSeries: &yes}}}}})
	labels := office.Child(office.All(d.Root(), cns, "ser")[0], cns, "dLbls")
	if office.Child(labels, cns, "separator").Text() != sep {
		t.Fatal("separator missing")
	}
	for _, tag := range []string{"showLeaderLines", "showLegendKey"} {
		if office.Child(labels, cns, tag).SelectAttrValue("val", "") != "1" {
			t.Fatal("flag missing", tag)
		}
	}
	label := office.Child(labels, cns, "dLbl")
	if office.Child(label, cns, "separator") == nil || office.Child(label, cns, "separator").Text() != "" {
		t.Fatal("empty separator missing")
	}
	for tag, want := range map[string]string{"showVal": "0", "showCatName": "1", "showSerName": "1"} {
		if office.Child(label, cns, tag).SelectAttrValue("val", "") != want {
			t.Fatal("point flag missing", tag)
		}
	}
	if office.Child(label, cns, "showPercent") != nil {
		t.Fatal("omitted flag changed")
	}
	bad := ChartFormat{Series: []ChartSeriesFormat{{Index: 0, LabelOverrides: []ChartLabelOverride{{Index: 0, ShowPercent: &yes}}}}}
	if _, err := EditFile(context.Background(), in, filepath.Join(t.TempDir(), "bad.pptx"), EditSpec{Edits: []Edit{{Slide: 1, ShapeID: "2", Operation: "chart-format", ChartFormat: &bad}}}); err == nil {
		t.Fatal("line percent accepted")
	}
}

func TestChartTitleFormatting(t *testing.T) {
	in, _ := chartControlFixture(t, "line")
	title := "Revenue"
	size := 24.0
	yes := true
	d := editChartControl(t, in, ChartFormat{Title: &title, TitleStyle: &ChartTextStyle{FontSize: &size, Bold: &yes}, TitleOverlay: &yes})
	n := office.Child(office.Child(d.Root(), cns, "chart"), cns, "title")
	if office.Child(n, cns, "overlay").SelectAttrValue("val", "") != "1" {
		t.Fatal("overlay missing")
	}
	rich := office.Child(office.Child(n, cns, "tx"), cns, "rich")
	if office.All(rich, ans, "defRPr")[0].SelectAttrValue("sz", "") != "2400" {
		t.Fatal("title style missing")
	}
	empty := ""
	f := ChartFormat{Title: &empty, TitleOverlay: &yes}
	if _, err := EditFile(context.Background(), in, filepath.Join(t.TempDir(), "bad.pptx"), EditSpec{Edits: []Edit{{Slide: 1, ShapeID: "2", Operation: "chart-format", ChartFormat: &f}}}); err == nil {
		t.Fatal("formatted nonexistent title")
	}
}

func TestChartUnderlineAlignment(t *testing.T) {
	in, _ := chartControlFixture(t, "line")
	style := &ChartTextStyle{Underline: "dbl", Alignment: "right"}
	text := "Point"
	d := editChartControl(t, in, ChartFormat{Legend: &ChartLegendFormat{TextStyle: style}, Series: []ChartSeriesFormat{{Index: 0, LabelOverrides: []ChartLabelOverride{{Index: 0, Text: &text, TextStyle: style}}}}})
	for _, parent := range []*etree.Element{office.All(d.Root(), cns, "legend")[0], office.All(office.All(d.Root(), cns, "dLbl")[0], cns, "rich")[0]} {
		ps := office.All(parent, ans, "pPr")
		if len(ps) == 0 || ps[0].SelectAttrValue("algn", "") != "r" {
			t.Fatal("alignment missing")
		}
		if office.All(parent, ans, "defRPr")[0].SelectAttrValue("u", "") != "dbl" {
			t.Fatal("underline missing")
		}
	}
	for _, bad := range []ChartTextStyle{{Underline: "bogus"}, {Alignment: "bogus"}} {
		f := ChartFormat{Legend: &ChartLegendFormat{TextStyle: &bad}}
		if _, err := EditFile(context.Background(), in, filepath.Join(t.TempDir(), "bad.pptx"), EditSpec{Edits: []Edit{{Slide: 1, ShapeID: "2", Operation: "chart-format", ChartFormat: &f}}}); err == nil {
			t.Fatal("invalid text style accepted")
		}
	}
}

func TestChartTrendlineLabel(t *testing.T) {
	in, _ := chartControlFixture(t, "line")
	text := "Trend & forecast"
	yes := true
	size := 18.0
	trends := []ChartTrendline{{Type: "linear", DisplayEquation: &yes, Label: &ChartTrendlineLabel{Text: &text, TextStyle: &ChartTextStyle{FontSize: &size, Underline: "sng"}, Fill: &DrawingStyle{Fill: "EEEEEE"}, Outline: &TextOutline{Width: 1, Fill: &DrawingStyle{Fill: "FF0000"}}}}}
	d := editChartControl(t, in, ChartFormat{Series: []ChartSeriesFormat{{Index: 0, Trendlines: &trends}}})
	label := office.All(d.Root(), cns, "trendlineLbl")[0]
	if office.All(label, ans, "t")[0].Text() != text {
		t.Fatal("trendline label text missing")
	}
	if office.All(label, ans, "defRPr")[0].SelectAttrValue("sz", "") != "1800" {
		t.Fatal("trendline label style missing")
	}
	sp := office.Child(label, cns, "spPr")
	if office.Child(sp, ans, "solidFill") == nil || office.Child(sp, ans, "ln") == nil {
		t.Fatal("trendline label paint missing")
	}
}

func TestChartSeriesLabelAppearance(t *testing.T) {
	in, _ := chartControlFixture(t, "line")
	yes, no := true, false
	format := "0.0"
	f := ChartFormat{Series: []ChartSeriesFormat{{Index: 0, Labels: &ChartLabelsFormat{Visible: &no, Fill: &DrawingStyle{Fill: "FFFF00"}, Outline: &TextOutline{Width: 1, Fill: &DrawingStyle{Fill: "FF0000"}}, NumberFormat: &format, NumberFormatSourceLinked: &yes}}}}
	d := editChartControl(t, in, f)
	l := office.Child(office.All(d.Root(), cns, "ser")[0], cns, "dLbls")
	if office.Child(l, cns, "delete").SelectAttrValue("val", "") != "1" {
		t.Fatal("visibility missing")
	}
	n := office.Child(l, cns, "numFmt")
	if n.SelectAttrValue("sourceLinked", "") != "1" || n.SelectAttrValue("formatCode", "") != format {
		t.Fatal("number format missing")
	}
	if office.All(office.Child(l, cns, "spPr"), ans, "ln")[0].SelectAttrValue("w", "") != "12700" {
		t.Fatal("label outline missing")
	}
	f.Series[0].Labels.Visible = &yes
	d = editChartControl(t, in, f)
	if office.Child(office.Child(office.All(d.Root(), cns, "ser")[0], cns, "dLbls"), cns, "delete").SelectAttrValue("val", "") != "0" {
		t.Fatal("labels not shown")
	}
}

func TestAxisTitleStyleSeparateFromTicks(t *testing.T) {
	in, source := chartControlFixture(t, "line")
	id := office.Child(office.All(source.Root(), cns, "valAx")[0], cns, "axId").SelectAttrValue("val", "")
	title, size, tickSize := "Revenue", 21.0, 10.0
	d := editChartControl(t, in, ChartFormat{Axes: []ChartAxisFormat{{ID: id, Title: &title, TitleStyle: &ChartTextStyle{FontSize: &size, Color: "FF0000"}, TextStyle: &ChartTextStyle{FontSize: &tickSize}}}})
	axis := office.All(d.Root(), cns, "valAx")[0]
	n := office.Child(axis, cns, "title")
	if office.All(n, ans, "t")[0].Text() != title || office.All(n, ans, "defRPr")[0].SelectAttrValue("sz", "") != "2100" {
		t.Fatal("axis title style missing")
	}
	if office.All(office.Child(axis, cns, "txPr"), ans, "defRPr")[0].SelectAttrValue("sz", "") != "1000" {
		t.Fatal("title style changed tick labels")
	}
	empty := ""
	bad := ChartFormat{Axes: []ChartAxisFormat{{ID: id, Title: &empty, TitleStyle: &ChartTextStyle{FontSize: &size}}}}
	if _, err := EditFile(context.Background(), in, filepath.Join(t.TempDir(), "bad.pptx"), EditSpec{Edits: []Edit{{Slide: 1, ShapeID: "2", Operation: "chart-format", ChartFormat: &bad}}}); err == nil {
		t.Fatal("styled removed axis title")
	}
}

func TestChartTextGradient(t *testing.T) {
	in, source := chartControlFixture(t, "line")
	id := office.Child(office.All(source.Root(), cns, "valAx")[0], cns, "axId").SelectAttrValue("val", "")
	title := "Revenue"
	fill := &DrawingStyle{Gradient: &Gradient{Angle: 45, Stops: []GradientStop{{Position: 0, Color: "FF0000"}, {Position: 1, Color: "0000FF"}}}}
	style := &ChartTextStyle{Fill: fill}
	d := editChartControl(t, in, ChartFormat{Title: &title, TitleStyle: style, Axes: []ChartAxisFormat{{ID: id, Title: &title, TitleStyle: style, TextStyle: style}}, Legend: &ChartLegendFormat{TextStyle: style}, Series: []ChartSeriesFormat{{Index: 0, Labels: &ChartLabelsFormat{TextStyle: style}, LabelOverrides: []ChartLabelOverride{{Index: 0, Text: &title, TextStyle: style}}}}})
	for _, tag := range []string{"title", "valAx", "legend", "dLbls", "dLbl"} {
		nodes := office.All(d.Root(), cns, tag)
		if len(nodes) == 0 {
			t.Fatalf("%s missing", tag)
		}
		for _, n := range nodes {
			if tag == "dLbls" && n.Parent().Tag != "ser" {
				continue
			}
			gradients := office.All(n, ans, "gradFill")
			if len(gradients) == 0 {
				t.Fatalf("%s gradient missing", tag)
			}
			for _, g := range gradients {
				if len(office.All(g, ans, "gs")) != 2 {
					t.Fatal("missing gradient stops")
				}
			}
		}
	}
	// Imported explicit run fills must be replaced, not left overriding defaults.
	doc := etree.NewDocument()
	if err := doc.ReadFromString(`<c:rich xmlns:c="` + cns + `" xmlns:a="` + ans + `"><a:bodyPr/><a:lstStyle/><a:p><a:r><a:rPr b="1"><a:solidFill><a:srgbClr val="00FF00"/></a:solidFill></a:rPr><a:t>Keep text</a:t></a:r></a:p></c:rich>`); err != nil {
		t.Fatal(err)
	}
	if err := formatChartTextBody(doc.Root(), style); err != nil {
		t.Fatal(err)
	}
	run := office.All(doc.Root(), ans, "rPr")[0]
	if office.Child(run, ans, "solidFill") != nil || office.Child(run, ans, "gradFill") == nil || run.SelectAttrValue("b", "") != "1" || office.All(doc.Root(), ans, "t")[0].Text() != "Keep text" {
		t.Fatal("run fill replacement changed content or left old fill")
	}
	if err := formatChartTextBody(doc.Root(), &ChartTextStyle{Color: "123456"}); err != nil {
		t.Fatal(err)
	}
	if office.Child(run, ans, "gradFill") != nil || office.Child(run, ans, "solidFill") == nil {
		t.Fatal("solid color did not replace gradient")
	}
	for _, bad := range []*ChartTextStyle{{Color: "FF0000", Fill: fill}, {Fill: &DrawingStyle{Gradient: &Gradient{Stops: []GradientStop{{Position: 0, Color: "bad"}}}}}} {
		f := ChartFormat{Title: &title, TitleStyle: bad}
		if _, err := EditFile(context.Background(), in, filepath.Join(t.TempDir(), "bad.pptx"), EditSpec{Edits: []Edit{{Slide: 1, ShapeID: "2", Operation: "chart-format", ChartFormat: &f}}}); err == nil {
			t.Fatal("invalid chart text paint accepted")
		}
	}
}
