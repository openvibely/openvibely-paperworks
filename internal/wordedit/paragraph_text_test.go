package wordedit

import (
	"strings"
	"testing"

	"github.com/beevik/etree"
	"github.com/openvibely/openvibely-paperworks/internal/office"
)

// UNRUN: source port of reference DOCX library 1.2.0 CT_R.text, CT_Hyperlink.text,
// CT_P.text, and BlockItemContainer.paragraphs. Descendant w:t collection
// is deliberately separate: original tests sometimes explicitly request it.
func referenceRunText(r *etree.Element) string {
	var b strings.Builder
	for _, e := range r.ChildElements() {
		if e.NamespaceURI() != nsW {
			continue
		}
		switch e.Tag {
		case "t":
			b.WriteString(e.Text())
		case "tab", "ptab":
			b.WriteByte('\t')
		case "cr":
			b.WriteByte('\n')
		case "noBreakHyphen":
			b.WriteByte('-')
		case "br":
			typ := "textWrapping"
			for _, a := range e.Attr {
				if a.Key == "type" && a.NamespaceURI() == nsW {
					typ = a.Value
				}
			}
			if typ == "textWrapping" {
				b.WriteByte('\n')
			}
		}
	}
	return b.String()
}

func referenceParagraphText(p *etree.Element) string {
	var b strings.Builder
	for _, e := range p.ChildElements() {
		if e.NamespaceURI() != nsW {
			continue
		}
		switch e.Tag {
		case "r":
			b.WriteString(referenceRunText(e))
		case "hyperlink":
			for _, r := range office.Children(e, nsW, "r") {
				b.WriteString(referenceRunText(r))
			}
		}
	}
	return b.String()
}

// referenceCommentText mirrors Comment.text: only direct paragraphs contribute,
// and each paragraph boundary contributes a newline, including empty paragraphs.
func referenceCommentText(comment *etree.Element) string {
	paragraphs := office.Children(comment, nsW, "p")
	texts := make([]string, len(paragraphs))
	for i, p := range paragraphs {
		texts[i] = referenceParagraphText(p)
	}
	return strings.Join(texts, "\n")
}

func referenceFirstCommentText(t *testing.T, path string) string {
	t.Helper()
	comments := office.Children(referenceXML(t, path, "word/comments.xml"), nsW, "comment")
	if len(comments) == 0 {
		t.Fatal("missing first comment")
	}
	return referenceCommentText(comments[0])
}

func referenceStoryParagraphs(t *testing.T, path, name string) []*etree.Element {
	t.Helper()
	root := referenceXML(t, path, name)
	if root.NamespaceURI() == nsW && root.Tag == "document" {
		root = office.Child(root, nsW, "body")
	}
	if root == nil {
		t.Fatal("missing paragraph container")
	}
	return office.Children(root, nsW, "p")
}

func referenceStoryTexts(t *testing.T, path, name string, descendant bool) []string {
	t.Helper()
	out := []string{}
	for _, p := range referenceStoryParagraphs(t, path, name) {
		if descendant {
			out = append(out, referenceTexts(p))
		} else {
			out = append(out, referenceParagraphText(p))
		}
	}
	return out
}

