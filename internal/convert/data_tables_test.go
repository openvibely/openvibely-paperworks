package convert

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/beevik/etree"
	"github.com/openvibely/openvibely-paperworks/internal/lo"
	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"github.com/xuri/excelize/v2"
)

// Ports all three native What-If cases, including reopening the calculated
// workbook and changing the driver formula before the second calculation.
func TestLibreOfficeNativeDataTables(t *testing.T) {
	if _, e := lo.Find(); e != nil {
		t.Skip("LibreOffice unavailable; native Data Table runtime acceptance not run")
	}
	for _, mode := range []string{"two-input", "column-input", "row-input"} {
		t.Run(mode, func(t *testing.T) {
			ctx := context.Background()
			dir := t.TempDir()
			source := referenceRuntimeOriginalWorkbookFixture(t, "library-report-fixtures.zip", mode)
			tableRange := "E3:E4"
			expected := map[string]float64{"E3": 52, "E4": 53}
			switch mode {
			case "two-input":
				tableRange = "E3:F4"
				expected = map[string]float64{"E3": 102, "F3": 103, "E4": 202, "F4": 203}
			case "row-input":
				tableRange = "E3:F3"
				expected = map[string]float64{"E3": 52, "F3": 53}
			}
			before, e := os.ReadFile(source)
			if e != nil {
				t.Fatal(e)
			}
			var calculated string
			for pass := 0; pass < 2; pass++ {
				input := source
				if pass == 1 {
					input = filepath.Join(dir, "changed.xlsx")
					saved, e := opc.Open(calculated)
					if e != nil {
						t.Fatal(e)
					}
					sheet, e := saved.XML("xl/worksheets/sheet1.xml")
					if e != nil {
						t.Fatal(e)
					}
					referenceDataTableSecondSave(t, sheet.Root())
					if e = saved.SetXML("xl/worksheets/sheet1.xml", sheet); e != nil {
						t.Fatal(e)
					}
					if e = saved.SaveNew(input); e != nil {
						t.Fatal(e)
					}
					expected = map[string]float64{"E3": 102}
					if mode == "two-input" {
						expected["E3"] = 202
					}
				}
				outdir := filepath.Join(dir, []string{"calculated", "recalculated"}[pass])
				result, e := Convert(ctx, input, outdir, Options{Recalculate: true})
				if e != nil {
					t.Fatal(e)
				}
				output := result["output"].(string)
				calculated = output
				f, e := excelize.OpenFile(output)
				if e != nil {
					t.Fatal(e)
				}
				for ref, want := range expected {
					if got := referenceRuntimeValue(t, f, "Model", ref); got != want {
						t.Errorf("pass %d %s: %v want %v", pass, ref, got, want)
					}
				}
				f.Close()
				p, e := opc.Open(output)
				if e != nil {
					t.Fatal(e)
				}
				// Inspect the saved formula XML independently of production OpenWorkbook.
				sheet, e := p.XML("xl/worksheets/sheet1.xml")
				if e != nil {
					t.Fatal(e)
				}
				var master *etree.Element
				for _, cell := range office.All(sheet.Root(), office.NSS, "c") {
					if cell.SelectAttrValue("r", "") == "E3" {
						master = office.Child(cell, office.NSS, "f")
						break
					}
				}
				if master == nil || master.SelectAttrValue("t", "") != "dataTable" || master.SelectAttrValue("ref", "") != tableRange {
					t.Fatal("native table definition lost")
				}
				// Original source-read guard executes before driver editing.
				after, e := os.ReadFile(source)
				if e != nil || !bytes.Equal(before, after) {
					t.Fatal("source changed", e)
				}
				original, e := opc.Open(input)
				if e != nil {
					t.Fatal(e)
				}
				for _, part := range original.Parts {
					if part.Name == "xl/worksheets/sheet1.xml" {
						continue
					}
					if !bytes.Equal(part.Data, p.Get(part.Name)) {
						t.Fatalf("unrelated part changed: %s", part.Name)
					}
				}
			}
			after, _ := os.ReadFile(source)
			if !bytes.Equal(before, after) {
				t.Fatal("source changed")
			}
		})
	}
}

// Bounded counterpart of openpyxl's formula-mode cell writer for these numeric
// Data Table fixtures. Saving clears every formula cache, not just the edited
// driver. Numeric follower cells are ordinary cells and retain their values.
// Other package rewrites by openpyxl remain a separate fixture-fidelity gate.
func referenceDataTableSecondSave(t *testing.T, root *etree.Element) {
	t.Helper()
	found := false
	for _, cell := range office.All(root, office.NSS, "c") {
		formula := office.Child(cell, office.NSS, "f")
		if cell.SelectAttrValue("r", "") == "B3" {
			if formula == nil {
				t.Fatal("driver formula missing")
			}
			formula.SetText("B1+20*B2")
			found = true
		}
		if formula != nil {
			for _, value := range office.Children(cell, office.NSS, "v") {
				cell.RemoveChild(value)
			}
			value := etree.NewElement("v")
			value.Space = cell.Space
			cell.AddChild(value)
			cell.RemoveAttr("t")
		} else if office.Child(cell, office.NSS, "v") != nil {
			kind := cell.SelectAttrValue("t", "n")
			if kind != "n" {
				t.Fatalf("unsupported nonnumeric Data Table fixture cell %s: %s", cell.SelectAttrValue("r", ""), kind)
			}
			cell.CreateAttr("t", "n")
		}
	}
	if !found {
		t.Fatal("driver B3 missing")
	}
}
