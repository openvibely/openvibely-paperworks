package convert

import (
	"bytes"
	"context"
	pdfcpuapi "github.com/pdfcpu/pdfcpu/pkg/api"
	"math"
	"strings"
	"testing"
)

// Independent acquisition for the bounded default-direction runtime readers.
// General font/operator acquisition limits remain visible in PageGeometry.
func referenceRuntimePDFTexts(t *testing.T, data []byte) []string {
	t.Helper()
	count, err := pdfcpuapi.PageCount(context.Background(), bytes.NewReader(data), nil)
	if err != nil {
		t.Fatal(err)
	}
	texts := make([]string, count)
	for i := range texts {
		texts[i] = referenceRuntimePDFTextMap(referencePDFDefaultWords(referencePDFPageGeometry(t, data, i, true).chars))
	}
	return texts
}

// The axis test requests pdfplumber line_dir_rotated=ltr,char_dir_rotated=btt.
// Reflect the rotated glyphs' Y bounds during word grouping; return word boxes
// to page coordinates before the ordinary top-to-bottom line clustering.
func referenceRuntimePDFBottomToTopText(t *testing.T, data []byte, page int) string {
	chars := referencePDFPageGeometry(t, data, page, true).chars
	var words []referencePDFTableChar
	for i := 0; i < len(chars); {
		j := i + 1
		for j < len(chars) && chars[j].upright == chars[i].upright {
			j++
		}
		run := make([]referencePDFTableChar, 0, j-i)
		for _, c := range chars[i:j] {
			v := c.char
			if !c.upright {
				v.box[1], v.box[3] = -v.box[3], -v.box[1]
			}
			run = append(run, v)
		}
		group := referencePDFRunWords(run, chars[i].upright)
		if !chars[i].upright {
			for k := range group {
				group[k].box[1], group[k].box[3] = -group[k].box[3], -group[k].box[1]
			}
		}
		words = append(words, group...)
		i = j
	}
	return referenceRuntimePDFTextMap(words)
}

// Pinned pdfplumber 0.11.10 chars_to_textmap sets presorted=True.
// cluster_objects(preserve_order=True) groups consecutive cluster IDs, not all
// words sharing a height. This differs from table extract_text's final sorting.
func referenceRuntimePDFTextMap(words []referencePDFTableChar) string {
	ids := map[float64]int{}
	for id, group := range referencePDFClusterChars(words, 1) {
		for _, w := range group {
			ids[w.box[1]] = id
		}
	}
	var lines []string
	previous := -1
	for _, w := range words {
		id := ids[w.box[1]]
		if id != previous {
			lines = append(lines, w.text)
		} else {
			lines[len(lines)-1] += " " + w.text
		}
		previous = id
	}
	return strings.Join(lines, "\n")
}

func TestReferenceRuntimePDFTextMapPreservesOrientationRuns(t *testing.T) {
	w := func(text string, top float64) referencePDFTableChar {
		return referencePDFTableChar{text, referencePDFCell{0, top, 5, top + 5}}
	}
	for _, c := range []struct {
		name  string
		words []referencePDFTableChar
		want  string
	}{
		{"separate-runs", []referencePDFTableChar{w("A", 0), w("B", 20), w("C", 0)}, "A\nB\nC"},
		{"adjacent-tolerance", []referencePDFTableChar{w("A", 0), w("B", 3), w("C", 20)}, "A B\nC"},
		{"empty", nil, ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := referenceRuntimePDFTextMap(c.words); got != c.want {
				t.Fatalf("%q != %q", got, c.want)
			}
		})
	}
}

// pdfplumber.Page.width/height use the normalized, inherited MediaBox (not
// CropBox) with axes swapped for quarter turns. Read dictionary numbers directly
// so a production PageDims implementation cannot validate itself.
func referenceRuntimePDFDimensions(t *testing.T, data []byte) []struct{ Width, Height float64 } {
	t.Helper()
	c, err := pdfcpuapi.ReadContext(context.Background(), bytes.NewReader(data), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.EnsurePageCount(); err != nil {
		t.Fatal(err)
	}
	dims := make([]struct{ Width, Height float64 }, c.PageCount)
	for i := range dims {
		_, _, attrs, err := c.PageDict(context.Background(), i+1, false)
		if err != nil {
			t.Fatal(err)
		}
		if attrs.MediaBox == nil {
			t.Fatal("missing inherited MediaBox")
		}
		b := attrs.MediaBox
		width, height := math.Abs(b.UR.X-b.LL.X), math.Abs(b.UR.Y-b.LL.Y)
		rotation := ((attrs.Rotate % 360) + 360) % 360
		if rotation == 90 || rotation == 270 {
			width, height = height, width
		}
		dims[i].Width, dims[i].Height = width, height
	}
	return dims
}
