package pptx

import (
	"context"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
)

func pictureMaskFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "image.png")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = png.Encode(f, image.NewRGBA(image.Rect(0, 0, 200, 100))); err != nil {
		t.Fatal(err)
	}
	f.Close()
	in := filepath.Join(dir, "in.pptx")
	if err = Create(context.Background(), Spec{Slides: []Slide{{Elements: []Element{{Type: "image", Path: path, X: 1, Y: 1, W: 4, H: 2}}}}}, dir, in); err != nil {
		t.Fatal(err)
	}
	return in
}

func TestPictureCornerRadius(t *testing.T) {
	in := pictureMaskFixture(t)
	out := filepath.Join(t.TempDir(), "out.pptx")
	radius := 28.8
	if _, err := EditFile(context.Background(), in, out, EditSpec{Edits: []Edit{{Operation: "image-mask", Slide: 1, ShapeID: "2", Mask: "roundRect", CornerRadius: &radius}, {Operation: "image-mask", Slide: 1, ShapeID: "2", Mask: "roundRect"}}}); err != nil {
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
	pic := office.All(d.Root(), pns, "pic")[0]
	geom := office.Child(office.Child(pic, pns, "spPr"), ans, "prstGeom")
	if geom.SelectAttrValue("prst", "") != "roundRect" || office.All(geom, ans, "gd")[0].SelectAttrValue("fmla", "") != "val 20000" {
		t.Fatal("radius lost")
	}
	if _, err = office.VerifyPreservation(in, out, []string{"ppt/slides/slide1.xml"}); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []float64{-1, 73} {
		if _, err := EditFile(context.Background(), in, filepath.Join(t.TempDir(), "bad.pptx"), EditSpec{Edits: []Edit{{Operation: "image-mask", Slide: 1, ShapeID: "2", Mask: "roundRect", CornerRadius: &bad}}}); err == nil {
			t.Fatal("invalid radius accepted")
		}
	}
	zero := 0.0
	if _, err := EditFile(context.Background(), out, filepath.Join(t.TempDir(), "zero.pptx"), EditSpec{Edits: []Edit{{Operation: "image-mask", Slide: 1, ShapeID: "2", CornerRadius: &zero}}}); err != nil {
		t.Fatal(err)
	}
}
