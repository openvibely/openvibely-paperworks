package equation

import (
	"context"
	"crypto/sha256"
	"fmt"
	"testing"
)

// Fresh unchanged equation.cjs; pinned MathJax/Sharp/Cairo glyph coverage.
// Hashes are exact black RGBA masks, no tolerance or encoder-byte comparison.
// MathJax 3.2.2, Sharp 0.34.5/libvips 8.17.3:
// CSS lengths pass through float32, then the SVG viewBox is fitted to the
// rounded canvas. Hashes are decoded RGBA pixels, not encoder-specific PNGs.
func TestPinnedGlyphCoveragePixels(t *testing.T) {
	for _, tc := range []struct{ tex, rgbaSHA256 string }{
		{`a`, "71a23ebef787ab4cdb5425ebf01fd93e29805d64153649fd50e9a25e8c21f0b4"},
		{`b`, "a8f7c30ec3eb643c5c9a28a6c9fe70253c9a9b9df753d5cc1e7467d2a56e109e"},
		{`x`, "dc7e73696880b59369346b33a054bdd4890694051021b45e9048539c63c61151"},
		{`+`, "b8c994780cb8e302555080226bca98c23de8bc50b2d7f241152673f4328f833b"},
		{`\mathrm{abcd}`, "53394f909b13213bd31294da3ea8327b643ed84ca86ab1c64871a9feeb25728b"},
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
