package wordedit

import (
	"bytes"
	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"testing"
)

func TestReferenceRunTextSpaceEdges(t *testing.T) {
	for _, tc := range []struct {
		text     string
		preserve bool
	}{
		{"Revenue results", false}, {": Revenue results", false}, {"Figure ", true},
		{" leading", true}, {"a\u00a0b", false}, {"\u00a0a", true}, {"a\u2003", true},
	} {
		t.Run(tc.text, func(t *testing.T) {
			r := W("r")
			setRunText(r, tc.text)
			nodes := r.ChildElements()
			if len(nodes) != 1 || nodes[0].Tag != "t" {
				t.Fatal(nodes)
			}
			if got := nodes[0].SelectAttrValue("xml:space", "") == "preserve"; got != tc.preserve {
				t.Fatalf("preserve=%v want %v", got, tc.preserve)
			}
			if nodes[0].Text() != tc.text {
				t.Fatal("text changed")
			}
		})
	}
	r := W("r")
	setRunText(r, " a\tb \nc")
	tags := []string{"t", "tab", "t", "br", "t"}
	for i, c := range r.ChildElements() {
		if c.Tag != tags[i] {
			t.Fatal(c.Tag, tags[i])
		}
	}
}

func TestAddedWordPartRelationshipOrderAndOrphanRetention(t *testing.T) {
	// This supplemental ordering case requires document-before-core root order.
	// Keep that explicit fixture independent of the pinned Reference setup template.
	src := mk(t, "ordering", `<w:p>`+run("Source")+`</w:p>`, withCore("Private author"))
	src = referenceRewrite(t, src, func(p *opc.Package) {
		p.Set("orphan.bin", []byte("keep orphan"))
		rels, err := p.XML("_rels/.rels")
		if err != nil {
			t.Fatal(err)
		}
		for _, rel := range rels.Root().ChildElements() {
			if rel.SelectAttrValue("Target", "") == "word/document.xml" {
				rels.Root().RemoveChild(rel)
				rels.Root().InsertChildAt(0, rel)
				break
			}
		}
		if err := p.SetXML("_rels/.rels", rels); err != nil {
			t.Fatal(err)
		}
	})
	before, err := opc.Open(src)
	if err != nil {
		t.Fatal(err)
	}
	out := mustEdit(t, src, map[string]any{"operation": "note", "paragraph": float64(0), "text": "Source checked"})
	after, err := opc.Open(out)
	if err != nil {
		t.Fatal(err)
	}
	pos := map[string]int{}
	for i, n := range after.Names() {
		pos[n] = i
	}
	if pos["_rels/.rels"] != 1 {
		t.Fatal("root relationships must follow content types", after.Names())
	}
	if pos["word/footnotes.xml"] >= pos["docProps/core.xml"] {
		t.Fatal("new related part must precede next root sibling", after.Names())
	}
	if !bytes.Equal(after.Get("orphan.bin"), before.Get("orphan.bin")) {
		t.Fatal("orphan lost")
	}
	ct, err := after.XML("[Content_Types].xml")
	if err != nil {
		t.Fatal(err)
	}
	prev := ""
	for _, c := range office.Children(ct.Root(), opc.NSCT, "Override") {
		name := c.SelectAttrValue("PartName", "")
		if name < prev {
			t.Fatal("unsorted override", prev, name)
		}
		prev = name
	}
	for _, part := range before.Parts {
		switch part.Name {
		case "word/document.xml", "word/_rels/document.xml.rels", "[Content_Types].xml":
			continue
		}
		if !bytes.Equal(part.Data, after.Get(part.Name)) {
			t.Fatal("unrelated bytes changed", part.Name)
		}
	}
}
