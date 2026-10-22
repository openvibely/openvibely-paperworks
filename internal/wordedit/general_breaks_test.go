package wordedit

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/beevik/etree"
	"github.com/openvibely/openvibely-paperworks/internal/office"
)

func runShape(p *etree.Element) []string {
	var out []string
	for _, r := range office.Children(p, nsW, "r") {
		var kids []string
		for _, c := range r.ChildElements() {
			switch {
			case is(c, "rPr"):
				if office.Child(c, nsW, "b") != nil {
					kids = append(kids, "b")
				}
			case is(c, "t"):
				kids = append(kids, "t:"+c.Text())
			default:
				kids = append(kids, c.Tag)
			}
		}
		out = append(out, strings.Join(kids, ","))
	}
	return out
}

func noRawControls(t *testing.T, root *etree.Element) {
	t.Helper()
	office.Iter(root, func(e *etree.Element) bool {
		if is(e, "t") && strings.ContainsAny(e.Text(), "\t\n\r") {
			t.Fatalf("raw control in w:t: %q", e.Text())
		}
		return true
	})
}

func storyParas(t *testing.T, out, part string) []*etree.Element {
	root := referenceXML(t, out, part)
	noRawControls(t, root)
	if c := office.Child(root, nsW, "body"); c != nil {
		root = c
	}
	return office.Children(root, nsW, "p")
}

func TestGeneralEditConvertsLineBreaksAndTabs(t *testing.T) {
	pPr := `<w:pPr><w:jc w:val="center"/></w:pPr>`
	table := `<w:tbl><w:tblGrid><w:gridCol w:w="1000"/></w:tblGrid><w:tr><w:tc><w:p>` + run("h") + `</w:p></w:tc></w:tr><w:tr><w:tc><w:p>` + run("old") + `</w:p></w:tc></w:tr></w:tbl>`
	for _, tc := range []struct {
		name  string
		body  string
		edit  map[string]any
		check func(t *testing.T, ps []*etree.Element, root *etree.Element)
	}{
		{"paragraph-insert", `<w:p>` + pPr + run("Intro") + `</w:p>`,
			map[string]any{"operation": "paragraph-insert", "paragraph": 0.0, "position": "after", "text": "A\nB\tC"},
			func(t *testing.T, ps []*etree.Element, _ *etree.Element) {
				referenceEqual(t, runShape(ps[1]), []string{"t:A,br,t:B,tab,t:C"})
				if jc := office.Child(office.Child(ps[1], nsW, "pPr"), nsW, "jc"); jc == nil {
					t.Fatal("paragraph properties lost")
				}
			}},
		{"paragraph-insert CRLF", `<w:p>` + run("Intro") + `</w:p>`,
			map[string]any{"operation": "paragraph-insert", "paragraph": 0.0, "position": "after", "text": "A\r\nB\rC"},
			func(t *testing.T, ps []*etree.Element, _ *etree.Element) {
				referenceEqual(t, runShape(ps[1]), []string{"t:A,br,t:B,br,t:C"})
			}},
		{"table-cell-text", table,
			map[string]any{"operation": "table-cell-text", "table": 0.0, "row": 1.0, "column": 0.0, "text": "A\nB"},
			func(t *testing.T, _ []*etree.Element, root *etree.Element) {
				cell := root.FindElements("//w:tr")[1].FindElement("w:tc")
				cps := office.Children(cell, nsW, "p")
				if len(cps) != 1 {
					t.Fatal("cell paragraphs", len(cps))
				}
				referenceEqual(t, runShape(cps[0]), []string{"t:A,br,t:B"})
			}},
		{"replace-text in bold run", `<w:p>` + bold("pre Intro post") + `</w:p>`,
			map[string]any{"operation": "replace-text", "paragraph": 0.0, "old": "Intro", "new": "X\nY"},
			func(t *testing.T, ps []*etree.Element, _ *etree.Element) {
				referenceEqual(t, runShape(ps[0]), []string{"b,t:pre ", "b,t:X,br,t:Y", "b,t: post"})
			}},
		{"replace-text at run edges", `<w:p>` + bold("Intro") + `</w:p>`,
			map[string]any{"operation": "replace-text", "paragraph": 0.0, "old": "Intro", "new": "X\r\nY"},
			func(t *testing.T, ps []*etree.Element, _ *etree.Element) {
				referenceEqual(t, runShape(ps[0]), []string{"b,t:X,br,t:Y"})
			}},
		{"replace-text at run start with suffix", `<w:p>` + bold("Intro post") + `</w:p>`,
			map[string]any{"operation": "replace-text", "paragraph": 0.0, "old": "Intro", "new": "X\nY"},
			func(t *testing.T, ps []*etree.Element, _ *etree.Element) {
				referenceEqual(t, runShape(ps[0]), []string{"b,t:X,br,t:Y", "b,t: post"})
			}},
		{"replace-text spanning runs", `<w:p>` + bold("Hello In") + run("tro end") + `</w:p>`,
			map[string]any{"operation": "replace-text", "paragraph": 0.0, "old": "Intro", "new": "X\tY"},
			func(t *testing.T, ps []*etree.Element, _ *etree.Element) {
				referenceEqual(t, runShape(ps[0]), []string{"b,t:Hello ", "b,t:X,tab,t:Y", "t: end"})
			}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := mk(t, "breaks", tc.body, nil)
			out := mustEdit(t, src, tc.edit)
			tc.check(t, storyParas(t, out, "word/document.xml"), referenceXML(t, out, "word/document.xml"))
		})
	}
}

