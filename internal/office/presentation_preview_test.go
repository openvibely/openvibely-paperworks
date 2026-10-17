package office

import (
	"bytes"
	"github.com/openvibely/openvibely-paperworks/internal/testsupport"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/beevik/etree"
)

func TestReferencePresentationStandardPreview(t *testing.T) {
	referencePresentationRoot(t)
	dir := referencePresentationRoot(t)
	for _, kind := range []string{"pie", "doughnut", "scatter"} {
		t.Run(kind, func(t *testing.T) {
			source := filepath.Join("../../testdata/fixtures", "reference-"+kind+".pptx")
			before, e := os.ReadFile(source)
			if e != nil {
				t.Fatal(e)
			}
			out := referencePresentationOutput(t, dir)
			changed, e := OfficePreviewCopy(source, out)
			if e != nil || !changed {
				t.Fatal(changed, e)
			}
			z := referencePreviewZIP(t, out)
			parts := referencePreviewChartParts(t, z)
			if len(parts) == 0 {
				t.Fatal("no chart parts")
			}
			d := etree.NewDocument()
			if e := referencePreviewReadXML(d, referencePreviewZIPRead(t, z, parts[0])); e != nil {
				t.Fatal(e)
			}
			if kind == "scatter" {
				s := referencePreviewFind(d.Root(), [2]string{NSC, "ser"})
				if s == nil {
					t.Fatal("missing first series")
				}
				if referencePreviewDirect(s, [2]string{NSC, "spPr"}, [2]string{NSA, "ln"}, [2]string{NSA, "noFill"}) == nil {
					t.Fatal("missing noFill")
				}
				symbol := referencePreviewDirect(s, [2]string{NSC, "marker"}, [2]string{NSC, "symbol"})
				if symbol == nil || symbol.SelectAttrValue("val", "") != "circle" {
					t.Fatal("missing circle marker")
				}
				lit := referencePreviewDirect(s, [2]string{NSC, "xVal"}, [2]string{NSC, "numLit"})
				if lit == nil || len(Children(lit, NSC, "pt")) != 3 {
					t.Fatal("wrong x values")
				}
			} else {
				c := referencePreviewFind(d.Root(), [2]string{NSC, kind + "Chart"})
				if c == nil {
					t.Fatal("missing chart")
				}
				v := Child(c, NSC, "varyColors")
				if v == nil || v.SelectAttrValue("val", "") != "1" {
					t.Fatal("missing varying colors")
				}
				var labels []string
				for _, lit := range All(c, NSC, "strLit") {
					if lit == c {
						continue
					}
					for _, pt := range Children(lit, NSC, "pt") {
						for _, value := range Children(pt, NSC, "v") {
							labels = append(labels, referencePreviewText(value))
						}
					}
				}
				if !reflect.DeepEqual(labels, []string{"A", "B", "C"}) {
					t.Fatal(labels)
				}
			}
			after, e := os.ReadFile(source)
			if e != nil || !bytes.Equal(before, after) {
				t.Fatal("source changed", e)
			}
		})
	}
}

