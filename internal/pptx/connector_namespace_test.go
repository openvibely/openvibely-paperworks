package pptx

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"

	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
)

func TestConnectorEditWithImportedDrawingNamespace(t *testing.T) {
	for _, conflict := range []bool{false, true} {
		dir := t.TempDir()
		seed, in, out := filepath.Join(dir, "seed.pptx"), filepath.Join(dir, "in.pptx"), filepath.Join(dir, "out.pptx")
		els := []Element{{Type: "shape", Shape: "ellipse", X: 1, Y: 1, W: 2, H: 1}, {Type: "shape", Shape: "rect", X: 6, Y: 1, W: 2, H: 1}, {Type: "connector", X: 3, Y: 1.5, W: 3, H: .1, Connector: &Connector{Kind: "straight", Start: &Connection{ShapeID: "2", Site: 6}, End: &Connection{ShapeID: "3", Site: 1}}}}
		if err := Create(context.Background(), Spec{Slides: []Slide{{Elements: els}}}, dir, seed); err != nil {
			t.Fatal(err)
		}
		p, err := opc.Open(seed)
		if err != nil {
			t.Fatal(err)
		}
		part := "ppt/slides/slide1.xml"
		raw := p.Get(part)
		raw = bytes.ReplaceAll(raw, []byte("xmlns:a="), []byte("xmlns:aa="))
		raw = bytes.ReplaceAll(raw, []byte("<a:"), []byte("<aa:"))
		raw = bytes.ReplaceAll(raw, []byte("</a:"), []byte("</aa:"))
		if conflict {
			raw = bytes.Replace(raw, []byte("<p:sld "), []byte(`<p:sld xmlns:a="urn:other" `), 1)
		}
		p.Set(part, raw)
		if err := p.SaveNew(in); err != nil {
			t.Fatal(err)
		}
		_, err = EditFile(context.Background(), in, out, EditSpec{Edits: []Edit{{Operation: "connector", Slide: 1, ShapeID: "4", Connector: &Connector{Kind: "elbow4", Start: &Connection{ShapeID: "2", Side: "right"}, End: &Connection{ShapeID: "3", Side: "left"}}}}})
		if err != nil {
			t.Fatal(err)
		}
		p, err = opc.Open(out)
		if err != nil {
			t.Fatal(err)
		}
		d, err := p.XML(part)
		if err != nil {
			t.Fatal(err)
		}
		for tag, want := range map[string]string{"stCxn": "6", "endCxn": "1"} {
			nodes := office.All(d.Root(), ans, tag)
			if len(nodes) != 1 || nodes[0].SelectAttrValue("idx", "") != want {
				t.Fatalf("incorrect namespace/attachment for %s (conflict=%v)", tag, conflict)
			}
		}
		connectors := office.All(d.Root(), pns, "cxnSp")
		geom := office.Child(office.Child(connectors[0], pns, "spPr"), ans, "prstGeom")
		if geom.SelectAttrValue("prst", "") != "bentConnector4" || office.Child(geom, ans, "avLst") == nil {
			t.Fatal("kind edit lost geometry namespace")
		}
	}
}
