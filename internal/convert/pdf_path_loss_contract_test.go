package convert

import (
	"reflect"
	"testing"
)

// PDFium loses explicit-return identity, but pinned PDFMiner 20260107
// normalizes that same redundant line before classification. Original operator
// provenance is still unavailable; these checks cover classification only.
func TestReferencePDFPathClosureInformationLoss(t *testing.T) {
	a, b, c, d := [2]float64{0, 0}, [2]float64{10, 0}, [2]float64{10, 20}, [2]float64{0, 20}
	for _, tc := range []struct {
		name, implicit, explicit           string
		implicitPoints, explicitPoints     [][2]float64
		wantImplicitLine, wantImplicitRect bool
	}{
		{"line", "mlh", "mllh", [][2]float64{a, b, a}, [][2]float64{a, b, a, a}, true, false},
		{"rectangle", "mlllh", "mllllh", [][2]float64{a, b, c, d, a}, [][2]float64{a, b, c, d, a, a}, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// PDFMiner retains a point for every operator including h. PDFium
			// appends the start only if needed, otherwise marks the existing endpoint
			// closed. Both streams consequently expose the same closed segment list.
			closeSegments := func(raw [][2]float64) [][2]float64 {
				points := append([][2]float64(nil), raw[:len(raw)-1]...)
				if points[len(points)-1] != points[0] {
					points = append(points, points[0])
				}
				return points
			}
			left, right := closeSegments(tc.implicitPoints), closeSegments(tc.explicitPoints)
			if !reflect.DeepEqual(left, right) {
				t.Fatalf("expected identical PDFium segment points: %v versus %v", left, right)
			}
			line, rect := referencePDFClassifyPath(tc.implicit, tc.implicitPoints)
			if line != tc.wantImplicitLine || rect != tc.wantImplicitRect {
				t.Fatalf("implicit classification=(%v,%v)", line, rect)
			}
			line, rect = referencePDFClassifyPath(tc.explicit, tc.explicitPoints)
			if line != tc.wantImplicitLine || rect != tc.wantImplicitRect {
				t.Fatalf("normalized explicit classification=(%v,%v)", line, rect)
			}
		})
	}
}
