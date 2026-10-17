package equation

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"image/color"
	"os"
	"testing"
)

// Exact decoded RGBA from unchanged pinned equation.cjs. Supported
// text, font and negation families are exercised through actual rendering.
// Hashes are image equality, not PNG-container equality.
func TestPinnedSharedFontPixels(t *testing.T) {
	var fixture struct {
		Cases []struct {
			Name   string `json:"name"`
			Tex    string `json:"tex"`
			Width  int    `json:"width"`
			Height int    `json:"height"`
			Hash   string `json:"reference_rgba_sha256"`
		}
	}
	raw, err := os.ReadFile("testdata/shared-font-pixels.json")
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	for _, tc := range fixture.Cases {
		t.Run(tc.Name, func(t *testing.T) {
			o := Defaults()
			raw, err := PNG(context.Background(), tc.Tex, o)
			if err != nil {
				t.Fatal(err)
			}
			im := decode(t, raw)
			if im.Bounds().Dx() != tc.Width || im.Bounds().Dy() != tc.Height {
				t.Fatalf("dimensions %v, want %dx%d", im.Bounds(), tc.Width, tc.Height)
			}
			h := sha256.New()
			for y := im.Bounds().Min.Y; y < im.Bounds().Max.Y; y++ {
				for x := im.Bounds().Min.X; x < im.Bounds().Max.X; x++ {
					c := color.NRGBAModel.Convert(im.At(x, y)).(color.NRGBA)
					h.Write([]byte{c.R, c.G, c.B, c.A})
				}
			}
			if got := fmt.Sprintf("%x", h.Sum(nil)); got != tc.Hash {
				t.Fatalf("RGBA hash %s, want %s", got, tc.Hash)
			}
		})
	}
}
