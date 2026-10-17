package convert

// UNRUN source port of pdfplumber v0.11.0 WordExtractor and extract_text
// defaults, including consecutive upright/rotated runs. This does not assert
// that PDFium boxes match LTChar; the connected acquisition adapter still needs
// equivalence review before any runtime results can count as equivalence.
import (
	"math"
	"sort"
	"strings"
	"testing"
	"unicode"
)

// Orientation is separate from TableChar so row selection remains independent
// of the character-source adapter. Reference groups consecutive equal upright flags.
type referencePDFDirectedChar struct {
	char    referencePDFTableChar
	upright bool
}

func referencePDFClusterChars(items []referencePDFTableChar, axis int) [][]referencePDFTableChar {
	coords := make([]float64, 0, len(items))
	for _, c := range items {
		coords = append(coords, c.box[axis])
	}
	sort.Float64s(coords)
	ids := map[float64]int{}
	var groups [][]referencePDFTableChar
	for i, value := range coords {
		if i == 0 || value > coords[i-1]+3 {
			groups = append(groups, nil)
		}
		ids[value] = len(groups) - 1
	}
	for _, c := range items {
		id := ids[c.box[axis]]
		groups[id] = append(groups[id], c)
	}
	return groups
}

func referencePDFUprightCellText(chars []referencePDFTableChar) string {
	directed := make([]referencePDFDirectedChar, len(chars))
	for i, c := range chars {
		directed[i] = referencePDFDirectedChar{c, true}
	}
	return referencePDFDefaultCellText(directed)
}

func referencePDFDefaultWords(chars []referencePDFDirectedChar) []referencePDFTableChar {
	var words []referencePDFTableChar
	for i := 0; i < len(chars); {
		j := i + 1
		for j < len(chars) && chars[j].upright == chars[i].upright {
			j++
		}
		run := make([]referencePDFTableChar, 0, j-i)
		for _, c := range chars[i:j] {
			run = append(run, c.char)
		}
		words = append(words, referencePDFRunWords(run, chars[i].upright)...)
		i = j
	}
	return words
}

func referencePDFDefaultCellText(chars []referencePDFDirectedChar) string {
	words := referencePDFDefaultWords(chars)
	var lines []string
	// extract_text's final clustering uses the default line_dir (ttb), even
	// for words from rotated runs, and preserves word order within clusters.
	for _, line := range referencePDFClusterChars(words, 1) {
		texts := make([]string, len(line))
		for i, w := range line {
			texts[i] = w.text
		}
		lines = append(lines, strings.Join(texts, " "))
	}
	return strings.Join(lines, "\n")
}

func referencePDFRunWords(chars []referencePDFTableChar, upright bool) []referencePDFTableChar {

	expansions := map[string]string{"ﬀ": "ff", "ﬃ": "ffi", "ﬄ": "ffl", "ﬁ": "fi", "ﬂ": "fl", "ﬆ": "st", "ﬅ": "st"}
	var words []referencePDFTableChar
	axis := 0
	if upright {
		axis = 1
	}
	for _, line := range referencePDFClusterChars(chars, axis) {
		sort.SliceStable(line, func(i, j int) bool {
			if upright {
				return line[i].box[0] < line[j].box[0]
			}
			if line[i].box[1] != line[j].box[1] {
				return line[i].box[1] < line[j].box[1]
			}
			return line[i].box[3] < line[j].box[3]
		})
		var current []referencePDFTableChar
		flush := func() {
			if len(current) == 0 {
				return
			}
			w := referencePDFTableChar{box: current[0].box}
			for _, c := range current {
				text, ok := expansions[c.text]
				if !ok {
					text = c.text
				}
				w.text += text
				w.box[0] = math.Min(w.box[0], c.box[0])
				w.box[1] = math.Min(w.box[1], c.box[1])
				w.box[2] = math.Max(w.box[2], c.box[2])
				w.box[3] = math.Max(w.box[3], c.box[3])
			}
			words = append(words, w)
			current = nil
		}
		for _, c := range line {
			// Reference str.isspace is false for the empty string.
			space := c.text != ""
			for _, r := range c.text {
				if !unicode.IsSpace(r) && !(r >= 0x1c && r <= 0x1f) {
					space = false
				}
			}
			if space {
				flush()
				continue
			}
			// With default split_at_punctuation="", Reference's string membership
			// still matches an empty decoded CID ("" in ""). Keep it as an
			// isolated empty word; dropping it changes inter-word spaces.
			if c.text == "" {
				flush()
				current = append(current, c)
				flush()
				continue
			}
			if len(current) > 0 {
				prev := current[len(current)-1]
				// Deliberately compare with the previous character, not word bounds;
				// the interline comparison is directional, not absolute distance.
				ax, bx, cx, ay, cy := prev.box[0], prev.box[2], c.box[0], prev.box[1], c.box[1]
				if !upright {
					ax, bx, cx, ay, cy = prev.box[1], prev.box[3], c.box[1], prev.box[0], c.box[0]
				}
				if cx < ax || cx > bx+3 || cy > ay+3 {
					flush()
				}
			}
			current = append(current, c)
		}
		flush()
	}
	return words
}

