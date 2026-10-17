package convert

import (
	"reflect"
	"testing"
)

// UNRUN source port of pdfminer.six 20231228 PDFTextDevice.render_string and
// render_string_horizontal/vertical. Inputs are decoded CIDs and font advances,
// not PDFium's reordered Unicode records. Actual stream/font decoding is open.
type referencePDFTextItem struct {
	adjustment *float64
	cids       []int
}

func referencePDFMinerTextPositions(items []referencePDFTextItem, start [2]float64, size, scale, charspace, wordspace float64, vertical, multibyte bool, widths map[int]float64) ([][2]float64, [2]float64) {
	axis := 0
	if vertical {
		axis = 1
	}
	charspace *= scale
	wordspace *= scale
	if multibyte {
		wordspace = 0
	}
	pos := start
	var origins [][2]float64
	needCharSpace := false
	for _, item := range items {
		if item.adjustment != nil {
			pos[axis] -= *item.adjustment * (.001 * size * scale)
			needCharSpace = true
			continue
		}
		for _, cid := range item.cids {
			if needCharSpace {
				pos[axis] += charspace
			}
			origins = append(origins, pos)
			pos[axis] += widths[cid] * size * scale
			if cid == 32 {
				pos[axis] += wordspace
			}
			needCharSpace = true
		}
	}
	return origins, pos
}

func TestReferencePDFMinerTextAdvanceContracts(t *testing.T) {
	chars := func(cids ...int) referencePDFTextItem { return referencePDFTextItem{cids: cids} }
	adjust := func(n float64) referencePDFTextItem { return referencePDFTextItem{adjustment: &n} }
	// CID 99 may map to Unicode space, but must not receive word spacing.
	widths := map[int]float64{1: .5, 32: .5, 99: .5}
	for _, tc := range []struct {
		name                string
		items               []referencePDFTextItem
		scale               float64
		vertical, multibyte bool
		origins             [][2]float64
		end                 [2]float64
	}{
		{"no-leading-charspace", []referencePDFTextItem{chars(1, 1)}, 1, false, false, [][2]float64{{0, 0}, {7, 0}}, [2]float64{12, 0}},
		{"string-boundary-keeps-spacing", []referencePDFTextItem{chars(1), chars(), chars(1)}, 1, false, false, [][2]float64{{0, 0}, {7, 0}}, [2]float64{12, 0}},
		{"leading-number-enables-charspace", []referencePDFTextItem{adjust(100), chars(1)}, 1, false, false, [][2]float64{{1, 0}}, [2]float64{6, 0}},
		{"zero-number-enables-charspace", []referencePDFTextItem{adjust(0), chars(1)}, 1, false, false, [][2]float64{{2, 0}}, [2]float64{7, 0}},
		{"consecutive-numbers-one-charspace", []referencePDFTextItem{chars(1), adjust(100), adjust(-200), chars(1)}, 1, false, false, [][2]float64{{0, 0}, {8, 0}}, [2]float64{13, 0}},
		{"trailing-number-updates-position", []referencePDFTextItem{chars(1), adjust(100)}, 1, false, false, [][2]float64{{0, 0}}, [2]float64{4, 0}},
		{"cid-32-wordspace", []referencePDFTextItem{chars(32, 1)}, 1, false, false, [][2]float64{{0, 0}, {10, 0}}, [2]float64{15, 0}},
		{"unicode-space-is-not-cid-32", []referencePDFTextItem{chars(99, 1)}, 1, false, false, [][2]float64{{0, 0}, {7, 0}}, [2]float64{12, 0}},
		{"multibyte-disables-wordspace", []referencePDFTextItem{chars(32, 1)}, 1, false, true, [][2]float64{{0, 0}, {7, 0}}, [2]float64{12, 0}},
		{"scales-spacing-and-advance", []referencePDFTextItem{chars(32, 1)}, .5, false, false, [][2]float64{{0, 0}, {5, 0}}, [2]float64{7.5, 0}},
		{"vertical-adjustment-and-spacing", []referencePDFTextItem{chars(32), adjust(100), chars(1)}, 1, true, false, [][2]float64{{0, 0}, {0, 9}}, [2]float64{0, 14}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, end := referencePDFMinerTextPositions(tc.items, [2]float64{}, 10, tc.scale, 2, 3, tc.vertical, tc.multibyte, widths)
			if !reflect.DeepEqual(got, tc.origins) || end != tc.end {
				t.Fatalf("origins=%v end=%v; want %v %v", got, end, tc.origins, tc.end)
			}
		})
	}
}

func TestReferencePDFMinerTextShowResetsSpacingFlag(t *testing.T) {
	// UNRUN. Each render_string invocation resets needcharspace, while retaining
	// linematrix. Splitting a TJ sequence into separate Tj calls changes spacing.
	items := []referencePDFTextItem{{cids: []int{1}}}
	widths := map[int]float64{1: .5}
	_, end := referencePDFMinerTextPositions(items, [2]float64{4, 9}, 10, 1, 2, 3, false, false, widths)
	origins, end := referencePDFMinerTextPositions(items, end, 10, 1, 2, 3, false, false, widths)
	if !reflect.DeepEqual(origins, [][2]float64{{9, 9}}) || end != [2]float64{14, 9} {
		t.Fatalf("second show origins=%v end=%v; want [[9 9]] [14 9]", origins, end)
	}
}
