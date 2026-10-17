package office

import (
	"archive/zip"
	"bytes"
	"encoding/hex"
	"errors"
	"io"
	"math"
	"math/big"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/beevik/etree"

	"github.com/openvibely/openvibely-paperworks/internal/opc"
)

// Mirrors SpreadsheetThreadsTrace.chart_preview and its three callers in
// test_spreadsheet_threads_trace.py, including exact unrelated-part preservation.
func TestReferenceChartPreviewPreservation(t *testing.T) {
	referencePresentationRoot(t)
	for _, kind := range []string{"pie", "doughnut", "scatter"} {
		t.Run(kind, func(t *testing.T) {
			source := filepath.Join("..", "..", "testdata", "fixtures", "reference-"+kind+".xlsx")
			before, err := os.ReadFile(source)
			if err != nil {
				t.Fatal(err)
			}
			need, err := NeedsOfficePreview(source)
			if err != nil || !need {
				t.Fatalf("need: %v %v", need, err)
			}
			output := filepath.Join(t.TempDir(), "preview.xlsx")
			changed, err := OfficePreviewCopy(source, output)
			if err != nil || !changed {
				t.Fatalf("preview: %v %v", changed, err)
			}
			after, err := os.ReadFile(source)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatal("source changed", err)
			}
			old, err := opc.Open(source)
			if err != nil {
				t.Fatal(err)
			}
			charts, err := ChartParts(old)
			if err != nil || len(charts) != 1 {
				t.Fatalf("charts: %v %v", charts, err)
			}
			oldZIP := referencePreviewZIP(t, source)
			newZIP := referencePreviewZIP(t, output)
			oldNames, newNames := map[string]bool{}, map[string]bool{}
			for _, f := range oldZIP.File {
				oldNames[f.Name] = true
			}
			for _, f := range newZIP.File {
				newNames[f.Name] = true
			}
			if !reflect.DeepEqual(oldNames, newNames) {
				t.Fatal("ZIP name set changed")
			}
			for _, f := range oldZIP.File {
				if f.Name != charts[0] && !bytes.Equal(referencePreviewZIPRead(t, oldZIP, f.Name), referencePreviewZIPRead(t, newZIP, f.Name)) {
					t.Fatalf("unrelated part changed: %s", f.Name)
				}
			}
			doc := etree.NewDocument()
			if err := referencePreviewReadXML(doc, referencePreviewZIPRead(t, newZIP, charts[0])); err != nil {
				t.Fatal(err)
			}
			if kind != "scatter" {
				if e := referencePreviewFind(doc.Root(), [2]string{NSC, kind + "Chart"}, [2]string{NSC, "varyColors"}); e == nil || e.SelectAttrValue("val", "") != "1" {
					t.Fatal("missing varyColors=1")
				}
				ref := referencePreviewFind(doc.Root(), [2]string{NSC, "cat"}, [2]string{NSC, "strRef"}, [2]string{NSC, "f"})
				if ref == nil || referencePreviewText(ref) != "'Chart'!$A$2:$A$4" {
					t.Fatal("category reference changed")
				}
			} else {
				series := referencePreviewFind(doc.Root(), [2]string{NSC, "scatterChart"}, [2]string{NSC, "ser"})
				if referencePreviewDirect(series, [2]string{NSC, "spPr"}, [2]string{NSA, "ln"}, [2]string{NSA, "noFill"}) == nil {
					t.Fatal("connecting line retained")
				}
				if referencePreviewDirect(series, [2]string{NSC, "marker"}, [2]string{NSC, "spPr"}, [2]string{NSA, "solidFill"}) == nil {
					t.Fatal("missing marker fill")
				}
				ref := referencePreviewDirect(series, [2]string{NSC, "yVal"}, [2]string{NSC, "numRef"}, [2]string{NSC, "f"})
				if ref == nil || referencePreviewText(ref) != "'Chart'!$B$2:$B$4" {
					t.Fatal("value reference changed")
				}
			}
		})
	}
}

// Preserve Reference ZipFile.namelist (including directories) and read(name)
// (the last entry for duplicate names). Do not use OPC's filtered member list.
func referencePreviewZIP(t *testing.T, path string) *zip.ReadCloser {
	t.Helper()
	z, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		// Mutation owns and closes its acquired source before the callback.
		// Other readers retain ownership until this cleanup.
		if err := z.Close(); err != nil && !errors.Is(err, os.ErrClosed) {
			t.Error(err)
		}
	})
	return z
}
func referencePreviewZIPRead(t *testing.T, z *zip.ReadCloser, name string) []byte {
	t.Helper()
	for i := len(z.File) - 1; i >= 0; i-- {
		if z.File[i].Name != name {
			continue
		}
		r, err := z.File[i].Open()
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(r)
		closeErr := r.Close()
		if err != nil {
			t.Fatal(err)
		}
		if closeErr != nil {
			t.Fatal(closeErr)
		}
		return data
	}
	t.Fatalf("missing ZIP member %q", name)
	return nil
}

