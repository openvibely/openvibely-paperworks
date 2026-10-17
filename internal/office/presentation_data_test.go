package office

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/openvibely/openvibely-paperworks/internal/testsupport"
)

func referenceChartJSON(t *testing.T, v any) map[string]any {
	t.Helper()
	b, e := json.Marshal(v)
	if e != nil {
		t.Fatal(e)
	}
	var m map[string]any
	if e = json.Unmarshal(b, &m); e != nil {
		t.Fatal(e)
	}
	return m
}

func TestReferenceReferenceStandardChartValidation(t *testing.T) {
	referencePresentationRoot(t)
	p := fixture(t, "reference-bar.pptx")
	r, e := VerifyChartData(p, "powerpoint")
	if e != nil || len(r) != 1 {
		t.Fatal(r, e)
	}
	part := referenceChartJSON(t, r[0])["chart"].(string)
	if part != "ppt/slides/charts/chart1.xml" {
		t.Fatal(part)
	}
	d, e := p.XML(part)
	if e != nil {
		t.Fatal(e)
	}
	Child(Child(All(d.Root(), NSC, "numLit")[0], NSC, "pt"), NSC, "v").SetText("BAD-NUMBER")
	if e = p.SetXML(part, d); e != nil {
		t.Fatal(e)
	}
	// Original mutate() writes a new ZIP before either verifier reads it.
	p = previewOpen(t, referenceWriteMutation(t, p))
	if _, e = VerifyChartData(p, "powerpoint"); e == nil {
		t.Fatal("accepted invalid number")
	}
	report, e := PackageReportOf(p)
	if e != nil {
		t.Fatal(e)
	}
	app := "powerpoint"
	if _, e = VerifyPresentation(p, report, SlideRequirements{TargetApplication: &app}); e == nil {
		t.Fatal("presentation accepted invalid number")
	}
}

func TestReferenceAlternateChartMaterializationLineage(t *testing.T) {
	referencePresentationRoot(t)
	p := fixture(t, "reference-bar.pptx")
	if _, e := MaterializeLiteralCharts(p); e != nil {
		t.Fatal(e)
	}
	out := referenceNextPresentation(t)
	if e := p.SaveNew(out); e != nil {
		t.Fatal(e)
	}
	p = previewOpen(t, out)
	r, e := VerifyChartData(p, "")
	if e != nil || len(r) != 1 {
		t.Fatal(r, e)
	}
	if referenceChartJSON(t, r[0])["references"].(float64) <= 0 {
		t.Fatal(r)
	}
	report, e := PackageReportOf(p)
	if e != nil {
		t.Fatal(e)
	}
	b, e := json.Marshal(report["external_relationships"])
	if e != nil || string(b) != "[]" {
		t.Fatalf("external relationships: %s %v", b, e)
	}
}

func TestReferenceExtendedChartsCountAndNonfinite(t *testing.T) {
	referencePresentationRoot(t)
	for _, kind := range []string{"treemap", "histogram"} {
		t.Run(kind, func(t *testing.T) {
			p := fixture(t, "reference-"+kind+".pptx")
			report, e := PackageReportOf(p)
			if e != nil {
				t.Fatal(e)
			}
			result, e := VerifyPresentation(p, report, SlideRequirements{NativeCharts: []int{1}})
			if e != nil {
				t.Fatal(e)
			}
			if !reflect.DeepEqual(result.ChartOwners, []int{1}) {
				t.Fatal(result.ChartOwners)
			}
			chart := result.ChartData[0].(*ChartExReport)
			if chart.Series[0].Type != kind {
				t.Fatal(chart)
			}
			d, e := p.XML(chart.Chart)
			if e != nil {
				t.Fatal(e)
			}
			Child(Child(All(d.Root(), NSCX, "numDim")[0], NSCX, "lvl"), NSCX, "pt").SetText("nan")
			if e = p.SetXML(chart.Chart, d); e != nil {
				t.Fatal(e)
			}
			p = previewOpen(t, referenceWriteMutation(t, p))
			if _, e = VerifyChartData(p, ""); e == nil || !strings.Contains(e.Error(), "nonfinite") {
				t.Fatalf("want nonfinite error: %v", e)
			}
		})
	}
}

