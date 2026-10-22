package pptx

import (
	"context"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/openvibely/openvibely-paperworks/internal/opc"
)

func TestEditsAllocateNumberedPartNames(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	f, err := os.Create(filepath.Join(dir, "pic.png"))
	if err != nil {
		t.Fatal(err)
	}
	png.Encode(f, image.NewRGBA(image.Rect(0, 0, 4, 4)))
	f.Close()
	pic := filepath.Join(dir, "pic.png")
	in := filepath.Join(dir, "in.pptx")
	spec := Spec{Slides: []Slide{
		{Elements: []Element{{Type: "image", Path: "pic.png", X: 1, Y: 1, W: 2, H: 2}, {Type: "shape", Shape: "rect", X: 4, Y: 1, W: 2, H: 2}}},
		{Elements: []Element{{Type: "text", Text: "Second", X: 1, Y: 1, W: 4, H: 1}}},
	}}
	if err := Create(ctx, spec, dir, in); err != nil {
		t.Fatal(err)
	}
	stripNotes(t, in, "ppt/slides/slide2.xml", "ppt/notesSlides/notesSlide2.xml")
	out := filepath.Join(dir, "out.pptx")
	edits := []Edit{
		{Operation: "replace-image", Slide: 1, ShapeID: "2", Path: pic},
		{Operation: "replace-image", Slide: 1, ShapeID: "2", Path: pic},
		{Operation: "drawing-style", Slide: 1, ShapeID: "3", DrawingStyle: &DrawingStyle{Image: &ImageFill{Path: pic, Fit: "tile"}}},
		{Operation: "notes", Slide: 2, Text: "Added notes"},
	}
	if _, err := EditFile(ctx, in, out, EditSpec{Edits: edits}); err != nil {
		t.Fatal(err)
	}
	p, err := opc.Open(out)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"ppt/media/replacement1.png", "ppt/media/replacement2.png", "ppt/media/fill1.png", "ppt/notesSlides/notesSlide2.xml"} {
		if !p.Has(name) {
			t.Error("missing", name)
		}
	}
	if p.Has("ppt/media/replacement3.png") || p.Has("ppt/media/fill2.png") {
		t.Error("unexpected extra media part")
	}

	deck := filepath.Join(dir, "deck.pptx")
	extra := Element{Type: "shape", Shape: "ellipse", X: 1, Y: 3, W: 1, H: 1}
	added := Slide{Elements: []Element{extra}}
	if err := DeckEditFile(ctx, out, deck, dir, DeckEditSpec{Edits: []DeckEdit{{Operation: "add-slide", Content: &added}, {Operation: "add-slide", Content: &added}}}); err != nil {
		t.Fatal(err)
	}
	d, err := opc.Open(deck)
	if err != nil {
		t.Fatal(err)
	}
	var imported []string
	for _, name := range d.Names() {
		if strings.HasPrefix(name, "ppt/slides/imported-") {
			imported = append(imported, name)
		}
	}
	sort.Strings(imported)
	if strings.Join(imported, ",") != "ppt/slides/imported-1.xml,ppt/slides/imported-2.xml" {
		t.Fatal(imported)
	}
}

func stripNotes(t *testing.T, file, slide, notes string) {
	t.Helper()
	p, err := opc.Open(file)
	if err != nil {
		t.Fatal(err)
	}
	p.Remove(notes)
	p.Remove(opc.RelsName(notes))
	rels, err := p.XML(opc.RelsName(slide))
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rels.Root().ChildElements() {
		if strings.HasSuffix(r.SelectAttrValue("Type", ""), "/notesSlide") {
			rels.Root().RemoveChild(r)
		}
	}
	if err := p.SetXML(opc.RelsName(slide), rels); err != nil {
		t.Fatal(err)
	}
	data, err := p.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, data, 0o644); err != nil {
		t.Fatal(err)
	}
}
