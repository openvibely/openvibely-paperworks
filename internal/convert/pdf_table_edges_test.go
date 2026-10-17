package convert

// UNRUN: test-only ports of pdfplumber v0.11.0 default lines strategy,
// snap_edges, merge_edges, join_edge_group and minimum-length filtering.
import (
	"reflect"
	"sort"
	"testing"
)

func referencePDFOrientedTableEdges(g referencePDFGeometry) []referencePDFTableEdge {
	var out []referencePDFTableEdge
	for _, p := range g.paths {
		for _, e := range referencePDFEdges(referencePDFGeometry{height: g.height, paths: []referencePDFPath{p}}) {
			var orientation byte
			if e.top == e.bottom {
				orientation = 'h'
			} else if p.line || e.x0 == e.x1 {
				orientation = 'v'
			}
			// Diagonal LTLines are vertical in line_to_edge; diagonal curve
			// segments have no orientation and the default strategy excludes them.
			// filter_edges defaults to min_length=1 before merging.
			length := e.x1 - e.x0
			if orientation == 'v' {
				length = e.bottom - e.top
			}
			if orientation != 0 && length >= 1 {
				out = append(out, referencePDFTableEdge{e, orientation})
			}
		}
	}
	return out
}

func referencePDFMergeTableEdges(input []referencePDFTableEdge, snapX, snapY, joinX, joinY, minLength float64) []referencePDFTableEdge {
	var snapped []referencePDFTableEdge
	position := func(e referencePDFTableEdge) float64 {
		if e.orientation == 'v' {
			return e.x0
		}
		return e.top
	}
	start := func(e referencePDFTableEdge) float64 {
		if e.orientation == 'v' {
			return e.top
		}
		return e.x0
	}
	end := func(e referencePDFTableEdge) float64 {
		if e.orientation == 'v' {
			return e.bottom
		}
		return e.x1
	}
	for _, orientation := range []byte{'v', 'h'} {
		var edges []referencePDFTableEdge
		for _, e := range input {
			if e.orientation == orientation {
				edges = append(edges, e)
			}
		}
		sort.SliceStable(edges, func(i, j int) bool { return position(edges[i]) < position(edges[j]) })
		tol := snapX
		if orientation == 'h' {
			tol = snapY
		}
		for i := 0; i < len(edges); {
			j := i + 1
			// Clusters chain adjacent coordinates, not distance from the first.
			for j < len(edges) && position(edges[j])-position(edges[j-1]) <= tol {
				j++
			}
			sum := 0.0
			for _, e := range edges[i:j] {
				sum += position(e)
			}
			avg := sum / float64(j-i)
			for _, e := range edges[i:j] {
				delta := avg - position(e)
				if orientation == 'v' {
					e.x0 += delta
					e.x1 += delta
				} else {
					e.top += delta
					e.bottom += delta
				}
				snapped = append(snapped, e)
			}
			i = j
		}
	}
	sort.SliceStable(snapped, func(i, j int) bool {
		a, b := snapped[i], snapped[j]
		if a.orientation != b.orientation {
			return a.orientation < b.orientation
		}
		if position(a) != position(b) {
			return position(a) < position(b)
		}
		return start(a) < start(b)
	})
	var joined []referencePDFTableEdge
	for _, e := range snapped {
		tol := joinX
		if e.orientation == 'v' {
			tol = joinY
		}
		if len(joined) > 0 {
			last := &joined[len(joined)-1]
			if last.orientation == e.orientation && position(*last) == position(e) && start(e) <= end(*last)+tol {
				if end(e) > end(*last) {
					if e.orientation == 'v' {
						last.bottom = e.bottom
					} else {
						last.x1 = e.x1
					}
				}
				continue
			}
		}
		joined = append(joined, e)
	}
	var out []referencePDFTableEdge
	for _, e := range joined {
		if end(e)-start(e) >= minLength {
			out = append(out, e)
		}
	}
	return out
}

func TestReferencePDFTableEdgeMergeContracts(t *testing.T) {
	h := func(x0, x1, y float64) referencePDFTableEdge {
		return referencePDFTableEdge{referencePDFEdge{x0, x1, y, y}, 'h'}
	}
	v := func(x, y0, y1 float64) referencePDFTableEdge {
		return referencePDFTableEdge{referencePDFEdge{x, x, y0, y1}, 'v'}
	}
	for _, tc := range []struct {
		name        string
		input, want []referencePDFTableEdge
	}{
		{"adjacent-chain-average", []referencePDFTableEdge{h(0, 10, 0), h(0, 10, 3), h(0, 10, 6)}, []referencePDFTableEdge{h(0, 10, 3)}},
		{"weighted-average-duplicates", []referencePDFTableEdge{h(0, 10, 0), h(0, 10, 0), h(0, 10, 3)}, []referencePDFTableEdge{h(0, 10, 1)}},
		{"inclusive-join", []referencePDFTableEdge{h(0, 4, 0), h(7, 12, 0)}, []referencePDFTableEdge{h(0, 12, 0)}},
		{"outside-join", []referencePDFTableEdge{h(0, 4, 0), h(7.01, 12, 0)}, []referencePDFTableEdge{h(0, 4, 0), h(7.01, 12, 0)}},
		{"length-filter-after-join", []referencePDFTableEdge{h(0, 1, 0), h(2, 3, 0)}, []referencePDFTableEdge{h(0, 3, 0)}},
		{"short-edge-filter", []referencePDFTableEdge{h(0, 2.99, 0)}, nil},
		{"vertical-join", []referencePDFTableEdge{v(10, 0, 5), v(10, 8, 12)}, []referencePDFTableEdge{v(10, 0, 12)}},
		{"contained-edge", []referencePDFTableEdge{h(0, 20, 0), h(2, 5, 0)}, []referencePDFTableEdge{h(0, 20, 0)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := referencePDFMergeTableEdges(tc.input, 3, 3, 3, 3, 3); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("edges=%v; want %v", got, tc.want)
			}
		})
	}
}

func TestReferencePDFTableEdgeOrientationContracts(t *testing.T) {
	g := referencePDFGeometry{height: 100, paths: []referencePDFPath{
		{line: true, x0: 10, x1: 30, y0: 20, y1: 40},
		{points: [][2]float64{{10, 20}, {30, 40}, {50, 40}}},
	}}
	want := []referencePDFTableEdge{{referencePDFEdge{10, 30, 60, 80}, 'v'}, {referencePDFEdge{30, 50, 60, 60}, 'h'}}
	if got := referencePDFOrientedTableEdges(g); !reflect.DeepEqual(got, want) {
		t.Fatalf("edges=%v; want %v", got, want)
	}
}
