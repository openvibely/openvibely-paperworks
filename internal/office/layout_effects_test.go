package office

import (
	"github.com/beevik/etree"
	"testing"
)

func TestLayoutInheritedEffects(t *testing.T) {
	theme := paintFixture(t, `<a:theme xmlns:a="`+NSA+`"><a:themeElements><a:fmtScheme><a:effectStyleLst><a:effectStyle><a:effectLst><a:outerShdw blurRad="12700" dist="25400"><a:schemeClr val="phClr"><a:alpha val="50000"/></a:schemeClr></a:outerShdw></a:effectLst></a:effectStyle></a:effectStyleLst></a:fmtScheme></a:themeElements></a:theme>`)
	inherited := paintFixture(t, `<p:sp xmlns:p="`+NSP+`" xmlns:d="`+NSA+`"><p:style><d:effectRef idx="1"><d:srgbClr val="123456"/></d:effectRef></p:style></p:sp>`)
	local := paintFixture(t, `<p:sp xmlns:p="`+NSP+`" xmlns:a="`+NSA+`"><p:spPr/></p:sp>`)
	palette := newLayoutPalette(theme, nil)
	r := palette.shapeEffects([]*etree.Element{local, inherited})
	c := r["effects"].([]map[string]any)[0]["color"].(map[string]any)
	if r["resolved"] != true || c["rgb"] != "#123456" || c["alpha"] != .5 {
		t.Fatal(r)
	}
	Child(local, NSP, "spPr").CreateElement("a:effectLst")
	r = palette.shapeEffects([]*etree.Element{local, inherited})
	if len(r["effects"].([]map[string]any)) != 0 {
		t.Fatal("empty local effects did not override theme")
	}
}
