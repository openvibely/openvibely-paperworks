package pptx

import (
	"context"
	"github.com/beevik/etree"
	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"path/filepath"
	"reflect"
	"testing"
)

func TestOnePositionStacking(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	in := filepath.Join(dir, "in.pptx")
	es := []Element{{Type: "shape", Shape: "rect", X: 1, Y: 1, W: 2, H: 2, Text: "One"}, {Type: "text", X: 1, Y: 1, W: 2, H: 2, Text: "Two"}, {Type: "shape", Shape: "ellipse", X: 1, Y: 1, W: 2, H: 2, Text: "Three"}}
	if err := Create(ctx, Spec{Slides: []Slide{{Elements: es}}}, dir, in); err != nil {
		t.Fatal(err)
	}
	p, _ := opc.Open(in)
	d, _ := p.XML("ppt/slides/slide1.xml")
	tree := office.All(d.Root(), pns, "spTree")[0]
	tree.AddChild(etree.NewElement("p:extLst"))
	p.SetXML("ppt/slides/slide1.xml", d)
	in = filepath.Join(dir, "ext.pptx")
	if err := p.SaveNew(in); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		op, id string
		want   []string
	}{{"forward", "2", []string{"3", "2", "4"}}, {"backward", "4", []string{"2", "4", "3"}}, {"forward", "4", []string{"2", "3", "4"}}, {"backward", "2", []string{"2", "3", "4"}}} {
		out := filepath.Join(dir, tc.op+tc.id+".pptx")
		if _, err := EditFile(ctx, in, out, EditSpec{Edits: []Edit{{Operation: tc.op, Slide: 1, ShapeID: tc.id}}}); err != nil {
			t.Fatal(err)
		}
		p, _ = opc.Open(out)
		d, _ = p.XML("ppt/slides/slide1.xml")
		tree = office.All(d.Root(), pns, "spTree")[0]
		var ids []string
		for _, c := range tree.ChildElements() {
			if c.Tag == "sp" {
				ids = append(ids, office.All(c, pns, "cNvPr")[0].SelectAttrValue("id", ""))
			}
		}
		if !reflect.DeepEqual(ids, tc.want) {
			t.Fatalf("%s: %v", tc.op, ids)
		}
		children := tree.ChildElements()
		if children[0].Tag != "nvGrpSpPr" || children[1].Tag != "grpSpPr" || children[len(children)-1].Tag != "extLst" {
			t.Fatal("non-drawing properties reordered")
		}
	}
}
