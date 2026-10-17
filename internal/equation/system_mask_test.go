package equation

import (
	"github.com/luo-studio/go-tex/tex/path"
	"image"
	"image/color"
	"math"
	"testing"
)

func TestSystemMaskOutlineBounds(t *testing.T) {
	m := image.NewAlpha(image.Rect(4, 5, 6, 6))
	m.SetAlpha(4, 5, color.Alpha{127})
	m.SetAlpha(5, 5, color.Alpha{128})
	p, e := systemMaskOutline(m, .001, 2)
	if e != nil || len(p) != 5 || p[0].X != 1 || p[0].Y != 2 {
		t.Fatalf("unexpected mask path %v %v", p, e)
	}
	if _, e = systemMaskOutline(nil, 0, 0); e == nil {
		t.Fatal("nil mask")
	}
	if _, e = systemMaskOutline(m, math.NaN(), 0); e == nil {
		t.Fatal("nonfinite position")
	}
}

// Check the threshold, subimage origin, and every filled pixel independently of
// the compressed representation. Adjacent pixels must not create gaps/overlap.
func TestSystemMaskRunsPreserveCoverage(t *testing.T) {
	m := image.NewAlpha(image.Rect(5, 7, 37, 24))
	for y := 7; y < 24; y++ {
		for x := 5; x < 37; x++ {
			m.SetAlpha(x, y, color.Alpha{A: byte((x*71 + y*43) % 256)})
		}
	}
	for _, runs := range []bool{false, true} {
		p, e := systemMaskPath(m, .125, -2.25, runs)
		if e != nil {
			t.Fatal(e)
		}
		hits := make(map[[2]int]int)
		for i := 0; i < len(p); i += 5 {
			a, b := p[i], p[i+2]
			if p[i+4].Kind != path.KindClose {
				t.Fatal("open mask contour")
			}
			for y := int(a.Y + 2.25); y < int(b.Y+2.25); y++ {
				for x := int(a.X - .125); x < int(b.X-.125); x++ {
					hits[[2]int{x, y}]++
				}
			}
		}
		for y := 0; y < m.Bounds().Dy(); y++ {
			for x := 0; x < m.Bounds().Dx(); x++ {
				want := 0
				if m.AlphaAt(x+5, y+7).A >= 128 {
					want = 1
				}
				if hits[[2]int{x, y}] != want {
					t.Fatalf("runs=%v pixel %d,%d", runs, x, y)
				}
			}
		}
	}
	full := image.NewAlpha(image.Rect(0, 0, 80, 10))
	for i := range full.Pix {
		full.Pix[i] = 255
	}
	p, e := systemMaskRuns(full, 0, 0)
	if e != nil || len(p) != 50 {
		t.Fatalf("solid rows not compacted: %d %v", len(p), e)
	}
}
