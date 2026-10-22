package office

import (
	"fmt"
	"math"
	"strings"
	"unicode"

	"golang.org/x/image/font"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"
)

// TextRun supplies resolved font properties for text measurement in points.
type TextRun struct {
	Text, Font   string
	Size         float64
	Bold, Italic bool
}

// TextHeight measures wrapped text using glyph advances and font line metrics.
// Runs share a line; explicit newlines always start a new line. Font substitution
// is deliberately left to the caller so an unavailable face cannot silently fit.
func TextHeight(runs []TextRun, width float64, wrap bool) (float64, error) {
	if width <= 0 || math.IsNaN(width) || math.IsInf(width, 0) {
		return 0, fmt.Errorf("text width must be positive")
	}
	type glyph struct {
		advance, height float64
		r               rune
	}
	var glyphs []glyph
	for _, r := range runs {
		if r.Size <= 0 || r.Size > 400 || math.IsNaN(r.Size) || math.IsInf(r.Size, 0) {
			return 0, fmt.Errorf("invalid font size")
		}
		f := FindFont(r.Font, r.Bold, r.Italic)
		if f == nil {
			return 0, fmt.Errorf("font unavailable for measurement: %s (bold=%t, italic=%t)", r.Font, r.Bold, r.Italic)
		}
		var b sfnt.Buffer
		m, err := f.Metrics(&b, fixed.Int26_6(r.Size*64), font.HintingNone)
		if err != nil {
			return 0, err
		}
		h := math.Max(float64(m.Height)/64, r.Size*1.2)
		for _, ch := range strings.ReplaceAll(strings.ReplaceAll(r.Text, "\r\n", "\n"), "\r", "\n") {
			if ch == '\n' {
				glyphs = append(glyphs, glyph{height: h, r: ch})
				continue
			}
			if ch == '\t' {
				glyphs = append(glyphs, glyph{Advance(f, "    ", r.Size), h, ch})
				continue
			}
			idx, err := f.GlyphIndex(&b, ch)
			if err != nil || idx == 0 {
				return 0, fmt.Errorf("font %s lacks glyph %U", r.Font, ch)
			}
			glyphs = append(glyphs, glyph{Advance(f, string(ch), r.Size), h, ch})
		}
		if r.Text == "" && len(glyphs) == 0 {
			glyphs = append(glyphs, glyph{height: h})
		}
	}
	lineWidth, lineHeight, total := 0.0, 0.0, 0.0
	flush := func() { total += lineHeight; lineWidth = 0; lineHeight = 0 }
	for i := 0; i < len(glyphs); {
		g := glyphs[i]
		if g.r == '\n' {
			lineHeight = math.Max(lineHeight, g.height)
			flush()
			lineHeight = g.height
			i++
			continue
		}
		// Keep a word together across run boundaries where possible.
		end := i + 1
		if !unicode.IsSpace(g.r) {
			for end < len(glyphs) && !unicode.IsSpace(glyphs[end].r) {
				end++
			}
		}
		wordWidth := 0.0
		for _, v := range glyphs[i:end] {
			wordWidth += v.advance
		}
		if wrap && !unicode.IsSpace(g.r) && lineWidth > 0 && lineWidth+wordWidth > width {
			flush()
		}
		for _, v := range glyphs[i:end] {
			if wrap && lineWidth > 0 && lineWidth+v.advance > width {
				flush()
				if unicode.IsSpace(v.r) {
					continue
				}
			}
			lineWidth += v.advance
			lineHeight = math.Max(lineHeight, v.height)
		}
		i = end
	}
	return total + lineHeight, nil
}
