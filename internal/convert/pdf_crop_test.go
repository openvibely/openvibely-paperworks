package convert

import (
	"fmt"
	"math"
	"os"
	"reflect"
	"strings"
	"testing"
)

// pdfplumber 0.11.10 geometry.clip_obj: retain edge intersections (but not
// point-only intersections), clip character boxes, and preserve text/order.
// Cropping is not midpoint selection or PDFium bounded-text extraction.
func referencePDFCropChars(chars []referencePDFDirectedChar, box referencePDFCell) []referencePDFDirectedChar {
	var out []referencePDFDirectedChar
	for _, c := range chars {
		b := c.char.box
		clipped := referencePDFCell{math.Max(b[0], box[0]), math.Max(b[1], box[1]), math.Min(b[2], box[2]), math.Min(b[3], box[3])}
		w, h := clipped[2]-clipped[0], clipped[3]-clipped[1]
		if w >= 0 && h >= 0 && w+h > 0 {
			c.char.box = clipped
			out = append(out, c)
		}
	}
	return out
}

// Bounded default upright-text path. This does not implement layout=True,
// rotated text or all PDFMiner character acquisition; callers keep those gaps
// explicit. The original column crops use the default upright configuration.
func referencePDFCropText(chars []referencePDFDirectedChar, box, page referencePDFCell) (string, error) {
	if box[0] >= box[2] || box[1] >= box[3] || box[0] < page[0] || box[1] < page[1] || box[2] > page[2] || box[3] > page[3] {
		return "", fmt.Errorf("invalid or outside page crop")
	}
	cropped := referencePDFCropChars(chars, box)
	for _, c := range cropped {
		if !c.upright {
			return "", fmt.Errorf("rotated crop text is not yet verified")
		}
	}
	return referencePDFDefaultCellText(cropped), nil
}

func TestReferenceStandaloneRuntimeColumnCrops(t *testing.T) {
	data, err := os.ReadFile("../../testdata/reference-mirror/runtime-two-column.pdf")
	if err != nil {
		t.Fatal(err)
	}
	g := referencePDFPageGeometry(t, data, 0, true)
	for _, c := range []struct {
		name string
		box  referencePDFCell
		want []string
	}{
		{"revenue", referencePDFCell{20, 70, 280, 170}, []string{"Revenue", "Q1: $120", "Q2: $180"}},
		{"costs", referencePDFCell{310, 70, 580, 170}, []string{"Costs", "Q1: $40", "Q2: $50"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			got, err := referencePDFCropText(g.chars, c.box, referencePDFCell{0, 0, 600, 400})
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(strings.Split(got, "\n"), c.want) {
				t.Fatalf("crop lines=%q want %q", got, c.want)
			}
		})
	}
}

func TestReferencePDFCropIntersections(t *testing.T) {
	c := func(s string, b referencePDFCell) referencePDFDirectedChar {
		return referencePDFDirectedChar{referencePDFTableChar{s, b}, true}
	}
	input := []referencePDFDirectedChar{c("edge", referencePDFCell{0, 2, 10, 8}), c("point", referencePDFCell{0, 0, 10, 0}), c("partial", referencePDFCell{9, 1, 14, 6}), c("outside", referencePDFCell{0, 0, 9, 9})}
	want := []referencePDFDirectedChar{c("edge", referencePDFCell{10, 2, 10, 8}), c("partial", referencePDFCell{10, 1, 14, 6})}
	if got := referencePDFCropChars(input, referencePDFCell{10, 0, 20, 10}); !reflect.DeepEqual(got, want) {
		t.Fatalf("%+v want %+v", got, want)
	}
	if input[2].char.box[0] != 9 {
		t.Fatal("crop mutated original")
	}
}
