package office

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/beevik/etree"
)

func TestReferenceHistogramFrequencyBoundaries(t *testing.T) {
	for _, c := range []struct {
		name    string
		values  []float64
		closed  string
		options map[string]string
		want    []Bin
	}{
		{"default", []float64{7, 15, 30}, "r", nil, []Bin{{"[7, 27]", 2}, {"(27, 47]", 1}}},
		{"integer_count", []float64{0, 1, 2, 3, 4}, "r", map[string]string{"binCount": "2"}, []Bin{{"[0, 2]", 3}, {"(2, 4]", 2}}},
		{"float_count", []float64{0, 1, 2, 3, 4}, "r", map[string]string{"binCount": "2.0"}, []Bin{{"[0, 2]", 3}, {"(2, 4]", 2}}},
		{"left_closed", []float64{0, 1, 2, 3, 4}, "l", map[string]string{"binCount": "2"}, []Bin{{"[0, 2)", 2}, {"[2, 4]", 3}}},
		{"under_overflow", []float64{-2, 0, 2, 4, 6}, "r", map[string]string{"underflow": "0", "overflow": "4", "binCount": "4"}, []Bin{{"≤ 0", 2}, {"(0, 2]", 1}, {"(2, 4]", 1}, {"> 4", 1}}},
		{"constant", []float64{3, 3, 3}, "r", nil, []Bin{{"[3, 4]", 3}}},
		{"decimal", []float64{0, .1, .2, .3}, "r", map[string]string{"binSize": "0.1"}, []Bin{{"[0, 0.1]", 2}, {"(0.1, 0.2]", 1}, {"(0.2, 0.3]", 1}}},
		{"large_offset", []float64{1000000, 1000000.1, 1000000.2}, "r", map[string]string{"binCount": "2"}, []Bin{{"[1000000, 1000000.1]", 2}, {"(1000000.1, 1000000.2]", 1}}},
	} {
		got, e := HistogramBins(c.values, referenceBinning(c.closed, c.options))
		if e != nil || !reflect.DeepEqual(got, c.want) {
			t.Fatalf("got %v (%v), want %v", got, e, c.want)
		}
	}
	for _, opts := range []map[string]string{{"underflow": "4", "overflow": "2"}, {"underflow": "0", "overflow": "4", "binCount": "2"}, {"binCount": "1001"}} {
		if _, e := HistogramBins([]float64{0, 1, 2, 3, 4}, referenceBinning("r", opts)); e == nil {
			t.Fatal("accepted", opts)
		}
	}
}

func referenceBinning(closed string, options map[string]string) *etree.Element {
	e := etree.NewElement("cx:binning")
	e.CreateAttr("xmlns:cx", NSCX)
	e.CreateAttr("intervalClosed", closed)
	for key, v := range options {
		if key == "binSize" || key == "binCount" {
			e.CreateElement("cx:" + key).SetText(v)
		} else {
			e.CreateAttr(key, v)
		}
	}
	return e
}

func TestReferenceHistogramNumberFormatting(t *testing.T) {
	for _, group := range []struct {
		name, fixture string
		cases         []struct {
			value      float64
			code, want string
		}
		labels []string
	}{
		{"padding", "reference-histogram-padding.pptx", []struct {
			value      float64
			code, want string
		}{{2, "0000", "0002"}, {0, "0000", "0000"}, {-2, "0000", "-0002"}, {12345, "0000", "12345"}, {2.5, "0000.00", "0002.50"}, {2, "00,000", "00,002"}, {-2, "0000;(0000)", "(0002)"}}, []string{"0000", "0002", "0004", "0006", "0008", "0010"}},
		{"scientific", "reference-histogram-scientific.pptx", []struct {
			value      float64
			code, want string
		}{{2, "0.00E+00", "2.00E+00"}, {0, "0.00E+00", "0.00E+00"}, {-.02, "0.00E+00", "-2.00E-02"}, {10, "0.00E+00", "1.00E+01"}, {9.999, "0.00E+00", "1.00E+01"}, {100, "0.0E-00", "1.0E02"}, {.001, "0.0e-00", "1.0e-03"}, {12345, "##0.0E+00", "12.3E+03"}, {1e-308, "0.00E+00", "1.00E-308"}, {1e308, "0.00E+00", "1.00E+308"}, {120, "0.##E+00", "1.2E+02"}}, []string{"0.00E+00", "2.00E+00", "4.00E+00", "6.00E+00", "8.00E+00", "1.00E+01"}},
	} {
		t.Run(group.name, func(t *testing.T) {
			for i, c := range group.cases {
				t.Run(strconv.Itoa(i), func(t *testing.T) {
					s, e := ChartNumber(c.value, c.code)
					if e != nil || s != c.want {
						t.Fatalf("%g %q: got %q (%v), want %q", c.value, c.code, s, e, c.want)
					}
				})
			}
			// Exact original fixture; reconstruction from the axes deck changed 17 parts.
			source := filepath.Join("..", "..", "testdata", "fixtures", group.fixture)
			before, e := os.ReadFile(source)
			if e != nil {
				t.Fatal(e)
			}
			// Each group represents a distinct original unittest method.
			out := referencePresentationOutput(t, t.TempDir())
			if changed, e := OfficePreviewCopy(source, out); e != nil || !changed {
				t.Fatal(changed, e)
			}
			p := previewOpen(t, out)
			slides, e := PresentationSlides(p)
			if e != nil || len(slides) == 0 {
				t.Fatal("first slide", slides, e)
			}
			slide, e := p.XML(slides[0])
			if e != nil {
				t.Fatal(e)
			}
			labels := referenceHistogramShapeTexts(t, slide.Root())
			for _, label := range group.labels {
				if !slices.Contains(labels, label) {
					t.Fatalf("missing %q in %v", label, labels)
				}
			}
			after, e := os.ReadFile(source)
			if e != nil || !bytes.Equal(before, after) {
				t.Fatal("source changed", e)
			}
		})
	}
}

