package pptx

import (
	"fmt"
	"math"
)

// imagePlacement preserves the embedded source and expresses cropping entirely
// in native DrawingML srcRect coordinates. Cover adds an anchored crop inside
// the explicit source crop, rather than stretching the visible pixels.
func imagePlacement(e Element, width, height int) (Element, string, error) {
	if e.Fit != "" && e.Fit != "contain" && e.Fit != "cover" {
		return e, "", fmt.Errorf("image fit must be contain or cover")
	}
	ax, ay := 0.5, 0.5
	if e.Anchor != nil {
		ax, ay = e.Anchor.X, e.Anchor.Y
		if !finite(ax) || !finite(ay) || ax < 0 || ax > 1 || ay < 0 || ay > 1 {
			return e, "", fmt.Errorf("image anchor coordinates must be finite and in [0,1]")
		}
	}
	c := Crop{}
	if e.Crop != nil {
		c = *e.Crop
	}
	for _, v := range []float64{c.Left, c.Top, c.Right, c.Bottom} {
		if !finite(v) || v < 0 || v >= 1 {
			return e, "", fmt.Errorf("image crop fractions must be finite and in [0,1)")
		}
	}
	if c.Left+c.Right >= 1 || c.Top+c.Bottom >= 1 {
		return e, "", fmt.Errorf("image crop removes all source pixels")
	}
	sw, sh := 1-c.Left-c.Right, 1-c.Top-c.Bottom
	ratio := float64(width) * sw / (float64(height) * sh)
	if e.Fit == "cover" {
		if e.W/e.H > ratio {
			remove := sh * (1 - ratio/(e.W/e.H))
			c.Top += remove * ay
			c.Bottom += remove * (1 - ay)
		} else {
			remove := sw * (1 - (e.W/e.H)/ratio)
			c.Left += remove * ax
			c.Right += remove * (1 - ax)
		}
	} else if e.W/e.H > ratio {
		w := e.H * ratio
		e.X += (e.W - w) * ax
		e.W = w
	} else {
		h := e.W / ratio
		e.Y += (e.H - h) * ay
		e.H = h
	}
	l, t, r, b := int(math.Round(c.Left*100000)), int(math.Round(c.Top*100000)), int(math.Round(c.Right*100000)), int(math.Round(c.Bottom*100000))
	if l+r >= 100000 || t+b >= 100000 {
		return e, "", fmt.Errorf("image crop is too small at DrawingML precision")
	}
	rect := ""
	if l+t+r+b > 0 {
		rect = fmt.Sprintf(`<a:srcRect l="%d" t="%d" r="%d" b="%d"/>`, l, t, r, b)
	}
	return e, rect, nil
}
