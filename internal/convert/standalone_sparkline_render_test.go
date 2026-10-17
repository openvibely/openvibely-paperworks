package convert

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"github.com/openvibely/openvibely-paperworks/internal/pdf"
	"github.com/openvibely/openvibely-paperworks/internal/testsupport"
)

// Pinned runtime lines 657-701. Scoped reader evidence is recorded in batches
// 36 and 39; general PDF acquisition and serialized artifact equivalence remain open.
func TestReferenceStandaloneRuntimeSparklineRender(t *testing.T) {
	referenceRuntimeOffice(t)
	dir := t.TempDir()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	kinds := []string{"line", "column", "stacked"}
	source := testsupport.OriginalPackageFixture(t, "runtime-workflow-fixtures.zip", "sparklines-source", ".xlsx")
	for i, kind := range kinds {
		p, err := opc.Open(source)
		must(err)
		raw, err := json.Marshal(map[string]any{"type": kind, "colorSeries": "245A81", "colorNegative": "CC3333", "negative": true, "markers": kind == "line", "lineWeight": 2})
		must(err)
		options := map[string]json.RawMessage{}
		must(json.Unmarshal(raw, &options))
		row := i + 2
		_, err = office.EditSparklines(p, office.SparklineSpec{Sheet: "Trends", Sparklines: []office.SparklineItem{{Cell: fmt.Sprintf("F%d", row), Range: fmt.Sprintf("'Trends'!B%d:E%d", row, row)}}, Options: options})
		must(err)
		source = filepath.Join(dir, "sparklines-"+kind+".xlsx")
		must(p.SaveNew(source))
	}
	images, err := RenderFile(context.Background(), source, filepath.Join(dir, "sparklines-render"), "", "", pdf.RenderOptions{})
	must(err)
	t.Run("pages", func(t *testing.T) {
		if len(images) != 1 {
			t.Errorf("pages=%d, want 1", len(images))
		}
	})
	t.Run("geometry-count", func(t *testing.T) {
		marks := referencePDFPaths(t, contents(t, filepath.Join(dir, "sparklines-render", "sparklines-stacked.pdf")))
		if got := referencePDFCurveRectCount(marks); got <= 5 {
			t.Errorf("curves+rects=%d, want >5: Sparklines missing from rendered page", got)
		}
	})
	t.Run("series-colors", func(t *testing.T) {
		marks := referencePDFPaths(t, contents(t, filepath.Join(dir, "sparklines-render", "sparklines-stacked.pdf")))
		for _, mark := range marks {
			if referenceIsSparklineMark(mark) {
				return
			}
		}
		t.Errorf("Sparkline series colors missing: %+v", marks)
	})
	t.Run("visible-line", func(t *testing.T) {
		marks := referencePDFPaths(t, contents(t, filepath.Join(dir, "sparklines-render", "sparklines-stacked.pdf")))
		for _, curve := range marks {
			if !curve.line && !curve.rect && curve.stroked && curve.linewidth >= 1 && curve.x1-curve.x0 > 30 && curve.y1-curve.y0 > 15 {
				return
			}
		}
		t.Fatal("Visible line-sparkline path missing from rendered page")
	})

	t.Run("placement", func(t *testing.T) {
		g := referencePDFPageGeometry(t, contents(t, filepath.Join(dir, "sparklines-render", "sparklines-stacked.pdf")), 0, true)
		heading := referencePDFRequireWord(t, g, "Trend")
		previous := referencePDFRequireWord(t, g, "Apr")
		columnWidth := heading.x0 - previous.x0
		for _, curve := range g.paths {
			if curve.line || curve.rect || !curve.stroked || curve.linewidth < 1 || curve.x1-curve.x0 <= 30 || curve.y1-curve.y0 <= 15 {
				continue
			}
			top, bottom := g.height-curve.y1, g.height-curve.y0
			if !(heading.x0 <= curve.x0 && curve.x0 < curve.x1 && curve.x1 <= heading.x0+columnWidth && heading.bottom < top && top < bottom && bottom <= heading.bottom+50) {
				t.Errorf("Line sparkline is outside its F2 cell: heading=%+v previous=%+v curve=%+v", heading, previous, curve)
			}
		}
	})

	t.Run("native-groups", func(t *testing.T) {
		report, err := office.PackageReport(source)
		if err != nil {
			t.Fatal(err)
		}
		features, ok := report["native_features"].(map[string][]any)
		if !ok {
			t.Fatalf("native_features has unexpected type: %T", report["native_features"])
		}
		if got := len(features["sparklineGroup"]); got != 3 {
			t.Errorf("native sparkline groups=%d, want 3", got)
		}
	})

	t.Run("deleted-colors", func(t *testing.T) {
		p, err := opc.Open(source)
		if err != nil {
			t.Fatal(err)
		}
		_, err = office.EditSparklines(p, office.SparklineSpec{Sheet: "Trends", Action: "clear"})
		if err != nil {
			t.Fatal(err)
		}
		cleared := filepath.Join(dir, "sparklines-cleared.xlsx")
		if err := p.SaveNew(cleared); err != nil {
			t.Fatal(err)
		}
		out := filepath.Join(dir, "sparklines-cleared-render")
		_, err = RenderFile(context.Background(), cleared, out, "", "", pdf.RenderOptions{})
		if err != nil {
			t.Fatal(err)
		}
		marks := referencePDFPaths(t, contents(t, filepath.Join(out, "sparklines-cleared.pdf")))
		for _, mark := range marks {
			if referenceIsSparklineMark(mark) {
				t.Errorf("Deleted sparklines still rendered: %+v", mark)
			}
		}
	})

}
