package pptx

import (
	"context"
	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"os"
	"path/filepath"
	"testing"
)

func TestInternalTextLinksAndNavigation(t *testing.T) {
	dir := t.TempDir()
	in, out := filepath.Join(dir, "in.pptx"), filepath.Join(dir, "out.pptx")
	spec := Spec{Slides: []Slide{{Elements: []Element{{Type: "text", X: 1, Y: 1, W: 3, H: 1, Paragraphs: []Paragraph{{Runs: []Run{{Text: "Next", Link: &TextLink{URI: "ppt/slides/slide2.xml"}}}}}}}}, {Elements: []Element{{Type: "text", Text: "Target", X: 1, Y: 1, W: 3, H: 1}}}}}
	if err := Create(context.Background(), spec, dir, in); err != nil {
		t.Fatal(err)
	}
	p, _ := opc.Open(in)
	rels, _ := p.Rels("ppt/slides/slide1.xml")
	found := false
	for _, r := range rels {
		if r.Type == rns+"/slide" {
			found = r.TargetMode != "External" && opc.Resolve("ppt/slides/slide1.xml", r.Target) == "ppt/slides/slide2.xml"
		}
	}
	if !found {
		t.Fatal("missing internal slide relationship")
	}
	d, _ := p.XML("ppt/slides/slide1.xml")
	h := office.All(d.Root(), ans, "hlinkClick")
	if len(h) != 1 || h[0].SelectAttrValue("action", "") != "ppaction://hlinksldjump" {
		t.Fatal("missing slide jump")
	}
	_, err := EditFile(context.Background(), in, out, EditSpec{Edits: []Edit{{Operation: "run-format", Slide: 1, ShapeID: "2", Link: &TextLink{Action: "ppaction://hlinkshowjump?jump=nextslide"}}}})
	if err != nil {
		t.Fatal(err)
	}
	p, _ = opc.Open(out)
	d, _ = p.XML("ppt/slides/slide1.xml")
	h = office.All(d.Root(), ans, "hlinkClick")
	if len(h) != 1 || h[0].SelectAttrValue("action", "") != "ppaction://hlinkshowjump?jump=nextslide" {
		t.Fatal("navigation not updated")
	}
	for _, l := range []*TextLink{{URI: "ppt/slides/missing.xml"}, {Action: "ppaction://program"}, {URI: "ppt/slides/slide2.xml", Action: "bad"}} {
		bad := filepath.Join(dir, "bad.pptx")
		_, err := EditFile(context.Background(), in, bad, EditSpec{Edits: []Edit{{Operation: "run-format", Slide: 1, ShapeID: "2", Link: l}}})
		if err == nil {
			t.Fatal("accepted invalid link")
		}
		if _, err = os.Stat(bad); !os.IsNotExist(err) {
			t.Fatal("published invalid link")
		}
	}
}
