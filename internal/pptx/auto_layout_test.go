package pptx

import (
	"context"
	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"math"
	"path/filepath"
	"testing"
)

func TestAutoLayoutAlignmentAndGaps(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	in := filepath.Join(dir, "in.pptx")
	if err := Create(ctx, Spec{Slides: []Slide{{Elements: []Element{{Type: "text", Text: "One", X: 0, Y: 0, W: 2, H: 1}, {Type: "text", Text: "Two", X: 0, Y: 0, W: 1, H: 2}}}}}, dir, in); err != nil {
		t.Fatal(err)
	}
	for align, f := range map[string][2]float64{"center": {.5, .5}, "topLeft": {0, 0}, "topCenter": {.5, 0}, "topRight": {1, 0}, "left": {0, .5}, "right": {1, .5}, "bottomLeft": {0, 1}, "bottomCenter": {.5, 1}, "bottomRight": {1, 1}} {
		for _, direction := range []string{"horizontal", "vertical"} {
			gap := .5
			l := &AutoLayout{ShapeIDs: []string{"2", "3"}, Direction: direction, Align: align, X: 1, Y: 1, Width: 8, Height: 6, Gap: &gap}
			out := filepath.Join(dir, align+direction+".pptx")
			if _, err := EditFile(ctx, in, out, EditSpec{Edits: []Edit{{Operation: "auto-layout", Slide: 1, AutoLayout: l}}}); err != nil {
				t.Fatal(err)
			}
			p, _ := opc.Open(out)
			d, _ := p.XML("ppt/slides/slide1.xml")
			shapes := office.All(d.Root(), pns, "sp")
			for i, shape := range shapes {
				r, err := explicitDrawingRect(shape)
				if err != nil {
					t.Fatal(err)
				}
				w, h := 2.0, 1.0
				if i == 1 {
					w, h = 1, 2
				}
				x, y := 1+(8-3.5)*f[0], 1+(6-h)*f[1]
				if i == 1 {
					x += 2.5
				}
				if direction == "vertical" {
					x = 1 + (8-w)*f[0]
					y = 1 + (6-3.5)*f[1]
					if i == 1 {
						y += 1.5
					}
				}
				if math.Abs(r.x-x) > 1e-5 || math.Abs(r.y-y) > 1e-5 || r.w != w || r.h != h {
					t.Fatalf("%s %s: %#v want %v,%v %v,%v", align, direction, r, x, y, w, h)
				}
			}
		}
	}
	l := &AutoLayout{ShapeIDs: []string{"2", "3"}, Width: 8, Height: 4, AutoGap: true, PaddingX: 1, Align: "topLeft"}
	out := filepath.Join(dir, "auto.pptx")
	if _, err := EditFile(ctx, in, out, EditSpec{Edits: []Edit{{Operation: "auto-layout", Slide: 1, AutoLayout: l}}}); err != nil {
		t.Fatal(err)
	}
	p, _ := opc.Open(out)
	d, _ := p.XML("ppt/slides/slide1.xml")
	r, _ := explicitDrawingRect(office.All(d.Root(), pns, "sp")[1])
	if r.x != 6 {
		t.Fatal("wrong auto gap", r)
	}
	l.Width = 2
	if _, err := EditFile(ctx, in, filepath.Join(dir, "bad.pptx"), EditSpec{Edits: []Edit{{Operation: "auto-layout", Slide: 1, AutoLayout: l}}}); err == nil {
		t.Fatal("overflow accepted")
	}
}
