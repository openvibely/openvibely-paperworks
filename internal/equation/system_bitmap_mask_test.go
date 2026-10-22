package equation

import (
	"bytes"
	"image"
	"image/color"
	"math"
	"testing"
)

func TestSystemBitmapMaskPlacement(t *testing.T) {
	// Asymmetric alpha distinguishes y orientation and retains nonzero bounds.
	im := image.NewAlpha(image.Rect(4, 7, 6, 9))
	copy(im.Pix, []byte{0, 64, 128, 255})
	mask, x, y, err := systemBitmapMask(systemBitmap{Image: im}, 3, -2, 2, 2)
	if err != nil || x != 3 || y != 0 || string(mask.Pix) != string(im.Pix) {
		t.Fatalf("identity placement %v %v %v %v", mask, x, y, err)
	}
	shifted, x, y, err := systemBitmapMask(systemBitmap{Image: im}, 3.5, -2, 2, 2)
	if err != nil || x != 3 || y != 0 || shifted.Bounds() != image.Rect(0, 0, 3, 2) {
		t.Fatalf("fractional bounds %v %v %v %v", shifted, x, y, err)
	}
	want := []byte{0, 32, 32, 64, 192, 128}
	if string(shifted.Pix) != string(want) {
		t.Fatalf("fractional interpolation %v want %v", shifted.Pix, want)
	}
	// Downsample pixel centers: the single pixel averages the four source samples.
	small, _, _, err := systemBitmapMask(systemBitmap{Image: im}, 0, 0, 1, 1)
	if err != nil || small.Pix[0] != 112 {
		t.Fatalf("downsample %v %v", small, err)
	}
	for _, size := range []float64{0, -1, 4097, math.Inf(1), math.NaN()} {
		if _, _, _, err := systemBitmapMask(systemBitmap{Image: im}, 0, 0, size, 2); err == nil {
			t.Fatalf("accepted size %v", size)
		}
	}
	if _, _, _, err := systemBitmapMask(systemBitmap{}, 0, 0, 2, 2); err == nil {
		t.Fatal("accepted nil image")
	}
}

// Hide the concrete type to compare Pix fast paths with Image.At, including
// subimage strides, nonzero bounds, and the fallback's 16-bit alpha precision.
type genericBitmapImage struct{ image.Image }

func TestBitmapAlphaFastPaths(t *testing.T) {
	rect := image.Rect(-3, 4, 4, 10)
	nrgba := image.NewNRGBA(rect)
	rgba := image.NewRGBA(rect)
	rgba64 := image.NewRGBA64(rect)
	for y := rect.Min.Y; y < rect.Max.Y; y++ {
		for x := rect.Min.X; x < rect.Max.X; x++ {
			a := uint8((x+3)*31 + (y-4)*7)
			nrgba.SetNRGBA(x, y, color.NRGBA{A: a})
			rgba.SetRGBA(x, y, color.RGBA{A: a})
			rgba64.SetRGBA64(x, y, color.RGBA64{A: uint16(a)*257 + 13})
		}
	}
	sub := image.Rect(-2, 5, 3, 9)
	for _, im := range []image.Image{nrgba.SubImage(sub), rgba.SubImage(sub), rgba64.SubImage(sub)} {
		for _, quantized := range []bool{false, true} {
			got, gx, gy, err := sampleSystemBitmap(systemBitmap{Image: im}, .3, -.7, 17.2, 19.4, quantized)
			if err != nil {
				t.Fatal(err)
			}
			want, wx, wy, err := sampleSystemBitmap(systemBitmap{Image: genericBitmapImage{im}}, .3, -.7, 17.2, 19.4, quantized)
			if err != nil {
				t.Fatal(err)
			}
			if gx != wx || gy != wy || !bytes.Equal(got.Pix, want.Pix) {
				t.Fatalf("alpha mismatch for %T quantized=%v", im, quantized)
			}
		}
	}
}
