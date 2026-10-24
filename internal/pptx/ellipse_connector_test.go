package pptx

import (
	"context"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
)

func TestEllipseConnectorSidesAndAutomaticEdits(t *testing.T) {
	// Cardinal indices from DrawingML ellipse/cxnLst (eight connection sites).
	for side, want := range map[string]int{"top": 0, "left": 2, "bottom": 4, "right": 6} {
		t.Run(side, func(t *testing.T) {
			dir := t.TempDir()
			in := filepath.Join(dir, "in.pptx")
			out := filepath.Join(dir, "out.pptx")
			elements := []Element{{Type: "shape", Shape: "ellipse", X: 1, Y: 1, W: 2, H: 1}, {Type: "shape", Shape: "ellipse", X: 6, Y: 1, W: 2, H: 1}, {Type: "connector", X: 3, Y: 1.5, W: 3, H: .1, Connector: &Connector{Kind: "straight", Start: &Connection{ShapeID: "2", Side: side}, End: &Connection{ShapeID: "3", Site: 7}}}}
			if err := Create(context.Background(), Spec{Slides: []Slide{{Elements: elements}}}, dir, in); err != nil {
				t.Fatal(err)
			}
			check := func(file string, start, end int) {
				t.Helper()
				p, err := opc.Open(file)
				if err != nil {
					t.Fatal(err)
				}
				d, err := p.XML("ppt/slides/slide1.xml")
				if err != nil {
					t.Fatal(err)
				}
				for tag, expected := range map[string]int{"stCxn": start, "endCxn": end} {
					r := office.All(d.Root(), ans, tag)
					if len(r) != 1 || r[0].SelectAttrValue("idx", "") != strconv.Itoa(expected) {
						t.Fatalf("%s expected site %d", tag, expected)
					}
				}
			}
			check(in, want, 7)
			if _, err := EditFile(context.Background(), in, out, EditSpec{Edits: []Edit{{Operation: "connector", Slide: 1, ShapeID: "4", Connector: &Connector{Start: &Connection{ShapeID: "2", Side: "auto"}, End: &Connection{ShapeID: "3", Side: "auto"}}}}}); err != nil {
				t.Fatal(err)
			}
			check(out, 6, 2)
			vertical := filepath.Join(dir, "vertical.pptx")
			x, y := 1.0, 4.0
			if _, err := EditFile(context.Background(), out, vertical, EditSpec{Edits: []Edit{{Operation: "geometry", Slide: 1, ShapeID: "3", X: &x, Y: &y}, {Operation: "connector", Slide: 1, ShapeID: "4", Connector: &Connector{Start: &Connection{ShapeID: "2", Side: "auto"}, End: &Connection{ShapeID: "3", Side: "auto"}}}}}); err != nil {
				t.Fatal(err)
			}
			check(vertical, 4, 0)
		})
	}
}
