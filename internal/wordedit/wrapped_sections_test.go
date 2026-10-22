package wordedit

import (
	"testing"

	"github.com/openvibely/openvibely-paperworks/internal/office"
)

func TestWrappedSectionInspectionAndEdits(t *testing.T) {
	for _, wrapper := range []struct{ name, open, close, path string }{
		{"content-control", `<w:sdt><w:sdtPr><w:id w:val="42"/></w:sdtPr><w:sdtContent>`, `</w:sdtContent></w:sdt>`, "w:sdt/w:sdtContent"},
		{"nested-content-control", `<w:sdt><w:sdtContent><w:sdt><w:sdtContent>`, `</w:sdtContent></w:sdt></w:sdtContent></w:sdt>`, "w:sdt/w:sdtContent/w:sdt/w:sdtContent"},
		{"custom-xml", `<w:customXml w:element="section">`, `</w:customXml>`, "w:customXml"},
	} {
		t.Run(wrapper.name, func(t *testing.T) {
			// Both revision containers hold old sectPr values that must not be
			// mistaken for sections while traversing the content wrapper.
			pr := `<w:pPr><w:sectPr><w:type w:val="nextPage"/><w:pgSz w:w="15840" w:h="12240"/><w:pgMar w:top="1440"/><w:sectPrChange w:id="1" w:author="Reviewer"><w:sectPr><w:pgMar w:top="777"/></w:sectPr></w:sectPrChange></w:sectPr><w:pPrChange w:id="2" w:author="Reviewer"><w:pPr><w:sectPr/></w:pPr></w:pPrChange></w:pPr>`
			src := mk(t, "wrapped", `<w:p>`+run("Split here")+`</w:p>`+wrapper.open+`<w:p>`+pr+run("First section end")+`</w:p>`+wrapper.close+`<w:p>`+run("Second section")+`</w:p>`, nil)
			path := "w:body/" + wrapper.path + "/w:p/w:pPr/w:sectPr"
			report, err := office.PackageReport(src)
			if err != nil {
				t.Fatal(err)
			}
			for _, key := range []string{"sections", "section_links", "section_details"} {
				if got := len(report[key].([]any)); got != 2 {
					t.Errorf("%s has %d sections, want 2", key, got)
				}
			}
			before := referenceXML(t, src, "word/document.xml")
			out := mustEdit(t, src, map[string]any{"operation": "section-format", "section": 0.0, "top": 3333.0})
			root := referenceXML(t, out, "word/document.xml")
			if getW(root.FindElement(path+"/w:pgMar"), "top") != "3333" {
				t.Error("wrapped section not formatted")
			}
			if getW(root.FindElement("w:body/w:sectPr/w:pgMar"), "top") != getW(before.FindElement("w:body/w:sectPr/w:pgMar"), "top") {
				t.Error("formatted final section instead")
			}
			if getW(root.FindElement(path+"/w:sectPrChange/w:sectPr/w:pgMar"), "top") != "777" {
				t.Error("changed historical properties")
			}
			out = mustEdit(t, src, map[string]any{"operation": "header-footer-link", "section": 0.0, "kind": "footer", "linked": false})
			root = referenceXML(t, out, "word/document.xml")
			if root.FindElement(path+"/w:footerReference") == nil || root.FindElement("w:body/w:sectPr/w:footerReference") != nil {
				t.Error("unlinked wrong section")
			}
			out = mustEdit(t, src, map[string]any{"operation": "section-insert", "section": 0.0, "paragraph": 0.0, "type": "evenPage"})
			root = referenceXML(t, out, "word/document.xml")
			if getW(root.FindElement(path+"/w:type"), "val") != "evenPage" {
				t.Error("insert did not update wrapped following section")
			}
			if getW(root.FindElement("w:body/w:p/w:pPr/w:sectPr/w:pgSz"), "w") != "15840" {
				t.Error("insert copied wrong section")
			}
			if root.FindElement("w:body/w:sectPr/w:type") != nil {
				t.Error("insert changed final section start")
			}
		})
	}
}
