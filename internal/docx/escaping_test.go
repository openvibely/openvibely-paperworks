package docx

import (
	"strings"
	"testing"

	"github.com/openvibely/openvibely-paperworks/internal/opc"
)

func TestAuthoredTextAndAttributesEscape(t *testing.T) {
	text := "A & B < C > \"quoted\" 'apostrophe' café\x01"
	style := "A & B < C > \"style\" 'café'"
	target := "https://example.test/?q=A&B<tea>\"café\""

	raw, err := Build(Spec{Blocks: []Block{{
		Type:  "paragraph",
		Style: style,
		Runs:  []Run{{Text: text, Link: target}},
	}}}, "")
	if err != nil {
		t.Fatal(err)
	}
	pkg, err := opc.Read(raw)
	if err != nil {
		t.Fatal(err)
	}

	doc, err := pkg.XML("word/document.xml")
	if err != nil {
		t.Fatal(err)
	}
	paragraph := doc.Root().FindElement("w:body").FindElement("w:p")
	if paragraph == nil {
		t.Fatal("authored paragraph missing")
	}
	pPr := paragraph.FindElement("w:pPr")
	if pPr == nil {
		t.Fatal("paragraph properties missing")
	}
	styleElement := pPr.FindElement("w:pStyle")
	if styleElement == nil {
		t.Fatal("paragraph style missing")
	}
	if got := styleElement.SelectAttrValue("w:val", ""); got != style {
		t.Fatalf("paragraph style attribute = %q, want %q", got, style)
	}
	hyperlink := paragraph.FindElement("w:hyperlink")
	if hyperlink == nil {
		t.Fatal("hyperlink missing")
	}
	run := hyperlink.FindElement("w:r")
	if run == nil {
		t.Fatal("hyperlink run missing")
	}
	textElement := run.FindElement("w:t")
	if textElement == nil {
		t.Fatal("paragraph text missing")
	}
	if got, want := textElement.Text(), strings.TrimSuffix(text, "\x01"); got != want {
		t.Fatalf("paragraph text = %q, want %q", got, want)
	}

	rels, err := pkg.XML("word/_rels/document.xml.rels")
	if err != nil {
		t.Fatal(err)
	}
	foundTarget := false
	for _, relationship := range rels.Root().ChildElements() {
		if relationship.SelectAttrValue("Target", "") == target {
			foundTarget = true
		}
	}
	if !foundTarget {
		t.Fatalf("relationship target %q missing", target)
	}

	if got, want := esc(text), "A &amp; B &lt; C &gt; &quot;quoted&quot; 'apostrophe' café"; got != want {
		t.Fatalf("DOCX esc() = %q, want %q", got, want)
	}
	for _, part := range pkg.Parts {
		if strings.HasSuffix(part.Name, ".xml") || strings.HasSuffix(part.Name, ".rels") {
			if _, err := opc.ParseXML(part.Data); err != nil {
				t.Errorf("generated XML part %s does not parse: %v", part.Name, err)
			}
		}
	}
}
