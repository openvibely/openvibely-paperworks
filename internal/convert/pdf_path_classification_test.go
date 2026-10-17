package convert

import (
	"strings"
	"testing"
)

// Source port of pinned PDFMiner 20260107 PDFLayoutAnalyzer.paint_path.
// Operators describe one subpath; points include each operator endpoint,
// including h even when it duplicates the preceding l endpoint. PDFium may
// normalize the source operators, so this does not establish reader equivalence.
func referencePDFNormalizePath(shape string, points [][2]float64) (string, [][2]float64) {
	if len(shape) > 3 && strings.HasSuffix(shape, "lh") && len(points) >= 2 && points[len(points)-2] == points[0] {
		return shape[:len(shape)-2] + "h", points[:len(points)-1]
	}
	return shape, points
}

func referencePDFClassifyPath(shape string, points [][2]float64) (line, rect bool) {
	shape, points = referencePDFNormalizePath(shape, points)
	if shape == "ml" || shape == "mlh" {
		return true, false
	}
	if (shape != "mlllh" && shape != "mllll") || len(points) != 5 {
		return false, false
	}
	a, b, c, d, end := points[0], points[1], points[2], points[3], points[4]
	return false, a == end && ((a[0] == b[0] && b[1] == c[1] && c[0] == d[0] && d[1] == a[1]) ||
		(a[1] == b[1] && b[0] == c[0] && c[1] == d[1] && d[0] == a[0]))
}

func TestReferencePDFPathClassificationContracts(t *testing.T) {
	a, b, c, d := [2]float64{0, 0}, [2]float64{10, 0}, [2]float64{10, 20}, [2]float64{0, 20}
	for _, tc := range []struct {
		name, shape string
		points      [][2]float64
		line, rect  bool
	}{
		{"open-line", "ml", [][2]float64{a, b}, true, false},
		{"closed-line", "mlh", [][2]float64{a, b, a}, true, false},
		{"explicit-return-line-is-curve", "mll", [][2]float64{a, b, a}, false, false},
		{"close-operator-rectangle", "mlllh", [][2]float64{a, b, c, d, a}, false, true},
		{"explicit-return-rectangle", "mllll", [][2]float64{a, b, c, d, a}, false, true},
		{"redundant-return-plus-close-rectangle", "mllllh", [][2]float64{a, b, c, d, a, a}, false, true},
		{"remove-only-one-redundant-line", "mlllllh", [][2]float64{a, b, c, d, a, a, a}, false, false},
		{"reverse-rectangle", "mlllh", [][2]float64{a, d, c, b, a}, false, true},
		{"open-four-segments", "mllll", [][2]float64{a, b, c, d, b}, false, false},
		{"skew-quadrilateral", "mlllh", [][2]float64{a, b, {12, 20}, {2, 20}, a}, false, false},
		{"cubic-with-rectangle-endpoints", "mccch", [][2]float64{a, b, c, d, a}, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			line, rect := referencePDFClassifyPath(tc.shape, tc.points)
			if line != tc.line || rect != tc.rect {
				t.Fatalf("classification=(line=%v rect=%v); want (line=%v rect=%v)", line, rect, tc.line, tc.rect)
			}
		})
	}
}

// LTLine keeps its two endpoints; LTRect constructs four corners from the
// first and third path endpoints. Only LTCurve retains the full path points.
func referencePDFLayoutPoints(points [][2]float64, line, rect bool) [][2]float64 {
	if line {
		return append([][2]float64(nil), points[:2]...)
	}
	if rect {
		a, c := points[0], points[2]
		return [][2]float64{a, {c[0], a[1]}, c, {a[0], c[1]}}
	}
	return append([][2]float64(nil), points...)
}

// UNRUN source contracts for LTLine/LTRect/LTCurve point storage.
func TestReferencePDFLayoutPointContracts(t *testing.T) {
	a, b, c, d := [2]float64{0, 0}, [2]float64{10, 0}, [2]float64{10, 20}, [2]float64{0, 20}
	for _, tc := range []struct {
		name, shape  string
		points, want [][2]float64
	}{
		{"closed-line-loses-close-point", "mlh", [][2]float64{a, b, a}, [][2]float64{a, b}},
		{"vertical-first-rectangle", "mlllh", [][2]float64{a, d, c, b, a}, [][2]float64{a, b, c, d}},
		{"reverse-corners-preserved", "mllll", [][2]float64{c, b, a, d, c}, [][2]float64{c, d, a, b}},
		{"rectangle-drops-redundant-close", "mllllh", [][2]float64{a, b, c, d, a, a}, [][2]float64{a, b, c, d}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			line, rect := referencePDFClassifyPath(tc.shape, tc.points)
			got := referencePDFLayoutPoints(tc.points, line, rect)
			if len(got) != len(tc.want) {
				t.Fatalf("points=%v; want %v", got, tc.want)
			}
			for i, p := range got {
				if p != tc.want[i] {
					t.Fatalf("points=%v; want %v", got, tc.want)
				}
			}
		})
	}
}
