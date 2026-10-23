package pptx

import (
	"context"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"path/filepath"
	"testing"
)

func TestSlideVisibility(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	in := filepath.Join(dir, "in.pptx")
	hidden := true
	if err := Create(ctx, Spec{Slides: []Slide{{Hidden: &hidden}, {}}}, dir, in); err != nil {
		t.Fatal(err)
	}
	p, err := opc.Open(in)
	if err != nil {
		t.Fatal(err)
	}
	d, _ := p.XML("ppt/slides/slide1.xml")
	if d.Root().SelectAttrValue("show", "") != "0" {
		t.Fatal("slide not hidden")
	}
	original := opc.Hash(p.Get("ppt/slides/slide2.xml"))
	hidden = false
	out := filepath.Join(dir, "out.pptx")
	if _, err = EditFile(ctx, in, out, EditSpec{Edits: []Edit{{Operation: "visibility", Slide: 1, Hidden: &hidden}}}); err != nil {
		t.Fatal(err)
	}
	p, err = opc.Open(out)
	if err != nil {
		t.Fatal(err)
	}
	d, _ = p.XML("ppt/slides/slide1.xml")
	if d.Root().SelectAttrValue("show", "") != "1" || opc.Hash(p.Get("ppt/slides/slide2.xml")) != original {
		t.Fatal("visibility edit changed wrong content")
	}
	if _, err = EditFile(ctx, in, filepath.Join(dir, "bad.pptx"), EditSpec{Edits: []Edit{{Operation: "visibility", Slide: 1}}}); err == nil {
		t.Fatal("missing hidden accepted")
	}
}
