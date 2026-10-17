package equation

import (
	"context"
	"crypto/sha256"
	"fmt"
	"testing"
)

// Fresh unmodified equation.cjs output. Exact decoded black RGBA, not PNG bytes.
func TestPinnedFractionPixels(t *testing.T) {
	for _, c := range []struct {
		name          string
		inline        bool
		width, height int
		hash          string
	}{
		{"display", false, 292, 244, "04e109bfccdc269b3bb7a0f0c7c5e51029d095a6a316e7bda7f35d0d199a3a89"},
		{"inline", true, 197, 159, "faa8c9eb0ed37496cde4d7a3cbb17322461f71027009bd4c0c01b1dbaf2f2604"},
	} {
		t.Run(c.name, func(t *testing.T) {
			o := Defaults()
			o.Inline = c.inline
			b, e := PNG(context.Background(), `\frac{a+b}{c+d}`, o)
			if e != nil {
				t.Fatal(e)
			}
			im := decode(t, b)
			if im.Bounds().Dx() != c.width || im.Bounds().Dy() != c.height {
				t.Fatal(im.Bounds())
			}
			h := sha256.New()
			for y := 0; y < c.height; y++ {
				for x := 0; x < c.width; x++ {
					r, g, b, a := im.At(x, y).RGBA()
					h.Write([]byte{byte(r >> 8), byte(g >> 8), byte(b >> 8), byte(a >> 8)})
				}
			}
			if got := fmt.Sprintf("%x", h.Sum(nil)); got != c.hash {
				t.Fatalf("pixels %s; want %s", got, c.hash)
			}
		})
	}
}
