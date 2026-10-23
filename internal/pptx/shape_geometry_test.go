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
