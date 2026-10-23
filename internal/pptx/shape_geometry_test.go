package pptx

import (
	"context"
	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"path/filepath"
	"testing"
)

func TestShapeGeometryRoundTrip(t *testing.T) {
	ctx := context.Background()
	in := filepath.Join(t.TempDir(), "in.pptx")
	out := filepath.Join(t.TempDir(), "out.pptx")
	radius := 18.0
	if err := Create(ctx, Spec{Slides: []Slide{{Elements: []Element{{Type: "shape", Shape: "rect", W: 4, H: 2, Text: "Keep me", ShapeGeometry: &ShapeGeometry{CornerRadius: &radius}}}}}}, "", in); err != nil {
		t.Fatal(err)
	}
	p, _ := opc.Open(in)
	d, _ := p.XML("ppt/slides/slide1.xml")
	g := office.All(d.Root(), ans, "gd")[0]
	if g.SelectAttrValue("fmla", "") != "val 12500" {
		t.Fatal("radius not saved")
	}
	edits := EditSpec{Edits: []Edit{{Slide: 1, ShapeID: "2", Operation: "shape-geometry", ShapeGeometry: &ShapeGeometry{Adjustments: []ShapeAdjustment{{Name: "adj", Formula: "val 20000"}}}}}}
	if _, err := EditFile(ctx, in, out, edits); err != nil {
		t.Fatal(err)
	}
	q, _ := opc.Open(out)
	d, _ = q.XML("ppt/slides/slide1.xml")
	if office.All(d.Root(), ans, "gd")[0].SelectAttrValue("fmla", "") != "val 20000" || office.All(d.Root(), ans, "t")[0].Text() != "Keep me" {
		t.Fatal("geometry or text lost")
	}
	if _, err := office.VerifyPreservation(in, out, []string{"ppt/slides/slide1.xml"}); err != nil {
		t.Fatal(err)
	}
	edits.Edits[0].ShapeGeometry.Adjustments[0].Formula = "invalid"
	if _, err := EditFile(ctx, in, filepath.Join(t.TempDir(), "bad.pptx"), edits); err == nil {
		t.Fatal("invalid formula accepted")
	}
}

func TestShapeRadiusUsesFinalLayoutDimensions(t *testing.T) {
	for _, tc := range []struct {
		name                               string
		initialHeight, finalHeight, radius float64
		wantFormula                        string
		wantError                          bool
	}{
		{"shrink retains radius", 2, 1, 18, "val 25000", false},
		{"grow validates final dimensions", 0.25, 1, 18, "val 25000", false},
		{"shrink rejects oversized radius", 2, 0.25, 18, "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out := filepath.Join(t.TempDir(), "out.pptx")
			err := Create(context.Background(), Spec{Slides: []Slide{{
				Elements: []Element{{Type: "shape", Shape: "rect", W: 4, H: tc.initialHeight, Text: "Keep me", ShapeGeometry: &ShapeGeometry{CornerRadius: &tc.radius}}},
				Layout:   &Layout{ShapeIDs: []string{"2"}, Mode: "row", Width: 4, Height: tc.finalHeight},
			}}}, "", out)
			if tc.wantError {
				if err == nil {
					t.Fatal("accepted radius exceeding final dimensions")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			p, err := opc.Open(out)
			if err != nil {
				t.Fatal(err)
			}
			d, err := p.XML("ppt/slides/slide1.xml")
			if err != nil {
				t.Fatal(err)
			}
			shape := office.All(d.Root(), pns, "sp")[0]
			geom := office.Child(office.Child(shape, pns, "spPr"), ans, "prstGeom")
			if geom.SelectAttrValue("prst", "") != "roundRect" || office.All(geom, ans, "gd")[0].SelectAttrValue("fmla", "") != tc.wantFormula {
				t.Fatal("radius not based on final layout")
			}
			if office.All(shape, ans, "t")[0].Text() != "Keep me" {
				t.Fatal("text changed")
			}
		})
	}
}
