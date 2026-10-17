package convert

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/beevik/etree"
	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"github.com/openvibely/openvibely-paperworks/internal/pdf"
	"github.com/openvibely/openvibely-paperworks/internal/testsupport"
)

// Original rolling payroll model, including its actual row and every extended native range.
func TestReferenceStandaloneRuntimeRollingPayroll(t *testing.T) {
	referenceRuntimeOffice(t)
	dir := t.TempDir()
	src := referenceRuntimeOriginalWorkbookFixture(t, "runtime-workflow-fixtures.zip", "rolling-model")
	before := contents(t, src)
	base := referenceStandaloneCalculate(t, src, filepath.Join(dir, "base"))
	saved := referenceRuntimeRead(t, base)
	t.Run("base-values", func(t *testing.T) {
		for i, want := range []float64{60000, 69300, 75600, 81900} {
			if got := referenceRuntimeValue(t, saved, "Build", fmt.Sprintf("D%d", i+2)); got != want {
				t.Error(got, want)
			}
		}
		if v := referenceRuntimeValue(t, saved, "Summary", "B3"); v != 226800 {
			t.Error(v)
		}
		if v, e := saved.GetCellValue("Summary", "B4"); e != nil || v != "Base selected" {
			t.Error(v, e)
		}
	})
	t.Run("ranges", func(t *testing.T) {
		ref, e := testsupport.NamedWorksheetTableRef(contents(t, base), "Build", "Payroll")
		if e != nil || ref != "A1:D5" {
			t.Error(ref, e)
		}
		v, e := saved.GetDataValidations("Build")
		if e != nil || len(v) != 1 || v[0].Sqref != "C2:C5" {
			t.Error(v, e)
		}
	})
	t.Run("name", func(t *testing.T) {
		for _, n := range saved.GetDefinedName() {
			if n.Name == "ForecastPay" {
				if n.RefersTo != "'Build'!$D$3:$D$5" {
					t.Error(n)
				}
				return
			}
		}
		t.Error("missing ForecastPay")
	})
	t.Run("chart", func(t *testing.T) {
		p, e := opc.Open(base)
		if e != nil {
			t.Fatal(e)
		}
		parts, e := testsupport.NamedWorksheetCharts(contents(t, base), "Summary")
		if e != nil || len(parts) != 1 {
			t.Fatal(parts, e)
		}
		d, e := p.XML(parts[0])
		if e != nil {
			t.Fatal(e)
		}
		var refs []*etree.Element
		for _, val := range office.All(d.Root(), office.NSC, "val") {
			if ref := office.Child(val, office.NSC, "numRef"); ref != nil {
				if f := office.Child(ref, office.NSC, "f"); f != nil {
					refs = append(refs, f)
				}
			}
		}
		if len(refs) != 1 || refs[0].Text() != "'Build'!$D$2:$D$5" {
			t.Errorf("chart value refs: %v", refs)
		}
		var blanks *etree.Element
		if nodes := office.All(d.Root(), office.NSC, "dispBlanksAs"); len(nodes) > 0 {
			blanks = nodes[0]
		}
		if blanks == nil || blanks.SelectAttrValue("val", "") != "gap" {
			t.Error("chart blank policy", blanks)
		}
		if !reflect.DeepEqual(saved.GetSheetList(), []string{"Summary", "Build", "Inputs"}) {
			t.Error(saved.GetSheetList())
		}
	})
	edited := referenceStandaloneInputEdit(t, base, filepath.Join(dir, "downside.xlsx"), "Inputs", map[string]any{"B1": float64(2)})
	down := referenceStandaloneCalculate(t, edited, filepath.Join(dir, "down"))
	t.Run("downside", func(t *testing.T) {
		book := referenceRuntimeRead(t, down)
		if v := referenceRuntimeValue(t, book, "Build", "D2"); v != 60000 {
			t.Error(v)
		}
		if v := referenceRuntimeValue(t, book, "Summary", "B3"); v != 216000 {
			t.Error(v)
		}
		if v, e := book.GetCellValue("Summary", "B4"); e != nil || v != "Downside selected" {
			t.Error(v, e)
		}
	})
	t.Run("source", func(t *testing.T) {
		if !bytes.Equal(before, contents(t, src)) {
			t.Error("source changed")
		}
	})
	for _, c := range []struct{ name, path string }{{"base-render", base}, {"downside-render", down}} {
		t.Run(c.name, func(t *testing.T) {
			pages, e := RenderFile(context.Background(), c.path, filepath.Join(dir, c.name), "", "", pdf.RenderOptions{})
			if e != nil || len(pages) != 3 {
				t.Error(len(pages), e)
			}
		})
	}
}
