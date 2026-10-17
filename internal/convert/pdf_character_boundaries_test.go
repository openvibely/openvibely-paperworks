package convert

import (
	"reflect"
	"testing"
)

// UNRUN source contracts for PDFLayoutAnalyzer.render_char (pdfminer.six
// 20231228) and WordExtractor/Table.extract (pdfplumber v0.11.0). render_char
// creates ONE LTChar with the entire font.to_unichr(cid) string, including
// multi-codepoint strings. These cases start after decoding, not at a PDFium
// text page: they do not establish acquisition or font-metric equivalence.
func TestReferencePDFDecodedCharacterBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name, decoded, want string
	}{
		{"leading-space-in-one-cid", " A", " A"},
		{"trailing-space-in-one-cid", "A ", "A "},
		{"embedded-newline-in-one-cid", "A\nB", "A\nB"},
		// merge_chars expands only exact ligature dictionary keys. It does not
		// replace ligature runes inside a longer CID mapping string.
		{"ligature-within-multicodepoint-cid", "ﬁX", "ﬁX"},
		{"single-ligature-cid", "ﬁ", "fi"},
		{"nul-is-not-missing-mapping", "\x00", "\x00"},
		{"supplementary-unicode", "\U0001f600", "\U0001f600"},
		// handle_undefined_char returns this literal string instead of aborting
		// extraction. Acquisition must know the CID to construct the fallback.
		{"undefined-cid-fallback", "(cid:42)", "(cid:42)"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			chars := []referencePDFTableChar{{tc.decoded, referencePDFCell{0, 0, 8, 8}}}
			if got := referencePDFUprightCellText(chars); got != tc.want {
				t.Fatalf("decoded CID text=%q; want %q", got, tc.want)
			}
		})
	}
}

// UNRUN. A multi-codepoint CID has one metric box and is assigned to one cell
// as a unit. Splitting a decoded string into runes with subdivided boxes changes
// the table, even when the page's concatenated text is identical.
func TestReferencePDFCIDBoundaryTableAssignment(t *testing.T) {
	cells := []referencePDFCell{{0, 0, 10, 10}, {10, 0, 20, 10}}
	empty, ab, a, b := "", "AB", "A", "B"
	for _, tc := range []struct {
		name  string
		chars []referencePDFTableChar
		want  [][]*string
	}{
		{"one-cid-crosses-cell-edge", []referencePDFTableChar{{"AB", referencePDFCell{6, 1, 14, 9}}}, [][]*string{{&empty, &ab}}},
		{"two-cids-straddle-cell-edge", []referencePDFTableChar{{"A", referencePDFCell{6, 1, 10, 9}}, {"B", referencePDFCell{10, 1, 14, 9}}}, [][]*string{{&a, &b}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := referencePDFExtractTableRows(cells, tc.chars, referencePDFUprightCellText)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("CID-to-cell assignment=%v; want %v", got, tc.want)
			}
		})
	}
}

// UNRUN. pdfplumber v0.11.0 preserves decoded LTChars (including empty text,
// overprinting and zero-width boxes). Stable coordinate sorting must retain
// acquisition order for tied positions. These are post-acquisition contracts;
// they do not claim PDFium can supply the original character sequence.
func TestReferencePDFDecodedCharacterOrdering(t *testing.T) {
	c := func(text string, left, right float64) referencePDFTableChar {
		return referencePDFTableChar{text, referencePDFCell{left, 0, right, 8}}
	}
	for _, tc := range []struct {
		name      string
		chars     []referencePDFTableChar
		wantWords []string
		wantText  string
	}{
		{"empty-cid-between-letters", []referencePDFTableChar{c("A", 0, 4), c("", 4, 4), c("B", 4, 8)}, []string{"A", "", "B"}, "A  B"},
		{"leading-empty-cid", []referencePDFTableChar{c("", 0, 0), c("A", 0, 4)}, []string{"", "A"}, " A"},
		{"trailing-empty-cid", []referencePDFTableChar{c("A", 0, 4), c("", 4, 4)}, []string{"A", ""}, "A "},
		{"two-empty-cids", []referencePDFTableChar{c("", 0, 0), c("", 0, 0)}, []string{"", ""}, " "},
		{"overprinted-cid-not-deduplicated", []referencePDFTableChar{c("A", 0, 4), c("A", 0, 4)}, []string{"AA"}, "AA"},
		{"tied-position-keeps-input-order", []referencePDFTableChar{c("B", 0, 4), c("A", 0, 4)}, []string{"BA"}, "BA"},
		{"zero-width-cid-not-suppressed", []referencePDFTableChar{c("A", 0, 0), c("B", 0, 4)}, []string{"AB"}, "AB"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			directed := make([]referencePDFDirectedChar, len(tc.chars))
			for i, char := range tc.chars {
				directed[i] = referencePDFDirectedChar{char, true}
			}
			words := referencePDFDefaultWords(directed)
			gotWords := make([]string, len(words))
			for i, word := range words {
				gotWords[i] = word.text
			}
			if !reflect.DeepEqual(gotWords, tc.wantWords) {
				t.Fatalf("words=%q; want %q", gotWords, tc.wantWords)
			}
			if got := referencePDFDefaultCellText(directed); got != tc.wantText {
				t.Fatalf("text=%q; want %q", got, tc.wantText)
			}
		})
	}
}
