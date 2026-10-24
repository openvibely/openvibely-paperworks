package pptx

import (
	"context"
	"fmt"
	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"os"
	"path/filepath"
	"testing"
)

func TestDeckAuthoringLinksUseDestinationSlides(t *testing.T) {
	for _, operation := range []string{"add-element", "replace-element", "add-slide"} {
		for _, target := range []int{1, 2} {
			t.Run(fmt.Sprintf("%s/%d", operation, target), func(t *testing.T) {
				dir := t.TempDir()
				in, out := filepath.Join(dir, "in.pptx"), filepath.Join(dir, "out.pptx")
				plain := Element{Type: "text", Text: "Original", X: 1, Y: 1, W: 3, H: 1}
				if err := Create(context.Background(), Spec{Slides: []Slide{{Elements: []Element{plain}}, {Elements: []Element{plain}}}}, dir, in); err != nil {
					t.Fatal(err)
				}
				targetPart := fmt.Sprintf("ppt/slides/slide%d.xml", target)
				element := Element{Type: "text", X: 1, Y: 3, W: 3, H: 1, Paragraphs: []Paragraph{{Runs: []Run{{Text: "Jump", Link: &TextLink{URI: targetPart}}}}}}
				edit := DeckEdit{Operation: operation, Slide: 2, ShapeID: "2", Element: &element}
				if operation == "add-slide" {
					edit.Element = nil
					edit.Content = &Slide{Elements: []Element{element}, NotesParagraphs: []Paragraph{{Runs: []Run{{Text: "Note link", Link: &TextLink{URI: targetPart}}}}}}
				}
				if err := DeckEditFile(context.Background(), in, out, dir, DeckEditSpec{Edits: []DeckEdit{edit}}); err != nil {
					t.Fatal(err)
				}
				p, _ := opc.Open(out)
				slides, err := office.PresentationSlides(p)
				if err != nil {
					t.Fatal(err)
				}
				want := 2
				if operation == "add-slide" {
					want = 3
				}
				if len(slides) != want {
					t.Fatal("unexpected slide count")
				}
				source := slides[len(slides)-1]
				d, _ := p.XML(source)
				links := office.All(d.Root(), ans, "hlinkClick")
				if len(links) != 1 {
					t.Fatal("missing link", len(links))
				}
				rels, _ := p.Rels(source)
				found := false
				for _, r := range rels {
					if r.ID == links[0].SelectAttrValue("r:id", "") {
						found = r.Type == rns+"/slide" && r.TargetMode != "External" && opc.Resolve(source, r.Target) == targetPart
					}
				}
				if !found || links[0].SelectAttrValue("action", "") != "ppaction://hlinksldjump" {
					t.Fatal("wrong destination link")
				}
				if element.Paragraphs[0].Runs[0].Link.URI != targetPart {
					t.Fatal("mutated caller spec")
				}
				if operation == "add-slide" {
					for _, r := range rels {
						if r.Type == rns+"/notesSlide" {
							part := opc.Resolve(source, r.Target)
							nd, _ := p.XML(part)
							hs := office.All(nd.Root(), ans, "hlinkClick")
							nr, _ := p.Rels(part)
							ok := false
							for _, link := range nr {
								if len(hs) == 1 && link.ID == hs[0].SelectAttrValue("r:id", "") {
									ok = opc.Resolve(part, link.Target) == targetPart && link.TargetMode != "External"
								}
							}
							if !ok {
								t.Fatal("notes target lost")
							}
						}
					}
				}
				element.Paragraphs[0].Runs[0].Link.URI = "ppt/slides/missing.xml"
				bad := filepath.Join(dir, "bad.pptx")
				if err := DeckEditFile(context.Background(), in, bad, dir, DeckEditSpec{Edits: []DeckEdit{{Operation: "add-element", Slide: 1, Element: &element}}}); err == nil {
					t.Fatal("accepted missing target")
				}
				if _, err := os.Stat(bad); !os.IsNotExist(err) {
					t.Fatal("published failed edit")
				}
			})
		}
	}
}