func TestReferencePDFUprightCellTextContracts(t *testing.T) {
	c := func(text string, x, top, right float64) referencePDFTableChar {
		return referencePDFTableChar{text, referencePDFCell{x, top, right, top + 8}}
	}
	for _, tc := range []struct {
		name  string
		chars []referencePDFTableChar
		want  string
	}{
		{"empty", nil, ""},
		{"spatial-order", []referencePDFTableChar{c("B", 5, 0, 8), c("A", 0, 0, 4)}, "AB"},
		{"inclusive-gap", []referencePDFTableChar{c("A", 0, 0, 4), c("B", 7, 0, 10)}, "AB"},
		{"beyond-gap", []referencePDFTableChar{c("A", 0, 0, 4), c("B", 7.01, 0, 10)}, "A B"},
		{"blank-break", []referencePDFTableChar{c("A", 0, 0, 4), c("\u00a0", 4, 0, 5), c("B", 5, 0, 9)}, "A B"},
		{"python-control-space", []referencePDFTableChar{c("A", 0, 0, 4), c("\x1c", 4, 0, 5), c("B", 5, 0, 9)}, "A B"},
		{"ligatures", []referencePDFTableChar{c("ﬀ", 0, 0, 2), c("ﬃ", 2, 0, 4), c("ﬄ", 4, 0, 6), c("ﬁ", 6, 0, 8), c("ﬂ", 8, 0, 10), c("ﬆ", 10, 0, 12), c("ﬅ", 12, 0, 14)}, "ffffifflfiflstst"},
		{"line-break", []referencePDFTableChar{c("B", 0, 12, 4), c("A", 0, 0, 4)}, "A\nB"},
		{"chained-tops", []referencePDFTableChar{c("A", 0, 0, 4), c("B", 4, 3, 8), c("C", 8, 6, 12)}, "ABC"},
		{"previous-char-not-union", []referencePDFTableChar{c("A", 0, 0, 30), c("B", 4, 0, 5), c("C", 9, 0, 12)}, "AB C"},
		{"upward-top-not-absolute", []referencePDFTableChar{c("A", 0, 6, 4), c("B", 4, 3, 8), c("C", 8, 0, 12)}, "ABC"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := referencePDFUprightCellText(tc.chars); got != tc.want {
				t.Fatalf("text=%q; want %q", got, tc.want)
			}
		})
	}
}

// Supplemental contracts, UNRUN. These exercise the default rotated directions
// and itertools.groupby semantics, not arbitrary user-selected reading modes.
func TestReferencePDFRotatedCellTextContracts(t *testing.T) {
	c := func(text string, x, top, right, bottom float64, upright bool) referencePDFDirectedChar {
		return referencePDFDirectedChar{referencePDFTableChar{text, referencePDFCell{x, top, right, bottom}}, upright}
	}
	for _, tc := range []struct {
		name  string
		chars []referencePDFDirectedChar
		want  string
	}{
		{"vertical-spatial-order", []referencePDFDirectedChar{c("B", 0, 5, 8, 9, false), c("A", 0, 0, 8, 4, false)}, "AB"},
		{"vertical-inclusive-gap", []referencePDFDirectedChar{c("A", 0, 0, 8, 4, false), c("B", 0, 7, 8, 11, false)}, "AB"},
		{"vertical-beyond-gap", []referencePDFDirectedChar{c("A", 0, 0, 8, 4, false), c("B", 0, 7.01, 8, 11, false)}, "A\nB"},
		{"vertical-secondary-bottom-sort", []referencePDFDirectedChar{c("B", 0, 0, 8, 8, false), c("A", 0, 0, 8, 4, false)}, "AB"},
		{"rotated-column-order", []referencePDFDirectedChar{c("B", 12, 0, 20, 4, false), c("A", 0, 0, 8, 4, false)}, "A B"},
		{"consecutive-orientation-runs", []referencePDFDirectedChar{c("A", 0, 0, 4, 8, true), c("B", 4, 0, 8, 8, false), c("C", 8, 0, 12, 8, true)}, "A B C"},
		{"run-order-within-final-line", []referencePDFDirectedChar{c("C", 8, 0, 12, 8, true), c("B", 4, 0, 8, 8, false), c("A", 0, 0, 4, 8, true)}, "C B A"},
		{"rotated-chained-columns", []referencePDFDirectedChar{c("A", 0, 0, 8, 4, false), c("B", 3, 4, 11, 8, false), c("C", 6, 8, 14, 12, false)}, "ABC"},
		{"rotated-ligature", []referencePDFDirectedChar{c("ﬁ", 0, 0, 8, 4, false), c("X", 0, 4, 8, 8, false)}, "fiX"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := referencePDFDefaultCellText(tc.chars); got != tc.want {
				t.Fatalf("text=%q; want %q", got, tc.want)
			}
		})
	}
}
