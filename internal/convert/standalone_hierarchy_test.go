package convert

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"github.com/openvibely/openvibely-paperworks/internal/pdf"
)

// Original runtime hierarchy loop, lines 705-733. All original assertions are retained;
// the separate path-count acquisition review (731) remains open.
func TestReferenceStandaloneRuntimeHierarchy(t *testing.T) {
	referenceRuntimeOffice(t)
	for _, kind := range []string{"treemap", "sunburst"} {
		t.Run(kind, func(t *testing.T) {
			ctx := context.Background()
			dir := t.TempDir()
			p, e := opc.Open("../../testdata/fixtures/reference-hierarchy-" + kind + ".pptx")
			if e != nil {
				t.Fatal(e)
			}
			// Only this member differs from the original pinned reference fixture.
			// Reuse captured Reference title serialization, not production discovery/editing.
			titleXML, e := os.ReadFile("../../testdata/reference-mirror/runtime-titled-" + kind + ".xml")
			if e != nil {
				t.Fatal(e)
			}
			p.Set("ppt/slides/extendedCharts/chart.xml", titleXML)
			src := filepath.Join(dir, "titled-"+kind+".pptx")
			if e = p.SaveNew(src); e != nil {
				t.Fatal(e)
			}
			before := contents(t, src)
			out := filepath.Join(dir, "render")
			images, e := RenderFile(ctx, src, out, "", "", pdf.RenderOptions{})
			if e != nil {
				t.Fatal(e)
			}
			t.Run("images", func(t *testing.T) {
				if len(images) != 1 {
					t.Errorf("images %d, want 1", len(images))
				}
			})
			pages := referenceRuntimePDFTexts(t, contents(t, filepath.Join(out, "titled-"+kind+".pdf")))
			if len(pages) == 0 {
				t.Fatal("no rendered page")
			}
			text := pages[0]
			for _, group := range []struct {
				name   string
				labels []string
			}{{"categories", []string{"North", "South", "A", "B", "C"}}, {"title", []string{"Quarterly regional sales"}}, {"legend", []string{"A", "B", "C", "North", "South"}}} {
				t.Run(group.name, func(t *testing.T) {
					for _, label := range group.labels {
						t.Run(label, func(t *testing.T) {
							if !strings.Contains(text, label) {
								t.Errorf("missing %q in %q", label, text)
							}
						})
					}
				})
			}
			if kind == "sunburst" {
				t.Run("series", func(t *testing.T) {
					if !strings.Contains(text, "Values") {
						t.Errorf("missing Values in %q", text)
					}
				})
			}
			t.Run("supported", func(t *testing.T) {
				if strings.Contains(strings.ToLower(text), "unsupported") {
					t.Errorf("unsupported chart: %s", text)
				}
			})
			t.Run("geometry", func(t *testing.T) {
				marks := referencePDFPaths(t, contents(t, filepath.Join(out, "titled-"+kind+".pdf")))
				if got := referencePDFCurveRectCount(marks); got < 3 {
					t.Errorf("curves+rects=%d, want >=3", got)
				}
			})
			t.Run("source", func(t *testing.T) {
				if !bytes.Equal(before, contents(t, src)) {
					t.Error("source changed")
				}
			})
			t.Run("extended", func(t *testing.T) {
				saved, e := opc.Open(src)
				if e != nil {
					t.Fatal(e)
				}
				reports, e := office.VerifyChartData(saved, "")
				if e != nil || len(reports) == 0 {
					t.Fatal(reports, e)
				}
				r, ok := reports[0].(*office.ChartExReport)
				if !ok || !r.Extended {
					t.Errorf("expected extended report, got %#v", reports[0])
				}
			})
		})
	}
}
