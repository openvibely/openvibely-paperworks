package equation

import (
	"context"
	"crypto/sha256"
	"fmt"
	"image/color"
	"testing"
)

// Exact decoded RGBA from unchanged pinned equation.cjs. Four MathChoice
// selections and invisible-body metrics are exercised through actual rendering.
// Hashes are image equality, not PNG-container equality.
func TestPinnedChoiceAndPhantomPixels(t *testing.T) {
	for _, tc := range []struct {
		name, tex     string
		inline        bool
		width, height int
		hash          string
	}{
		{"choice-display", `\mathchoice{A}{B}{C}{D}`, false, 105, 102, "788e9504696027bb69758a25744b653fb59d6cacd795944dc292c985ee29217b"},
		{"choice-inline", `\mathchoice{A}{B}{C}{D}`, true, 106, 98, "7464b318d33bb8d19790d4c8c618b4dba7284f0f69cdad953553de1de474025f"},
		{"choice-script", `\frac{\mathchoice{A}{B}{C}{D}}{x}`, true, 128, 155, "d851412f9ce1840fc0e7bad3a367f9299719a920b1ceff6aad9f52c112336593"},
		{"choice-scriptscript", `\frac{\frac{\mathchoice{A}{B}{C}{D}}{x}}{y}`, true, 147, 200, "0b53c02e83472929c08603e35fb2fe06055b5dbeec733db2783a84d9528df61e"},
		{"phantom", `\phantom{x}+y`, false, 258, 109, "91eed81e2ef6482ff719f5088de9ca9df503bb58e26560c1c47c263c1a8f2269"},
		{"phantom-tall", `\phantom{\frac{a+b}{c+d}}+x`, false, 471, 244, "ae1376da8073a4d0c8bcfa742b54fd432cf409e5de12df8a4abf5bc9de58201a"},
		{"choice-color", `\color{red}{\mathchoice{A}{B}{C}{D}}`, false, 105, 102, "3490dcf0eb7424df24124c85a2bf557279f8f01b699d83c4897db739e0777a71"},
		{"phantom-roman", `\mathrm{\phantom{ab}+c}`, false, 302, 108, "d6d9ab8fe0c8e375b977ed9a8cdf0adeea2669804f9d9fada20e6545125e74d0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o := Defaults()
			o.Inline = tc.inline
			raw, err := PNG(context.Background(), tc.tex, o)
			if err != nil {
				t.Fatal(err)
			}
			im := decode(t, raw)
			if im.Bounds().Dx() != tc.width || im.Bounds().Dy() != tc.height {
				t.Fatalf("dimensions %v, want %dx%d", im.Bounds(), tc.width, tc.height)
			}
			h := sha256.New()
			for y := im.Bounds().Min.Y; y < im.Bounds().Max.Y; y++ {
				for x := im.Bounds().Min.X; x < im.Bounds().Max.X; x++ {
					c := color.NRGBAModel.Convert(im.At(x, y)).(color.NRGBA)
					h.Write([]byte{c.R, c.G, c.B, c.A})
				}
			}
			if got := fmt.Sprintf("%x", h.Sum(nil)); got != tc.hash {
				t.Fatalf("RGBA hash %s, want %s", got, tc.hash)
			}
		})
	}
}
