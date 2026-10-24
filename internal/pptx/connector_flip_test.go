package pptx

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
)

func TestConnectorSideSelectionChecksXMLBooleanFlips(t *testing.T) {
	for _, attr := range []string{"flipH", "flipV"} {
		for _, value := range []string{"1", "true", " true ", "0", "false", "invalid"} {
			for _, side := range []string{"right", "auto"} {
				t.Run(attr+"/"+value+"/"+side, func(t *testing.T) {
					dir := t.TempDir()
					seed, in, out := filepath.Join(dir, "seed.pptx"), filepath.Join(dir, "in.pptx"), filepath.Join(dir, "out.pptx")
					els := []Element{{Type: "shape", Shape: "ellipse", X: 1, Y: 1, W: 2, H: 1}, {Type: "shape", Shape: "rect", X: 6, Y: 1, W: 2, H: 1}, {Type: "connector", X: 3, Y: 1.5, W: 3, H: .1, Connector: &Connector{Kind: "straight", Start: &Connection{ShapeID: "2", Site: 6}, End: &Connection{ShapeID: "3", Site: 1}}}}
					if err := Create(context.Background(), Spec{Slides: []Slide{{Elements: els}}}, dir, seed); err != nil {
						t.Fatal(err)
					}
					p, _ := opc.Open(seed)
					part := "ppt/slides/slide1.xml"
					d, _ := p.XML(part)
					sp := office.All(d.Root(), pns, "sp")[0]
					xf := office.Child(office.Child(sp, pns, "spPr"), ans, "xfrm")
					xf.CreateAttr(attr, value)
					if err := p.SetXML(part, d); err != nil {
						t.Fatal(err)
					}
					if err := p.SaveNew(in); err != nil {
						t.Fatal(err)
					}
					_, err := EditFile(context.Background(), in, out, EditSpec{Edits: []Edit{{Operation: "connector", Slide: 1, ShapeID: "4", Connector: &Connector{Start: &Connection{ShapeID: "2", Side: side}}}}})
					if value == "0" || value == "false" {
						if err != nil {
							t.Fatal(err)
						}
					} else {
						if err == nil {
							t.Fatal("accepted flipped or invalid endpoint")
						}
						if _, err := os.Stat(out); !os.IsNotExist(err) {
							t.Fatal("published rejected edit")
						}
					}
					// Explicit sites remain available for transformed shapes.
					explicit := filepath.Join(dir, "explicit.pptx")
					if _, err := EditFile(context.Background(), in, explicit, EditSpec{Edits: []Edit{{Operation: "connector", Slide: 1, ShapeID: "4", Connector: &Connector{Start: &Connection{ShapeID: "2", Site: 2}}}}}); err != nil {
						t.Fatal(err)
					}
				})
			}
		}
	}
}
