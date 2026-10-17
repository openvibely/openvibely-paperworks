package pptx

import (
	"github.com/beevik/etree"
	"testing"
)

func TestTableRunBoldInheritance(t *testing.T) {
	yes, no := true, false
	for _, tc := range []struct {
		name               string
		element, paragraph bool
		run                *bool
		want               string
	}{
		{"inherit", false, false, nil, ""},
		{"element", true, false, nil, "1"},
		{"paragraph", false, true, nil, "1"},
		{"explicit-true", false, false, &yes, "1"},
		{"explicit-false", true, true, &no, "0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := Element{Bold: tc.element, Paragraphs: []Paragraph{{Bold: tc.paragraph, Runs: []Run{{Text: "value", Bold: tc.run}}}}}
			d := etree.NewDocument()
			if err := d.ReadFromString(`<root xmlns:a="` + ans + `">` + authoredTextBody(e, "Arial", true) + `</root>`); err != nil {
				t.Fatal(err)
			}
			props := d.FindElement(".//a:rPr")
			if props == nil || props.SelectAttrValue("b", "") != tc.want {
				t.Fatalf("bold: %v; want %q", props, tc.want)
			}
		})
	}
}

func TestTableRunDecorationInheritance(t *testing.T) {
	for i := 0; i < 50; i++ {
		r := Run{Text: "decoration", Italic: i%2 == 1, Underline: i%3 == 1}
		d := etree.NewDocument()
		e := Element{FontSize: float64(10 + i), Paragraphs: []Paragraph{{Runs: []Run{r}}}}
		if err := d.ReadFromString(`<root xmlns:a="` + ans + `">` + authoredTextBody(e, "Arial", true) + `</root>`); err != nil {
			t.Fatal(err)
		}
		p := d.FindElement(".//a:rPr")
		wantItalic, wantUnderline := "", ""
		if r.Italic {
			wantItalic = "1"
		}
		if r.Underline {
			wantUnderline = "sng"
		}
		if p == nil || p.SelectAttrValue("i", "") != wantItalic || p.SelectAttrValue("u", "") != wantUnderline {
			t.Fatalf("case %d: %v", i, p)
		}
	}
}
