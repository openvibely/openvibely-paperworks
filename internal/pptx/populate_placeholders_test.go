package pptx

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"

	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
)

func TestPopulateLayoutPlaceholdersAndFill(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	in, styled, out, again, filled := filepath.Join(dir, "in.pptx"), filepath.Join(dir, "styled.pptx"), filepath.Join(dir, "out.pptx"), filepath.Join(dir, "again.pptx"), filepath.Join(dir, "filled.pptx")
	if err := Create(ctx, Spec{Slides: []Slide{{Elements: []Element{{Type: "text", Text: "Keep title", Placeholder: "title", X: 1, Y: 1, W: 5, H: 1}}}}}, dir, in); err != nil {
		t.Fatal(err)
	}
	layout := "ppt/slideLayouts/custom.xml"
	edits := []DeckEdit{{Operation: "add-layout", Part: layout, ParentPart: "ppt/slideMasters/slideMaster1.xml", Placeholders: []LayoutPlaceholder{{Type: "title", Index: 0, X: 1, Y: 1, W: 5, H: 1}, {Type: "body", Index: 1, X: 1, Y: 3, W: 5, H: 2}, {Type: "pic", Index: 2, X: 7, Y: 1, W: 2, H: 2}, {Type: "ftr", Index: 3, X: 1, Y: 6, W: 5, H: .5}}}, {Operation: "set-layout", Slide: 1, Part: layout}}
	if err := DeckEditFile(ctx, in, styled, dir, DeckEditSpec{Edits: edits}); err != nil {
		t.Fatal(err)
	}
	// Imported prefixes and layout prompt text must not leak into slide content.
	p, err := opc.Open(styled)
	if err != nil {
		t.Fatal(err)
	}
	ld, _ := p.XML(layout)
	for _, body := range office.All(ld.Root(), pns, "txBody") {
		replaceBody(body, "Layout prompt")
	}
	p.SetXML(layout, ld)
	raw := p.Get(layout)
	raw = bytes.ReplaceAll(raw, []byte("xmlns:p="), []byte("xmlns:pp="))
	raw = bytes.ReplaceAll(raw, []byte("<p:"), []byte("<pp:"))
	raw = bytes.ReplaceAll(raw, []byte("</p:"), []byte("</pp:"))
	p.Set(layout, raw)
	alias := filepath.Join(dir, "alias.pptx")
	if err := p.SaveNew(alias); err != nil {
		t.Fatal(err)
	}
	populate := DeckEditSpec{Edits: []DeckEdit{{Operation: "populate-placeholders", Slide: 1}}}
	if err := DeckEditFile(ctx, alias, out, dir, populate); err != nil {
		t.Fatal(err)
	}
	q, _ := opc.Open(out)
	d, _ := q.XML("ppt/slides/slide1.xml")
	phs := office.All(d.Root(), pns, "ph")
	if len(phs) != 3 {
		t.Fatalf("expected title/body/picture, got %d", len(phs))
	}
	if !bytes.Contains(q.Get("ppt/slides/slide1.xml"), []byte("Keep title")) || bytes.Contains(q.Get("ppt/slides/slide1.xml"), []byte("Layout prompt")) {
		t.Fatal("existing text changed or prompt copied")
	}
	if !bytes.Equal(p.Get(layout), q.Get(layout)) {
		t.Fatal("layout mutated")
	}
	ids := map[string]bool{}
	bodyID := ""
	for _, sp := range office.All(d.Root(), pns, "sp") {
		pr := office.All(sp, pns, "cNvPr")[0]
		id := pr.SelectAttrValue("id", "")
		if ids[id] {
			t.Fatal("duplicate id")
		}
		ids[id] = true
		ph := office.All(sp, pns, "ph")[0]
		if ph.SelectAttrValue("type", "") == "body" {
			bodyID = id
			xf := office.Child(office.Child(sp, pns, "spPr"), ans, "xfrm")
			if xf == nil {
				t.Fatal("lost geometry")
			}
		}
	}
	if bodyID == "" {
		t.Fatal("missing body")
	}
	if err := DeckEditFile(ctx, out, again, dir, populate); err != nil {
		t.Fatal(err)
	}
	r, _ := opc.Open(again)
	if !bytes.Equal(q.Get("ppt/slides/slide1.xml"), r.Get("ppt/slides/slide1.xml")) {
		t.Fatal("second population changed slide")
	}
	if _, err := EditFile(ctx, again, filled, EditSpec{Edits: []Edit{{Operation: "text", Slide: 1, ShapeID: bodyID, Text: "Filled body"}}}); err != nil {
		t.Fatal(err)
	}
	f, _ := opc.Open(filled)
	if !bytes.Contains(f.Get("ppt/slides/slide1.xml"), []byte("Filled body")) {
		t.Fatal("body not editable")
	}
}

