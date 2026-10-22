package wordedit

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/beevik/etree"
	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
)

func TestHeaderLinkKeepsSharedDefinitionAndPageFlags(t *testing.T) {
	src := mk(t, "three-sections", `<w:p><w:pPr><w:sectPr/></w:pPr>`+run("One")+`</w:p><w:p><w:pPr><w:sectPr/></w:pPr>`+run("Two")+`</w:p><w:p>`+run("Three")+`</w:p>`, nil)
	src = mustEdit(t, src, map[string]any{"operation": "header-footer", "section": 0.0, "kind": "header", "text": "First"})
	src = mustEdit(t, src, map[string]any{"operation": "header-footer", "section": 1.0, "kind": "header", "text": "Shared"})
	src = referenceRewrite(t, src, func(pkg *opc.Package) {
		referenceSetXML(t, pkg, "word/document.xml", func(root *etree.Element) {
			sections := all(office.Child(root, nsW, "body"), "sectPr")
			insertWordProperty(sections[2], child(sections[1], "headerReference").Copy())
		})
	})
	out := mustEdit(t, src, map[string]any{"operation": "header-footer-link", "section": 1.0, "kind": "header", "linked": true})
	if linkedStoryText(t, out, 1, "header", "default") != "First" || linkedStoryText(t, out, 2, "header", "default") != "Shared" {
		t.Fatal("linking changed another explicit section definition")
	}
	for _, typ := range []string{"first", "even"} {
		out = mustEdit(t, out, map[string]any{"operation": "header-footer-link", "section": 1.0, "kind": "footer", "type": typ, "linked": false})
	}
	root := referenceXML(t, out, "word/document.xml")
	if len(all(root, "titlePg")) != 0 {
		t.Fatal("linking enabled first-page mode")
	}
	if part(t, out, "word/settings.xml") != part(t, src, "word/settings.xml") {
		t.Fatal("linking changed even-page mode")
	}
}

// Resolve the saved reference chain, rather than relying on a count of parts.
func linkedStoryText(t *testing.T, file string, section int, kind, typ string) string {
	t.Helper()
	pkg, err := opc.Open(file)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := pkg.XML("word/document.xml")
	if err != nil {
		t.Fatal(err)
	}
	sections := all(office.Child(doc.Root(), nsW, "body"), "sectPr")
	rels, err := pkg.Rels("word/document.xml")
	if err != nil {
		t.Fatal(err)
	}
	for i := section; i >= 0; i-- {
		for _, ref := range sections[i].ChildElements() {
			if !is(ref, kind+"Reference") || getW(ref, "type") != typ {
				continue
			}
			id := office.AttrV(ref, opc.NSOffR, "id")
			for _, rel := range rels {
				if rel.ID != id {
					continue
				}
				if rel.Type != relBase+kind || rel.TargetMode == "External" {
					t.Fatal("invalid story relationship")
				}
				story, err := pkg.XML(referencePartTarget("word/document.xml", rel.Target))
				if err != nil {
					t.Fatal(err)
				}
				var text strings.Builder
				for _, el := range all(story.Root(), "t") {
					text.WriteString(el.Text())
				}
				return text.String()
			}
			t.Fatal("unresolved story relationship")
		}
	}
	return ""
}

func TestHeaderFooterLinkRoundTrip(t *testing.T) {
	for _, kind := range []string{"header", "footer"} {
		for _, typ := range []string{"default", "first", "even"} {
			t.Run(kind+"-"+typ, func(t *testing.T) {
				src := mk(t, "sections", `<w:p><w:pPr><w:sectPr/></w:pPr>`+run("First")+`</w:p><w:p>`+run("Second")+`</w:p>`, nil)
				src = mustEdit(t, src, map[string]any{"operation": "header-footer", "section": 0.0, "kind": kind, "type": typ, "text": "Original"})
				before, err := os.ReadFile(src)
				if err != nil {
					t.Fatal(err)
				}
				if got := linkedStoryText(t, src, 1, kind, typ); got != "Original" {
					t.Fatal(got)
				}
				link := func(input string, linked bool) string {
					return mustEdit(t, input, map[string]any{"operation": "header-footer-link", "section": 1.0, "kind": kind, "type": typ, "linked": linked})
				}
				unlinked := link(src, false)
				if got := linkedStoryText(t, unlinked, 1, kind, typ); got != "" {
					t.Fatal("unlink should create empty story", got)
				}
				if got := linkedStoryText(t, unlinked, 0, kind, typ); got != "Original" {
					t.Fatal("previous section changed", got)
				}
				if part(t, unlinked, "word/settings.xml") != part(t, src, "word/settings.xml") {
					t.Fatal("link changed page variants")
				}
				filled := mustEdit(t, unlinked, map[string]any{"operation": "header-footer", "section": 1.0, "kind": kind, "type": typ, "rows": [][]string{{"Independent"}}, "widths_twips": []int{3000}})
				noop := link(filled, false)
				if got := linkedStoryText(t, noop, 1, kind, typ); got != "Independent" {
					t.Fatal("repeated unlink erased content", got)
				}
				relinked := link(noop, true)
				if got := linkedStoryText(t, relinked, 1, kind, typ); got != "Original" {
					t.Fatal("did not restore inheritance", got)
				}
				// Keep the detached definition: other sections may reference it.
				if part(t, relinked, "word/"+kind+"Edit3.xml") != part(t, filled, "word/"+kind+"Edit3.xml") {
					t.Fatal("detached story deleted or changed")
				}
				for _, output := range []string{unlinked, noop, relinked} {
					if got := paras(t, output); len(got) != 2 || got[0] != "First" || got[1] != "Second" {
						t.Fatal("body changed", got)
					}
				}
				after, err := os.ReadFile(src)
				if err != nil || !bytes.Equal(before, after) {
					t.Fatal("source changed", err)
				}
			})
		}
	}
}

func TestHeaderFooterLinkValidationAndBatch(t *testing.T) {
	src := mk(t, "one-section", `<w:p>`+run("Body")+`</w:p>`, nil)
	// First-section linkage has no previous definition, so linking yields blank.
	filled := mustEdit(t, src, map[string]any{"operation": "header-footer", "section": 0.0, "kind": "header", "text": "Remove reference"})
	batch := mustEdit(t, filled, map[string]any{"operation": "batch", "edits": []any{
		map[string]any{"operation": "header-footer-link", "section": 0.0, "kind": "header", "linked": true},
		map[string]any{"operation": "header-footer-link", "section": 0.0, "kind": "footer", "linked": false},
	}})
	if got := linkedStoryText(t, batch, 0, "header", "default"); got != "" {
		t.Fatal(got)
	}
	for i, extra := range []map[string]any{{}, {"linked": "false"}, {"linked": false, "text": "Ignore"}, {"linked": true, "rows": [][]string{{"Ignore"}}}, {"linked": true, "section": 10.0}, {"linked": true, "type": "bogus"}, {"linked": true, "part": "word/headerEdit1.xml"}} {
		spec := map[string]any{"operation": "header-footer-link", "section": 0.0, "kind": "header"}
		for k, v := range extra {
			spec[k] = v
		}
		out := filepath.Join(t.TempDir(), fmt.Sprintf("invalid-%d.docx", i))
		if _, err := Edit(filled, out, spec, ""); err == nil {
			t.Fatal("accepted invalid linkage", extra)
		}
		if _, err := os.Stat(out); !os.IsNotExist(err) {
			t.Fatal("published invalid edit")
		}
	}
}
