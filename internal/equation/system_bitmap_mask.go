package equation

import (
	"fmt"
	"image"
	"math"
)

// systemBitmapMask samples an explicitly selected strike into a y-up glyph
// rectangle, then stores the alpha mask in top-down image order. Placement is
// SVG-local, before the final viewport transform. This entry uses bilinear
// sampling; the emoji text-path entry selects the Quartz quantized filter.
// The returned origin locates the top-left mask pixel relative to the baseline.
func systemBitmapMask(bitmap systemBitmap, x, y, width, height float64) (*image.Alpha, float64, float64, error) {
	return sampleSystemBitmap(bitmap, x, y, width, height, false)
}

func systemEmojiMask(bitmap systemBitmap, x, y, width, height float64) (*image.Alpha, float64, float64, error) {
	return sampleSystemBitmap(bitmap, x, y, width, height, true)
}

func sampleSystemBitmap(bitmap systemBitmap, x, y, width, height float64, quantized bool) (*image.Alpha, float64, float64, error) {
	for _, v := range []float64{x, y, width, height} {
		if math.IsNaN(v) || math.IsInf(v, 0) || math.Abs(v) > 1e6 {
			return nil, 0, 0, fmt.Errorf("invalid bitmap placement")
		}
	}
	if bitmap.Image == nil || width <= 0 || height <= 0 {
		return nil, 0, 0, fmt.Errorf("empty bitmap placement")
	}
	bounds := bitmap.Image.Bounds()
	if bounds.Dx() <= 0 || bounds.Dy() <= 0 || bounds.Dx() > 4096 || bounds.Dy() > 4096 {
		return nil, 0, 0, fmt.Errorf("invalid bitmap dimensions")
	}
	left, top := math.Floor(x), math.Floor(-y-height)
	w, h := int(math.Ceil(x+width)-left), int(math.Ceil(-y)-top)
	if w <= 0 || h <= 0 || w > 4096 || h > 4096 {
		return nil, 0, 0, fmt.Errorf("bitmap mask exceeds bounds")
	}
	out := image.NewAlpha(image.Rect(0, 0, w, h))
	alpha := func(px, py int) float64 {
		if px < 0 || py < 0 || px >= bounds.Dx() || py >= bounds.Dy() {
			return 0
		}
		_, _, _, a := bitmap.Image.At(px+bounds.Min.X, py+bounds.Min.Y).RGBA()
		return float64(a) / 257
	}
	for row := 0; row < h; row++ {
		sy := (top+float64(row)+.5+y+height)*float64(bounds.Dy())/height - .5
		iy := int(math.Floor(sy))
		fy := sy - float64(iy)
		for col := 0; col < w; col++ {
			sx := (left+float64(col)+.5-x)*float64(bounds.Dx())/width - .5
			ix := int(math.Floor(sx))
			fx := sx - float64(ix)
			a := (alpha(ix, iy)*(1-fx)+alpha(ix+1, iy)*fx)*(1-fy) + (alpha(ix, iy+1)*(1-fx)+alpha(ix+1, iy+1)*fx)*fy
			if quantized {
				a = bitmapInterpolate(bitmapInterpolate(alpha(ix, iy), alpha(ix, iy+1), fy), bitmapInterpolate(alpha(ix+1, iy), alpha(ix+1, iy+1), fy), fx)
			}
			out.Pix[row*out.Stride+col] = uint8(math.Floor(a + .5))
		}
	}
	return out, left, top, nil
}

// Quartz's default upsampler quantizes the fractional coordinate to eighths,
// then uses power-of-two weights (0,1/16,1/8,1/4,1/2,3/4,7/8,15/16,1).
// Shift the two samples separately: rounding their difference is not equivalent.
// The midpoint belongs to the lower sample. Vertical sampling precedes horizontal.
// Confirmed against independent alpha ramps and the original font bitmap.
func bitmapInterpolate(a, b, f float64) float64 {
	q := int(math.Floor(f*8 + .5))
	if q == 0 {
		return a
	}
	if q == 8 {
		return b
	}
	if f <= .5 {
		return a + math.Floor(b/float64(uint(1)<<uint(5-q))) - math.Floor(a/float64(uint(1)<<uint(5-q)))
	}
	return b + math.Floor(a/float64(uint(1)<<uint(q-3))) - math.Floor(b/float64(uint(1)<<uint(q-3)))
}
