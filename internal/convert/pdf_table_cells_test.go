package convert

// UNRUN test-only translation of pdfplumber v0.11.0 table.py
// edges_to_intersections and intersections_to_cells. Callers must first apply
// the default orientation filtering, snapping, joining and minimum length.
// This is a cell-discovery stage, not yet a complete table/text extractor.
import (
	"reflect"
	"sort"
	"testing"
)

type referencePDFTableEdge struct {
	referencePDFEdge
	orientation byte
}
type referencePDFCell [4]float64 // x0, top, x1, bottom

func referencePDFTableCells(edges []referencePDFTableEdge, xtol, ytol float64) []referencePDFCell {
	type point [2]float64
	type incident struct{ v, h map[referencePDFEdge]bool }
	intersections := map[point]*incident{}
	for _, v := range edges {
		if v.orientation != 'v' {
			continue
		}
		for _, h := range edges {
			if h.orientation != 'h' {
				continue
			}
			if v.top > h.top+ytol || v.bottom < h.top-ytol || v.x0 < h.x0-xtol || v.x0 > h.x1+xtol {
				continue
			}
			p := point{v.x0, h.top}
			if intersections[p] == nil {
				intersections[p] = &incident{map[referencePDFEdge]bool{}, map[referencePDFEdge]bool{}}
			}
			intersections[p].v[v.referencePDFEdge] = true
			intersections[p].h[h.referencePDFEdge] = true
		}
	}
	points := make([]point, 0, len(intersections))
	for p := range intersections {
		points = append(points, p)
	}
	sort.Slice(points, func(i, j int) bool {
		if points[i][0] != points[j][0] {
			return points[i][0] < points[j][0]
		}
		return points[i][1] < points[j][1]
	})
	common := func(a, b map[referencePDFEdge]bool) bool {
		for e := range a {
			if b[e] {
				return true
			}
		}
		return false
	}
	connects := func(a, b point) bool {
		ia, ib := intersections[a], intersections[b]
		if ia == nil || ib == nil {
			return false
		}
		return (a[0] == b[0] && common(ia.v, ib.v)) || (a[1] == b[1] && common(ia.h, ib.h))
	}
	var cells []referencePDFCell
	for i, p := range points {
		found := false
		for _, below := range points[i+1:] {
			if below[0] != p[0] || !connects(p, below) {
				continue
			}
			for _, right := range points[i+1:] {
				if right[1] != p[1] || !connects(p, right) {
					continue
				}
				br := point{right[0], below[1]}
				if connects(br, right) && connects(br, below) {
					cells = append(cells, referencePDFCell{p[0], p[1], br[0], br[1]})
					found = true
					break
				}
			}
			if found {
				break
			}
		}
	}
	return cells
}

func TestReferencePDFTableCellContracts(t *testing.T) {
	h := func(x0, x1, y float64) referencePDFTableEdge {
		return referencePDFTableEdge{referencePDFEdge{x0, x1, y, y}, 'h'}
	}
	v := func(x, y0, y1 float64) referencePDFTableEdge {
		return referencePDFTableEdge{referencePDFEdge{x, x, y0, y1}, 'v'}
	}
	for _, tc := range []struct {
		name      string
		edges     []referencePDFTableEdge
		tolerance float64
		want      []referencePDFCell
	}{
		{"closed", []referencePDFTableEdge{h(0, 20, 0), h(0, 20, 10), v(0, 0, 10), v(20, 0, 10)}, 0, []referencePDFCell{{0, 0, 20, 10}}},
		{"missing-side", []referencePDFTableEdge{h(0, 20, 0), h(0, 20, 10), v(0, 0, 10)}, 0, nil},
		{"smallest-cells-not-enclosing-rectangle", []referencePDFTableEdge{h(0, 20, 0), h(0, 20, 10), v(0, 0, 10), v(10, 0, 10), v(20, 0, 10)}, 0, []referencePDFCell{{0, 0, 10, 10}, {10, 0, 20, 10}}},
		{"unjoined-side-does-not-connect", []referencePDFTableEdge{h(0, 20, 0), h(0, 20, 10), v(0, 0, 4), v(0, 6, 10), v(20, 0, 10)}, 0, nil},
		{"intersection-tolerance-inclusive", []referencePDFTableEdge{h(3, 17, 0), h(3, 17, 10), v(0, 3, 7), v(20, 3, 7)}, 3, []referencePDFCell{{0, 0, 20, 10}}},
		{"outside-intersection-tolerance", []referencePDFTableEdge{h(3.01, 17, 0), h(3.01, 17, 10), v(0, 3, 7), v(20, 3, 7)}, 3, nil},
		{"duplicate-edge-bounds", []referencePDFTableEdge{h(0, 20, 0), h(0, 20, 0), h(0, 20, 10), v(0, 0, 10), v(20, 0, 10)}, 0, []referencePDFCell{{0, 0, 20, 10}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := referencePDFTableCells(tc.edges, tc.tolerance, tc.tolerance); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("cells=%v, want %v", got, tc.want)
			}
		})
	}
}