func TestReferenceMissingAndUnsupportedExtendedChart(t *testing.T) {
	referencePresentationRoot(t)
	for _, kind := range []string{"missing", "unsupported"} {
		t.Run(kind, func(t *testing.T) {
			p := fixture(t, "reference-treemap.pptx")
			part := "ppt/slides/extendedCharts/chart.xml"
			want := "Missing chart"
			if kind == "missing" {
				p.Remove(part)
			} else {
				d, e := p.XML(part)
				if e != nil {
					t.Fatal(e)
				}
				All(d.Root(), NSCX, "series")[0].CreateAttr("layoutId", "unimplemented")
				if e = p.SetXML(part, d); e != nil {
					t.Fatal(e)
				}
				want = "unsupported ChartEx"
			}
			p = previewOpen(t, referenceWriteMutation(t, p))
			if _, e := VerifyChartData(p, ""); e == nil || !strings.Contains(e.Error(), want) {
				t.Fatalf("want %q: %v", want, e)
			}
		})
	}
}

func TestReferenceCategoricalHistogramEditPreservation(t *testing.T) {
	referencePresentationRoot(t)
	source := "../../testdata/fixtures/reference-categorical-histogram.pptx"
	original, e := os.ReadFile(source)
	if e != nil {
		t.Fatal(e)
	}
	verified, e := VerifyChartData(previewOpen(t, source), "")
	if e != nil {
		t.Fatal(e)
	}
	r := verified[0].(*ChartExReport)
	if r.Series[0].Type != "histogram" || !reflect.DeepEqual(r.Series[0].Dimensions, map[string]any{"cat": []string{"A", "B", "C"}, "val": []float64{10, 20, 5}}) {
		t.Fatal(r)
	}
	for _, labels := range [][]string{nil, {"North", "South", "West"}} {
		name := "retained"
		if labels != nil {
			name = "replaced"
		}
		t.Run(name, func(t *testing.T) {
			p := previewOpen(t, source)
			before := map[string][]byte{}
			var names []string
			for _, part := range p.Parts {
				names = append(names, part.Name)
				before[part.Name] = append([]byte(nil), part.Data...)
			}
			results, e := ExtendedCharts(p, ExtendedChartSpec{Action: "replace-data", Part: r.Chart, Values: []float64{30, 15, 7}, Categories: labels})
			if e != nil {
				t.Fatal(e)
			}
			want := labels
			if want == nil {
				want = []string{"A", "B", "C"}
			}
			if !reflect.DeepEqual(results[0].Series[0].Dimensions, map[string]any{"cat": want, "val": []float64{30, 15, 7}}) {
				t.Fatal(results)
			}
			out := referenceNextPresentation(t)
			if e = p.SaveNew(out); e != nil {
				t.Fatal(e)
			}
			p = previewOpen(t, out)
			// Match Presentation(out).slides through independent saved-package
			// relationships, not the production slide-discovery implementation.
			slides := referencePresentationRoots(t, out)
			if len(slides) != 1 {
				t.Fatalf("slide count=%d; want 1", len(slides))
			}
			var afterNames []string
			for _, part := range p.Parts {
				afterNames = append(afterNames, part.Name)
			}
			if !reflect.DeepEqual(names, afterNames) {
				t.Fatal("package member order changed")
			}
			for _, part := range p.Parts {
				if part.Name != r.Chart && !bytes.Equal(part.Data, before[part.Name]) {
					t.Error("changed unrelated part", part.Name)
				}
			}
			old := referenceChartCanonicalParse(t, string(before[r.Chart]))
			now := referenceChartCanonicalParse(t, string(p.Get(r.Chart)))
			old.RemoveChild(Child(old, NSCX, "chartData"))
			now.RemoveChild(Child(now, NSCX, "chartData"))
			if referenceChartCanonicalRoot(t, old) != referenceChartCanonicalRoot(t, now) {
				t.Fatal("chart style outside chartData changed")
			}
		})
	}
	after, e := os.ReadFile(source)
	if e != nil || !bytes.Equal(original, after) {
		t.Fatal("source changed", e)
	}
}

