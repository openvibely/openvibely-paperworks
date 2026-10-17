package pptx

import (
	"context"
	"encoding/json"
	"image"
	imagecolor "image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/beevik/etree"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
)

func TestPinnedPackageManifests(t *testing.T) {
	var oracle struct {
		Parts       map[string]string
		TableStyles string
	}
	raw, err := os.ReadFile("testdata/package-manifests.json")
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(raw, &oracle); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err = os.WriteFile(filepath.Join(dir, "tables.xml"), []byte(oracle.TableStyles), 0600); err != nil {
		t.Fatal(err)
	}
	im := image.NewRGBA(image.Rect(0, 0, 12, 12))
	for y := 0; y < 12; y++ {
		for x := 0; x < 12; x++ {
			im.Set(x, y, imagecolor.RGBA{R: 255, A: 255})
		}
	}
	f, err := os.Create(filepath.Join(dir, "image.png"))
	if err != nil {
		t.Fatal(err)
	}
	if err = png.Encode(f, im); err != nil {
		t.Fatal(err)
	}
	if err = f.Close(); err != nil {
		t.Fatal(err)
	}
	index := 5
	s := Spec{StyleResources: &StyleResources{TemplateLayout: &index, TableStyles: "tables.xml"}, Slides: []Slide{
		{Notes: "notes", Elements: []Element{
			{Type: "text", Text: "text", X: 1, Y: 1, W: 1, H: 1},
			{Type: "image", Path: "image.png", X: 1, Y: 2, W: 1, H: 1},
			{Type: "chart", X: 3, Y: 1, W: 3, H: 2, Chart: &Chart{Type: "column", Categories: []string{"A", "B"}, Series: []Series{{Name: "S", Values: []float64{1, 2}}}}}},
		},
		{Notes: "second"},
	}}
	out := filepath.Join(dir, "deck.pptx")
	if err = Create(context.Background(), s, dir, out); err != nil {
		t.Fatal(err)
	}
	p, err := opc.Open(out)
	if err != nil {
		t.Fatal(err)
	}
	for name, want := range oracle.Parts {
		if got := string(p.Get(name)); got != want {
			t.Errorf("%s differs\ngot %s\nwant %s", name, got, want)
		}
	}
	// Independently verify allocated IDs resolve from authored XML after text
	// shapes that don't allocate a relationship and across multiple slides.
	for _, part := range p.Parts {
		if strings.HasSuffix(part.Name, ".rels") {
			source := opc.SourceOf(part.Name)
			rs, e := p.Rels(source)
			if e != nil {
				t.Fatal(e)
			}
			seen := map[string]bool{}
			for _, r := range rs {
				if seen[r.ID] {
					t.Fatal("duplicate", source, r.ID)
				}
				seen[r.ID] = true
				if r.TargetMode != "External" && !p.Has(opc.Resolve(source, r.Target)) {
					t.Fatal("dangling", source, r.Target)
				}
			}
		} else if strings.HasSuffix(part.Name, ".xml") {
			d, e := p.XML(part.Name)
			if e != nil {
				t.Fatal(e)
			}
			rs, e := p.Rels(part.Name)
			if e != nil {
				t.Fatal(e)
			}
			ids := map[string]bool{}
			for _, r := range rs {
				ids[r.ID] = true
			}
			var walk func(*etree.Element)
			walk = func(el *etree.Element) {
				for _, a := range el.Attr {
					if a.Space == "r" && (a.Key == "id" || a.Key == "embed" || a.Key == "link") && !ids[a.Value] {
						t.Errorf("unresolved %s %s", part.Name, a.Value)
					}
				}
				for _, c := range el.ChildElements() {
					walk(c)
				}
			}
			walk(d.Root())
		}
	}
}
