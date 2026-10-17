package convert

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"github.com/openvibely/openvibely-paperworks/internal/pdf"
	"github.com/openvibely/openvibely-paperworks/internal/pptx"
	"github.com/openvibely/openvibely-paperworks/internal/testsupport"
	"github.com/xuri/excelize/v2"
)

// Runtime 429-460: real engine recalculation, formula identity, exact ZIP order,
// every unrelated embedded part, both rejected edits, and source bytes.
// Corrected acceptance baseline: repair the captured reference notes-master defect.
func TestReferenceStandaloneRuntimeChartDrivers(t *testing.T) {
	referenceRuntimeOffice(t)
	ctx := context.Background()
	dir := t.TempDir()
	must := func(e error) {
		t.Helper()
		if e != nil {
			t.Fatal(e)
		}
	}
	src := referenceRuntimeOriginalPackageFixture(t, "runtime-workflow-fixtures.zip", "chart-formula", ".pptx")
	p, e := opc.Open(src)
	must(e)
	// Reference selects the first embedding by ZIP order, not chart discovery.
	embedded := ""
	for _, name := range p.Names() {
		if strings.HasPrefix(name, "ppt/embeddings/") && strings.HasSuffix(name, ".xlsx") {
			embedded = name
			break
		}
	}
	if embedded == "" {
		t.Fatal("missing embedded workbook")
	}
	book, e := opc.Read(p.Get(embedded))
	must(e)
	before := contents(t, src)
	t.Run("source-render", func(t *testing.T) {
		images, e := RenderFile(ctx, src, filepath.Join(dir, "source-render"), "", "", pdf.RenderOptions{})
		if e != nil {
			t.Fatal(e)
		}
		if len(images) != 2 {
			t.Errorf("rendered images=%d, want 2", len(images))
		}
	})
	dst := filepath.Join(dir, "chart-formula-edited.pptx")
	report, e := ChartInputs(ctx, src, dst, ChartInputSpec{Chart: 1, Sheet: "Drivers", Cells: map[string]any{"A1": float64(100)}})
	must(e)
	changed, ok := report["changed_parts"].([]string)
	if !ok {
		t.Fatalf("changed parts type: %T", report["changed_parts"])
	}
	if _, e = office.VerifyPreservation(src, dst, changed); e != nil {
		t.Fatal(e)
	}
	saved, e := opc.Open(dst)
	must(e)
	afterBook, e := opc.Read(saved.Get(embedded))
	must(e)
	values, e := excelize.OpenReader(bytes.NewReader(saved.Get(embedded)))
	must(e)
	defer values.Close()
	for _, c := range []struct {
		cell, formula string
		value         float64
	}{{"B2", "Drivers!A1/2", 50}, {"B3", "Drivers!A2/2", 30}} {
		t.Run(c.cell, func(t *testing.T) {
			formula, e := values.GetCellFormula("Sheet1", c.cell)
			if e != nil {
				t.Fatal(e)
			}
			value := referenceRuntimeValue(t, values, "Sheet1", c.cell)
			if formula != c.formula || value != c.value {
				t.Errorf("formula=%q cached value=%v, want %q and %v", formula, value, c.formula, c.value)
			}
		})
	}
	t.Run("archive-order", func(t *testing.T) {
		if !reflect.DeepEqual(book.Names(), afterBook.Names()) {
			t.Errorf("ZIP order changed: %v -> %v", book.Names(), afterBook.Names())
		}
	})
	t.Run("unrelated-parts", func(t *testing.T) {
		for _, part := range book.Names() {
			if part == "xl/worksheets/sheet1.xml" || part == "xl/worksheets/sheet2.xml" {
				continue
			}
			t.Run(part, func(t *testing.T) {
				if !bytes.Equal(book.Get(part), afterBook.Get(part)) {
					t.Error("part bytes changed")
				}
			})
		}
	})
	t.Run("source", func(t *testing.T) {
		if !bytes.Equal(before, contents(t, src)) {
			t.Error("source changed")
		}
	})
	bad := filepath.Join(dir, "bad-chart.pptx")
	for _, c := range []struct {
		name, sheet, cell string
		value             any
	}{{"overwrite-formula", "Sheet1", "B2", float64(999)}, {"insert-formula", "Drivers", "A1", "=1+1"}} {
		t.Run(c.name, func(t *testing.T) {
			_, e := ChartInputs(ctx, src, bad, ChartInputSpec{Chart: 1, Sheet: c.sheet, Cells: map[string]any{c.cell: c.value}})
			if e == nil {
				t.Error("formula edit accepted")
			}
			if _, e = os.Stat(bad); !os.IsNotExist(e) {
				t.Errorf("rejected edit published output: %v", e)
			}
		})
	}
	final := filepath.Join(dir, "presentation-final.pptx")
	_, e = pptx.EditFile(ctx, dst, final, pptx.EditSpec{Edits: []pptx.Edit{
		{Slide: 2, ShapeID: "2", Operation: "run-text", Text: "Revenue totaled $80"},
		{Slide: 2, Operation: "notes", Text: "Revenue: $80; Q1; USD. Only listed revenue streams; no profitability inference."},
	}})
	must(e)
	// The original Presentation.save writes the reachable package in relationship
	// depth-first order. Adapt fixture serialization without changing payloads,
	// verification requirements, or the production writer's preservation policy.
	finalPackage, e := opc.Open(final)
	must(e)
	// The pinned Reference fixture omits the presentation notes-master list.
	// Preserve evidence of its rejection, then exercise the native repair.
	brokenReport, e := office.PackageReportOf(finalPackage)
	must(e)
	if _, err := office.VerifyPresentation(finalPackage, brokenReport, office.SlideRequirements{}); err == nil {
		t.Fatal("missing notes-master list accepted")
	}
	must(office.RepairNotesPackage(finalPackage))
	must(testsupport.OrderPresentationFixture(finalPackage))
	finalBytes, e := testsupport.PresentationFixtureBytes(finalPackage)
	must(e)
	must(os.WriteFile(final, finalBytes, 0600))
	if _, e = office.VerifyPreservation(dst, final, []string{"ppt/slides/slide2.xml", "ppt/notesSlides/notesSlide2.xml", "ppt/presentation.xml"}); e != nil {
		t.Fatal(e)
	}
	if !t.Run("final-requirements", func(t *testing.T) {
		deck, e := opc.Open(final)
		if e != nil {
			t.Fatal(e)
		}
		report, e := office.PackageReportOf(deck)
		if e != nil {
			t.Fatal(e)
		}
		count := 2
		if _, e = office.VerifyPresentation(deck, report, office.SlideRequirements{Slides: &count, SizeEMU: []int64{12191695, 6858000}, NativeCharts: []int{2}, Fonts: []string{"Arial"}}); e != nil {
			t.Fatal(e)
		}
	}) {
		// The original sequential Reference block stops at this failed prerequisite.
		return
	}
	t.Run("final-render", func(t *testing.T) {
		images, e := RenderFile(ctx, final, filepath.Join(dir, "final-render"), "", "", pdf.RenderOptions{})
		if e != nil {
			t.Fatal(e)
		}
		if len(images) != 2 {
			t.Errorf("rendered images=%d, want 2", len(images))
		}
	})

}