// ElementPath .// followed by direct-child steps: find the first complete
// matching path, not the first ancestor whether or not its children match.
func referencePreviewFind(root *etree.Element, steps ...[2]string) *etree.Element {
	for _, e := range All(root, steps[0][0], steps[0][1]) {
		if e == root {
			continue
		}
		if found := referencePreviewDirect(e, steps[1:]...); found != nil {
			return found
		}
	}
	return nil
}
func referencePreviewDirect(e *etree.Element, steps ...[2]string) *etree.Element {
	if len(steps) == 0 {
		return e
	}
	for _, c := range Children(e, steps[0][0], steps[0][1]) {
		if found := referencePreviewDirect(c, steps[1:]...); found != nil {
			return found
		}
	}
	return nil
}

// lxml findtext returns text before the first child, including comment/PI
// children. etree.Text ignores comments and can incorrectly include their tail.
func referencePreviewText(e *etree.Element) string {
	var text strings.Builder
	for _, token := range e.Child {
		ch, ok := token.(*etree.CharData)
		if !ok {
			break
		}
		text.WriteString(ch.Data)
	}
	return text.String()
}

// Mirrors PresentationNative.test_categorical_histogram_rejects_invalid_dimension_types_and_lengths.
func TestReferenceHistogramInvalidDimensions(t *testing.T) {
	referencePresentationRoot(t)
	for _, invalid := range []string{"count", "category_type", "value_type"} {
		t.Run(invalid, func(t *testing.T) {
			p := fixture(t, "reference-categorical-histogram.pptx")
			// Reference chooses the first matching ZIP member before corrupting it.
			var part string
			for _, name := range p.Names() {
				if strings.Contains(name, "extendedCharts/chart.xml") {
					part = name
					break
				}
			}
			if part == "" {
				t.Fatal("fixture has no extendedCharts/chart.xml member")
			}
			doc, err := p.XML(part)
			if err != nil {
				t.Fatal(err)
			}
			data := childPath(doc.Root(), [2]string{NSCX, "chartData"}, [2]string{NSCX, "data"})
			switch invalid {
			case "count":
				level := Child(Child(data, NSCX, "strDim"), NSCX, "lvl")
				children := level.ChildElements()
				level.RemoveChild(children[len(children)-1])
				level.CreateAttr("ptCount", "2")
			case "category_type":
				dim := Child(data, NSCX, "strDim")
				dim.Tag = "numDim"
				for i, pt := range All(dim, NSCX, "pt") {
					pt.SetText(strconv.Itoa(i))
				}
			case "value_type":
				Child(data, NSCX, "numDim").Tag = "strDim"
			}
			if err := p.SetXML(part, doc); err != nil {
				t.Fatal(err)
			}
			// self.mutate writes a fresh ZIP; validation reads that saved package.
			broken := referenceWriteMutation(t, p)
			reopened, err := opc.Open(broken)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := VerifyChartData(reopened, ""); err == nil {
				t.Fatal("accepted invalid dimension", invalid)
			}
		})
	}
}

