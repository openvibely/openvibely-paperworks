package equation

import (
	"context"
	"crypto/sha256"
	"fmt"
	"testing"
)

// Unchanged equation.cjs, MathJax 3.2.2, Sharp 0.34.5/libvips 8.17.3:
// CSS lengths pass through float32, then the SVG viewBox is fitted to the
// rounded canvas. Hashes are decoded RGBA pixels, not encoder-specific PNGs.
func TestPinnedViewportFitPixels(t *testing.T) {
	for _, tc := range []struct{ tex, rgbaSHA256 string }{
		{`\rule{0.101em}{0.102em}`, "98a4edd4b748ae4970df589b4497465bd7a96cf7ab30ac2e3050fb1094c8ef8b"},
		{`\rule{0.104em}{0.106em}`, "5a7c61024b94d136e0dc6d37971f263ae041f48346526c44ac6123eba95a4d4d"},
	} {
		t.Run(tc.tex, func(t *testing.T) {
			b, err := PNG(context.Background(), tc.tex, Defaults())
			if err != nil {
				t.Fatal(err)
			}
			img := decode(t, b)
			h := sha256.New()
			for y := img.Bounds().Min.Y; y < img.Bounds().Max.Y; y++ {
				for x := img.Bounds().Min.X; x < img.Bounds().Max.X; x++ {
					r, g, b, a := img.At(x, y).RGBA()
					h.Write([]byte{byte(r >> 8), byte(g >> 8), byte(b >> 8), byte(a >> 8)})
				}
			}
			if got := fmt.Sprintf("%x", h.Sum(nil)); got != tc.rgbaSHA256 {
				t.Fatalf("decoded RGBA hash %s, want pinned %s", got, tc.rgbaSHA256)
			}
		})
	}
}

func TestPinnedCSSLengthPrecision(t *testing.T) {
	for _, tc := range []struct {
		tex  string
		w, h int
	}{
		{`+`, 108, 96}, {`\rule{0.105em}{0.205em}`, 41, 50}, {`\rule{0.475em}{0.665em}`, 78, 96},
	} {
		t.Run(tc.tex, func(t *testing.T) {
			b, e := PNG(context.Background(), tc.tex, Defaults())
			if e != nil {
				t.Fatal(e)
			}
			got := decode(t, b).Bounds()
			if got.Dx() != tc.w || got.Dy() != tc.h {
				t.Fatalf("%v, want %dx%d", got, tc.w, tc.h)
			}
		})
	}
}
