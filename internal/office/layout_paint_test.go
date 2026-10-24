package office

import (
	"github.com/beevik/etree"
	"testing"
)

func paintFixture(t *testing.T, xml string) *etree.Element {
	t.Helper()
	d := etree.NewDocument()
	if e := d.ReadFromString(xml); e != nil {
		t.Fatal(e)
	}
	return d.Root()
}
func TestLayoutInheritedPaint(t *testing.T) {
	theme := paintFixture(t, `<a:theme xmlns:a="`+NSA+`"><a:themeElements><a:clrScheme><a:accent1><a:srgbClr val="123456"/></a:accent1></a:clrScheme><a:fmtScheme><a:fillStyleLst><a:solidFill><a:schemeClr val="phClr"/></a:solidFill></a:fillStyleLst></a:fmtScheme></a:themeElements></a:theme>`)
	master := paintFixture(t, `<p:sldMaster xmlns:p="`+NSP+`"><p:clrMap bg1="accent1"/></p:sldMaster>`)
	sh := paintFixture(t, `<p:sp xmlns:p="`+NSP+`" xmlns:a="`+NSA+`"><p:style><a:fillRef idx="1"><a:schemeClr val="bg1"><a:alpha val="50000"/></a:schemeClr></a:fillRef></p:style></p:sp>`)
	p := newLayoutPalette(theme, []*etree.Element{master})
	empty := paintFixture(t, `<p:sp xmlns:p="`+NSP+`"/>`)
	fill := p.shapeFill([]*etree.Element{empty, sh})
	c := fill["color"].(map[string]any)
	if c["resolved"] != true || c["rgb"] != "#123456" || c["alpha"] != .5 {
		t.Fatal(fill)
	}
	direct := paintFixture(t, `<p:sp xmlns:p="`+NSP+`" xmlns:a="`+NSA+`"><p:spPr><a:noFill/></p:spPr></p:sp>`)
	if p.shapeFill([]*etree.Element{direct, sh})["kind"] != "noFill" {
		t.Fatal("local fill must win")
	}
	unsupported := paintFixture(t, `<a:solidFill xmlns:a="`+NSA+`"><a:srgbClr val="123456"><a:unknownTransform val="100"/></a:srgbClr></a:solidFill>`)
	if p.fill(unsupported)["color"].(map[string]any)["resolved"] != false {
		t.Fatal("unsupported effect claimed resolved")
	}
}

func TestLayoutMasterMappingReset(t *testing.T) {
	theme := paintFixture(t, `<a:theme xmlns:a="`+NSA+`"><a:themeElements><a:clrScheme><a:accent1><a:srgbClr val="112233"/></a:accent1><a:accent2><a:srgbClr val="445566"/></a:accent2></a:clrScheme></a:themeElements></a:theme>`)
	master := paintFixture(t, `<p:sldMaster xmlns:p="`+NSP+`"><p:clrMap bg1="accent1"/></p:sldMaster>`)
	layout := paintFixture(t, `<p:sldLayout xmlns:p="`+NSP+`" xmlns:a="`+NSA+`"><p:clrMapOvr><a:overrideClrMapping bg1="accent2"/></p:clrMapOvr></p:sldLayout>`)
	fill := paintFixture(t, `<a:solidFill xmlns:a="`+NSA+`"><a:schemeClr val="bg1"/></a:solidFill>`)
	for _, tc := range []struct{ name, override, want string }{
		{"inherit", "", "#445566"},
		{"master", "<a:masterClrMapping/>", "#112233"},
		{"explicit", "<a:overrideClrMapping bg1=\"accent1\"/>", "#112233"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			slide := paintFixture(t, `<p:sld xmlns:p="`+NSP+`" xmlns:a="`+NSA+`"><p:clrMapOvr>`+tc.override+`</p:clrMapOvr></p:sld>`)
			c := newLayoutPalette(theme, []*etree.Element{slide, layout, master}).color(fill)
			if c["resolved"] != true || c["rgb"] != tc.want {
				t.Fatal(c)
			}
		})
	}
}

func TestLayoutStyleColorNamespaces(t *testing.T) {
	for _, tc := range []struct{ name, decl, prefix string }{
		{"alternate", `xmlns:d="` + NSA + `"`, "d:"},
		{"default", `xmlns="` + NSA + `"`, ""},
		{"conflicting", `xmlns:d="` + NSA + `" xmlns:a="urn:other"`, "d:"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			theme := paintFixture(t, `<a:theme xmlns:a="`+NSA+`"><a:themeElements><a:fmtScheme><a:fillStyleLst><a:solidFill><a:schemeClr val="phClr"><a:alpha val="50000"/></a:schemeClr></a:solidFill></a:fillStyleLst></a:fmtScheme></a:themeElements></a:theme>`)
			shape := paintFixture(t, `<p:sp xmlns:p="`+NSP+`" `+tc.decl+`><p:style><`+tc.prefix+`fillRef idx="1"><`+tc.prefix+`srgbClr val="123456"/></`+tc.prefix+`fillRef></p:style></p:sp>`)
			fill := newLayoutPalette(theme, nil).shapeFill([]*etree.Element{shape})
			c := fill["color"].(map[string]any)
			if c["resolved"] != true || c["rgb"] != "#123456" || c["alpha"] != .5 {
				t.Fatal(fill)
			}
			// The serialized report must retain the same namespace semantics.
			saved := paintFixture(t, fill["xml"].(string))
			again := newLayoutPalette(nil, nil).color(saved)
			if again["rgb"] != "#123456" || again["alpha"] != .5 {
				t.Fatal(again)
			}
		})
	}
}
