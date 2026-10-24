package pptx

import (
	"context"
	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"os"
	"path/filepath"
	"testing"
)

func TestConnectorRoutingAndEndpointEdits(t *testing.T) {
	for _, kind := range []string{"elbow2", "elbow3", "elbow4", "elbow5"} {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			in, out := filepath.Join(dir, "in.pptx"), filepath.Join(dir, "out.pptx")
			elements := []Element{{Type: "shape", Shape: "rect", X: 1, Y: 1, W: 2, H: 1}, {Type: "shape", Shape: "rect", X: 6, Y: 1, W: 2, H: 1}, {Type: "connector", X: 3, Y: 1.5, W: 3, H: .1, Connector: &Connector{Kind: kind, Start: &Connection{ShapeID: "2", Side: "auto"}, End: &Connection{ShapeID: "3", Side: "auto"}}}}
			if err := Create(context.Background(), Spec{Slides: []Slide{{Elements: elements}}}, dir, in); err != nil {
				t.Fatal(err)
			}
			p, err := opc.Open(in)
			if err != nil {
				t.Fatal(err)
			}
			d, _ := p.XML("ppt/slides/slide1.xml")
			refs := office.All(d.Root(), ans, "stCxn")
			if len(refs) != 1 || refs[0].SelectAttrValue("idx", "") != "3" {
				t.Fatal("missing right-side attachment")
			}
			ends := office.All(d.Root(), ans, "endCxn")
			if len(ends) != 1 || ends[0].SelectAttrValue("idx", "") != "1" {
				t.Fatal("missing left-side attachment")
			}
			if err := func() error {
				_, err := EditFile(context.Background(), in, out, EditSpec{Edits: []Edit{{Operation: "connector", Slide: 1, ShapeID: "4", Connector: &Connector{Kind: "straight", Start: &Connection{ShapeID: "3", Side: "bottom"}, End: &Connection{ShapeID: "2", Side: "top"}}}}})
				return err
			}(); err != nil {
				t.Fatal(err)
			}
			p, _ = opc.Open(out)
			d, _ = p.XML("ppt/slides/slide1.xml")
			if office.All(d.Root(), ans, "stCxn")[0].SelectAttrValue("id", "") != "3" || office.All(d.Root(), ans, "endCxn")[0].SelectAttrValue("idx", "") != "0" {
				t.Fatal("endpoints not updated")
			}
			if len(office.All(d.Root(), pns, "sp")) != 2 {
				t.Fatal("endpoint edit changed shapes")
			}
			bad := filepath.Join(dir, "bad.pptx")
			_, err = EditFile(context.Background(), in, bad, EditSpec{Edits: []Edit{{Operation: "connector", Slide: 1, ShapeID: "4", Connector: &Connector{Start: &Connection{ShapeID: "99"}}}}})
			if err == nil {
				t.Fatal("accepted missing endpoint")
			}
			if _, err = os.Stat(bad); !os.IsNotExist(err) {
				t.Fatal("published failed edit")
			}
		})
	}
}