// Mirrors PresentationNative.test_waterfall_preview_negative_steps_and_totals.
func TestReferenceWaterfallNegativeTotal(t *testing.T) {
	referencePresentationRoot(t)
	original := filepath.Join("..", "..", "testdata", "fixtures", "reference-waterfall.pptx")
	z := referencePreviewZIP(t, original)
	var part string
	for _, member := range z.File {
		name := member.Name
		if strings.Contains(name, "extendedCharts/") && strings.HasSuffix(name, ".xml") {
			part = name
			break
		}
	}
	if part == "" {
		t.Fatal("missing extended chart")
	}
	doc := etree.NewDocument()
	if err := referencePreviewReadXML(doc, referencePreviewZIPRead(t, z, part)); err != nil {
		t.Fatal(err)
	}
	var points []*etree.Element
	for _, dimension := range All(doc.Root(), NSCX, "numDim") {
		for _, level := range Children(dimension, NSCX, "lvl") {
			points = append(points, Children(level, NSCX, "pt")...)
		}
	}
	for i, value := range []string{"10", "-15", "-5"} {
		if i >= len(points) {
			break
		} // Reference zip stops at the shorter input.
		points[i].SetText(value)
	}
	scaling := All(doc.Root(), NSCX, "valScaling")[0]
	scaling.CreateAttr("min", "auto")
	scaling.CreateAttr("max", "auto")
	series := All(doc.Root(), NSCX, "series")[0]
	layout := NewChild(series, NSCX, "layoutPr", nil)
	NewChild(NewChild(layout, NSCX, "subtotals", nil), NSCX, "idx", nil).CreateAttr("val", "2")
	data, err := doc.WriteToBytes()
	if err != nil {
		t.Fatal(err)
	}
	source := referencePreviewMutatedZIP(t, z, part, data)
	output := referenceNextPresentation(t)
	changed, err := OfficePreviewCopy(source, output)
	if err != nil {
		t.Fatal(err)
	}
	roots := referencePresentationRoots(t, output)
	if len(roots) == 0 {
		t.Fatal("missing first slide")
	}
	slide := roots[0]
	var names []string
	var bars []*etree.Element
	for _, shape := range Child(Child(slide, NSP, "cSld"), NSP, "spTree").ChildElements() {
		// SlideShapes iterates these direct shape types in document order.
		name, supported, err := referenceHistogramShapeName(shape)
		if err != nil {
			t.Fatal(err)
		}
		if !supported {
			continue
		}
		if strings.HasPrefix(name, "Waterfall step ") {
			names = append(names, name)
			bars = append(bars, shape)
		}
	}
	want := []string{"Waterfall step 1: A, 0 to 10", "Waterfall step 2: B, 10 to -5", "Waterfall step 3: C, 0 to -5"}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("bars %v want %v", names, want)
	}
	height := func(i int) float64 {
		ext := childPath(bars[i], [2]string{NSP, "spPr"}, [2]string{NSA, "xfrm"}, [2]string{NSA, "ext"})
		if ext == nil {
			t.Fatal("missing bar height")
		}
		// reference presentation library reads positive coordinates as integer EMUs, not floats.
		value, err := strconv.ParseInt(strings.TrimSpace(ext.SelectAttrValue("cy", "")), 10, 64)
		if err != nil {
			t.Fatal(err)
		}
		return float64(value)
	}
	denominator := height(0)
	if denominator == 0 {
		t.Fatal("zero bar height: reference division would fail")
	}
	referencePreviewAlmostFourPlaces(t, height(1)/denominator, 1.5)
	referencePreviewAlmostFourPlaces(t, height(2)/denominator, .5)
	colours := map[string]bool{}
	for _, bar := range bars {
		colour := childPath(bar, [2]string{NSP, "spPr"}, [2]string{NSA, "solidFill"}, [2]string{NSA, "srgbClr"})
		colours[referencePreviewRGB(t, colour)] = true
	}
	if len(colours) != 3 {
		t.Fatal(colours)
	}

	// Supplemental assertion follows all original assertions.
	if !changed {
		t.Fatal("preview unchanged")
	}
}

// assertAlmostEqual(..., places=4) compares round(abs(first-second), 4)
// with zero. Compare the exact binary64 difference to the rational half-step;
// a float64 literal 0.00005 lies slightly above that boundary. Ties round to zero.
func referencePreviewAlmostFourPlaces(t *testing.T, first, second float64) {
	t.Helper()
	if first == second {
		return
	}
	difference := math.Abs(first - second)
	exact := new(big.Rat).SetFloat64(difference)
	if exact == nil || exact.Cmp(big.NewRat(1, 20000)) > 0 {
		t.Fatalf("%v != %v within 4 decimal places (difference %v)", first, second, difference)
	}
}

// Decode explicit six-digit DrawingML RGB values before comparing colors.
// Non-RGB fills and malformed values cannot silently count as distinct colors.
func referencePreviewRGB(t *testing.T, colour *etree.Element) string {
	t.Helper()
	if colour == nil {
		t.Fatal("explicit RGB colour missing")
	}
	value := colour.SelectAttrValue("val", "")
	decoded, err := hex.DecodeString(value)
	if err != nil || len(decoded) != 3 {
		t.Fatalf("invalid RGB colour %q: %v", value, err)
	}
	return strings.ToUpper(hex.EncodeToString(decoded))
}