func TestReferencePresentationExplicitStandardStyle(t *testing.T) {
	referencePresentationRoot(t)
	load := func(kind string) *etree.Document {
		z := referencePreviewZIP(t, filepath.Join("../../testdata/fixtures", "reference-"+kind+".pptx"))
		parts := referencePreviewChartParts(t, z)
		if len(parts) == 0 {
			t.Fatal("no chart parts")
		}
		d := etree.NewDocument()
		if err := referencePreviewReadXML(d, referencePreviewZIPRead(t, z, parts[0])); err != nil {
			t.Fatal(err)
		}
		return d
	}
	serialized := func(element *etree.Element) []byte {
		// Reference compares tostring(element), not the surrounding document.
		b, e := referencePreviewSerializeElement(element)
		if e != nil {
			t.Fatal(e)
		}
		return b
	}
	pie := load("pie")
	chart := All(pie.Root(), NSC, "pieChart")[0]
	v := NewChild(chart, NSC, "varyColors", nil)
	chart.InsertChildAt(0, v)
	v.CreateAttr("val", "0")
	before := serialized(pie.Root())
	if StandardChartPreview(pie.Root()) {
		t.Fatal("modified explicit pie")
	}
	if !bytes.Equal(before, serialized(pie.Root())) {
		t.Fatal("pie XML changed")
	}
	scatter := load("scatter")
	chart = All(scatter.Root(), NSC, "scatterChart")[0]
	style := Child(chart, NSC, "scatterStyle")
	style.CreateAttr("val", "smoothMarker")
	before = serialized(scatter.Root())
	if StandardChartPreview(scatter.Root()) {
		t.Fatal("modified smooth marker")
	}
	if !bytes.Equal(before, serialized(scatter.Root())) {
		t.Fatal("scatter XML changed")
	}
	style.CreateAttr("val", "marker")
	s := Child(chart, NSC, "ser")
	marker := NewChild(s, NSC, "marker", nil)
	NewChild(marker, NSC, "symbol", nil).CreateAttr("val", "diamond")
	NewChild(marker, NSC, "size", nil).CreateAttr("val", "8")
	props := NewChild(marker, NSC, "spPr", nil)
	// lxml creates the absent drawing namespace at this new subtree, rather
	// than changing the chart root before the smoothMarker assertions.
	fill := props.CreateElement("ns0:solidFill")
	fill.CreateAttr("xmlns:ns0", NSA)
	fill.CreateElement("ns0:srgbClr").CreateAttr("val", "FF0000")
	before = serialized(marker)
	StandardChartPreview(scatter.Root())
	after := serialized(marker)
	if !bytes.Equal(before, after) {
		t.Fatalf("explicit marker changed:\nbefore: %s\nafter: %s", before, after)
	}
}

// Read shape geometry directly from the saved preview, independently of the
// preview layout functions used to produce it.
func referencePreviewShapes(t *testing.T, path string) ([]*etree.Element, []string) {
	t.Helper()
	roots := referencePresentationRoots(t, path)
	if len(roots) == 0 {
		t.Fatal("missing first slide")
	}
	tree := Child(Child(roots[0], NSP, "cSld"), NSP, "spTree")
	var shapes []*etree.Element
	for _, shape := range tree.ChildElements() {
		if shape.NamespaceURI() == NSP && strings.Contains("|sp|grpSp|graphicFrame|cxnSp|pic|contentPart|", "|"+shape.Tag+"|") {
			shapes = append(shapes, shape)
		}
	}
	texts := referenceHistogramShapeTexts(t, roots[0])
	return shapes, texts
}

func referenceShapeName(t *testing.T, s *etree.Element) string {
	t.Helper()
	name, supported, err := referenceHistogramShapeName(s)
	if err != nil || !supported {
		t.Fatalf("shape name: supported=%v err=%v", supported, err)
	}
	return name
}
func referenceShapeMetric(t *testing.T, s *etree.Element, element, attr string) float64 {
	t.Helper()
	node := childPath(s, [2]string{NSP, "spPr"}, [2]string{NSA, "xfrm"}, [2]string{NSA, element})
	if node == nil {
		t.Fatal("missing shape geometry")
	}
	v, e := strconv.ParseInt(strings.TrimSpace(node.SelectAttrValue(attr, "")), 10, 64)
	if e != nil {
		t.Fatal(e)
	}
	return float64(v)
}