func TestReferenceHistogramInvalidAxes(t *testing.T) {
	for _, c := range []struct {
		value      float64
		code, want string
	}{{1234.5, "$#,##0.00", "$1,234.50"}, {.125, "0.0%", "12.5%"}, {-12.5, "0.0;(0.0)", "(12.5)"}, {1.2, "0.###", "1.2"}, {0, `0.0;(0.0);"-"`, "-"}, {1500000, `0.0,,"M"`, "1.5M"}, {.125, `0.000"%"`, "0.125%"}} {
		got, e := ChartNumber(c.value, c.code)
		if e != nil || got != c.want {
			t.Fatalf("got %q (%v), want %q", got, e, c.want)
		}
	}
	for _, attrs := range []map[string]string{{"min": "10", "max": "0"}, {"max": "nan"}, {"majorUnit": "0"}, {"majorUnit": "0.00000001"}} {
		axis := etree.NewElement("cx:axis")
		axis.CreateAttr("xmlns:cx", NSCX)
		scaling := axis.CreateElement("cx:valScaling")
		for k, v := range attrs {
			scaling.CreateAttr(k, v)
		}
		if _, _, _, e := HistogramScale(axis, 3); e == nil {
			t.Fatal("accepted invalid axis", attrs)
		}
	}
	if _, e := ChartNumber(3, "yyyy-mm-dd"); e == nil {
		t.Fatal("accepted date format")
	}
}

// Match reference presentation library slide.shapes / has_text_frame / shape.text for these decks.
// Groups are not recursively flattened; paragraph and soft-break boundaries matter.
func referenceHistogramShapeTexts(t *testing.T, slide *etree.Element) []string {
	t.Helper()
	tree := Child(Child(slide, NSP, "cSld"), NSP, "spTree")
	if tree == nil {
		t.Fatal("missing slide shape tree")
	}
	var labels []string
	for _, shape := range Children(tree, NSP, "sp") {
		body := Child(shape, NSP, "txBody")
		// AutoShape.has_text_frame is true even without txBody; its reader creates
		// an empty body in memory. The resulting text is empty.
		if body == nil {
			labels = append(labels, "")
			continue
		}
		var paragraphs []string
		for _, para := range Children(body, NSA, "p") {
			var text strings.Builder
			for _, item := range para.ChildElements() {
				switch {
				case Is(item, NSA, "br"):
					text.WriteByte('\v')
				case Is(item, NSA, "r"), Is(item, NSA, "fld"):
					value := Child(item, NSA, "t")
					if value == nil {
						if Is(item, NSA, "r") {
							t.Fatal("run missing required a:t")
						}
						continue
					}
					// lxml .text is initial character data only, not descendant text/tails.
					for _, token := range value.Child {
						data, ok := token.(*etree.CharData)
						if !ok {
							break
						}
						text.WriteString(data.Data)
					}
				}
			}
			paragraphs = append(paragraphs, text.String())
		}
		labels = append(labels, strings.Join(paragraphs, "\n"))
	}
	return labels
}
