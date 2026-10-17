package office

import "testing"

func TestPresentationFontsPreserveArchiveOrder(t *testing.T) {
	// Start with a valid package; deliberately reverse lexical order for fonts.
	p := fixture(t, "reference-bar.pptx")
	p.Set("ppt/z-audit.xml", []byte(`<a:root xmlns:a="`+NSA+`"><a:latin typeface="First"/><a:ea typeface="Second"/></a:root>`))
	p.Set("ppt/a-audit.xml", []byte(`<a:root xmlns:a="`+NSA+`"><a:latin typeface="Third"/></a:root>`))
	r, err := PackageReportOf(p)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, item := range r["font_definitions"].([]any) {
		f := item.(map[string]string)
		if f["part"] == "ppt/z-audit.xml" || f["part"] == "ppt/a-audit.xml" {
			got = append(got, f["typeface"])
		}
	}
	if len(got) != 3 || got[0] != "First" || got[1] != "Second" || got[2] != "Third" {
		t.Fatalf("font order = %v", got)
	}
}
