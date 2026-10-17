package office

import "testing"

// document_preview.label_box assigns paragraph.font, not run.font.
func TestPreviewTextInheritsParagraphFont(t *testing.T) {
	italic := true
	var c slideCanvas
	c.textbox(0, 0, 1000000, 1000000, "Histogram", textStyle{size: 8, bold: true, italic: &italic, font: "Arial", colour: "123456", center: true})
	p := c.shapes[0].FindElement("p:txBody/a:p")
	if p == nil {
		t.Fatal("missing paragraph")
	}
	defaults := p.FindElement("a:pPr/a:defRPr")
	if defaults == nil {
		t.Fatal("missing paragraph font")
	}
	for k, want := range map[string]string{"sz": "800", "b": "1", "i": "1"} {
		if got := defaults.SelectAttrValue(k, ""); got != want {
			t.Errorf("%s=%q, want %q", k, got, want)
		}
	}
	if defaults.SelectAttr("lang") != nil {
		t.Error("unexpected language override")
	}
	if p.FindElement("a:r/a:rPr") != nil {
		t.Error("unexpected run override")
	}
	if p.FindElement("a:r/a:t").Text() != "Histogram" {
		t.Error("text changed")
	}
	if defaults.FindElement("a:latin").SelectAttrValue("typeface", "") != "Arial" {
		t.Error("font changed")
	}
	if defaults.FindElement("a:solidFill/a:srgbClr").SelectAttrValue("val", "") != "123456" {
		t.Error("color changed")
	}
}
