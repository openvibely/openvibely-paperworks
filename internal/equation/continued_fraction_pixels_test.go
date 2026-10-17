package equation

import (
	"context"
	"crypto/sha256"
	"fmt"
	"image/color"
	"testing"
)

// Exact RGBA from the unchanged pinned helper: continued-fraction struts,
// surrounding style, alignment, nesting, and invisible vertical metrics.
func TestPinnedContinuedFractionPixels(t *testing.T) {
	for _, tc := range []struct {
		name, tex     string
		inline        bool
		width, height int
		hash          string
	}{
		{"continued", `\cfrac{1}{1+\cfrac{1}{x}}`, false, 347, 376, "85a9b1684cd1177965695182d2823e28b9582c488f3f82270b7d8adc2d2b3025"},
		{"simple", `\cfrac{a}{b}`, false, 127, 304, "aa80e10ddb6f691902799855eb4f1d3e881feddfb1a14c7402a663c5caf717cc"},
		{"inline", `\cfrac{a}{b}`, true, 127, 221, "b906212fbe012c067a6302afa38e93021f98c0f53f461185c1aa5a2568959e7f"},
		{"scripts", `\cfrac{x^2}{y_i}`, false, 175, 304, "0e9650958654b346b15b858828b7b1fdd8d5b5a28330568b29ed00a7c51a5a80"},
		{"tall", `\cfrac{\frac{a}{b}}{\frac{c}{d}}`, false, 155, 314, "2abf0b18a6c18635ca64f353bef89b21328e82a3348e365d0c6ec51bef517f2a"},
		{"fraction", `\frac{\cfrac{a}{b}}{x}`, true, 158, 256, "90100ebc5cc42fd145ea2ff5fe589cf2bdda59319a4240c213e4ae4ce23830e2"},
		{"color", `\color{red}{\cfrac{a}{b}}`, false, 127, 304, "c8188e3d792c244cd890a526cbd67a97528c5713250f390bb03256d84d69a80e"},
		{"mathstrut", `\mathstrut x`, false, 87, 130, "5c426cf29d128763e17498f00426e2db0ffe88cb0f6e8fff73cae09ff37b2091"},
		{"vphantom", `\vphantom{\frac{a}{b}}+x`, false, 209, 211, "27c7e6d09a2191f75831f557d89b8aab0dfcf0b635e417de4195682a028cf928"},
		{"left", `\cfrac[l]{a}{b+c}`, false, 282, 304, "835574913c89f413a67a924a6f3e940c36be0de33a252ad00f0cd45e12a1b55d"},
		{"right", `\cfrac[r]{a+b}{c}`, true, 292, 221, "875d974a2e52423c5e342bc84c643181a4587b0a742b5f2a137aa533f59b9d67"},
		{"space-align", `\cfrac[ l ]{a+b}{c}`, false, 292, 304, "7c2cf5518893df40da2469ab87e7840e17752516c1b387516f3597f951a8687c"},
		{"empty-align", `\cfrac[]{a}{b}`, false, 127, 304, "aa80e10ddb6f691902799855eb4f1d3e881feddfb1a14c7402a663c5caf717cc"},
		{"deep", `x^{\cfrac{a}{b}}`, false, 180, 198, "6231baf167949b1dcfe59e35b87b218045eaa8de135ec9ba23cbf1efc7d76ff4"},
		{"vphantom-script", `x^{\vphantom{\frac{a}{b}}+y}`, false, 185, 122, "cb9ee1185934eda2ca8040a8d0f633c5fa19c48026b9fbe6c5b72a2fe0e6e358"},
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
