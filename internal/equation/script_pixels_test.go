package equation

import (
	"context"
	"crypto/sha256"
	"fmt"
	"image/color"
	"testing"
)

// Exact decoded RGBA from unchanged pinned equation.cjs, including italic
// correction, nested script styles and fraction children.
// Hashes are image equality, not PNG-container equality.
func TestPinnedCharacterScriptPixels(t *testing.T) {
	for _, tc := range []struct {
		name, tex     string
		inline        bool
		width, height int
		hash          string
	}{
		{"sup", `x^2`, false, 131, 119, "bd504965d3c566db2e8107af673e5aac1b5633423bf24a6bf05750533015987d"},
		{"color", `\color{red}{x^2}`, false, 131, 119, "1011a1c2ee923b67c01c67ef2f030131264dad272212c343d0b2d48bf7a7b8b9"},
		{"inline", `x^2`, true, 131, 114, "3ffe5f6f96ec5ff93d36cd5960a703a2efba3bf615de22661303c2108fd8f626"},
		{"sub", `x_i`, false, 120, 90, "4851be980ed8f9f68044ee59532698e5478745ded2644afdb17e3e2b2c0ded34"},
		{"pair", `x_i^2`, false, 131, 144, "1f0e1bf10bad6fe7ae4151f97dd1d7a06dfd38825b84dee8badfb7b9decaf78a"},
		{"italic", `f_i^j`, false, 128, 157, "e6ee9da9e1c0a61c94cc658a67482d63dd07457d70fad1464a276102ff29de4e"},
		{"nested", `x^{y^2}`, false, 161, 131, "f29a15a4ff34d2e44c73d1b64eb198efdefe0f095499da3301c8bcfc9bc25b64"},
		{"fraction", `\frac{x^2}{y_i}`, false, 175, 270, "81f428713550453c2900a0e9fafd01a443f5b3098fe01998ebea4a58762b55b3"},
		{"tall", `x_{\frac{a}{b}}^{\frac{c}{d}}`, false, 153, 198, "fa28d97262ef944450e60cfb3588871780c580b055818a4ee8f1a985bc236b0d"},
		{"roman", `\mathrm{f}_i^2`, false, 116, 144, "5127735e435f56330980adbb21ee77cd20dc226501738faa74704659b7331fdb"},
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