// Mirrors PresentationNative.test_histogram_hidden_axis_and_tick_text_style.
func TestReferenceHistogramHiddenAxisTextStyle(t *testing.T) {
	referencePresentationRoot(t)
	original := filepath.Join("..", "..", "testdata", "fixtures", "reference-histogram-axes.pptx")
	z := referencePreviewZIP(t, original)
	var part string
	for _, member := range z.File {
		name := member.Name
		if strings.Contains(name, "extendedCharts") && strings.HasSuffix(name, ".xml") {
			part = name
			break
		}
	}
	if part == "" {
		t.Fatal("missing extended chart")
	}
	doc := etree.NewDocument()
	if err := referencePreviewReadXML(doc, referencePreviewZIPRead(t, z, part)); err != nil {
		t.Fatal(err)
	}
	doc.Root().CreateAttr("xmlns:a", NSA)
	axes := All(doc.Root(), NSCX, "axis")
	axes[0].CreateAttr("hidden", "1")
	style := NewChild(axes[1], NSCX, "txPr", nil)
	// Bind DrawingML for the added text style, as lxml does in Reference.
	props := NewChild(NewChild(NewChild(style, NSA, "p", nil), NSA, "pPr", nil), NSA, "defRPr", nil)
	props.CreateAttr("sz", "1000")
	props.CreateAttr("b", "1")
	NewChild(NewChild(props, NSA, "solidFill", nil), NSA, "srgbClr", nil).CreateAttr("val", "7030A0")
	data, err := doc.WriteToBytes()
	if err != nil {
		t.Fatal(err)
	}
	source := referencePreviewMutatedZIP(t, z, part, data)
	output := referenceNextPresentation(t)
	changed, err := OfficePreviewCopy(source, output)
	if err != nil {
		t.Fatal(err)
	}
	roots := referencePresentationRoots(t, output)
	if len(roots) == 0 {
		t.Fatal("missing first slide")
	}
	slide := roots[0]
	labels := referenceHistogramShapeTexts(t, slide)
	for _, text := range labels {
		if text == "Observations" || strings.HasPrefix(text, "[5,") {
			t.Fatalf("hidden axis label: %s", text)
		}
	}
	// Reference next(...) chooses only the first matching top-level shape.
	shapes := Children(Child(Child(slide, NSP, "cSld"), NSP, "spTree"), NSP, "sp")
	var tick *etree.Element
	for i, text := range labels {
		if text == "4.0" {
			tick = shapes[i]
			break
		}
	}
	if tick == nil {
		t.Fatal("4.0 tick missing")
	}
	body := Child(tick, NSP, "txBody")
	props = childPath(body, [2]string{NSA, "p"}, [2]string{NSA, "pPr"}, [2]string{NSA, "defRPr"})
	if props == nil {
		t.Fatal("paragraph font missing")
	}
	size, err := strconv.Atoi(props.SelectAttrValue("sz", ""))
	if err != nil || float64(size)/100 != 10 {
		t.Fatal("paragraph font size", size, err)
	}
	bold := props.SelectAttrValue("b", "")
	if bold != "1" && bold != "true" {
		t.Fatal("paragraph font not bold", bold)
	}
	colour := childPath(props, [2]string{NSA, "solidFill"}, [2]string{NSA, "srgbClr"})
	if referencePreviewRGB(t, colour) != "7030A0" {
		t.Fatal("paragraph colour not retained")
	}
	// Supplemental check retained after the original assertions.
	if !changed {
		t.Fatal("preview unchanged")
	}
}

func TestReferenceExplicitChartStylesNeedNoNormalization(t *testing.T) {
	referencePresentationRoot(t)
	for _, kind := range []string{"pie", "doughnut", "scatter"} {
		t.Run(kind, func(t *testing.T) {
			p := previewOpen(t, filepath.Join("..", "..", "testdata", "fixtures", "reference-"+kind+".xlsx"))
			parts, e := ChartParts(p)
			if e != nil || len(parts) != 1 {
				t.Fatal(parts, e)
			}
			d, e := p.XML(parts[0])
			if e != nil {
				t.Fatal(e)
			}
			if kind == "scatter" {
				All(d.Root(), NSC, "scatterStyle")[0].CreateAttr("val", "smoothMarker")
			} else {
				chart := All(d.Root(), NSC, kind+"Chart")[0]
				vary := newNS(chart, NSC, "varyColors")
				vary.CreateAttr("val", "0")
				chart.InsertChildAt(0, vary)
			}
			if e := p.SetXML(parts[0], d); e != nil {
				t.Fatal(e)
			}
			source := filepath.Join(t.TempDir(), "explicit.xlsx")
			if e := p.SaveNew(source); e != nil {
				t.Fatal(e)
			}
			before := previewRead(t, source)
			if need, e := NeedsOfficePreview(source); e != nil || need {
				t.Error("unexpected normalization needed", need, e)
			}
			if changed, e := OfficePreviewCopy(source, filepath.Join(t.TempDir(), "unused.xlsx")); e != nil || changed {
				t.Error("unexpected normalized copy", changed, e)
			}
			previewUnchanged(t, source, before)
		})
	}
}

// Matches mutate's ordered dict of ZIP names and uncompressed contents. Reference
// dict keeps the first insertion position and ZipFile.read(name) reads the last
// duplicate; directory entries are included. Only the callback's part changes.
func referencePreviewMutatedZIP(t *testing.T, original *zip.ReadCloser, changed string, data []byte) string {
	t.Helper()
	output, err := referenceMutatePresentation(t, original, func(parts *referenceOrderedZIP) error { parts.set(changed, data); return nil })
	if err != nil {
		t.Fatal(err)
	}
	return output
}
