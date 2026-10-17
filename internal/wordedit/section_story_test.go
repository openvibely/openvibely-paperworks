package wordedit

import (
	"testing"

	"github.com/beevik/etree"
	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
)

// UNRUN: read-only model of sections[0].header/footer. Reference creates an
// empty definition when the first section lacks a reference; model its visible
// paragraph content without mutating or saving the result being inspected.
func referenceFirstSectionStory(t *testing.T, path, kind string) (string, *etree.Element) {
	t.Helper()
	root := referenceXML(t, path, "word/document.xml")
	sections := []*etree.Element{}
	for _, child := range office.Child(root, nsW, "body").ChildElements() {
		if child.NamespaceURI() != nsW {
			continue
		}
		if child.Tag == "sectPr" {
			sections = append(sections, child)
		}
		if child.Tag == "p" {
			for _, props := range office.Children(child, nsW, "pPr") {
				sections = append(sections, office.Children(props, nsW, "sectPr")...)
			}
		}
	}
	if len(sections) == 0 {
		t.Fatal("missing first section")
	}
	for _, ref := range office.Children(sections[0], nsW, kind+"Reference") {
		if office.AttrV(ref, nsW, "type") != "default" {
			continue
		}
		id := office.AttrV(ref, relBase[:len(relBase)-1], "id")
		pkg, err := opc.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		rels, err := pkg.Rels("word/document.xml")
		if err != nil {
			t.Fatal(err)
		}
		for _, rel := range rels {
			if rel.ID != id {
				continue
			}
			if rel.TargetMode == "External" {
				t.Fatal("external section story")
			}
			name := referencePartTarget("word/document.xml", rel.Target)
			return name, referenceXML(t, path, name)
		}
		t.Fatalf("missing section story relationship %q", id)
	}
	tag := "hdr"
	if kind == "footer" {
		tag = "ftr"
	}
	empty := etree.NewElement("w:" + tag)
	empty.CreateAttr("xmlns:w", nsW)
	style := "Header"
	if kind == "footer" {
		style = "Footer"
	}
	empty.CreateElement("w:p").CreateElement("w:pPr").CreateElement("w:pStyle").CreateAttr("w:val", style)
	return "", empty
}

// UNRUN: renamed targets, first-section ordering, ignored even-page references,
// and missing-definition content rather than a later section definition.
func TestReferenceFirstSectionStoryContract(t *testing.T) {
	for _, kind := range []string{"header", "footer"} {
		for _, attached := range []bool{false, true} {
			t.Run(kind+map[bool]string{false: "-missing", true: "-attached"}[attached], func(t *testing.T) {
				src := mk(t, "story", `<w:p/>`, nil)
				src = referenceRewrite(t, src, func(pkg *opc.Package) {
					tag := "hdr"
					if kind == "footer" {
						tag = "ftr"
					}
					name := "word/renamed-" + kind + ".xml"
					pkg.Set(name, []byte(`<w:`+tag+` `+wNS+`><w:p>`+run("Selected")+`</w:p></w:`+tag+`>`))
					if err := pkg.EnsureContentType(name, ctBase+kind+"+xml"); err != nil {
						t.Fatal(err)
					}
					id, err := pkg.AddRel("word/document.xml", relBase+kind, "renamed-"+kind+".xml", false)
					if err != nil {
						t.Fatal(err)
					}
					referenceSetXML(t, pkg, "word/document.xml", func(root *etree.Element) {
						body := office.Child(root, nsW, "body")
						// Wrapped paragraphs are excluded by CT_Document.sectPr_lst.
						wrapped := etree.NewElement("w:sdt")
						badRef := wrapped.CreateElement("w:sdtContent").CreateElement("w:p").CreateElement("w:pPr").CreateElement("w:sectPr").CreateElement("w:" + kind + "Reference")
						badRef.CreateAttr("w:type", "default")
						badRef.CreateAttr("r:id", "must-not-use-wrapped-section")
						body.InsertChildAt(0, wrapped)
						// This paragraph section precedes the body's final section.
						first := office.Child(body, nsW, "p").CreateElement("w:pPr").CreateElement("w:sectPr")
						even := first.CreateElement("w:" + kind + "Reference")
						even.CreateAttr("w:type", "even")
						even.CreateAttr("r:id", "ignored")
						if attached {
							ref := first.CreateElement("w:" + kind + "Reference")
							ref.CreateAttr("w:type", "default")
							ref.CreateAttr("r:id", id)
							// get_*Reference returns the first matching default reference.
							duplicate := first.CreateElement("w:" + kind + "Reference")
							duplicate.CreateAttr("w:type", "default")
							duplicate.CreateAttr("r:id", "must-not-use-second-default")
						}
						later := office.Child(body, nsW, "sectPr").CreateElement("w:" + kind + "Reference")
						later.CreateAttr("w:type", "default")
						later.CreateAttr("r:id", "must-not-use-later-section")
					})
				})
				name, root := referenceFirstSectionStory(t, src, kind)
				wantName, wantText := "", ""
				if attached {
					wantName, wantText = "word/renamed-"+kind+".xml", "Selected"
				}
				referenceEqual(t, name, wantName)
				paragraphs := office.Children(root, nsW, "p")
				referenceEqual(t, len(paragraphs), 1)
				referenceEqual(t, referenceParagraphText(paragraphs[0]), wantText)
				if !attached {
					style := "Header"
					if kind == "footer" {
						style = "Footer"
					}
					props := office.Child(paragraphs[0], nsW, "pPr")
					referenceEqual(t, office.AttrV(office.Child(props, nsW, "pStyle"), nsW, "val"), style)
				}
			})
		}
	}
}
