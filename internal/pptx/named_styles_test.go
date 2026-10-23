package pptx

import (
	"bytes"
	"context"
	"github.com/beevik/etree"
	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNamedStylesCreateAndEdit(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "in.pptx")
	out := filepath.Join(dir, "out.pptx")
	size := 28.0
	yes, no := true, false
	styles := map[string]NamedTextStyle{"Heading": {FontSize: &size, Color: "123456", Bold: &yes, Align: "ctr"}, "Emphasis": {Italic: &yes, Underline: &yes}, "Plain": {Bold: &no, Color: "FF0000"}}
	spec := Spec{NamedStyles: styles, Slides: []Slide{{Elements: []Element{
		{Type: "text", Style: "Heading", W: 6, H: 2, Paragraphs: []Paragraph{{Runs: []Run{{Text: "Keep "}, {Text: "this", Style: "Emphasis"}}}}},
		{Type: "text", Style: "Heading", Text: "Another heading", Y: 3, W: 6, H: 2},
	}}}}
	if err := Create(context.Background(), spec, dir, in); err != nil {
		t.Fatal(err)
	}
	p, _ := opc.Open(in)
	d, err := p.XML("ppt/slides/slide1.xml")
	if err != nil {
		t.Fatal(err)
	}
	runs := office.All(d.Root(), ans, "r")
	if len(runs) != 3 {
		t.Fatal("text changed")
	}
	for _, r := range runs {
		pr := office.Child(r, ans, "rPr")
		if pr.SelectAttrValue("sz", "") != "2800" || pr.SelectAttrValue("b", "") != "1" {
			t.Fatal("style not reused")
		}
	}
	pr := office.Child(runs[1], ans, "rPr")
	if pr.SelectAttrValue("i", "") != "1" || pr.SelectAttrValue("u", "") != "sng" {
		t.Fatal("run style missing")
	}
	edits := EditSpec{NamedStyles: styles, Edits: []Edit{{Slide: 1, ShapeID: "2", Operation: "apply-style", Style: "Plain", Scope: "run", Run: 1}}}
	if _, err := EditFile(context.Background(), in, out, edits); err != nil {
		t.Fatal(err)
	}
	q, _ := opc.Open(out)
	after, _ := q.XML("ppt/slides/slide1.xml")
	rs := office.All(after.Root(), ans, "r")
	if office.Child(rs[0], ans, "rPr").SelectAttrValue("b", "") != "1" {
		t.Fatal("changed unrelated run")
	}
	edited := office.Child(rs[1], ans, "rPr")
	if edited.SelectAttrValue("b", "") != "0" || edited.SelectAttrValue("i", "") != "1" || office.All(edited, ans, "srgbClr")[0].SelectAttrValue("val", "") != "FF0000" {
		t.Fatal("selective style failed")
	}
	for i := range runs {
		if office.Child(runs[i], ans, "t").Text() != office.Child(rs[i], ans, "t").Text() {
			t.Fatal("changed text")
		}
	}
	edits.Edits[0].Style = "Missing"
	bad := filepath.Join(dir, "bad.pptx")
	if _, err := EditFile(context.Background(), in, bad, edits); err == nil {
		t.Fatal("accepted unknown style")
	}
	if _, err := os.Stat(bad); !os.IsNotExist(err) {
		t.Fatal("published rejected edit")
	}
	edits.Edits[0].Style = "Heading"
	if _, err := EditFile(context.Background(), in, bad, edits); err == nil {
		t.Fatal("run alignment silently applied")
	}
}

func TestNamedStyleKeepsHyperlinkAndFontOrder(t *testing.T) {
	d := etree.NewDocument()
	if err := d.ReadFromString(`<p:sp xmlns:p="` + pns + `" xmlns:z="` + ans + `"><p:txBody><z:bodyPr/><z:lstStyle/><z:p><z:r><z:rPr><z:hlinkClick xmlns:r="` + rns + `" r:id="rId42"/></z:rPr><z:t>Link</z:t></z:r></z:p></p:txBody></p:sp>`); err != nil {
		t.Fatal(err)
	}
	if err := applyNamedStyle(nil, "", d.Root(), map[string]NamedTextStyle{"Body": {Font: "Arial"}}, "Body", "shape", 0, 0); err != nil {
		t.Fatal(err)
	}
	raw, err := d.WriteToBytes()
	if err != nil {
		t.Fatal(err)
	}
	q := etree.NewDocument()
	if err = q.ReadFromBytes(raw); err != nil {
		t.Fatal(err)
	}
	pr := office.All(q.Root(), ans, "rPr")[0]
	children := pr.ChildElements()
	if len(children) != 2 || children[0].Tag != "latin" || children[1].Tag != "hlinkClick" {
		t.Fatal("font order or hyperlink changed")
	}
	if office.AttrV(children[1], rns, "id") != "rId42" {
		t.Fatal("lost hyperlink")
	}
}

