package pptx

import (
	"bytes"
	"context"
	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"path/filepath"
	"testing"
)

func TestStyleRegistrationWithAlternatePrefixes(t *testing.T) {
	dir := t.TempDir()
	seed, in, out := filepath.Join(dir, "seed.pptx"), filepath.Join(dir, "in.pptx"), filepath.Join(dir, "out.pptx")
	if err := Create(context.Background(), Spec{Slides: []Slide{{Elements: []Element{{Type: "text", Text: "Preserve", X: 1, Y: 1, W: 3, H: 1}}}}}, dir, seed); err != nil {
		t.Fatal(err)
	}
	p, _ := opc.Open(seed)
	for _, part := range []string{"ppt/presentation.xml", "ppt/slideMasters/slideMaster1.xml"} {
		raw := p.Get(part)
		raw = bytes.ReplaceAll(raw, []byte("xmlns:p="), []byte("xmlns:pp="))
		raw = bytes.ReplaceAll(raw, []byte("<p:"), []byte("<pp:"))
		raw = bytes.ReplaceAll(raw, []byte("</p:"), []byte("</pp:"))
		p.Set(part, raw)
	}
	if err := p.SaveNew(in); err != nil {
		t.Fatal(err)
	}
	err := DeckEditFile(context.Background(), in, out, dir, DeckEditSpec{Edits: []DeckEdit{{Operation: "add-master", Part: "ppt/slideMasters/brand.xml"}, {Operation: "add-layout", Part: "ppt/slideLayouts/extra.xml", ParentPart: "ppt/slideMasters/slideMaster1.xml"}}})
	if err != nil {
		t.Fatal(err)
	}
	p, _ = opc.Open(out)
	d, _ := p.XML("ppt/presentation.xml")
	if got := len(office.All(d.Root(), pns, "sldMasterId")); got != 2 {
		t.Fatalf("master registrations in correct namespace: %d", got)
	}
	d, _ = p.XML("ppt/slideMasters/slideMaster1.xml")
	if got := len(office.All(d.Root(), pns, "sldLayoutId")); got != 2 {
		t.Fatalf("layout registrations in correct namespace: %d", got)
	}
}
