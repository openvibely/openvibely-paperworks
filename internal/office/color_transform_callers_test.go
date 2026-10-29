package office

import (
	"strings"
	"testing"

	"github.com/beevik/etree"
)

func TestSavedColorTransformsMatchAcrossCallers(t *testing.T) {
	palette := newLayoutPalette(nil, nil)
	painter := &chartExPainter{theme: map[string]string{}}
	for _, tc := range []struct {
		name, base, transforms, want string
		alpha                        float64
	}{
		{"tint", "000000", `<a:tint val="25000"/>`, "#BFBFBF", 1},
		{"shade", "FFFFFF", `<a:shade val="25000"/>`, "#404040", 1},
		{"luminance-modulation", "FF0000", `<a:lumMod val="50000"/>`, "#800000", 1},
		{"luminance-offset", "FF0000", `<a:lumOff val="10000"/>`, "#FF3333", 1},
		{"ordered-tint-then-shade", "000000", `<a:tint val="25000"/><a:shade val="50000"/>`, "#606060", 1},
		{"ordered-shade-then-tint", "000000", `<a:shade val="50000"/><a:tint val="25000"/>`, "#BFBFBF", 1},
		{"alpha-remains-layout-only", "FF0000", `<a:alpha val="50000"/><a:alphaMod val="50000"/><a:alphaOff val="10000"/>`, "#FF0000", .35},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fill := paintFixture(t, `<a:solidFill xmlns:a="`+NSA+`"><a:srgbClr val="`+tc.base+`">`+tc.transforms+`</a:srgbClr></a:solidFill>`)
			layout := palette.color(fill)
			if layout["resolved"] != true || layout["rgb"] != tc.want || layout["alpha"] != tc.alpha {
				t.Fatalf("layout paint = %#v, want color %s alpha %v", layout, tc.want, tc.alpha)
			}
			if got, want := painter.saved(fill, "fallback"), strings.TrimPrefix(tc.want, "#"); got != want {
				t.Fatalf("ChartEx paint = %s, want %s", got, want)
			}
		})
	}
}

func TestSavedColorLookupAndFallbacksRemainCallerSpecific(t *testing.T) {
	theme := paintFixture(t, `<a:theme xmlns:a="`+NSA+`"><a:themeElements><a:clrScheme><a:dk1><a:srgbClr val="123456"/></a:dk1></a:clrScheme></a:themeElements></a:theme>`)
	palette := newLayoutPalette(theme, nil)
	painter := &chartExPainter{theme: map[string]string{"dk1": "123456"}}
	for _, tc := range []struct {
		name, color, want string
	}{
		{"theme-alias", `<a:schemeClr val="tx1"/>`, "123456"},
		{"system-last-color", `<a:sysClr val="window" lastClr="ABCDEF"/>`, "ABCDEF"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fill := paintFixture(t, `<a:solidFill xmlns:a="`+NSA+`">`+tc.color+`</a:solidFill>`)
			layout := palette.color(fill)
			if layout["resolved"] != true || layout["rgb"] != "#"+tc.want {
				t.Fatalf("layout paint = %#v, want #%s", layout, tc.want)
			}
			if got := painter.saved(fill, "fallback"); got != tc.want {
				t.Fatalf("ChartEx paint = %s, want %s", got, tc.want)
			}
		})
	}

	for _, fill := range []*etree.Element{
		paintFixture(t, `<a:solidFill xmlns:a="`+NSA+`"><a:schemeClr val="missing"/></a:solidFill>`),
		paintFixture(t, `<a:solidFill xmlns:a="`+NSA+`"><a:srgbClr val="123456"><a:unsupported val="100000"/></a:srgbClr></a:solidFill>`),
	} {
		if got := palette.color(fill)["resolved"]; got != false {
			t.Fatalf("unresolved layout paint = %#v", palette.color(fill))
		}
		if got := painter.saved(fill, "caller-fallback"); got != "caller-fallback" {
			t.Fatalf("ChartEx fallback = %s", got)
		}
	}
}

func TestChartExSeriesAndPointUseSharedTransforms(t *testing.T) {
	painter := &chartExPainter{theme: map[string]string{}, palette: []string{"112233"}}
	seriesNode := paintFixture(t, `<cx:series xmlns:cx="`+NSCX+`" xmlns:a="`+NSA+`"><cx:spPr><a:solidFill><a:srgbClr val="000000"><a:tint val="25000"/></a:srgbClr></a:solidFill></cx:spPr><cx:dataPt idx="1"><cx:spPr><a:solidFill><a:srgbClr val="FFFFFF"><a:shade val="25000"/></a:srgbClr></a:solidFill></cx:spPr></cx:dataPt></cx:series>`)
	frame := paintFixture(t, `<p:graphicFrame xmlns:p="`+NSP+`" xmlns:a="`+NSA+`"><p:xfrm><a:off x="0" y="0"/><a:ext cx="1000000" cy="1000000"/></p:xfrm></p:graphicFrame>`)
	chart := paintFixture(t, `<cx:chart xmlns:cx="`+NSCX+`"><cx:plotArea/></cx:chart>`)
	series := ChartExSeries{
		Type: "treemap",
		dims: &ChartExDims{
			Str:   map[string][]string{"cat": {"series", "point"}},
			Num:   map[string][]float64{"size": {1, 1}},
			Paths: [][]string{{"series"}, {"point"}},
		},
		node: seriesNode,
	}
	shapes, err := painter.paint(frame, chart, series)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]int{}
	for _, shape := range shapes {
		props := shape.SelectElement("p:spPr")
		if props == nil {
			continue
		}
		fill := props.SelectElement("a:solidFill")
		if fill == nil {
			continue
		}
		if color := fill.SelectElement("a:srgbClr"); color != nil {
			got[color.SelectAttrValue("val", "")]++
		}
	}
	if got["BFBFBF"] == 0 || got["404040"] == 0 {
		t.Fatalf("rendered ChartEx series and point colors = %#v, want BFBFBF and 404040", got)
	}
}