func TestReferenceFunnelAndWaterfallGeometry(t *testing.T) {
	for _, kind := range []string{"funnel", "waterfall"} {
		t.Run(kind, func(t *testing.T) {
			source := filepath.Join("../../testdata/fixtures", "reference-"+kind+".pptx")
			before, e := os.ReadFile(source)
			if e != nil {
				t.Fatal(e)
			}
			// These subtests port two separate Reference unittest methods.
			out := referencePresentationOutput(t, t.TempDir())
			if changed, e := OfficePreviewCopy(source, out); e != nil || !changed {
				t.Fatal(changed, e)
			}
			shapes, texts := referencePreviewShapes(t, out)
			prefix := "Funnel stage "
			if kind == "waterfall" {
				prefix = "Waterfall step "
			}
			var bars []*etree.Element
			var names []string
			for _, s := range shapes {
				n := referenceShapeName(t, s)
				if strings.HasPrefix(n, prefix) {
					bars = append(bars, s)
					names = append(names, n)
				}
			}
			labels := []string{"A", "B", "C"}
			if kind == "funnel" {
				want := []string{"Funnel stage 1: A, value 10", "Funnel stage 2: B, value 20", "Funnel stage 3: C, value 5"}
				if !reflect.DeepEqual(names, want) {
					t.Fatal(names)
				}
				widths := []float64{}
				centers := []float64{}
				for _, b := range bars {
					w := referenceShapeMetric(t, b, "ext", "cx")
					widths = append(widths, w)
					centers = append(centers, referenceShapeMetric(t, b, "off", "x")+w/2)
				}
				if widths[1] == 0 {
					t.Fatal("zero width: Python division would fail")
				}
				referencePreviewAlmostFourPlaces(t, widths[0]/widths[1], .5)
				referencePreviewAlmostFourPlaces(t, widths[2]/widths[1], .25)
				for _, center := range centers {
					if math.Abs(center-centers[0]) > 2 {
						t.Fatal(centers)
					}
				}
			} else {
				want := []string{"Waterfall step 1: A, 0 to 10", "Waterfall step 2: B, 10 to 30", "Waterfall step 3: C, 30 to 35"}
				if !reflect.DeepEqual(names, want) {
					t.Fatal(names)
				}
				h0 := referenceShapeMetric(t, bars[0], "ext", "cy")
				h1 := referenceShapeMetric(t, bars[1], "ext", "cy")
				h2 := referenceShapeMetric(t, bars[2], "ext", "cy")
				if h0 == 0 {
					t.Fatal("zero height: Python division would fail")
				}
				referencePreviewAlmostFourPlaces(t, h1/h0, 2)
				referencePreviewAlmostFourPlaces(t, h2/h0, .5)
				if math.Abs(referenceShapeMetric(t, bars[0], "off", "y")-referenceShapeMetric(t, bars[1], "off", "y")-h1) > 2 {
					t.Fatal("steps are not cumulative")
				}
				labels = append(labels, "Increase", "Decrease", "Total", "40.0")
			}
			for _, label := range labels {
				if !slices.Contains(texts, label) {
					t.Errorf("missing %q in %v", label, texts)
				}
			}
			after, e := os.ReadFile(source)
			if e != nil || !bytes.Equal(before, after) {
				t.Fatal("source changed", e)
			}
		})
	}
}

func TestReferenceHistogramObservationPreview(t *testing.T) {
	referencePresentationRoot(t)
	source := "../../testdata/fixtures/reference-categorical-histogram.pptx"
	before, e := os.ReadFile(source)
	if e != nil {
		t.Fatal(e)
	}
	if need, e := NeedsOfficePreview(source); e != nil || !need {
		t.Fatal(need, e)
	}
	out := referenceNextPresentation(t)
	if changed, e := OfficePreviewCopy(source, out); e != nil || !changed {
		t.Fatal(changed, e)
	}
	after, e := os.ReadFile(source)
	if e != nil || !bytes.Equal(before, after) {
		t.Fatal("source changed", e)
	}
	roots := referencePresentationRoots(t, out)
	if len(roots) == 0 {
		t.Fatal("missing first slide")
	}
	slide := roots[0]
	labels := referenceHistogramShapeTexts(t, slide)
	for _, label := range []string{"[5, 25]", "3"} {
		if !slices.Contains(labels, label) {
			t.Fatal("missing", label, labels)
		}
	}
	// reference presentation library checks top-level GraphicFrame.has_chart by graphicData URI.
	if referenceHistogramHasChart(t, slide) {
		t.Fatal("preview retains native chart shape")
	}
	bars := 0
	tree := Child(Child(slide, NSP, "cSld"), NSP, "spTree")
	for _, shape := range tree.ChildElements() {
		name, supported, err := referenceHistogramShapeName(shape)
		if err != nil {
			t.Fatal(err)
		}
		if !supported {
			continue
		}
		if strings.HasPrefix(name, "Histogram bin ") {
			bars++
		}
	}
	if bars != 1 {
		t.Fatal("wrong bin count", bars)
	}
	oldZIP := referencePreviewZIP(t, source)
	newZIP := referencePreviewZIP(t, out)
	for _, part := range referencePreviewChartParts(t, oldZIP) {
		if !bytes.Equal(referencePreviewZIPRead(t, oldZIP, part), referencePreviewZIPRead(t, newZIP, part)) {
			t.Fatal("native chart modified", part)
		}
	}
}

