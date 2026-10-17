package office

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/openvibely/openvibely-paperworks/internal/opc"
)

func TestReferenceHierarchyAuthorEditReference(t *testing.T) {
	referencePresentationRoot(t)
	paths := [][]string{{"North", "A"}, {"North", "B"}, {"South", "C"}}
	for _, kind := range []string{"treemap", "sunburst"} {
		t.Run(kind, func(t *testing.T) {
			reference, e := VerifyChartData(fixture(t, "reference-hierarchy-"+kind+".pptx"), "")
			if e != nil {
				t.Fatal(e)
			}
			report := reference[0].(*ChartExReport)
			if !reflect.DeepEqual(report.Series[0].Dimensions["category_paths"], paths) {
				t.Fatal(report.Series[0].Dimensions)
			}
			p := referenceSavePresentation(t, referenceCommentFixture(t, "comment-blank"))
			result, e := ExtendedCharts(p, ExtendedChartSpec{Action: "add", Slide: 1, Type: kind, Values: []float64{10, 20, 5}, Paths: paths, Position: []int64{914400, 914400, 5486400, 3657600}})
			if e != nil {
				t.Fatal(e)
			}
			a, e := json.Marshal(result[0].Series)
			if e != nil {
				t.Fatal(e)
			}
			b, e := json.Marshal(report.Series)
			if e != nil {
				t.Fatal(e)
			}
			if string(a) != string(b) {
				t.Errorf("series mismatch\n%s\n%s", a, b)
			}
			p = referenceSavePresentation(t, p)
			authored, e := p.Bytes()
			if e != nil {
				t.Fatal(e)
			}
			part := result[0].Chart
			if _, e := ExtendedCharts(p, ExtendedChartSpec{Action: "replace-data", Part: part, Values: []float64{1, 2}, Paths: [][]string{{"X", "Y"}, {"X", "Z"}}}); e != nil {
				t.Fatal(e)
			}
			checked, e := VerifyChartData(referenceSavePresentation(t, p), "")
			if e != nil {
				t.Fatal(e)
			}
			if !reflect.DeepEqual(checked[0].(*ChartExReport).Series[0].Dimensions["size"], []float64{1, 2}) {
				t.Error(checked)
			}
			bad := referenceNextPresentation(t)
			// Reference rejects against the authored output, not the later edited copy.
			p, e = opc.Read(authored)
			if e != nil {
				t.Fatal(e)
			}
			_, e = ExtendedCharts(p, ExtendedChartSpec{Action: "replace-data", Part: part, Values: []float64{1, 2}, Paths: [][]string{{"X"}, {"X", "Z"}}})
			if e == nil {
				e = p.SaveNew(bad)
			}
			if e == nil || !strings.Contains(e.Error(), "equal-depth") {
				t.Error("ragged paths", e)
			}
			if _, e := os.Stat(bad); !os.IsNotExist(e) {
				t.Error("bad output exists", e)
			}
		})
	}
}
func referenceLinkedHierarchy(t *testing.T) (*opc.Package, string) {
	t.Helper()
	p := fixture(t, "reference-hierarchy-treemap.pptx")
	// The reference helper uses this exact chart part and an openpyxl workbook.
	// Reuse its retained workbook rather than introducing Excelize defaults.
	part := "ppt/slides/extendedCharts/chart.xml"
	original, e := opc.Open("../../testdata/reference-mirror/linked-hierarchy.pptx")
	if e != nil {
		t.Fatal(e)
	}
	workbook := original.Get("ppt/embeddings/source.xlsx")
	if workbook == nil {
		t.Fatal("missing pinned embedded workbook")
	}
	if e := p.EnsureContentType("ppt/embeddings/source.xlsx", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"); e != nil {
		t.Fatal(e)
	}
	p.Set(opc.RelsName(part), []byte(`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="book" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/package" Target="../../embeddings/source.xlsx"/></Relationships>`))
	// Reference inserts the relationship before the embedded workbook.
	p.Set("ppt/embeddings/source.xlsx", workbook)
	d, e := p.XML(part)
	if e != nil {
		t.Fatal(e)
	}
	for kind, formula := range map[string]string{"strDim": "'Source'!$A$1:$B$3", "numDim": "'Source'!$C$1:$C$3"} {
		dim := All(d.Root(), NSCX, kind)[0]
		n := dim.CreateElement("cx:f")
		n.SetText(formula)
		dim.RemoveChild(n)
		dim.InsertChildAt(0, n)
		if kind == "numDim" {
			Child(dim, NSCX, "lvl").CreateAttr("formatCode", "0.00")
		}
	}
	ext := Child(d.Root(), NSCX, "chartData").CreateElement("cx:externalData")
	ext.CreateAttr("xmlns:r", NSR)
	ext.CreateAttr("r:id", "book")
	if e := p.SetXML(part, d); e != nil {
		t.Fatal(e)
	}
	return previewOpen(t, referenceWriteMutation(t, p)), part
}
func TestReferenceLinkedHierarchyVerification(t *testing.T) {
	referencePresentationRoot(t)
	p, part := referenceLinkedHierarchy(t)
	reports, e := VerifyChartData(p, "")
	if e != nil {
		t.Fatal(e)
	}
	report := reports[0].(*ChartExReport)
	if !report.WorkbookVerified {
		t.Error("workbook not verified")
	}
	if report.References != 2 {
		t.Error("references", report.References)
	}
	original, e := p.Bytes()
	if e != nil {
		t.Fatal(e)
	}
	for _, kind := range []string{"stale", "extra-formula", "external", "replace-data"} {
		t.Run(kind, func(t *testing.T) {
			p, e := opc.Read(original)
			if e != nil {
				t.Fatal(e)
			}
			want := ""
			var err error
			if kind == "external" {
				d, e := p.XML(opc.RelsName(part))
				if e != nil {
					t.Fatal(e)
				}
				r := d.Root().ChildElements()[0]
				r.CreateAttr("TargetMode", "External")
				r.CreateAttr("Target", "https://invalid.example/source.xlsx")
				if e := p.SetXML(opc.RelsName(part), d); e != nil {
					t.Fatal(e)
				}
				want = "unavailable"
			} else if kind == "replace-data" {
				out := referenceNextPresentation(t)
				_, err = ExtendedCharts(p, ExtendedChartSpec{Action: "replace-data", Part: part, Values: []float64{1, 2, 3}, Categories: []string{"A", "B", "C"}})
				if err == nil {
					err = p.SaveNew(out)
				}
				if _, statErr := os.Stat(out); !os.IsNotExist(statErr) {
					t.Fatal("rejected replacement published output", statErr)
				}
				want = "chart-inputs"
			} else {
				d, e := p.XML(part)
				if e != nil {
					t.Fatal(e)
				}
				if kind == "stale" {
					Child(Child(All(d.Root(), NSCX, "numDim")[0], NSCX, "lvl"), NSCX, "pt").SetText("11")
					want = "differs from workbook"
				} else {
					All(d.Root(), NSCX, "txData")[0].CreateElement("cx:f").SetText("'Source'!$A$1")
					want = "non-dimension"
				}
				if e := p.SetXML(part, d); e != nil {
					t.Fatal(e)
				}
			}
			if kind != "replace-data" {
				// Reference mutate writes a new package before the verifier reads it.
				// Keep the original stale/extra-formula/external checks at that boundary.
				p = previewOpen(t, referenceWriteMutation(t, p))
				_, err = VerifyChartData(p, "")
			}
			if err == nil || !strings.Contains(err.Error(), want) {
				t.Errorf("want %q: %v", want, err)
			}
		})
	}
}
