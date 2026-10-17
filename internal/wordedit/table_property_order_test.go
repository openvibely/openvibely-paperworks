package wordedit

import (
	"reflect"
	"testing"

	"github.com/beevik/etree"
	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
)

// Pinned word_edit sets autofit first, appends missing width/indent properties,
// and updates existing properties in place. It does not sort arbitrary input.
func TestReferenceTablePropertyInsertionOrder(t *testing.T) {
	for _, c := range []struct {
		name                                           string
		missingWidth, existingIndent, missingCellWidth bool
		want                                           []string
	}{
		{"new-indent", false, false, false, []string{"tblW", "tblLayout", "tblLook", "tblInd"}},
		{"new-width-and-indent", true, false, false, []string{"tblLayout", "tblLook", "tblW", "tblInd"}},
		{"existing-indent", false, true, false, []string{"tblInd", "tblW", "tblLayout", "tblLook"}},
		{"new-cell-width", false, false, true, []string{"tblW", "tblLayout", "tblLook", "tblInd"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			source := referenceRewrite(t, base(t), func(p *opc.Package) {
				referenceSetXML(t, p, "word/document.xml", func(root *etree.Element) {
					tbl := office.All(root, nsW, "tbl")[0]
					pr := child(tbl, "tblPr")
					if c.missingWidth {
						pr.RemoveChild(child(pr, "tblW"))
					}
					if c.existingIndent {
						e := W("tblInd")
						setW(e, "w", "123")
						pr.InsertChildAt(0, e)
					}
					if c.missingCellWidth {
						for _, cell := range all(tbl, "tc") {
							cp := child(cell, "tcPr")
							cp.RemoveChild(child(cp, "tcW"))
							cp.AddChild(W("vAlign"))
						}
					}
				})
			})
			out := mustEdit(t, source, map[string]any{"operation": "table-geometry", "table": 0.0, "widths_twips": []any{2000.0, 2000.0}, "indent_twips": 0.0})
			tbl := office.All(referenceXML(t, out, "word/document.xml"), nsW, "tbl")[0]
			var names []string
			for _, e := range child(tbl, "tblPr").ChildElements() {
				names = append(names, e.Tag)
			}
			if !reflect.DeepEqual(names, c.want) {
				t.Fatalf("properties %v want %v", names, c.want)
			}
			if c.missingCellWidth {
				for _, cell := range all(tbl, "tc") {
					kids := child(cell, "tcPr").ChildElements()
					if len(kids) != 2 || kids[0].Tag != "vAlign" || kids[1].Tag != "tcW" {
						t.Fatal("new cell width must be appended")
					}
				}
			}
		})
	}
}
