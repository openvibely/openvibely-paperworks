package office

import (
	"math"
	"strconv"
	"strings"

	"github.com/beevik/etree"
)

func clampColorChannel(v float64) float64 { return math.Max(0, math.Min(1, v)) }

func colorRGBToHSL(c [3]float64) (h, s, l float64) {
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

func colorHSLToRGB(h, s, l float64) [3]float64 {
	h = math.Mod(math.Mod(h, 360)+360, 360)
	s = clampColorChannel(s)
	l = clampColorChannel(l)
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

// drawingMLColorTransforms applies saved DrawingML transforms in document order.
// Alpha is returned separately so callers that do not paint opacity can ignore it.
func drawingMLColorTransforms(rgb [3]float64, transforms []*etree.Element) ([3]float64, float64, bool) {
	alpha := 1.0
	for _, tr := range transforms {
		if tr.NamespaceURI() != NSA {
			return [3]float64{}, 0, false
		}
		val := 0.0
		switch tr.Tag {
		case "inv", "comp":
		default:
			n, err := strconv.ParseFloat(AttrOr(tr, "val", ""), 64)
			if err != nil || math.IsNaN(n) || math.IsInf(n, 0) {
				return [3]float64{}, 0, false
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
			h, s, l := colorRGBToHSL(rgb)
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
			rgb = colorHSLToRGB(h, s, l)
		default:
			channel := -1
			for i, name := range []string{"red", "green", "blue"} {
				if tr.Tag == name || tr.Tag == name+"Mod" || tr.Tag == name+"Off" {
					channel = i
					break
				}
			}
			if channel < 0 {
				return [3]float64{}, 0, false
			}
			if strings.HasSuffix(tr.Tag, "Mod") {
				rgb[channel] *= val
			} else if strings.HasSuffix(tr.Tag, "Off") {
				rgb[channel] += val
			} else {
				rgb[channel] = val
			}
		}
		alpha = clampColorChannel(alpha)
		for i := range rgb {
			rgb[i] = clampColorChannel(rgb[i])
		}
	}
	return rgb, alpha, true
}
