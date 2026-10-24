package pptx

import (
	"bytes"
	"context"
	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"os"
	"path/filepath"
	"testing"
)

func TestAddMasterLayoutAndAssign(t *testing.T) {
	dir := t.TempDir()
	in, out := filepath.Join(dir, "in.pptx"), filepath.Join(dir, "out.pptx")
	if err := Create(context.Background(), Spec{Slides: []Slide{{Elements: []Element{{Type: "text", Text: "Retain me", X: 1, Y: 1, W: 3, H: 1}}}}}, dir, in); err != nil {
		t.Fatal(err)
	}
	before, _ := opc.Open(in)
	master, layout := "ppt/slideMasters/brand.xml", "ppt/slideLayouts/title.xml"
	edits := []DeckEdit{{Operation: "add-master", Part: master, Name: "Brand"}, {Operation: "add-layout", Part: layout, ParentPart: master, Name: "Title", Placeholders: []LayoutPlaceholder{{Type: "title", Index: 0, X: 1, Y: .5, W: 8, H: 1}}}, {Operation: "set-layout", Slide: 1, Part: layout}}
	if err := DeckEditFile(context.Background(), in, out, dir, DeckEditSpec{Edits: edits}); err != nil {
		t.Fatal(err)
	}
	p, err := opc.Open(out)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before.Get("ppt/slides/slide1.xml"), p.Get("ppt/slides/slide1.xml")) {
		t.Fatal("layout assignment rewrote slide content")
	}
	d, _ := p.XML(layout)
	ph := office.All(d.Root(), pns, "ph")
	if len(ph) != 1 || ph[0].SelectAttrValue("type", "") != "title" {
		t.Fatal("placeholder missing")
	}
	for _, part := range []string{master, layout, "ppt/slides/slide1.xml", "ppt/presentation.xml"} {
		rels, err := p.Rels(part)
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range rels {
			if r.TargetMode != "External" && !p.Has(opc.Resolve(part, r.Target)) {
				t.Fatalf("broken relationship %s: %+v", part, r)
			}
		}
	}
	md, _ := p.XML(master)
	ids := office.All(md.Root(), pns, "sldLayoutId")
	if len(ids) != 2 {
		t.Fatalf("expected blank and authored layout, got %d", len(ids))
	}
	rels, _ := p.Rels(master)
	for _, id := range ids {
		matched := false
		for _, r := range rels {
			if r.ID == id.SelectAttrValue("r:id", "") && r.Type == rns+"/slideLayout" {
				matched = true
			}
		}
		if !matched {
			t.Fatal("unregistered layout")
		}
	}
	for _, bad := range []DeckEdit{{Operation: "add-layout", Part: "ppt/slideLayouts/bad.xml", ParentPart: "missing"}, {Operation: "add-master", Part: master}, {Operation: "add-master", Part: "../bad.xml"}} {
		failed := filepath.Join(dir, "bad.pptx")
		if err := DeckEditFile(context.Background(), out, failed, dir, DeckEditSpec{Edits: []DeckEdit{bad}}); err == nil {
			t.Fatal("accepted invalid style part")
		}
		if _, err = os.Stat(failed); !os.IsNotExist(err) {
			t.Fatal("published failed edit")
		}
	}
}
