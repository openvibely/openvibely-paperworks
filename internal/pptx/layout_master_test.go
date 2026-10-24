package pptx

import (
	"bytes"
	"context"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"os"
	"path/filepath"
	"testing"
)

func TestReparentLayout(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "in.pptx")
	prepared := filepath.Join(dir, "prepared.pptx")
	out := filepath.Join(dir, "out.pptx")
	if err := Create(context.Background(), Spec{Slides: []Slide{{Elements: []Element{{Type: "text", Text: "Keep", X: 1, Y: 1, W: 2, H: 1}}}}}, dir, in); err != nil {
		t.Fatal(err)
	}
	master := "ppt/slideMasters/new.xml"
	layout := "ppt/slideLayouts/slideLayout1.xml"
	if err := DeckEditFile(context.Background(), in, prepared, dir, DeckEditSpec{Edits: []DeckEdit{{Operation: "add-master", Part: master}}}); err != nil {
		t.Fatal(err)
	}
	before, _ := opc.Open(prepared)
	edit := DeckEdit{Operation: "set-layout-master", Part: layout, ParentPart: master}
	if err := DeckEditFile(context.Background(), prepared, out, dir, DeckEditSpec{Edits: []DeckEdit{edit, edit}}); err != nil {
		t.Fatal(err)
	}
	after, _ := opc.Open(out)
	for _, part := range []string{layout, "ppt/slides/slide1.xml", opc.RelsName("ppt/slides/slide1.xml")} {
		if !bytes.Equal(before.Get(part), after.Get(part)) {
			t.Fatalf("rewrote %s", part)
		}
	}
	rels, _ := after.Rels(layout)
	n := 0
	for _, r := range rels {
		if r.Type == rns+"/slideMaster" {
			n++
			if opc.Resolve(layout, r.Target) != master {
				t.Fatal("wrong master")
			}
		}
	}
	if n != 1 {
		t.Fatal("master count")
	}
	for _, part := range []string{"ppt/slideMasters/slideMaster1.xml", master} {
		rs, _ := after.Rels(part)
		found := false
		for _, r := range rs {
			if opc.Resolve(part, r.Target) == layout {
				found = true
			}
		}
		if found != (part == master) {
			t.Fatalf("reverse linkage %s", part)
		}
	}
	edit.ParentPart = "missing"
	bad := filepath.Join(dir, "bad.pptx")
	if err := DeckEditFile(context.Background(), out, bad, dir, DeckEditSpec{Edits: []DeckEdit{edit}}); err == nil {
		t.Fatal("accepted missing master")
	}
	if _, err := os.Stat(bad); !os.IsNotExist(err) {
		t.Fatal("published rejected edit")
	}
}
