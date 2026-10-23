package pptx

import (
	"context"
	"github.com/beevik/etree"
	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"path/filepath"
	"testing"
)

func lineFixture(t *testing.T, outline *Outline) string {
	t.Helper()
	out := filepath.Join(t.TempDir(), "line.pptx")
	if err := Create(context.Background(), Spec{Slides: []Slide{{Elements: []Element{{Type: "shape", W: 1, H: 1}, {Type: "connector", X: 1, W: 2, H: 1, Connector: &Connector{Kind: "straight", Start: &Connection{ShapeID: "2", Site: 3}}, DrawingStyle: &DrawingStyle{Outline: outline}}}}}}, "", out); err != nil {
		t.Fatal(err)
	}
	return out
}
func savedLine(t *testing.T, path string) *etree.Element {
	t.Helper()
	p, err := opc.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	d, err := p.XML("ppt/slides/slide1.xml")
	if err != nil {
		t.Fatal(err)
	}
	c := office.All(d.Root(), pns, "cxnSp")[0]
	if office.All(c, ans, "stCxn")[0].SelectAttrValue("id", "") != "2" {
		t.Fatal("attachment lost")
	}
	return office.Child(office.Child(c, pns, "spPr"), ans, "ln")
}
func TestArrowheadSizes(t *testing.T) {
	in := lineFixture(t, &Outline{Color: "000000", Width: 2, Head: "arrow", HeadWidth: "lg", HeadLength: "sm", Tail: "triangle", TailWidth: "sm", TailLength: "lg"})
	ln := savedLine(t, in)
	if office.Child(ln, ans, "headEnd").SelectAttrValue("w", "") != "lg" || office.Child(ln, ans, "tailEnd").SelectAttrValue("len", "") != "lg" {
		t.Fatal("sizes missing")
	}
	out := filepath.Join(t.TempDir(), "edit.pptx")
	spec := EditSpec{Edits: []Edit{{Operation: "drawing-style", Slide: 1, ShapeID: "3", DrawingStyle: &DrawingStyle{Outline: &Outline{Color: "000000", Width: 2, Tail: "arrow", TailWidth: "med", TailLength: "sm"}}}}}
	if _, err := EditFile(context.Background(), in, out, spec); err != nil {
		t.Fatal(err)
	}
	ln = savedLine(t, out)
	if office.Child(ln, ans, "tailEnd").SelectAttrValue("w", "") != "med" {
		t.Fatal("edit lost")
	}
	if _, err := office.VerifyPreservation(in, out, []string{"ppt/slides/slide1.xml"}); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []Outline{{Color: "000000", Width: 2, HeadWidth: "sm"}, {Color: "000000", Width: 2, Head: "arrow", HeadLength: "invalid"}} {
		spec.Edits[0].DrawingStyle.Outline = &bad
		if _, err := EditFile(context.Background(), in, filepath.Join(t.TempDir(), "bad.pptx"), spec); err == nil {
			t.Fatal("bad size accepted")
		}
	}
}