func TestReferenceHierarchyNativePreservation(t *testing.T) {
	referencePresentationRoot(t)
	for _, kind := range []string{"treemap", "sunburst"} {
		t.Run(kind, func(t *testing.T) {
			source := filepath.Join("../../testdata/fixtures", "reference-hierarchy-"+kind+".pptx")
			before, e := os.ReadFile(source)
			if e != nil {
				t.Fatal(e)
			}
			out := referenceNextPresentation(t)
			if changed, e := OfficePreviewCopy(source, out); e != nil || !changed {
				t.Fatal(changed, e)
			}
			after, e := os.ReadFile(source)
			if e != nil || !bytes.Equal(before, after) {
				t.Fatal("source changed", e)
			}
			_, texts := referencePreviewShapes(t, out)
			labels := strings.Join(texts, " ")
			for _, label := range []string{"North", "South", "A", "B", "C"} {
				if !strings.Contains(labels, label) {
					t.Error("missing", label, labels)
				}
			}
			old := previewOpen(t, source)
			roots := referencePresentationRoots(t, out)
			if len(roots) == 0 {
				t.Fatal("missing first slide")
			}
			if referenceHistogramHasChart(t, roots[0]) {
				t.Fatal("preview retains native chart shape")
			}
			shapes, _ := referencePreviewShapes(t, out)
			if len(shapes) <= 5 {
				t.Fatal("insufficient preview shapes", len(shapes))
			}
			oldZIP, newZIP := referencePreviewZIP(t, source), referencePreviewZIP(t, out)
			for _, part := range referencePreviewChartParts(t, oldZIP) {
				if !bytes.Equal(referencePreviewZIPRead(t, oldZIP, part), referencePreviewZIPRead(t, newZIP, part)) {
					t.Fatal("native chart modified", part)
				}
			}
			reports, e := ExtendedCharts(old, ExtendedChartSpec{})
			if e != nil || reports[0].Series[0].Type != kind {
				t.Fatal(reports, e)
			}
		})
	}
}

func TestReferenceEditedHistogramPreviewStyle(t *testing.T) {
	referencePresentationRoot(t)
	p := fixture(t, "reference-categorical-histogram.pptx")
	part := ""
	z := referencePreviewZIP(t, "../../testdata/fixtures/reference-categorical-histogram.pptx")
	for _, name := range p.Names() {
		if strings.HasSuffix(name, ".xml") && Is(referencePackageXML(t, z, name), NSCX, "chartSpace") {
			part = name
			break
		}
	}
	if part == "" {
		t.Fatal("missing extended chartSpace")
	}
	d, e := p.XML(part)
	if e != nil {
		t.Fatal(e)
	}
	d.Root().CreateAttr("xmlns:a", NSA)
	chart := Child(d.Root(), NSCX, "chart")
	title := NewChild(chart, NSCX, "title", nil)
	chart.InsertChildAt(0, title)
	title.CreateAttr("overlay", "0")
	NewChild(NewChild(NewChild(title, NSCX, "tx", nil), NSCX, "txData", nil), NSCX, "v", nil).SetText("Sample distribution")
	series := All(chart, NSCX, "series")[0]
	props := Child(series, NSCX, "spPr")
	if props == nil {
		props = NewChild(series, NSCX, "spPr", nil)
	}
	for _, fill := range Children(props, NSA, "solidFill") {
		props.RemoveChild(fill)
	}
	NewChild(NewChild(props, NSA, "solidFill", nil), NSA, "srgbClr", nil).CreateAttr("val", "C01020")
	if e = p.SetXML(part, d); e != nil {
		t.Fatal(e)
	}
	p = previewOpen(t, referenceWriteMutation(t, p))
	if _, e = ExtendedCharts(p, ExtendedChartSpec{Action: "replace-data", Part: part, Values: []float64{7, 15, 30}}); e != nil {
		t.Fatal(e)
	}
	source := referenceNextPresentation(t)
	if e = p.SaveNew(source); e != nil {
		t.Fatal(e)
	}
	out := referenceNextPresentation(t)
	if _, e = OfficePreviewCopy(source, out); e != nil {
		t.Fatal(e)
	}
	shapes, texts := referencePreviewShapes(t, out)
	for _, label := range []string{"Sample distribution", "[7, 27]", "(27, 47]"} {
		if !slices.Contains(texts, label) {
			t.Fatal("missing", label, texts)
		}
	}
	var bars []*etree.Element
	for _, s := range shapes {
		if strings.HasPrefix(referenceShapeName(t, s), "Histogram bin ") {
			bars = append(bars, s)
		}
	}
	if len(bars) != 2 {
		t.Fatal("wrong bin count", len(bars))
	}
	referencePreviewAlmostFivePlaces(t, referenceShapeMetric(t, bars[0], "ext", "cy")/referenceShapeMetric(t, bars[1], "ext", "cy"), 2)
	for _, bar := range bars {
		color := childPath(bar, [2]string{NSP, "spPr"}, [2]string{NSA, "solidFill"}, [2]string{NSA, "srgbClr"})
		if referencePreviewRGB(t, color) != "C01020" {
			t.Fatal("bar color lost")
		}
	}
}

