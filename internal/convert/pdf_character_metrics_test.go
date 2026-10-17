package convert

// Source contract for pinned pdfminer.six 20260107 LTChar.__init__.
// These inputs are PDF text-state/font metrics, NOT PDFium loose glyph boxes.
// Acquisition of these inputs from the actual PDF remains a review blocker.
import (
	"math"
	"testing"
)

type referencePDFCharMetrics struct {
	matrix                                [6]float64
	fontSize, scale, rise, width, descent float64
	vertical                              bool
	vx                                    *float64
	vy                                    float64
}

type referencePDFCharLayout struct {
	box           referencePDFCell // PDFMiner bottom-up coordinates, before page top conversion
	advance, size float64
	upright       bool
}

func referencePDFMinerCharLayout(in referencePDFCharMetrics) referencePDFCharLayout {
	advance := in.width * in.fontSize * in.scale
	// Reference rounds the multiplication before adding rise; prevent Go FMA.
	descent := float64(in.descent * in.fontSize)
	x0, y0, x1, y1 := 0.0, descent+in.rise, advance, descent+in.rise+in.fontSize
	if in.vertical {
		vx := in.fontSize * .5
		if in.vx != nil {
			vx = *in.vx * in.fontSize * .001
		}
		vy := (1000 - in.vy) * in.fontSize * .001
		x0, y0, x1, y1 = -vx, vy+in.rise+advance, -vx+in.fontSize, vy+in.rise
	}
	m := in.matrix
	// Pinned LTChar uses apply_matrix_rect: enclose all four corners.
	corners := [][2]float64{{x0, y0}, {x1, y0}, {x1, y1}, {x0, y1}}
	x0, y0, x1, y1 = math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)
	for _, p := range corners {
		x, y := m[0]*p[0]+m[2]*p[1]+m[4], m[1]*p[0]+m[3]*p[1]+m[5]
		x0, y0, x1, y1 = math.Min(x0, x), math.Min(y0, y), math.Max(x1, x), math.Max(y1, y)
	}

	size := y1 - y0
	if in.vertical {
		size = x1 - x0
	}
	return referencePDFCharLayout{referencePDFCell{x0, y0, x1, y1}, advance, size, m[0]*m[3]*in.scale > 0 && m[1]*m[2] <= 0}
}

func TestReferencePDFMinerCharacterMetricContracts(t *testing.T) {
	identity := [6]float64{1, 0, 0, 1, 0, 0}
	base := referencePDFCharMetrics{matrix: identity, fontSize: 10, scale: 1, width: .6, descent: -.2}
	change := func(fn func(*referencePDFCharMetrics)) referencePDFCharMetrics { in := base; fn(&in); return in }
	zero := 0.0
	for _, tc := range []struct {
		name string
		in   referencePDFCharMetrics
		want referencePDFCharLayout
	}{
		{"horizontal-descent", base, referencePDFCharLayout{referencePDFCell{0, -2, 6, 8}, 6, 10, true}},
		{"rise", change(func(p *referencePDFCharMetrics) { p.rise = 3 }), referencePDFCharLayout{referencePDFCell{0, 1, 6, 11}, 6, 10, true}},
		{"half-horizontal-scale", change(func(p *referencePDFCharMetrics) { p.scale = .5 }), referencePDFCharLayout{referencePDFCell{0, -2, 3, 8}, 3, 10, true}},
		{"negative-horizontal-scale", change(func(p *referencePDFCharMetrics) { p.scale = -1 }), referencePDFCharLayout{referencePDFCell{-6, -2, 0, 8}, -6, 10, false}},
		{"translated", change(func(p *referencePDFCharMetrics) { p.matrix = [6]float64{1, 0, 0, 1, 12, 30} }), referencePDFCharLayout{referencePDFCell{12, 28, 18, 38}, 6, 10, true}},
		{"quarter-turn", change(func(p *referencePDFCharMetrics) { p.matrix = [6]float64{0, 1, -1, 0, 0, 0} }), referencePDFCharLayout{referencePDFCell{-8, 0, 2, 6}, 6, 6, false}},
		{"mirrored-with-negative-scale", change(func(p *referencePDFCharMetrics) { p.scale = -1; p.matrix = [6]float64{-1, 0, 0, 1, 0, 0} }), referencePDFCharLayout{referencePDFCell{0, -2, 6, 8}, -6, 10, true}},
		{"skew-four-corner-enclosure", change(func(p *referencePDFCharMetrics) { p.matrix = [6]float64{1, 0, -1, 1, 0, 0} }), referencePDFCharLayout{referencePDFCell{-8, -2, 8, 8}, 6, 10, true}},
		{"vertical-default-displacement", change(func(p *referencePDFCharMetrics) { p.vertical = true; p.width = -1; p.vy = 1000 }), referencePDFCharLayout{referencePDFCell{-5, -10, 5, 0}, -10, 10, true}},
		{"vertical-explicit-zero-displacement", change(func(p *referencePDFCharMetrics) {
			p.vertical = true
			p.width = -1
			p.vx = &zero
			p.vy = 500
			p.rise = 2
		}), referencePDFCharLayout{referencePDFCell{0, -3, 10, 7}, -10, 10, true}},
		{"zero-scale-not-upright", change(func(p *referencePDFCharMetrics) { p.scale = 0 }), referencePDFCharLayout{referencePDFCell{0, -2, 0, 8}, 0, 10, false}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := referencePDFMinerCharLayout(tc.in); got != tc.want {
				t.Fatalf("layout=%+v; want %+v", got, tc.want)
			}
		})
	}
}