func TestGeneralEditBreaksInHeaderPart(t *testing.T) {
	src := mk(t, "hdr-breaks", `<w:p>`+run("Body")+`</w:p>`, withHeader(`<w:p>`+bold("Intro")+`</w:p><w:p>`+bold("Intro post")+`</w:p>`))
	out := mustEdit(t, src, map[string]any{"operation": "batch", "edits": []any{
		map[string]any{"operation": "paragraph-insert", "part": "word/header1.xml", "paragraph": 0.0, "position": "after", "text": "A\nB"},
		map[string]any{"operation": "replace-text", "part": "word/header1.xml", "paragraph": 0.0, "old": "Intro", "new": "X\nY"},
		map[string]any{"operation": "replace-text", "part": "word/header1.xml", "paragraph": 2.0, "old": "Intro", "new": "X\tY"},
	}})
	ps := storyParas(t, out, "word/header1.xml")
	referenceEqual(t, runShape(ps[0]), []string{"b,t:X,br,t:Y"})
	referenceEqual(t, runShape(ps[1]), []string{"t:A,br,t:B"})
	referenceEqual(t, runShape(ps[2]), []string{"b,t:X,tab,t:Y", "b,t: post"})
}

func TestGeneralEditReproBatch(t *testing.T) {
	table := `<w:tbl><w:tblGrid><w:gridCol w:w="1000"/></w:tblGrid><w:tr><w:tc><w:p>` + run("h") + `</w:p></w:tc></w:tr><w:tr><w:tc><w:p>` + run("old") + `</w:p></w:tc></w:tr></w:tbl>`
	src := mk(t, "repro", `<w:p>`+run("Intro")+`</w:p>`+table, nil)
	out := mustEdit(t, src, map[string]any{"operation": "batch", "edits": []any{
		map[string]any{"operation": "paragraph-insert", "paragraph": 0.0, "position": "after", "text": "Line one\nLine two\tTabbed"},
		map[string]any{"operation": "table-cell-text", "table": 0.0, "row": 1.0, "column": 0.0, "text": "Cell A\nCell B"},
		map[string]any{"operation": "replace-text", "paragraph": 0.0, "old": "Intro", "new": "Hello\nWorld"},
	}})
	xml := part(t, out, "word/document.xml")
	contains(t, xml, "<w:br/>")
	contains(t, xml, "<w:tab/>")
	noRawControls(t, referenceXML(t, out, "word/document.xml"))
}

func TestGeneralEditPlainTextUnchanged(t *testing.T) {
	src := mk(t, "plain", `<w:p>`+bold("pre Intro post")+`</w:p>`, nil)
	out := filepath.Join(t.TempDir(), "out.docx")
	if _, err := Edit(src, out, map[string]any{"operation": "batch", "edits": []any{
		map[string]any{"operation": "paragraph-insert", "paragraph": 0.0, "position": "after", "text": "Plain"},
		map[string]any{"operation": "replace-text", "paragraph": 0.0, "old": "Intro", "new": "Hi"},
	}}, ""); err != nil {
		t.Fatal(err)
	}
	xml := part(t, out, "word/document.xml")
	contains(t, xml, `<w:r><w:rPr><w:b/></w:rPr><w:t xml:space="preserve">pre Hi post</w:t></w:r>`)
	contains(t, xml, `<w:r><w:t xml:space="preserve">Plain</w:t></w:r>`)
}
