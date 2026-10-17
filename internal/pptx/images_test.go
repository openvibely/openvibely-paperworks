package pptx

import (
	"bytes"
	"context"
	"image"
	imgcolor "image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openvibely/openvibely-paperworks/internal/convert"
	"github.com/openvibely/openvibely-paperworks/internal/lo"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"github.com/openvibely/openvibely-paperworks/internal/pdf"
)

func cropDeck(t *testing.T) (string, []byte) {
	t.Helper()
	dir := t.TempDir()
	im := image.NewRGBA(image.Rect(0, 0, 400, 200))
	for y := 0; y < 200; y++ {
		for x := 0; x < 400; x++ {
			c := imgcolor.RGBA{0, 255, 0, 255}
			if x < 100 {
				c = imgcolor.RGBA{255, 0, 0, 255}
			}
			if x >= 300 {
				c = imgcolor.RGBA{0, 0, 255, 255}
			}
			im.SetRGBA(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, im); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(dir, "stripes.png")
	if err := os.WriteFile(source, buf.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	s := Spec{Width: 4, Height: 4}
	for _, e := range []Element{
		{Type: "image", Path: "stripes.png", X: 1, Y: 1, W: 2, H: 2},
		{Type: "image", Path: "stripes.png", X: 1, Y: 1, W: 2, H: 2, Fit: "cover"},
		{Type: "image", Path: "stripes.png", X: 1, Y: 1, W: 2, H: 2, Crop: &Crop{Left: .25, Right: .25}},
		{Type: "image", Path: "stripes.png", X: 1, Y: 1, W: 2, H: 2, Fit: "cover", Anchor: &ImageAnchor{X: 0, Y: 0}},
		{Type: "image", Path: "stripes.png", X: 1, Y: 1, W: 2, H: 2, Fit: "cover", Anchor: &ImageAnchor{X: 1, Y: 1}},
	} {
		s.Slides = append(s.Slides, Slide{Elements: []Element{e}})
	}
	out := filepath.Join(dir, "images.pptx")
	if err := Create(context.Background(), s, dir, out); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(source)
	if err != nil || !bytes.Equal(after, buf.Bytes()) {
		t.Fatal("source changed", err)
	}
	return out, buf.Bytes()
}
func TestNativePictureCropAndPreservation(t *testing.T) {
	in, source := cropDeck(t)
	p, err := opc.Open(in)
	if err != nil {
		t.Fatal(err)
	}
	for _, part := range p.Parts {
		if strings.HasPrefix(part.Name, "ppt/media/") && !bytes.Equal(part.Data, source) {
			t.Fatal("embedded image was resampled")
		}
	}
	for i, name := range []string{"ppt/slides/slide1.xml", "ppt/slides/slide2.xml", "ppt/slides/slide3.xml", "ppt/slides/slide4.xml", "ppt/slides/slide5.xml"} {
		d, err := p.XML(name)
		if err != nil {
			t.Fatal(err)
		}
		s, _ := d.WriteToString()
		if i == 0 {
			if strings.Contains(s, "srcRect") || !strings.Contains(s, `cy="914400"`) {
				t.Fatal("contain geometry", s)
			}
		} else if i >= 3 {
			want := `l="0" t="0" r="50000" b="0"`
			if i == 4 {
				want = `l="50000" t="0" r="0" b="0"`
			}
			if !strings.Contains(s, want) {
				t.Fatal("anchor geometry", s)
			}
		} else if !strings.Contains(s, `l="25000" t="0" r="25000" b="0"`) {
			t.Fatal("crop geometry", s)
		}
	}
	out := filepath.Join(t.TempDir(), "edited.pptx")
	if _, err := EditFile(context.Background(), in, out, EditSpec{Edits: []Edit{{Slide: 2, Operation: "notes", Text: "Crop stays editable"}}}); err != nil {
		t.Fatal(err)
	}
	q, err := opc.Open(out)
	if err != nil {
		t.Fatal(err)
	}
	for _, part := range p.Parts {
		if strings.HasPrefix(part.Name, "ppt/media/") || strings.HasPrefix(part.Name, "ppt/slides/") {
			found := false
			for _, other := range q.Parts {
				if part.Name == other.Name {
					found = true
					if !bytes.Equal(part.Data, other.Data) {
						t.Fatal("unrelated part changed", part.Name)
					}
				}
			}
			if !found {
				t.Fatal("part removed", part.Name)
			}
		}
	}
}
func TestPictureCropValidation(t *testing.T) {
	for _, e := range []Element{
		{Fit: "stretch"}, {Crop: &Crop{Left: -.1}}, {Crop: &Crop{Top: math.NaN()}}, {Crop: &Crop{Right: math.Inf(1)}}, {Crop: &Crop{Left: .6, Right: .4}}, {Crop: &Crop{Top: .9999999}},
	} {
		e.W = 2
		e.H = 2
		if _, _, err := imagePlacement(e, 400, 200); err == nil {
			t.Fatalf("accepted %+v", e)
		}
	}
	e, rect, err := imagePlacement(Element{W: 2, H: 1, Fit: "cover", Crop: &Crop{Left: .25, Right: .25}}, 400, 200)
	if err != nil || e.W != 2 || e.H != 1 || rect != `<a:srcRect l="25000" t="25000" r="25000" b="25000"/>` {
		t.Fatal(e, rect, err)
	}
}
func TestLibreOfficePictureCropRendering(t *testing.T) {
	if _, err := lo.Find(); err != nil {
		t.Skip("LibreOffice unavailable")
	}
	in, _ := cropDeck(t)
	ctx := context.Background()
	result, err := convert.Convert(ctx, in, filepath.Join(t.TempDir(), "pdf"), convert.Options{})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(result["output"].(string))
	if err != nil {
		t.Fatal(err)
	}
	_, err = pdf.RenderImages(ctx, data, pdf.RenderOptions{Pages: []int{1, 2, 3, 4, 5}, MaxPixels: 400}, func(page int, im *image.RGBA) error {
		// Interior of the requested 2-inch square, safely away from antialias edges.
		for y := 110; y < 290; y++ {
			for x := 110; x < 290; x++ {
				c := im.RGBAAt(x, y)
				if page >= 4 {
					if x > 190 && x < 210 {
						continue
					} // color boundary
					want := imgcolor.RGBA{0, 255, 0, 255}
					if page == 4 && x < 200 {
						want = imgcolor.RGBA{255, 0, 0, 255}
					}
					if page == 5 && x > 200 {
						want = imgcolor.RGBA{0, 0, 255, 255}
					}
					if math.Abs(float64(c.R)-float64(want.R)) > 15 || math.Abs(float64(c.G)-float64(want.G)) > 15 || math.Abs(float64(c.B)-float64(want.B)) > 15 {
						t.Fatalf("anchor page %d pixel %d,%d: %v want %v", page, x, y, c, want)
					}
				} else if page > 1 {
					if c.G < 240 || c.R > 15 || c.B > 15 {
						t.Fatalf("page %d crop pixel %d,%d: %v", page, x, y, c)
					}
				} else if y < 145 || y > 255 {
					if c.R < 240 || c.G < 240 || c.B < 240 {
						t.Fatal("contain image stretched", x, y, c)
					}
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestImageAnchorGeometry(t *testing.T) {
	for _, tc := range []struct {
		name          string
		e             Element
		width, height int
		x, y, w, h    float64
		rect          string
	}{
		{"top cover", Element{W: 2, H: 2, Fit: "cover", Anchor: &ImageAnchor{0, 0}}, 200, 400, 0, 0, 2, 2, `<a:srcRect l="0" t="0" r="0" b="50000"/>`},
		{"bottom cover", Element{W: 2, H: 2, Fit: "cover", Anchor: &ImageAnchor{1, 1}}, 200, 400, 0, 0, 2, 2, `<a:srcRect l="0" t="50000" r="0" b="0"/>`},
		{"fractional pre-crop", Element{W: 2, H: 2, Fit: "cover", Crop: &Crop{Left: .1, Right: .1}, Anchor: &ImageAnchor{.25, .75}}, 400, 200, 0, 0, 2, 2, `<a:srcRect l="17500" t="0" r="32500" b="0"/>`},
		{"bottom contain", Element{X: 1, Y: 1, W: 2, H: 2, Anchor: &ImageAnchor{1, 1}}, 400, 200, 1, 2, 2, 1, ""},
		{"right contain", Element{X: 1, Y: 1, W: 2, H: 2, Anchor: &ImageAnchor{1, 1}}, 200, 400, 2, 1, 1, 2, ""},
		{"fractional contain", Element{X: 1, Y: 1, W: 2, H: 2, Anchor: &ImageAnchor{.25, .75}}, 200, 400, 1.25, 1, 1, 2, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e, r, err := imagePlacement(tc.e, tc.width, tc.height)
			if err != nil || r != tc.rect || e.X != tc.x || e.Y != tc.y || e.W != tc.w || e.H != tc.h {
				t.Fatal(e, r, err)
			}
		})
	}
	for _, a := range []ImageAnchor{{-.1, 0}, {0, 1.1}, {math.NaN(), 0}, {0, math.Inf(1)}} {
		if _, _, err := imagePlacement(Element{W: 2, H: 2, Anchor: &a}, 400, 200); err == nil {
			t.Fatalf("accepted invalid anchor %+v", a)
		}
	}
	out := filepath.Join(t.TempDir(), "invalid.pptx")
	err := Create(context.Background(), Spec{Width: 4, Height: 4, Slides: []Slide{{Elements: []Element{{Type: "text", W: 2, H: 2, Anchor: &ImageAnchor{}}}}}}, "", out)
	if err == nil {
		t.Fatal("accepted anchor on non-image")
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Fatal("invalid request wrote output", err)
	}
}
