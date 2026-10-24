package office

import (
	"github.com/beevik/etree"
	"testing"
)

func TestTableStyleExplicitFonts(t *testing.T) {
	for _, prefix := range []string{"a", "d"} {
		t.Run(prefix, func(t *testing.T) {
			region := paintFixture(t, `<`+prefix+`:wholeTbl xmlns:`+prefix+`="`+NSA+`"><`+prefix+`:tcTxStyle><`+prefix+`:font><`+prefix+`:latin typeface="Arial"/><`+prefix+`:ea typeface="SimSun"/><`+prefix+`:cs typeface="Tahoma"/></`+prefix+`:font></`+prefix+`:tcTxStyle></`+prefix+`:wholeTbl>`)
			cell := paintFixture(t, `<a:tc xmlns:a="`+NSA+`"><a:txBody><a:p><a:r><a:t>Inherited</a:t></a:r><a:r><a:rPr><a:latin typeface="Calibri"/></a:rPr><a:t>Override</a:t></a:r></a:p></a:txBody></a:tc>`)
			effective, defaults := layoutTableStyleCell(cell, []*etree.Element{region})
			f := layoutTableFormatting(effective, newLayoutPalette(nil, nil), defaults...)
			runs := f["paragraphs"].([]map[string]any)[0]["runs"].([]map[string]any)
			for i, run := range runs {
				style := run["style"].(map[string]any)
				want := "Arial"
				if i == 1 {
					want = "Calibri"
				}
				for tag, family := range map[string]string{"latin": want, "ea": "SimSun", "cs": "Tahoma"} {
					font, ok := style[tag].(map[string]string)
					if !ok || font["typeface"] != family {
						t.Fatalf("%s: %v", tag, style)
					}
				}
			}
		})
	}
}

func TestTableStyleDefaultToggles(t *testing.T) {
	for _, tc := range []struct{ name, region, run, b, i string }{
		{"inherit", `b="def" i="def"`, "", "1", "0"},
		{"override", `b="off" i="on"`, "", "0", "1"},
		{"direct", `b="on" i="off"`, `b="0" i="1"`, "0", "1"},
		{"omitted", "", "", "1", "0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			whole := paintFixture(t, `<a:wholeTbl xmlns:a="`+NSA+`"><a:tcTxStyle b="on" i="off"/></a:wholeTbl>`)
			first := paintFixture(t, `<a:firstRow xmlns:a="`+NSA+`"><a:tcTxStyle `+tc.region+`/></a:firstRow>`)
			cell := paintFixture(t, `<a:tc xmlns:a="`+NSA+`"><a:txBody><a:p><a:r><a:rPr `+tc.run+`/><a:t>Text</a:t></a:r></a:p></a:txBody></a:tc>`)
			effective, defaults := layoutTableStyleCell(cell, []*etree.Element{whole, first})
			f := layoutTableFormatting(effective, newLayoutPalette(nil, nil), defaults...)
			props := f["paragraphs"].([]map[string]any)[0]["runs"].([]map[string]any)[0]["style"].(map[string]any)["properties"].(map[string]string)
			if props["b"] != tc.b || props["i"] != tc.i {
				t.Fatal(props)
			}
		})
	}
}
