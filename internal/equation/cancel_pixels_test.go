package equation

import (
	"context"
	"crypto/sha256"
	"fmt"
	"image/color"
	"testing"
)

// Exact decoded RGBA from unchanged pinned equation.cjs. Supported
// diagonal cancellations are exercised through actual rendering.
// Hashes are image equality, not PNG-container equality.
func TestPinnedDiagonalCancellationPixels(t *testing.T) {
	for _, tc := range []struct {
		name, tex     string
		inline        bool
		width, height int
		hash          string
	}{
		{"cancel", `\cancel{x}`, false, 141, 129, "20aab342af7b7cf9b1b38101f88adbd1ca2dddf5730c48e50ee1f4d2abd04b3a"},
		{"bcancel", `\bcancel{x}`, false, 141, 129, "4ee26b97be44ca089043f3b2183ff8a6668b6bdad8a8d1f8c9c88ec2879a372d"},
		{"xcancel", `\xcancel{x}`, false, 141, 129, "75f510120a9b5a30b1dcd7a2fe7dcdc3341c91437b7ee05685473756c6b294a0"},
		{"sum", `\cancel{a+b}`, false, 301, 161, "5cd0642ef47ddefa4f2800447b1914e324fe42dc8407ee526615325e93738900"},
		{"fraction", `\cancel{\frac{a}{b}}`, false, 180, 265, "f2bf86a6ab88fc6c25f6d4030e76c3bf14fb34733b8aff1e8df645b9c7b21479"},
		{"color", `\color{red}{\cancel{x}}`, false, 141, 129, "42a21c9b7a87c58455102a5a7e00dd75a8a1a644141bb83ce03dc4ecd50c08ea"},
		{"inline", `\cancel{x}`, true, 141, 129, "20aab342af7b7cf9b1b38101f88adbd1ca2dddf5730c48e50ee1f4d2abd04b3a"},
		{"nested", `\cancel{\bcancel{x}}`, false, 194, 182, "0784dca5c6a87dc5c7f4a3d8ff555dfe2af0815fa4f372ca44ee04249cb80382"},
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
