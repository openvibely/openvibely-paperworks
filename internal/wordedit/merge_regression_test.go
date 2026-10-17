package wordedit

import (
	"github.com/beevik/etree"
	"strconv"
	"testing"

	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
)

// These checks supplement (not replace) the original merge method, which only
// required numbering-part presence. The pinned composer restarts ListNumber and
// imports no unused numbering definitions from this appendix.
func TestMergeRestartsAppendedListsAndRegistersImages(t *testing.T) {
	for _, copies := range []int{1, 2} {
		t.Run(strconv.Itoa(copies), func(t *testing.T) {
			appendix := referenceOriginalWordFixture(t, "merge-append")
			files := []any{}
			for i := 0; i < copies; i++ {
				files = append(files, appendix)
			}
			out := mustEdit(t, base(t), map[string]any{"operation": "merge", "files": files})
			numbers := referenceXML(t, out, "word/numbering.xml")
			referenceEqual(t, len(office.Children(numbers, nsW, "abstractNum")), 9)
			nums := office.Children(numbers, nsW, "num")
			referenceEqual(t, len(nums), 9+copies)
			doc := referenceXML(t, out, "word/document.xml")
			found := 0
			for _, p := range office.All(doc, nsW, "p") {
				if referenceParagraphText(p) != "First item" {
					continue
				}
				found++
				props := office.Child(p, nsW, "pPr")
				n := office.Child(office.Child(props, nsW, "numPr"), nsW, "numId")
				want := strconv.Itoa(9 + found)
				referenceEqual(t, getW(n, "val"), want)
				var definitionFound bool
				for _, n := range nums {
					if getW(n, "numId") != want {
						continue
					}
					definitionFound = true
					referenceEqual(t, getW(office.Child(n, nsW, "abstractNumId"), "val"), "7")
					override := office.Child(n, nsW, "lvlOverride")
					referenceEqual(t, getW(override, "ilvl"), "0")
					referenceEqual(t, getW(office.Child(override, nsW, "startOverride"), "val"), "1")
				}
				if !definitionFound {
					t.Fatal("missing restarted numbering definition")
				}
			}
			referenceEqual(t, found, copies)
			for i, n := range office.All(doc, "http://schemas.openxmlformats.org/drawingml/2006/picture", "cNvPr") {
				referenceEqual(t, n.SelectAttrValue("id", ""), strconv.Itoa(i+1))
			}
			ct := referenceXML(t, out, "[Content_Types].xml")
			defaults := office.Children(ct, opc.NSCT, "Default")
			png := 0
			for _, n := range defaults {
				if n.SelectAttrValue("Extension", "") == "png" {
					png++
					referenceEqual(t, n.SelectAttrValue("ContentType", ""), "image/png")
				}
			}
			referenceEqual(t, png, 1)
		})
	}
}

func TestMergeRemapsExplicitNumberingDefinition(t *testing.T) {
	src := referenceOriginalWordFixture(t, "merge-append")
	src = referenceRewrite(t, src, func(p *opc.Package) {
		referenceSetXML(t, p, "word/document.xml", func(root *etree.Element) {
			for _, p := range office.All(root, nsW, "p") {
				if referenceParagraphText(p) != "First item" {
					continue
				}
				props := office.Child(p, nsW, "pPr")
				props.RemoveChild(office.Child(props, nsW, "pStyle"))
				setW(props.CreateElement("w:numPr").CreateElement("w:numId"), "val", "5")
			}
		})
	})
	out := mustEdit(t, base(t), map[string]any{"operation": "merge", "files": []any{src}})
	root := referenceXML(t, out, "word/numbering.xml")
	referenceEqual(t, len(office.Children(root, nsW, "abstractNum")), 10)
	referenceEqual(t, len(office.Children(root, nsW, "num")), 10)
	var abs string
	for _, n := range office.Children(root, nsW, "num") {
		if getW(n, "numId") == "10" {
			abs = getW(office.Child(n, nsW, "abstractNumId"), "val")
		}
	}
	referenceEqual(t, abs, "9")
}
