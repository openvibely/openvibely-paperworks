package office

import (
	"math"
	"testing"
)

func TestLayoutPaintTransforms(t *testing.T) {
	for _, tc := range []struct {
		name, hex, xml, want string
		alpha                float64
	}{
		{"tint", "000000", `<a:tint val="25000"/>`, "#BFBFBF", 1},
		{"shade", "FFFFFF", `<a:shade val="25000"/>`, "#404040", 1},
		{"hue", "FF0000", `<a:hueOff val="7200000"/>`, "#00FF00", 1},
		{"luminance", "FF0000", `<a:lumMod val="50000"/>`, "#800000", 1},
		{"saturation", "FF0000", `<a:sat val="0"/>`, "#808080", 1},
		{"opacity", "FF0000", `<a:alpha val="50000"/><a:alphaMod val="50000"/><a:alphaOff val="10000"/>`, "#FF0000", .35},
		{"order", "000000", `<a:tint val="0"/><a:shade val="50000"/>`, "#808080", 1},
		{"channel", "000000", `<a:red val="100000"/><a:blueOff val="50000"/>`, "#FF0080", 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := paintFixture(t, `<a:solidFill xmlns:a="`+NSA+`"><a:srgbClr val="`+tc.hex+`">`+tc.xml+`</a:srgbClr></a:solidFill>`)
			c := newLayoutPalette(nil, nil).color(f)
			if c["resolved"] != true || c["rgb"] != tc.want || math.Abs(c["alpha"].(float64)-tc.alpha) > 1e-9 {
				t.Fatal(c)
			}
		})
	}
}
