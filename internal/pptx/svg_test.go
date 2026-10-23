package pptx

import (
	"bytes"
	"context"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"os"
	"path/filepath"
	"testing"
)

func TestSVGCreationAndReplacement(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	svg := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 200 100"><rect width="200" height="100" fill="blue"/></svg>`
	path := filepath.Join(dir, "art.svg")
	os.WriteFile(path, []byte(svg), 0600)
	for _, source := range []string{"path", "inline"} {
		e := Element{Type: "image", X: 1, Y: 1, W: 4, H: 4, Fit: "contain"}
		if source == "path" {
			e.Path = path
		} else {
			e.SVG = svg
		}
		in := filepath.Join(dir, source+".pptx")
		if err := Create(ctx, Spec{Slides: []Slide{{Elements: []Element{e}}}}, dir, in); err != nil {
			t.Fatal(err)
		}
		p, _ := opc.Open(in)
		if !bytes.Equal(p.Get("ppt/media/image1.svg"), []byte(svg)) || p.ContentType("ppt/media/image1.svg") != "image/svg+xml" {
			t.Fatal("SVG not embedded intact")
		}
		xml := string(p.Get("ppt/slides/slide1.xml"))
		if !bytes.Contains([]byte(xml), []byte(`cy="1828800"`)) {
			t.Fatal("SVG aspect ratio lost")
		}
		out := filepath.Join(dir, source+"-out.pptx")
		replacement := `<svg xmlns="http://www.w3.org/2000/svg" width="100" height="100"><circle cx="50" cy="50" r="40" fill="green"/></svg>`
		if _, err := EditFile(ctx, in, out, EditSpec{Edits: []Edit{{Operation: "replace-image", Slide: 1, ShapeID: "2", SVG: replacement}}}); err != nil {
			t.Fatal(err)
		}
		p, _ = opc.Open(out)
		rels, _ := p.Rels("ppt/slides/slide1.xml")
		found := false
		for _, r := range rels {
			if bytes.Equal(p.Get(opc.Resolve("ppt/slides/slide1.xml", r.Target)), []byte(replacement)) {
				found = true
			}
		}
		if !found {
			t.Fatal("replacement missing")
		}
	}
	for _, svg := range []string{`<svg/>`, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 0 10"/>`, `<svg xmlns="http://www.w3.org/2000/svg" width="NaN" height="10"/>`} {
		if _, _, _, err := loadSlideImage("", svg); err == nil {
			t.Fatal("invalid SVG accepted")
		}
	}
}
