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