// UNRUN source contracts; expected strings come from the pinned selectors and
// text-element conversions, not from executing either implementation.
func TestReferenceParagraphTextContract(t *testing.T) {
	cases := []struct{ name, body, want string }{
		{"ordered hyperlink runs", `<w:r><w:t>A</w:t></w:r><w:hyperlink><w:r><w:t>B</w:t></w:r><w:r><w:t>C</w:t></w:r></w:hyperlink><w:r><w:t>D</w:t></w:r>`, "ABCD"},
		{"excluded wrappers", `<w:sdt><w:sdtContent><w:r><w:t>control</w:t></w:r></w:sdtContent></w:sdt><w:fldSimple><w:r><w:t>field</w:t></w:r></w:fldSimple><w:ins><w:r><w:t>insert</w:t></w:r></w:ins><w:del><w:r><w:delText>delete</w:delText></w:r></w:del><w:r><w:t>visible</w:t></w:r>`, "visible"},
		{"hyperlink excludes nested revision", `<w:hyperlink><w:ins><w:r><w:t>hidden</w:t></w:r></w:ins><w:r><w:t>shown</w:t></w:r></w:hyperlink>`, "shown"},
		{"run text elements", `<w:r><w:t>A</w:t><w:tab w:val="left"/><w:ptab/><w:cr/><w:noBreakHyphen/><w:br/><w:br w:type="textWrapping"/><w:br w:type="page"/><w:br w:type="column"/><w:t>Z</w:t></w:r>`, "A\t\t\n-\n\nZ"},
		{"run excludes nested text and instructions", `<w:r><w:instrText>REF x</w:instrText><w:delText>deleted</w:delText><w:drawing><w:t>drawing</w:t></w:drawing><w:t>cached</w:t></w:r>`, "cached"},
		{"XML entities", `<w:r><w:t>&amp;amp;&#65;&#x1F600;&lt;</w:t></w:r>`, "&amp;A😀<"},
		{"namespace identity", `<q:r xmlns:q="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><q:t>word</q:t><q:br q:type="page"/></q:r><w:r><x:t xmlns:x="urn:other">ignored</x:t></w:r>`, "word"},
		{"empty paragraph", ``, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d := etree.NewDocument()
			if err := d.ReadFromString(`<w:p ` + wNS + `>` + c.body + `</w:p>`); err != nil {
				t.Fatal(err)
			}
			referenceEqual(t, referenceParagraphText(d.Root()), c.want)
		})
	}
}

func TestReferenceParagraphAcquisitionContract(t *testing.T) {
	src := mk(t, "paragraphs", `<w:p><w:r><w:t>first</w:t></w:r><w:fldSimple><w:r><w:t>field</w:t></w:r></w:fldSimple></w:p><w:tbl><w:tr><w:tc><w:p><w:r><w:t>cell</w:t></w:r></w:p></w:tc></w:tr></w:tbl><w:sdt><w:sdtContent><w:p><w:r><w:t>block</w:t></w:r></w:p></w:sdtContent></w:sdt><w:ins><w:p><w:r><w:t>revision</w:t></w:r></w:p></w:ins><w:p/>`, nil)
	referenceEqual(t, paras(t, src), []string{"first", ""})
	referenceEqual(t, referenceStoryTexts(t, src, "word/document.xml", true), []string{"firstfield", ""})
}

// UNRUN: Comment.text source contracts from reference DOCX library v1.2.0.
func TestReferenceCommentTextContract(t *testing.T) {
	cases := []struct{ name, body, want string }{
		{"no paragraphs", ``, ""},
		{"empty boundaries", `<w:p/><w:p><w:r><w:t>middle</w:t></w:r></w:p><w:p/>`, "\nmiddle\n"},
		{"paragraphs tabs and breaks", `<w:p><w:r><w:t>A</w:t><w:tab/><w:t>B</w:t><w:br/><w:t>C</w:t></w:r></w:p><w:p><w:r><w:t>D</w:t></w:r></w:p>`, "A\tB\nC\nD"},
		{"exclude nested paragraphs", `<w:tbl><w:tr><w:tc><w:p><w:r><w:t>table</w:t></w:r></w:p></w:tc></w:tr></w:tbl><w:sdt><w:sdtContent><w:p><w:r><w:t>control</w:t></w:r></w:p></w:sdtContent></w:sdt><w:p><w:r><w:t>direct</w:t></w:r></w:p>`, "direct"},
		{"hyperlink and excluded field", `<w:p><w:r><w:annotationRef/></w:r><w:hyperlink><w:r><w:t>link</w:t></w:r></w:hyperlink><w:fldSimple><w:r><w:t>field</w:t></w:r></w:fldSimple></w:p>`, "link"},
		{"namespace identity", `<q:p xmlns:q="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><q:r><q:t>&amp;</q:t></q:r></q:p><x:p xmlns:x="urn:other"><w:r><w:t>ignored</w:t></w:r></x:p>`, "&"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d := etree.NewDocument()
			if err := d.ReadFromString(`<w:comment ` + wNS + `>` + c.body + `</w:comment>`); err != nil {
				t.Fatal(err)
			}
			referenceEqual(t, referenceCommentText(d.Root()), c.want)
		})
	}
}
