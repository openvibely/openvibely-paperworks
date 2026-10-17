package equation

import "testing"

// Dimensions from the unchanged equation.cjs used by runtime271, not a
// tolerance or an assertion of glyph raster equivalence.
func TestRuntimeFractionGlyphDimensions(t *testing.T) {
	for _, tc := range []struct {
		inline bool
		w, h   int
	}{{false, 292, 244}, {true, 197, 159}} {
		o := Defaults()
		o.Inline = tc.inline
		im := imageFor(t, `\frac{a+b}{c+d}`, o)
		if im.Bounds().Dx() != tc.w || im.Bounds().Dy() != tc.h {
			t.Fatalf("inline=%v: %v want %dx%d", tc.inline, im.Bounds(), tc.w, tc.h)
		}
	}
}

func TestMathJaxOrdinaryGlyphDimensions(t *testing.T) {
	im := imageFor(t, `x`, Defaults())
	if im.Bounds().Dx() != 87 || im.Bounds().Dy() != 75 {
		t.Fatalf("%v want 87x75 from pinned helper", im.Bounds())
	}
}
