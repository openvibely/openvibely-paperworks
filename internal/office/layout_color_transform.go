package office

import (
	"fmt"
	"github.com/beevik/etree"
	"math"
	"strconv"
	"strings"
)

func clampLayoutColor(v float64) float64 { return math.Max(0, math.Min(1, v)) }
func layoutRGBToHSL(c [3]float64) (h, s, l float64) {
	max := math.Max(c[0], math.Max(c[1], c[2]))
	min := math.Min(c[0], math.Min(c[1], c[2]))
	l = (max + min) / 2
	d := max - min
	if d == 0 {
		return
	}
	s = d / (1 - math.Abs(2*l-1))
	switch max {
	case c[0]:
		h = math.Mod((c[1]-c[2])/d, 6)
	case c[1]:
		h = (c[2]-c[0])/d + 2
	default:
		h = (c[0]-c[1])/d + 4
	}
	h = math.Mod(h*60+360, 360)
	return
}
func layoutHSLToRGB(h, s, l float64) [3]float64 {
	h = math.Mod(math.Mod(h, 360)+360, 360)
	s = clampLayoutColor(s)
	l = clampLayoutColor(l)
	c := (1 - math.Abs(2*l-1)) * s
	x := c * (1 - math.Abs(math.Mod(h/60, 2)-1))
	m := l - c/2
	var rgb [3]float64
	switch {
	case h < 60:
		rgb = [3]float64{c, x, 0}
	case h < 120:
		rgb = [3]float64{x, c, 0}
	case h < 180:
		rgb = [3]float64{0, c, x}
	case h < 240:
		rgb = [3]float64{0, x, c}
	case h < 300:
		rgb = [3]float64{x, 0, c}
	default:
		rgb = [3]float64{c, 0, x}
	}
	for i := range rgb {
		rgb[i] += m
	}
	return rgb
}
func layoutTransformColor(hex string, transforms []*etree.Element) (string, float64, bool) {
	var rgb [3]float64
	for i := range rgb {
		n, e := strconv.ParseUint(hex[i*2:i*2+2], 16, 8)
		if e != nil {
			return "", 0, false
		}
		rgb[i] = float64(n) / 255
	}
	alpha := 1.0
	for _, tr := range transforms {
		if tr.NamespaceURI() != NSA {
			return "", 0, false
		}
		val := 0.0
		switch tr.Tag {
		case "inv", "comp":
		default:
			n, e := strconv.ParseFloat(AttrOr(tr, "val", ""), 64)
			if e != nil || math.IsNaN(n) || math.IsInf(n, 0) {
				return "", 0, false
			}
			val = n / 100000
		}
		switch tr.Tag {
		case "alpha":
			alpha = val
		case "alphaMod":
			alpha *= val
		case "alphaOff":
			alpha += val
		case "tint":
			for i := range rgb {
				rgb[i] = rgb[i]*val + 1 - val
			}
		case "shade":
			for i := range rgb {
				rgb[i] *= val
			}
		case "inv":
			for i := range rgb {
				rgb[i] = 1 - rgb[i]
			}
		case "hue", "hueMod", "hueOff", "sat", "satMod", "satOff", "lum", "lumMod", "lumOff", "comp":
			h, s, l := layoutRGBToHSL(rgb)
			switch tr.Tag {
			case "hue":
				h = val * 100000 / 60000
			case "hueOff":
				h += val * 100000 / 60000
			case "hueMod":
				h *= val
			case "sat":
				s = val
			case "satMod":
				s *= val
			case "satOff":
				s += val
			case "lum":
				l = val
			case "lumMod":
				l *= val
			case "lumOff":
				l += val
			case "comp":
				h += 180
			}
			rgb = layoutHSLToRGB(h, s, l)
		default:
			channel := -1
			for i, name := range []string{"red", "green", "blue"} {
				if tr.Tag == name || tr.Tag == name+"Mod" || tr.Tag == name+"Off" {
					channel = i
					break
				}
			}
			if channel < 0 {
				return "", 0, false
			}
			if strings.HasSuffix(tr.Tag, "Mod") {
				rgb[channel] *= val
			} else if strings.HasSuffix(tr.Tag, "Off") {
				rgb[channel] += val
			} else {
				rgb[channel] = val
			}
		}
		alpha = clampLayoutColor(alpha)
		for i := range rgb {
			rgb[i] = clampLayoutColor(rgb[i])
		}
	}
	return fmt.Sprintf("#%02X%02X%02X", int(math.Round(rgb[0]*255)), int(math.Round(rgb[1]*255)), int(math.Round(rgb[2]*255))), alpha, true
}
