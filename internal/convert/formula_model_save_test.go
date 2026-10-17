package convert

import (
	"github.com/xuri/excelize/v2"
	"path/filepath"
	"testing"

	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
)

// Bounded formula-mode save for the original Model scenario. openpyxl loads
// formulas, not their cached results, and writes empty v elements for every
// formula cell when saving the changed driver. Preserve the chart's own caches.
// General openpyxl package rewriting/serialization is not reproduced here.
func referenceFormulaModelSecondSave(t *testing.T, source, destination string) {
	t.Helper()
	p, err := opc.Open(source)
	if err != nil {
		t.Fatal(err)
	}
	wb, err := office.OpenWorkbook(p)
	if err != nil {
		t.Fatal(err)
	}
	sheet := wb.Sheets["Model"]
	if len(wb.Sheets) != 1 || sheet == nil {
		t.Fatal("expected original single-sheet Model workbook")
	}
	driver := sheet.Cells["A2"]
	if driver == nil || office.Child(driver, office.NSS, "f") != nil {
		t.Fatal("expected numeric A2 driver")
	}
	value := office.Child(driver, office.NSS, "v")
	if value == nil {
		t.Fatal("missing A2 value")
	}
	value.SetText("5")
	driver.CreateAttr("t", "n")
	for _, cell := range sheet.Cells {
		if office.Child(cell, office.NSS, "f") == nil {
			continue
		}
		cell.RemoveAttr("t")
		for _, v := range office.Children(cell, office.NSS, "v") {
			cell.RemoveChild(v)
		}
		cell.CreateElement("v")
	}
	if err = p.SetXML(sheet.Part, sheet.Doc); err != nil {
		t.Fatal(err)
	}
	if err = p.SaveNew(destination); err != nil {
		t.Fatal(err)
	}
}

// Stale caches must disappear from all formulas, not merely the changed row.
// This offline regression deliberately does not invoke an office engine.
func TestReferenceFormulaModelSecondSaveClearsCaches(t *testing.T) {
	source := referenceRuntimeOriginalWorkbookFixture(t, "runtime-workflow-fixtures.zip", "formula-model")
	p, err := opc.Open(source)
	if err != nil {
		t.Fatal(err)
	}
	wb, err := office.OpenWorkbook(p)
	if err != nil {
		t.Fatal(err)
	}
	sheet := wb.Sheets["Model"]
	for addr, cached := range map[string]string{"C2": "21", "C3": "32", "C4": "53"} {
		v := office.Child(sheet.Cells[addr], office.NSS, "v")
		if v == nil {
			t.Fatal("missing original value element", addr)
		}
		v.SetText(cached)
	}
	if err = p.SetXML(sheet.Part, sheet.Doc); err != nil {
		t.Fatal(err)
	}
	calculated := filepath.Join(t.TempDir(), "calculated.xlsx")
	if err = p.SaveNew(calculated); err != nil {
		t.Fatal(err)
	}
	before := contents(t, calculated)
	scenario := filepath.Join(t.TempDir(), "scenario.xlsx")
	referenceFormulaModelSecondSave(t, calculated, scenario)
	saved := referenceRuntimeRead(t, scenario)
	for addr, want := range map[string]string{"C2": "A2*B2", "C3": "A3*B3", "C4": "SUM(C2:C3)"} {
		formula, err := saved.GetCellFormula("Model", addr)
		if err != nil || formula != want {
			t.Fatalf("%s formula %q: %v", addr, formula, err)
		}
		cache, err := saved.GetCellValue("Model", addr, excelize.Options{RawCellValue: true})
		if err != nil || cache != "" {
			t.Fatalf("%s retained cache %q: %v", addr, cache, err)
		}
	}
	for addr, want := range map[string]float64{"A2": 5, "B2": 7, "A3": 4, "B3": 8} {
		if got := referenceRuntimeValue(t, saved, "Model", addr); got != want {
			t.Fatalf("%s=%v, want %v", addr, got, want)
		}
	}
	assertUnchanged(t, calculated, before)
}