func TestReferenceCategoricalHistogramAuthoring(t *testing.T) {
	referencePresentationRoot(t)
	source := testsupport.OriginalPackageFixture(t, "runtime-workflow-fixtures.zip", "categorical-author-blank", ".pptx")
	raw, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	source = referenceNextPresentation(t)
	if err := os.WriteFile(source, raw, 0600); err != nil {
		t.Fatal(err)
	}
	p := previewOpen(t, source)
	r, e := ExtendedCharts(p, ExtendedChartSpec{Action: "add", Slide: 1, Type: "histogram", Categories: []string{"First", "Second"}, Values: []float64{3, 6}, Position: []int64{914400, 914400, 5486400, 3657600}})
	if e != nil {
		t.Fatal(e)
	}
	if len(r) != 1 || !reflect.DeepEqual(r[0].Series[0].Dimensions, map[string]any{"cat": []string{"First", "Second"}, "val": []float64{3, 6}}) {
		t.Fatal(r)
	}
	p = referenceSavePresentation(t, p)
	verified, e := VerifyChartData(p, "")
	if e != nil {
		t.Fatal(e)
	}
	a, e := json.Marshal(r)
	if e != nil {
		t.Fatal(e)
	}
	b, e := json.Marshal(verified)
	if e != nil {
		t.Fatal(e)
	}
	if !bytes.Equal(a, b) {
		t.Fatalf("author/verify reports differ: %s vs %s", a, b)
	}
}

func TestReferenceExtendedChartCreateEditKeepsSlideContent(t *testing.T) {
	referencePresentationRoot(t)
	source := testsupport.OriginalPackageFixture(t, "runtime-workflow-fixtures.zip", "extended-content-title", ".pptx")
	raw, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	source = filepath.Join(referencePresentationRoot(t), "base.pptx")
	if err := os.WriteFile(source, raw, 0600); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"treemap", "histogram"} {
		t.Run(kind, func(t *testing.T) {
			p := previewOpen(t, source)
			spec := ExtendedChartSpec{Action: "add", Type: kind, Slide: 1, Name: "Test", Values: []float64{2, 5, 8}, Position: []int64{914400, 914400, 5486400, 3657600}}
			if kind == "treemap" {
				spec.Categories = []string{"One", "Two", "Three"}
			}
			result, e := ExtendedCharts(p, spec)
			if e != nil {
				t.Fatal(e)
			}
			// Match Presentation(out).slides[0].shapes[0].text through the
			// saved package relationships and direct top-level shape sequence.
			out := referenceNextPresentation(t)
			if e := p.SaveNew(out); e != nil {
				t.Fatal(e)
			}
			p = previewOpen(t, out)
			shapes, _ := referencePreviewShapes(t, out)
			if len(shapes) == 0 || referenceShapeText(t, shapes[0]) != "Keep this title" {
				t.Fatal("first shape title changed")
			}
			report, e := PackageReportOf(p)
			if e != nil {
				t.Fatal(e)
			}
			if _, e = VerifyPresentation(p, report, SlideRequirements{NativeCharts: []int{1}}); e != nil {
				t.Fatal(e)
			}
			before := map[string][]byte{}
			for _, part := range p.Parts {
				before[part.Name] = append([]byte(nil), part.Data...)
			}
			update := ExtendedChartSpec{Action: "replace-data", Part: result[0].Chart, Values: []float64{7, 9}}
			if kind == "treemap" {
				update.Categories = []string{"A", "B"}
			}
			if _, e = ExtendedCharts(p, update); e != nil {
				t.Fatal(e)
			}
			p = referenceSavePresentation(t, p)
			var changed []string
			for _, part := range p.Parts {
				if !bytes.Equal(before[part.Name], part.Data) {
					changed = append(changed, part.Name)
				}
			}
			if !reflect.DeepEqual(changed, []string{result[0].Chart}) {
				t.Fatal("changed parts", changed)
			}
			checked, e := VerifyChartData(p, "")
			if e != nil {
				t.Fatal(e)
			}
			found := false
			for _, v := range checked[0].(*ChartExReport).Series[0].Dimensions {
				if reflect.DeepEqual(v, []float64{7, 9}) {
					found = true
				}
			}
			if !found {
				t.Fatal("missing replacement values", checked)
			}
		})
	}
}
