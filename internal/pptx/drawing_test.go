package pptx

import (
	"context"
	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"path/filepath"
	"testing"
)

func TestNativePathsStylesAndAttachedConnectors(t *testing.T) {
	dir := t.TempDir()
	in, out := filepath.Join(dir, "in.pptx"), filepath.Join(dir, "out.pptx")
	ctx := context.Background()
	opacity := 0.5
	elements := []Element{{Type: "shape", Shape: "hexagon", X: 1, Y: 1, W: 2, H: 1, DrawingStyle: &DrawingStyle{Gradient: &Gradient{Angle: 90, Stops: []GradientStop{{Position: 0, Color: "0000FF"}, {Position: 1, Color: "FFFFFF"}}}, Outline: &Outline{Color: "000000", Width: 2, Dash: "dash"}, Shadow: &Shadow{Color: "000000", Opacity: &opacity, Blur: 3, Distance: 2, Direction: 45}}}, {Type: "path", X: 5, Y: 2, W: 2, H: 2, Fill: "FF8800", Commands: []PathCommand{{Operation: "move", Points: [][2]float64{{0, 100000}}}, {Operation: "cubic", Points: [][2]float64{{0, 0}, {100000, 0}, {100000, 100000}}}, {Operation: "close"}}}, {Type: "connector", X: 2, Y: 2, W: 3, H: 1, Connector: &Connector{Kind: "elbow", Start: &Connection{ShapeID: "2", Site: 1}, End: &Connection{ShapeID: "3", Site: 0}}}}
	if err := Create(ctx, Spec{Slides: []Slide{{Elements: elements}}}, dir, in); err != nil {
		t.Fatal(err)
	}
	p, _ := opc.Open(in)
	d, _ := p.XML("ppt/slides/slide1.xml")
	if len(office.All(d.Root(), ans, "cubicBezTo")) != 1 || len(office.All(d.Root(), ans, "gradFill")) != 1 || len(office.All(d.Root(), ans, "outerShdw")) != 1 {
		t.Fatal("native drawing properties missing")
	}
	if end := d.FindElement("//p:cxnSp/p:nvCxnSpPr/p:cNvCxnSpPr/a:endCxn"); end == nil || end.SelectAttrValue("id", "") != "3" {
		t.Fatal("connector end missing", end)
	}
	connector := Element{Type: "connector", X: 1, Y: 1, W: 4, H: 2, Connector: &Connector{Kind: "straight", Start: &Connection{ShapeID: "2", Site: 0}, End: &Connection{ShapeID: "3", Site: 0}}}
	replacement := Element{Type: "shape", Shape: "diamond", X: 5, Y: 2, W: 2, H: 2, Fill: "00FF00"}
	if err := DeckEditFile(ctx, in, out, dir, DeckEditSpec{Edits: []DeckEdit{{Operation: "replace-element", Slide: 1, ShapeID: "3", Element: &replacement}, {Operation: "add-element", Slide: 1, Element: &connector}}}); err != nil {
		t.Fatal(err)
	}
	p, _ = opc.Open(out)
	d, _ = p.XML("ppt/slides/slide1.xml")
	ids := map[string]bool{}
	for _, pr := range office.All(d.Root(), pns, "cNvPr") {
		id := pr.SelectAttrValue("id", "")
		if ids[id] {
			t.Fatal("duplicate ID", id)
		}
		ids[id] = true
	}
	for _, tag := range []string{"stCxn", "endCxn"} {
		for _, c := range office.All(d.Root(), ans, tag) {
			if !ids[c.SelectAttrValue("id", "")] {
				t.Fatal("dangling connector", c)
			}
		}
	}
	if !ids["3"] {
		t.Fatal("replacement changed stable shape ID")
	}
}
func TestBubbleChartHasSizesAndEditableWorkbook(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "bubble.pptx")
	chart := &Chart{Type: "bubble", Categories: []string{"1", "2"}, Series: []Series{{Name: "Series", Values: []float64{2, 4}, Sizes: []float64{3, 5}}}}
	element := Element{Type: "chart", X: 1, Y: 1, W: 6, H: 4, Chart: chart}
	err := Create(context.Background(), Spec{Slides: []Slide{{Elements: []Element{element}}}}, dir, out)
	if err != nil {
		t.Fatal(err)
	}
	p, _ := opc.Open(out)
	d, err := p.XML("ppt/charts/chart1.xml")
	if err != nil {
		t.Fatal(err)
	}
	if len(office.All(d.Root(), cns, "bubbleSize")) != 1 {
		t.Fatal("missing bubble sizes")
	}
	rels, err := p.Rels("ppt/charts/chart1.xml")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, r := range rels {
		if r.Type == rns+"/package" {
			found = p.Has(opc.Resolve("ppt/charts/chart1.xml", r.Target))
		}
	}
	if !found {
		t.Fatal("missing editable workbook")
	}
}
