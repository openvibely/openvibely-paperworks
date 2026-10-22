package wordedit

import (
	"path/filepath"
	"testing"

	"github.com/beevik/etree"
	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
)

func TestSectionEditsIgnoreTrackedProperties(t *testing.T) {
	for _, kind := range []string{"header", "footer"} {
		for _, typ := range []string{"default", "first", "even"} {
			t.Run(kind+"-"+typ, func(t *testing.T) {
				src := mk(t, "tracked-sections", `<w:p><w:pPr><w:sectPr/></w:pPr>`+run("First")+`</w:p><w:p>`+run("Second")+`</w:p>`, nil)
				src = mustEdit(t, src, map[string]any{"operation": "header-footer", "section": 0.0, "kind": kind, "type": typ, "text": "Original"})
				src = referenceRewrite(t, src, func(pkg *opc.Package) {
					referenceSetXML(t, pkg, "word/document.xml", func(root *etree.Element) {
						sec := root.FindElement("w:body/w:p/w:pPr/w:sectPr")
						old := sec.Copy()
						change := W("sectPrChange")
						setW(change, "id", "12")
						setW(change, "author", "Reviewer")
						change.AddChild(old)
						sec.AddChild(change)
					})
				})
				before := referenceXML(t, src, "word/document.xml")
				for _, linked := range []bool{false, true} {
					input := src
					if linked {
						input = mustEdit(t, src, map[string]any{"operation": "header-footer", "section": 1.0, "kind": kind, "type": typ, "text": "Independent"})
					}
					out := mustEdit(t, input, map[string]any{"operation": "header-footer-link", "section": 1.0, "kind": kind, "type": typ, "linked": linked})
					root := referenceXML(t, out, "word/document.xml")
					// Use the actual final section directly, not the editor's selector.
					final := root.FindElement("w:body/w:sectPr")
					if got := child(final, kind+"Reference") == nil; got != linked {
						t.Errorf("requested linked=%v; actual second section linked=%v", linked, got)
					}
					if sectionSnapshot(t, root) != sectionSnapshot(t, before) {
						t.Error("first section or its tracked historical properties changed")
					}
				}
				formatted := mustEdit(t, src, map[string]any{"operation": "section-format", "section": 1.0, "top": 2222.0})
				root := referenceXML(t, formatted, "word/document.xml")
				if getW(root.FindElement("w:body/w:sectPr/w:pgMar"), "top") != "2222" || sectionSnapshot(t, root) != sectionSnapshot(t, before) {
					t.Error("section formatting targeted tracked properties instead of the second section")
				}
				report, err := office.PackageReport(src)
				if err != nil {
					t.Fatal(err)
				}
				for _, key := range []string{"sections", "section_links", "section_details"} {
					if got := len(report[key].([]any)); got != 2 {
						t.Errorf("inspect %s reports %d sections, want 2", key, got)
					}
				}
				if _, err := Edit(src, filepath.Join(t.TempDir(), "invalid.docx"), map[string]any{"operation": "header-footer-link", "section": 2.0, "kind": kind, "type": typ, "linked": false}, ""); err == nil {
					t.Error("accepted historical properties as a third section")
				}
			})
		}
	}
}

func sectionSnapshot(t *testing.T, root *etree.Element) string {
	t.Helper()
	doc := etree.NewDocument()
	doc.SetRoot(root.FindElement("w:body/w:p/w:pPr/w:sectPr").Copy())
	text, err := doc.WriteToString()
	if err != nil {
		t.Fatal(err)
	}
	return text
}
