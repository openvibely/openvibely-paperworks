package equation

import (
	"image"
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
