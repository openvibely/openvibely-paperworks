package equation

import (
	"context"
	"testing"
)

// Dimensions from unchanged equation.cjs (MathJax 3.2.2 / Sharp 0.34.5).
// Rules isolate SVG-unit and pixel rounding from the separate font-metric gap.
func TestPinnedEquationViewportDimensions(t *testing.T) {
	// Original runtime271 renders this fraction in both modes. The reference
	// image sizes differ by layout mode; the old ceiling added an inline row.
	for _, tc := range []struct {
		name          string
		inline        bool
		width, height int
	}{{"runtime-display", false, 292, 244}, {"runtime-inline", true, 197, 159}} {
		t.Run(tc.name, func(t *testing.T) {
			o := Defaults()
			o.Inline = tc.inline
			b, err := PNG(context.Background(), `\frac{a+b}{c+d}`, o)
			if err != nil {
				t.Fatal(err)
			}
			bounds := decode(t, b).Bounds()
			if bounds.Dx() != tc.width || bounds.Dy() != tc.height {
				t.Fatalf("dimensions=%v, want %dx%d", bounds, tc.width, tc.height)
			}
		})
	}
	for _, tc := range []struct {
		tex           string
		width, height int
	}{
		{`\rule{1em}{0.4em}`, 130, 70},
		{`\rule{0.101em}{0.102em}`, 40, 40},
		{`\rule{0.104em}{0.106em}`, 40, 41},
		{`\rule{0.33333em}{0.22222em}`, 63, 52},
		{`\rule{0.0009em}{0.0008em}`, 32, 32},
		{`\rule{1.00004em}{0.40004em}`, 130, 70},
	} {
		t.Run(tc.tex, func(t *testing.T) {
			for _, inline := range []bool{false, true} {
				o := Defaults()
				o.Inline = inline
				b, err := PNG(context.Background(), tc.tex, o)
				if err != nil {
					t.Fatal(err)
				}
				bounds := decode(t, b).Bounds()
				if bounds.Dx() != tc.width || bounds.Dy() != tc.height {
					t.Fatalf("inline=%v dimensions=%v, want %dx%d", inline, bounds, tc.width, tc.height)
				}
			}
		})
	}
}

// MathJax's mpadded (smash/lap) changes the declared box, not the ink.
// A tight-ink canvas incorrectly undoes that deliberate layout operation.
func TestPinnedEquationNominalBounds(t *testing.T) {
	for _, tc := range []struct {
		tex  string
		w, h int
	}{
		{`\smash{\rule{1em}{1em}}`, 130, 32},
		{`\smash[t]{\rule{1em}{1em}}`, 130, 32},
		{`\smash[b]{\rule[-.5em]{1em}{1em}}`, 130, 80},
		{`\llap{\rule{1em}{1em}}\rule{.3em}{.3em}`, 60, 130},
	} {
		t.Run(tc.tex, func(t *testing.T) {
			data, err := PNG(context.Background(), tc.tex, Defaults())
			if err != nil {
				t.Fatal(err)
			}
			b := decode(t, data).Bounds()
			if b.Dx() != tc.w || b.Dy() != tc.h {
				t.Fatalf("bounds %v, want %dx%d", b, tc.w, tc.h)
			}
		})
	}
}
