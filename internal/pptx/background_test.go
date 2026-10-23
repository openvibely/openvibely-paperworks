package pptx

import (
	"context"
	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestBackgroundFills(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	f, _ := os.Create(filepath.Join(dir, "image.png"))
	if err := png.Encode(f, image.NewRGBA(image.Rect(0, 0, 100, 50))); err != nil {
		t.Fatal(err)
	}
	f.Close()
	styles := []DrawingStyle{{Fill: "17324D"}, {Gradient: &Gradient{Stops: []GradientStop{{Position: 0, Color: "FFFFFF"}, {Position: 1, Color: "000000"}}}}, {Pattern: &PatternFill{Preset: "diagCross", Foreground: "000000", Background: "FFFFFF"}}, {Image: &ImageFill{Path: filepath.Join(dir, "image.png"), Fit: "cover"}}}
	for i, tag := range []string{"solidFill", "gradFill", "pattFill", "blipFill"} {
		in := filepath.Join(dir, tag+".pptx")
		if err := Create(ctx, Spec{Slides: []Slide{{BackgroundFill: &styles[i], Elements: []Element{{Type: "text", X: 1, Y: 1, W: 4, H: 2, Text: "Keep"}}}}}, dir, in); err != nil {
			t.Fatal(err)
		}
		p, _ := opc.Open(in)
		d, _ := p.XML("ppt/slides/slide1.xml")
		bg := office.All(d.Root(), pns, "bgPr")[0]
		if office.Child(bg, ans, tag) == nil {
			t.Fatal("missing", tag)
		}
		if tag == "blipFill" {
			rels, _ := p.Rels("ppt/slides/slide1.xml")
			found := false
			for _, r := range rels {
				if r.Type == rns+"/image" {
					found = len(p.Get(opc.Resolve("ppt/slides/slide1.xml", r.Target))) > 0
				}
			}
			if !found {
				t.Fatal("image missing")
			}
		}
		out := filepath.Join(dir, tag+"-edited.pptx")
		if _, err := EditFile(ctx, in, out, EditSpec{Edits: []Edit{{Operation: "background", Slide: 1, BackgroundFill: &DrawingStyle{Fill: "FF0000"}}}}); err != nil {
			t.Fatal(err)
		}
		p, _ = opc.Open(out)
		d, _ = p.XML("ppt/slides/slide1.xml")
		bg = office.All(d.Root(), pns, "bgPr")[0]
		if office.Child(bg, ans, "solidFill") == nil || len(office.All(d.Root(), ans, "t")) != 1 {
			t.Fatal("background edit lost content")
		}
		if tag != "solidFill" && office.Child(bg, ans, tag) != nil {
			t.Fatal("old fill retained")
		}
	}
}
