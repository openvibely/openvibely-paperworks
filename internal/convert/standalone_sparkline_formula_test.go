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
)

// Pinned test_document_runtime.py:975-990. Both driver values and all three
// rendered-path assertions are preserved; batch41 uses the original input.
func TestReferenceStandaloneRuntimeSparklineFormula(t *testing.T) {
	referenceRuntimeOffice(t)
	dir := t.TempDir()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	source := referenceRuntimeOriginalWorkbookFixture(t, "runtime-workflow-fixtures.zip", "spark-formula-source")
	p, err := opc.Open(source)
	must(err)
	_, err = office.EditSparklines(p, office.SparklineSpec{Sheet: "Inputs", Sparklines: []office.SparklineItem{{Cell: "E2", Range: "Inputs!A1:C1"}}, Options: map[string]json.RawMessage{"lineWeight": json.RawMessage("2")}})
	must(err)
	authored := filepath.Join(dir, "spark-formula.xlsx")
	must(p.SaveNew(authored))
	for index, value := range []float64{2, 8} {
		t.Run(fmt.Sprintf("driver-%d", index), func(t *testing.T) {
			changed := referenceStandaloneInputEdit(t, authored, filepath.Join(t.TempDir(), fmt.Sprintf("spark-input-%d.xlsx", index)), "Inputs", map[string]any{"A1": value})
			output := filepath.Join(t.TempDir(), "render")
			_, err := RenderFile(context.Background(), changed, output, "", "", pdf.RenderOptions{})
			if err != nil {
				t.Fatal(err)
			}
			paths := referencePDFPaths(t, contents(t, filepath.Join(output, fmt.Sprintf("spark-input-%d.pdf", index))))
			var lines []referencePDFPath
			for _, c := range paths {
				if !c.line && !c.rect && c.stroked && c.linewidth >= 1 && len(c.points) == 3 {
					lines = append(lines, c)
				}
			}
			t.Run("count", func(t *testing.T) {
				if len(lines) != 1 {
					t.Errorf("lines=%+v, want exactly one", lines)
				}
			})
			// Guard dependent assertions with failure, never a skip or vacuous pass.
			t.Run("peak", func(t *testing.T) {
				if len(lines) != 1 {
					t.Fatalf("cannot inspect points: lines=%+v", lines)
				}
				pts := lines[0].points
				// pdfplumber pts use top-down page coordinates; PDFium uses bottom-up.
				if !(-pts[1][1] < -pts[0][1] && -pts[1][1] < -pts[2][1]) {
					t.Errorf("middle point is not above both endpoints: %v", pts)
				}
			})
			t.Run("endpoint-order", func(t *testing.T) {
				if len(lines) != 1 {
					t.Fatalf("cannot inspect points: lines=%+v", lines)
				}
				pts := lines[0].points
				if (-pts[0][1] > -pts[2][1]) != (index == 0) {
					t.Errorf("driver=%v points=%v: endpoint order incorrect", value, pts)
				}
			})
		})
	}
}
