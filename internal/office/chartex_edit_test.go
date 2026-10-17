package office

import (
	"bytes"
	"encoding/json"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"path/filepath"
	"testing"
)

func fixture(t *testing.T, name string) *opc.Package {
	t.Helper()
	p, e := opc.Open(filepath.Join("../../testdata/fixtures", name))
	if e != nil {
		t.Fatal(e)
	}
	return p
}
func TestExtendedChartFixtures(t *testing.T) {
	for _, name := range []string{"reference-treemap.pptx", "reference-hierarchy-treemap.pptx", "reference-hierarchy-sunburst.pptx", "reference-categorical-histogram.pptx", "reference-histogram.pptx", "reference-waterfall.pptx", "reference-funnel.pptx"} {
		t.Run(name, func(t *testing.T) {
			p := fixture(t, name)
			r, e := ExtendedCharts(p, ExtendedChartSpec{})
			if e != nil || len(r) == 0 {
				t.Fatal(r, e)
			}
		})
	}
}
func TestExtendedChartDataPreservesStylesAndParts(t *testing.T) {
	p := fixture(t, "reference-categorical-histogram.pptx")
	before := map[string][]byte{}
	for _, v := range p.Parts {
		before[v.Name] = append([]byte(nil), v.Data...)
	}
	r, e := ExtendedCharts(p, ExtendedChartSpec{})
	if e != nil {
		t.Fatal(e)
	}
	part := r[0].Chart
	values := append([]float64(nil), r[0].Series[0].dims.Num["val"]...)
	if len(values) < 1 {
		t.Fatal(r)
	}
	values[0] += 1
	r, e = ExtendedCharts(p, ExtendedChartSpec{Action: "replace-data", Part: part, Values: values})
	if e != nil {
		t.Fatal(e)
	}
	old, _ := opc.ParseXML(before[part])
	now, _ := p.XML(part)
	oldChart := Child(old.Root(), NSCX, "chart")
	newChart := Child(now.Root(), NSCX, "chart")
	a := newRoot("test", "test")
	a.Root().AddChild(oldChart.Copy())
	b := newRoot("test", "test")
	b.Root().AddChild(newChart.Copy())
	aa, _ := a.WriteToBytes()
	bb, _ := b.WriteToBytes()
	if !bytes.Equal(aa, bb) {
		t.Fatal("chart styling changed")
	}
	for name, v := range before {
		if name != part && !bytes.Equal(v, p.Get(name)) {
			t.Fatal("unrelated part changed", name)
		}
	}
	oldReport, e := VerifyExtendedChart(old.Root(), part, nil)
	if e != nil {
		t.Fatal(e)
	}
	labels := r[0].Series[0].dims.Str["cat"]
	want := oldReport.Series[0].dims.Str["cat"]
	x, _ := json.Marshal(labels)
	y, _ := json.Marshal(want)
	if !bytes.Equal(x, y) {
		t.Fatal("category labels lost")
	}
	if _, e = ExtendedCharts(p, ExtendedChartSpec{Action: "replace-data", Part: part, Values: []float64{1}}); e == nil {
		t.Fatal("silently discarded labels on count change")
	}
}
func TestExtendedChartAddAndBins(t *testing.T) {
	for _, kind := range []string{"treemap", "sunburst", "histogram"} {
		t.Run(kind, func(t *testing.T) {
			p := fixture(t, "reference-bar.pptx")
			s := ExtendedChartSpec{Action: "add", Type: kind, Slide: 1, Name: "Samples", Values: []float64{10, 20, 5}, Position: []int64{914400, 914400, 5486400, 3657600}}
			if kind == "histogram" {
				n := 3
				s.BinCount = &n
				s.Categories = []string{"A", "B", "C"}
			} else {
				s.Paths = [][]string{{"A", "North"}, {"B", "North"}, {"C", "South"}}
			}
			r, e := ExtendedCharts(p, s)
			if e != nil || len(r) != 1 {
				t.Fatal(r, e)
			}
			if r[0].Series[0].Type != kind {
				t.Fatal(r)
			}
			if _, e = VerifyChartData(p, "powerpoint"); e != nil {
				t.Fatal(e)
			}
			path := filepath.Join(t.TempDir(), "chart.pptx")
			if e = p.SaveNew(path); e != nil {
				t.Fatal(e)
			}
			q, e := opc.Open(path)
			if e != nil {
				t.Fatal(e)
			}
			again, e := ExtendedCharts(q, ExtendedChartSpec{})
			if e != nil || len(again) != 1 {
				t.Fatal(again, e)
			}
		})
	}
}