// Match cells_to_tables: corner contact suffices, isolated single cells are
// discarded, and ordering uses the topmost/leftmost corner (not area).
func referencePDFGroupTableCells(cells []referencePDFCell) [][]referencePDFCell {
	corners := func(c referencePDFCell) [4][2]float64 {
		return [4][2]float64{{c[0], c[1]}, {c[0], c[3]}, {c[2], c[1]}, {c[2], c[3]}}
	}
	remaining := append([]referencePDFCell(nil), cells...)
	var tables [][]referencePDFCell
	for len(remaining) > 0 {
		var group []referencePDFCell
		seen := map[[2]float64]bool{}
		for {
			count := len(group)
			var next []referencePDFCell
			for _, c := range remaining {
				touches := len(group) == 0
				for _, p := range corners(c) {
					touches = touches || seen[p]
				}
				if touches {
					group = append(group, c)
					for _, p := range corners(c) {
						seen[p] = true
					}
				} else {
					next = append(next, c)
				}
			}
			remaining = next
			if len(group) == count || len(remaining) == 0 {
				break
			}
		}
		if len(group) > 1 {
			tables = append(tables, group)
		}
	}
	first := func(cells []referencePDFCell) [2]float64 {
		p := [2]float64{cells[0][1], cells[0][0]}
		for _, c := range cells {
			if c[1] < p[0] || (c[1] == p[0] && c[0] < p[1]) {
				p = [2]float64{c[1], c[0]}
			}
		}
		return p
	}
	sort.SliceStable(tables, func(i, j int) bool {
		a, b := first(tables[i]), first(tables[j])
		return a[0] < b[0] || (a[0] == b[0] && a[1] < b[1])
	})
	return tables
}

func TestReferencePDFTableGroupingContracts(t *testing.T) {
	a, b := referencePDFCell{0, 0, 10, 10}, referencePDFCell{10, 10, 20, 20}
	c, d := referencePDFCell{0, 30, 10, 40}, referencePDFCell{10, 30, 20, 40}
	for _, tc := range []struct {
		name  string
		cells []referencePDFCell
		want  [][]referencePDFCell
	}{
		{"single-cell-is-not-table", []referencePDFCell{a}, nil},
		{"corner-contact", []referencePDFCell{a, b}, [][]referencePDFCell{{a, b}}},
		{"position-not-input-order", []referencePDFCell{c, d, a, b}, [][]referencePDFCell{{a, b}, {c, d}}},
		{"revisit-after-bridge", []referencePDFCell{a, {20, 20, 30, 30}, b}, [][]referencePDFCell{{a, b, {20, 20, 30, 30}}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := referencePDFGroupTableCells(tc.cells); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("tables=%v, want %v", got, tc.want)
			}
		})
	}
}
