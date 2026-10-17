package pptx

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/openvibely/openvibely-paperworks/internal/opc"
)

func TestRunTextEditPreservesFormattingAndPackage(t *testing.T) {
	dir := t.TempDir()
	seed := filepath.Join(dir, "seed.pptx")
	s := Spec{Slides: []Slide{{Elements: []Element{{Type: "text", X: 1, Y: 1, W: 8, H: 2, Paragraphs: []Paragraph{{Bullet: true, Runs: []Run{{Text: "Keep ", Bold: boolPtr(true)}, {Text: "Old", Color: "FF0000", Italic: true}, {Text: " suffix", Underline: true}}}, {Text: "Other paragraph"}}}}}}}
	if err := Create(context.Background(), s, ".", seed); err != nil {
		t.Fatal(err)
	}
	p, err := opc.Open(seed)
	if err != nil {
		t.Fatal(err)
	}
	d, err := p.XML("ppt/slides/slide1.xml")
	if err != nil {
		t.Fatal(err)
	}
	para := d.FindElement("//a:p")
	field := para.CreateElement("a:fld")
	field.CreateAttr("id", "{12345678-1234-1234-1234-123456789ABC}")
	field.CreateAttr("type", "slidenum")
	field.CreateElement("a:t").SetText("1")
	para.CreateElement("a:br")
	// A native action hyperlink needs no external relationship.
	pr := d.FindElements("//a:r")[1].SelectElement("rPr")
	pr.CreateElement("a:hlinkClick").CreateAttr("action", "ppaction://hlinkshowjump?jump=nextslide")
	// External click/hover links must retain relationship IDs and exact targets.
	links, err := p.XML("ppt/slides/_rels/slide1.xml.rels")
	if err != nil {
		t.Fatal(err)
	}
	for _, link := range []struct{ id, tag, target string }{
		{"rIdExternalClick", "a:hlinkClick", "https://example.org/report?q=a&lang=日本語#section"},
		{"rIdExternalHover", "a:hlinkMouseOver", "mailto:review@example.org?subject=Deck"},
	} {
		rel := links.Root().CreateElement("Relationship")
		rel.CreateAttr("Id", link.id)
		rel.CreateAttr("Type", rns+"/hyperlink")
		rel.CreateAttr("Target", link.target)
		rel.CreateAttr("TargetMode", "External")
		h := d.FindElements("//a:r")[0].SelectElement("rPr").CreateElement(link.tag)
		h.CreateAttr("r:id", link.id)
	}
	if err := p.SetXML("ppt/slides/_rels/slide1.xml.rels", links); err != nil {
		t.Fatal(err)
	}
	if err := p.SetXML("ppt/slides/slide1.xml", d); err != nil {
		t.Fatal(err)
	}
	in := filepath.Join(dir, "input.pptx")
	if err := p.SaveNew(in); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		match, text string
		occurrence  int
		want        []string
		fail        bool
	}{
		{"ep Old suf", "新😀", 0, []string{"Ke新😀", "", "fix"}, false},
		{"Old", "", 0, []string{"Keep ", "", " suffix"}, false},
		{"e", "X", 1, []string{"KeXp ", "Old", " suffix"}, false},
		{"suffix1", "X", 0, nil, true},
		{"Old", "X", 1, nil, true},
		{"", "X", 0, nil, true},
		{"Old", "X", -1, nil, true},
		{"Old", "new\rline", 0, nil, true},
		{"Keep Old suffix", "complete", 0, []string{"complete", "", ""}, false},
		{"Old suffix", "tail", 0, []string{"Keep ", "tail", ""}, false},
	} {
		t.Run("replace/"+tc.match+tc.text+fmt.Sprint(tc.occurrence), func(t *testing.T) {
			target := filepath.Join(t.TempDir(), "result.pptx")
			_, err := EditFile(context.Background(), in, target, EditSpec{Edits: []Edit{{Slide: 1, ShapeID: "2", Operation: "replace-text", Match: tc.match, Text: tc.text, Occurrence: tc.occurrence}}})
			if tc.fail {
				if err == nil {
					t.Fatal("accepted invalid match")
				}
				if _, e := os.Stat(target); !os.IsNotExist(e) {
					t.Fatal("published failed edit")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			q, err := opc.Open(target)
			if err != nil {
				t.Fatal(err)
			}
			expected := d.Copy()
			runs := expected.FindElements("//a:r")
			for i, text := range tc.want {
				runs[i].SelectElement("t").SetText(text)
			}
			original := append([]byte(nil), p.Get("ppt/slides/slide1.xml")...)
			if err := p.SetXML("ppt/slides/slide1.xml", expected); err != nil {
				t.Fatal(err)
			}
			for _, part := range p.Parts {
				if !bytes.Equal(part.Data, q.Get(part.Name)) {
					t.Fatalf("unexpected mutation %s", part.Name)
				}
			}
			// Restore the expected source package for subsequent cases.
			for i := range p.Parts {
				if p.Parts[i].Name == "ppt/slides/slide1.xml" {
					p.Parts[i].Data = original
				}
			}
		})
	}
	before, _ := os.ReadFile(in)
	out := filepath.Join(dir, "edited.pptx")
	changed, err := EditFile(context.Background(), in, out, EditSpec{Edits: []Edit{{Slide: 1, ShapeID: "2", Operation: "run-text", Run: 1, Text: " 新 & <value> 😀 "}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(changed) != 1 || changed[0] != "ppt/slides/slide1.xml" {
		t.Fatal(changed)
	}
	q, err := opc.Open(out)
	if err != nil {
		t.Fatal(err)
	}
	d.FindElements("//a:r")[1].SelectElement("t").SetText(" 新 & <value> 😀 ")
	if err := p.SetXML("ppt/slides/slide1.xml", d); err != nil {
		t.Fatal(err)
	}
	if len(p.Parts) != len(q.Parts) {
		t.Fatal("part count changed")
	}
	for i, part := range p.Parts {
		if part.Name != q.Parts[i].Name || !bytes.Equal(part.Data, q.Parts[i].Data) {
			t.Fatalf("unexpected mutation: %s", part.Name)
		}
	}
	after, _ := os.ReadFile(in)
	if !bytes.Equal(before, after) {
		t.Fatal("source changed")
	}
	// Empty replacements retain the run and its formatting rather than removing it.
	empty := filepath.Join(dir, "empty.pptx")
	if _, err := EditFile(context.Background(), in, empty, EditSpec{Edits: []Edit{{Slide: 1, ShapeID: "2", Operation: "run-text", Run: 1}}}); err != nil {
		t.Fatal(err)
	}
	ep, err := opc.Open(empty)
	if err != nil {
		t.Fatal(err)
	}
	ed, err := ep.XML("ppt/slides/slide1.xml")
	if err != nil {
		t.Fatal(err)
	}
	er := ed.FindElements("//a:r")
	if len(er) != 4 || er[1].SelectElement("t").Text() != "" || er[1].FindElement("a:rPr/a:hlinkClick") == nil {
		t.Fatal("empty edit removed structure")
	}
	// An invalid later operation must not publish an earlier valid mutation.
	failed := filepath.Join(dir, "failed.pptx")
	if _, err := EditFile(context.Background(), in, failed, EditSpec{Edits: []Edit{
		{Slide: 1, ShapeID: "2", Operation: "run-text", Run: 1, Text: "intermediate"},
		{Slide: 1, ShapeID: "2", Operation: "run-text", Run: 99},
	}}); err == nil {
		t.Fatal("invalid batch accepted")
	}
	if _, err := os.Stat(failed); !os.IsNotExist(err) {
		t.Fatal("partial batch published", err)
	}
	for i, op := range []Edit{
		{Slide: 1, ShapeID: "2", Operation: "run-text", Run: -1},
		{Slide: 1, ShapeID: "2", Operation: "run-text", Run: 3},
		{Slide: 1, ShapeID: "2", Operation: "run-text", Paragraph: 2},
		{Slide: 1, ShapeID: "2", Operation: "run-text", Text: "new\nline"},
	} {
		bad := filepath.Join(t.TempDir(), "bad.pptx")
		if _, err := EditFile(context.Background(), in, bad, EditSpec{Edits: []Edit{op}}); err == nil {
			t.Fatalf("case %d accepted", i)
		}
		if _, err := os.Stat(bad); !os.IsNotExist(err) {
			t.Fatal("invalid output written", err)
		}
	}
}

func boolPtr(v bool) *bool { return &v }
