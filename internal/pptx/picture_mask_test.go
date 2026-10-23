package pptx

import (
	"context"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/beevik/etree"
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

func TestAdditionalPictureMasks(t *testing.T) {
	in := pictureMaskFixture(t)
	for _, mask := range []string{"hexagon", "triangle", "diamond", "star5", "heart", "rightArrow"} {
		t.Run(mask, func(t *testing.T) {
			out := filepath.Join(t.TempDir(), "out.pptx")
			if _, err := EditFile(context.Background(), in, out, EditSpec{Edits: []Edit{{Operation: "image-mask", Slide: 1, ShapeID: "2", Mask: mask}}}); err != nil {
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
			if geom.SelectAttrValue("prst", "") != mask {
				t.Fatal("mask missing")
			}
			if _, err = office.VerifyPreservation(in, out, []string{"ppt/slides/slide1.xml"}); err != nil {
				t.Fatal(err)
			}
		})
	}
	for _, mask := range []string{"bogus", "custom", "textbox"} {
		if _, err := EditFile(context.Background(), in, filepath.Join(t.TempDir(), "bad.pptx"), EditSpec{Edits: []Edit{{Operation: "image-mask", Slide: 1, ShapeID: "2", Mask: mask}}}); err == nil {
			t.Fatalf("invalid mask %s accepted", mask)
		}
	}
}

// Exercise saved-file edits with namespace declarations where real exporters
// put them: on the geometry itself or an ancestor, including default namespaces.
func TestPictureMaskImportedAdjustmentNamespaces(t *testing.T) {
	for _, scope := range []string{"geometry-prefix", "ancestor-prefix", "default"} {
		for _, update := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/update=%t", scope, update), func(t *testing.T) {
				base := pictureMaskFixture(t)
				p, err := opc.Open(base)
				if err != nil {
					t.Fatal(err)
				}
				d, err := p.XML("ppt/slides/slide1.xml")
				if err != nil {
					t.Fatal(err)
				}
				pic := office.All(d.Root(), pns, "pic")[0]
				pr := office.Child(pic, pns, "spPr")
				for _, tag := range []string{"prstGeom", "custGeom"} {
					if old := office.Child(pr, ans, tag); old != nil {
						pr.RemoveChild(old)
					}
				}
				geometry := etree.NewDocument()
				xml := `<d:prstGeom xmlns:d="` + ans + `" prst="roundRect"><d:avLst><d:gd name="adj" fmla="val 20000"/></d:avLst></d:prstGeom>`
				if scope == "default" {
					xml = `<prstGeom xmlns="` + ans + `" prst="roundRect"><avLst><gd name="adj" fmla="val 20000"/></avLst></prstGeom>`
				}
				if err = geometry.ReadFromString(xml); err != nil {
					t.Fatal(err)
				}
				g := geometry.Root().Copy()
				if scope == "ancestor-prefix" {
					g.RemoveAttr("xmlns:d")
					pr.CreateAttr("xmlns:d", ans)
				}
				insertDrawingProperty(pr, g)
				if err = p.SetXML("ppt/slides/slide1.xml", d); err != nil {
					t.Fatal(err)
				}
				data, err := p.Bytes()
				if err != nil {
					t.Fatal(err)
				}
				dir := t.TempDir()
				in, out := filepath.Join(dir, "imported.pptx"), filepath.Join(dir, "edited.pptx")
				if err = os.WriteFile(in, data, 0600); err != nil {
					t.Fatal(err)
				}
				edit := Edit{Operation: "image-mask", Slide: 1, ShapeID: "2", Mask: "roundRect"}
				want := "val 20000"
				radius := 14.4
				if update {
					edit.CornerRadius = &radius
					want = "val 10000"
				}
				if _, err = EditFile(context.Background(), in, out, EditSpec{Edits: []Edit{edit, edit}}); err != nil {
					t.Fatal(err)
				}
				q, err := opc.Open(out)
				if err != nil {
					t.Fatal(err)
				}
				saved, err := q.XML("ppt/slides/slide1.xml")
				if err != nil {
					t.Fatal(err)
				}
				result := office.Child(office.Child(office.All(saved.Root(), pns, "pic")[0], pns, "spPr"), ans, "prstGeom")
				if result == nil {
					t.Fatal("geometry lost namespace")
				}
				if len(result.ChildElements()) != 1 || len(office.Children(result, ans, "avLst")) != 1 {
					t.Fatal("duplicate or unbound adjustment list")
				}
				av := office.Child(result, ans, "avLst")
				if len(av.ChildElements()) != 1 || len(office.Children(av, ans, "gd")) != 1 {
					t.Fatal("duplicate or unbound adjustment")
				}
				if office.Child(av, ans, "gd").SelectAttrValue("fmla", "") != want {
					t.Fatal("incorrect radius")
				}
				if _, err = office.VerifyPreservation(in, out, []string{"ppt/slides/slide1.xml"}); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}