func TestReferenceTreemapFiveValueGeometry(t *testing.T) {
	referencePresentationRoot(t)
	tree := NewTree()
	for i, c := range []struct {
		label string
		value float64
	}{{"Alpha", 40}, {"Beta", 30}, {"Gamma", 15}, {"Delta", 10}, {"Epsilon", 5}} {
		tree.Get(c.label, i).Value = c.value
	}
	cells := TreemapCells(tree, 96, 98, 768, 420)
	want := [][4]float64{{96, 98, 537.6, 240}, {96, 338, 537.6, 180}, {633.6, 98, 230.4, 210}, {633.6, 308, 230.4, 140}, {633.6, 448, 230.4, 70}}
	// Reference zip compares the shorter sequence, then asserts the cell count.
	for i, cell := range cells[:min(len(cells), len(want))] {
		for j, got := range []float64{cell.X, cell.Y, cell.W, cell.H} {
			referencePreviewAlmostFivePlaces(t, got, want[i][j])
		}
	}
	if len(cells) != 5 {
		t.Fatal(cells)
	}
}

func TestReferenceHistogramAxesStyleAndScale(t *testing.T) {
	referencePresentationRoot(t)
	source := "../../testdata/fixtures/reference-histogram-axes.pptx"
	before, e := os.ReadFile(source)
	if e != nil {
		t.Fatal(e)
	}
	out := referenceNextPresentation(t)
	if _, e = OfficePreviewCopy(source, out); e != nil {
		t.Fatal(e)
	}
	shapes, labels := referencePreviewShapes(t, out)
	for _, text := range []string{"Frequency", "Observations", "0.0", "2.0", "4.0", "6.0", "8.0", "10.0"} {
		if !slices.Contains(labels, text) {
			t.Fatal("missing", text, labels)
		}
	}
	if slices.Contains(labels, "3") {
		t.Fatal("invented value label")
	}
	var bars []*etree.Element
	for _, s := range shapes {
		if strings.HasPrefix(referenceShapeName(t, s), "Histogram bin ") {
			bars = append(bars, s)
		}
	}
	if len(bars) == 0 {
		t.Fatal("missing bars")
	}
	slide := referencePresentationRoots(t, out)[0]
	var vertical []*etree.Element
	var grids []*etree.Element
	for _, line := range Children(Child(Child(slide, NSP, "cSld"), NSP, "spTree"), NSP, "cxnSp") {
		if referenceShapeMetric(t, line, "ext", "cx") == 0 && referenceShapeMetric(t, line, "ext", "cy") > 0 {
			vertical = append(vertical, line)
		}
		props := childPath(line, [2]string{NSP, "spPr"}, [2]string{NSA, "ln"})
		color := childPath(props, [2]string{NSA, "solidFill"}, [2]string{NSA, "srgbClr"})
		if color != nil && color.SelectAttrValue("val", "") == "C00000" {
			grids = append(grids, line)
		}
	}
	if len(vertical) != 1 {
		t.Fatal("vertical axis count", len(vertical))
	}
	referencePreviewAlmostFivePlaces(t, referenceShapeMetric(t, bars[0], "ext", "cy")/referenceShapeMetric(t, vertical[0], "ext", "cy"), .3)
	if len(grids) != 6 {
		t.Fatal("grid line count", len(grids))
	}
	for _, line := range grids {
		props := childPath(line, [2]string{NSP, "spPr"}, [2]string{NSA, "ln"})
		width, err := strconv.Atoi(props.SelectAttrValue("w", ""))
		if err != nil || width != 9525 {
			t.Fatal("grid line width", width, err)
		}
	}
	after, e := os.ReadFile(source)
	if e != nil || !bytes.Equal(before, after) {
		t.Fatal("source changed", e)
	}
}

// referencePresentationOutput mirrors PresentationNative.output: count every direct
// directory entry, without creating or reserving the selected path.
func referencePresentationOutput(t *testing.T, dir string) string {
	t.Helper()
	return testsupport.PresentationOutput(t, dir)
}
