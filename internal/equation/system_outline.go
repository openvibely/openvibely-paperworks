package equation

import (
	"fmt"

	"github.com/luo-studio/go-tex/tex/path"
	"golang.org/x/image/font"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"
)

// systemOutline loads an explicitly selected face, without host font discovery,
// substitution or hinting. Coordinates are in em units with a downward y axis.
// This is an outline primitive, not a Pango shaping or color-bitmap renderer.
// The caller owns the font bytes and the separately established face selection.
func systemOutline(data []byte, face int, scalar rune) ([]path.Command, float64, error) {
	if len(data) == 0 || len(data) > 256<<20 || face < 0 || scalar < 0 || scalar > 0x10ffff || scalar >= 0xd800 && scalar <= 0xdfff {
		return nil, 0, fmt.Errorf("invalid system font input")
	}
	collection, err := sfnt.ParseCollection(data)
	if err != nil {
		return nil, 0, err
	}
	f, err := collection.Font(face)
	if err != nil {
		return nil, 0, err
	}
	var buffer sfnt.Buffer
	glyph, err := f.GlyphIndex(&buffer, scalar)
	if err != nil {
		return nil, 0, err
	}
	if glyph == 0 {
		return nil, 0, fmt.Errorf("system font has no glyph for U+%04X", scalar)
	}
	units := f.UnitsPerEm()
	if units == 0 {
		return nil, 0, fmt.Errorf("zero font units")
	}
	size := fixed.Int26_6(units) * 64
	segments, err := f.LoadGlyph(&buffer, glyph, size, nil)
	if err != nil {
		return nil, 0, err
	}
	if len(segments) == 0 || len(segments) > 100000 {
		return nil, 0, fmt.Errorf("system glyph has no supported outline or exceeds limit")
	}
	advance, err := f.GlyphAdvance(&buffer, glyph, size, font.HintingNone)
	if err != nil {
		return nil, 0, err
	}
	scale := 1 / (float64(units) * 64)
	commands := make([]path.Command, 0, len(segments)+8)
	open := false
	for _, s := range segments {
		x, y := float64(s.Args[0].X)*scale, float64(s.Args[0].Y)*scale
		switch s.Op {
		case sfnt.SegmentOpMoveTo:
			if open {
				commands = append(commands, path.Close())
			}
			commands = append(commands, path.MoveTo(x, y))
			open = true
		case sfnt.SegmentOpLineTo:
			commands = append(commands, path.LineTo(x, y))
		case sfnt.SegmentOpQuadTo:
			commands = append(commands, path.QuadTo(x, y, float64(s.Args[1].X)*scale, float64(s.Args[1].Y)*scale))
		case sfnt.SegmentOpCubeTo:
			commands = append(commands, path.CubicTo(x, y, float64(s.Args[1].X)*scale, float64(s.Args[1].Y)*scale, float64(s.Args[2].X)*scale, float64(s.Args[2].Y)*scale))
		default:
			return nil, 0, fmt.Errorf("unsupported outline segment")
		}
	}
	if open {
		commands = append(commands, path.Close())
	}
	return commands, float64(advance) * scale, nil
}
