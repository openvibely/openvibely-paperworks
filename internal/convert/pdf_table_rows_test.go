package convert

// UNRUN structural port of Table.rows and Table.extract in pdfplumber v0.11.0.
// Text assembly is injected; the integrated adapter retains glyph orientation.
// PDFium character metrics remain an open review gate.
import (
	"reflect"
	"sort"
	"testing"
)

type referencePDFTableChar struct {
	text string
	box  referencePDFCell
}

func referencePDFExtractTableRows(cells []referencePDFCell, chars []referencePDFTableChar, extract func([]referencePDFTableChar) string) [][]*string {
	return referencePDFExtractRows(cells, chars, func(c referencePDFTableChar) referencePDFCell { return c.box }, extract)
}

func referencePDFExtractRows[T any](cells []referencePDFCell, chars []T, bounds func(T) referencePDFCell, extract func([]T) string) [][]*string {
	sorted := append([]referencePDFCell(nil), cells...)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i][1] != sorted[j][1] {
			return sorted[i][1] < sorted[j][1]
		}
		return sorted[i][0] < sorted[j][0]
	})
	seen := map[float64]bool{}
	var xs []float64
	for _, c := range sorted {
		if !seen[c[0]] {
			seen[c[0]] = true
			xs = append(xs, c[0])
		}
	}
	sort.Float64s(xs)
	inside := func(c T, b referencePDFCell) bool {
		box := bounds(c)
		x, y := (box[0]+box[2])/2, (box[1]+box[3])/2
		return x >= b[0] && x < b[2] && y >= b[1] && y < b[3]
	}
	var rows [][]*string
	for i := 0; i < len(sorted); {
		j := i + 1
		for j < len(sorted) && sorted[j][1] == sorted[i][1] {
			j++
		}
		byX := map[float64]referencePDFCell{}
		bbox := sorted[i]
		for _, c := range sorted[i:j] {
			byX[c[0]] = c
			if c[0] < bbox[0] {
				bbox[0] = c[0]
			}
			if c[2] > bbox[2] {
				bbox[2] = c[2]
			}
			if c[3] > bbox[3] {
				bbox[3] = c[3]
			}
		}
		var rowChars []T
		for _, c := range chars {
			if inside(c, bbox) {
				rowChars = append(rowChars, c)
			}
		}
		row := make([]*string, len(xs))
		for k, x := range xs {
			cell, ok := byX[x]
			if !ok {
				continue
			}
			var selected []T
			for _, c := range rowChars {
				if inside(c, cell) {
					selected = append(selected, c)
				}
			}
			text := ""
			if len(selected) > 0 {
				text = extract(selected)
			}
			row[k] = &text
		}
		rows = append(rows, row)
		i = j
	}
	return rows
}

func TestReferencePDFTableRowAssignmentContracts(t *testing.T) {
	str := func(s string) *string { return &s }
	join := func(chars []referencePDFTableChar) string {
		s := ""
		for _, c := range chars {
			s += c.text
		}
		return s
	}
	for _, tc := range []struct {
		name  string
		cells []referencePDFCell
		chars []referencePDFTableChar
		want  [][]*string
	}{
		{"row-column-order", []referencePDFCell{{10, 10, 20, 20}, {0, 10, 10, 20}, {10, 0, 20, 10}, {0, 0, 10, 10}}, []referencePDFTableChar{{"A", referencePDFCell{1, 1, 3, 3}}, {"B", referencePDFCell{11, 1, 13, 3}}, {"C", referencePDFCell{1, 11, 3, 13}}, {"D", referencePDFCell{11, 11, 13, 13}}}, [][]*string{{str("A"), str("B")}, {str("C"), str("D")}}},
		{"missing-versus-empty", []referencePDFCell{{0, 0, 10, 10}, {10, 10, 20, 20}}, nil, [][]*string{{str(""), nil}, {nil, str("")}}},
		{"midpoint-half-open-boundaries", []referencePDFCell{{0, 0, 10, 10}, {10, 0, 20, 10}}, []referencePDFTableChar{{"L", referencePDFCell{-1, -1, 1, 1}}, {"R", referencePDFCell{9, 4, 11, 6}}, {"outside-right", referencePDFCell{19, 4, 21, 6}}, {"outside-bottom", referencePDFCell{4, 9, 6, 11}}}, [][]*string{{str("L"), str("R")}}},
		{"preserve-character-input-order", []referencePDFCell{{0, 0, 20, 10}, {20, 0, 30, 10}}, []referencePDFTableChar{{"B", referencePDFCell{8, 1, 10, 3}}, {"A", referencePDFCell{1, 1, 3, 3}}}, [][]*string{{str("BA"), str("")}}},
		{"merged-cell-retains-absent-column", []referencePDFCell{{0, 0, 20, 10}, {0, 10, 10, 20}, {10, 10, 20, 20}}, nil, [][]*string{{str(""), nil}, {str(""), str("")}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := referencePDFExtractTableRows(tc.cells, tc.chars, join)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("rows=%v; want %v", got, tc.want)
			}
		})
	}
}