func TestNamedStyleAlignmentOnImportedSlide(t *testing.T) {
	for _, scope := range []string{"shape", "paragraph"} {
		for _, existing := range []bool{false, true} {
			label := scope + "/missing-properties"
			if existing {
				label = scope + "/existing-properties"
			}
			t.Run(label, func(t *testing.T) {
				dir := t.TempDir()
				original := filepath.Join(dir, "original.pptx")
				input := filepath.Join(dir, "imported.pptx")
				output := filepath.Join(dir, "edited.pptx")
				if err := Create(context.Background(), Spec{Slides: []Slide{{Elements: []Element{{Type: "text", Text: "Keep this text", W: 6, H: 2}}}}}, dir, original); err != nil {
					t.Fatal(err)
				}
				pkg, err := opc.Open(original)
				if err != nil {
					t.Fatal(err)
				}
				part := "ppt/slides/slide1.xml"
				raw := strings.NewReplacer("xmlns:a=", "xmlns:z=", "<a:", "<z:", "</a:", "</z:").Replace(string(pkg.Get(part)))
				doc := etree.NewDocument()
				if err := doc.ReadFromString(raw); err != nil {
					t.Fatal(err)
				}
				for _, p := range office.All(doc.Root(), ans, "p") {
					for _, pr := range office.Children(p, ans, "pPr") {
						p.RemoveChild(pr)
					}
					if existing {
						pr := etree.NewElement("z:pPr")
						pr.CreateAttr("algn", "l")
						pr.CreateAttr("marL", "12700")
						p.InsertChildAt(0, pr)
					}
				}
				if err := pkg.SetXML(part, doc); err != nil {
					t.Fatal(err)
				}
				if err := pkg.SaveNew(input); err != nil {
					t.Fatal(err)
				}
				yes := true
				if _, err := EditFile(context.Background(), input, output, EditSpec{NamedStyles: map[string]NamedTextStyle{"Centered": {Align: "ctr", Bold: &yes}}, Edits: []Edit{{Slide: 1, ShapeID: "2", Operation: "apply-style", Style: "Centered", Scope: scope}}}); err != nil {
					t.Fatal(err)
				}
				result, err := opc.Open(output)
				if err != nil {
					t.Fatal(err)
				}
				edited, err := result.XML(part)
				if err != nil {
					t.Fatal(err)
				}
				// etree accepts unresolved prefixes; check expanded names after serialization.
				var check func(*etree.Element)
				check = func(e *etree.Element) {
					if e.Space != "" && e.NamespaceURI() == "" {
						t.Fatalf("unbound prefix on %s:%s", e.Space, e.Tag)
					}
					for _, c := range e.ChildElements() {
						check(c)
					}
				}
				check(edited.Root())
				p := office.All(edited.Root(), ans, "p")[0]
				props := office.Children(p, ans, "pPr")
				if len(props) != 1 || props[0].SelectAttrValue("algn", "") != "ctr" {
					t.Fatal("expected one centered paragraph-properties node")
				}
				if existing && props[0].SelectAttrValue("marL", "") != "12700" {
					t.Fatal("lost existing margin")
				}
				if office.All(p, ans, "t")[0].Text() != "Keep this text" {
					t.Fatal("text changed")
				}
				if office.Child(props[0], ans, "defRPr").SelectAttrValue("b", "") != "1" {
					t.Fatal("default formatting missing")
				}
				for _, originalPart := range pkg.Parts {
					if originalPart.Name != part && !bytes.Equal(originalPart.Data, result.Get(originalPart.Name)) {
						t.Fatal("changed unrelated part", originalPart.Name)
					}
				}
			})
		}
	}
}