func TestCreateBodyPlaceholderIndex(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "out.pptx")
	idx := uint32(4)
	if err := Create(context.Background(), Spec{Slides: []Slide{{Elements: []Element{{Type: "text", Text: "Body", Placeholder: "body", PlaceholderIndex: &idx, X: 1, Y: 1, W: 5, H: 2}}}}}, dir, out); err != nil {
		t.Fatal(err)
	}
	p, _ := opc.Open(out)
	d, _ := p.XML("ppt/slides/slide1.xml")
	ph := office.All(d.Root(), pns, "ph")
	if len(ph) != 1 || ph[0].SelectAttrValue("type", "") != "body" || ph[0].SelectAttrValue("idx", "") != "4" {
		t.Fatal("incorrect placeholder")
	}
}

func TestPopulatePreservesReplacedTableSlot(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	in := filepath.Join(dir, "in.pptx")
	a := filepath.Join(dir, "a.pptx")
	b := filepath.Join(dir, "b.pptx")
	out := filepath.Join(dir, "out.pptx")
	if err := Create(ctx, Spec{Slides: []Slide{{Elements: []Element{{Type: "text", Text: "Keep", X: 1, Y: 1, W: 2, H: 1}}}}}, dir, in); err != nil {
		t.Fatal(err)
	}
	layout := "ppt/slideLayouts/table.xml"
	if err := DeckEditFile(ctx, in, a, dir, DeckEditSpec{Edits: []DeckEdit{{Operation: "add-layout", Part: layout, ParentPart: "ppt/slideMasters/slideMaster1.xml", Placeholders: []LayoutPlaceholder{{Type: "tbl", Index: 4, X: 1, Y: 3, W: 5, H: 2}}}, {Operation: "set-layout", Slide: 1, Part: layout}, {Operation: "populate-placeholders", Slide: 1}}}); err != nil {
		t.Fatal(err)
	}
	p, _ := opc.Open(a)
	d, _ := p.XML("ppt/slides/slide1.xml")
	id := ""
	for _, sp := range office.All(d.Root(), pns, "sp") {
		if len(office.All(sp, pns, "ph")) > 0 {
			id = office.All(sp, pns, "cNvPr")[0].SelectAttrValue("id", "")
		}
	}
	idx := uint32(4)
	if err := DeckEditFile(ctx, a, b, dir, DeckEditSpec{Edits: []DeckEdit{{Operation: "replace-element", Slide: 1, ShapeID: id, Element: &Element{Type: "table", Placeholder: "tbl", PlaceholderIndex: &idx, X: 1, Y: 3, W: 5, H: 2, Rows: [][]string{{"Name", "Value"}, {"Sales", "100"}}}}}}); err != nil {
		t.Fatal(err)
	}
	if err := DeckEditFile(ctx, b, out, dir, DeckEditSpec{Edits: []DeckEdit{{Operation: "populate-placeholders", Slide: 1}}}); err != nil {
		t.Fatal(err)
	}
	p, _ = opc.Open(out)
	d, _ = p.XML("ppt/slides/slide1.xml")
	if len(office.All(d.Root(), pns, "ph")) != 1 || len(office.All(d.Root(), ans, "tbl")) != 1 {
		t.Fatal("table placeholder duplicated or lost")
	}
}
