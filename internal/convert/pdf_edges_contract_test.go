package convert

import (
	"reflect"
	"testing"
)

// UNRUN. Source-derived adapter contracts, not additional original assertion
// sites. Source versions and fixture provenance are retained in testdata/reference-mirror.
func TestReferencePDFEdgeAdapterContracts(t *testing.T) {
	for _, tc := range []struct {
		name string
		path referencePDFPath
		want []referencePDFEdge
	}{
		{"open-line", referencePDFPath{line: true, x0: 10, x1: 30, y0: 20, y1: 20, points: [][2]float64{{10, 20}, {30, 20}}}, []referencePDFEdge{{10, 30, 80, 80}}},
		{"closed-line", referencePDFPath{line: true, x0: 10, x1: 30, y0: 20, y1: 20, points: [][2]float64{{10, 20}, {30, 20}, {10, 20}}}, []referencePDFEdge{{10, 30, 80, 80}}},
		{"diagonal-line", referencePDFPath{line: true, x0: 10, x1: 30, y0: 20, y1: 40, points: [][2]float64{{30, 20}, {10, 40}}}, []referencePDFEdge{{10, 30, 60, 80}}},
		{"rectangle-order", referencePDFPath{rect: true, x0: 10, x1: 30, y0: 20, y1: 40}, []referencePDFEdge{{10, 30, 60, 60}, {10, 30, 80, 80}, {10, 10, 60, 80}, {30, 30, 60, 80}}},
		{"curve-endpoints", referencePDFPath{bezier: true, points: [][2]float64{{10, 20}, {30, 40}, {50, 20}}}, []referencePDFEdge{{10, 30, 60, 80}, {30, 50, 60, 80}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := referencePDFEdges(referencePDFGeometry{height: 100, paths: []referencePDFPath{tc.path}})
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("edges=%#v; want %#v", got, tc.want)
			}
		})
	}
}

// Pinned pdfplumber 0.11.10 rect_to_edges: bottom-edge top uses addition,
// while bottom retains process_object's separate height-minus-y0 result.
func TestReferencePDFRectangleEdgeRounding(t *testing.T) {
	g := referencePDFGeometry{height: 540, paths: []referencePDFPath{{rect: true, x0: 345.969, x1: 353.962, y0: 140.031, y1: 148.025}}}
	got := referencePDFEdges(g)[1]
	if got.top != 399.96900000000005 || got.bottom != 399.969 {
		t.Fatalf("bottom edge=%+.17g/%+.17g", got.top, got.bottom)
	}
}
