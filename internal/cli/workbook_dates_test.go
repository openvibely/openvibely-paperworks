package cli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"github.com/xuri/excelize/v2"
)

func TestISODateCopiesThroughCLI(t *testing.T) {
	for _, date1904 := range []bool{false, true} {
		t.Run(fmt.Sprint(date1904), func(t *testing.T) {
			dir := t.TempDir()
			in, out := filepath.Join(dir, "input.xlsx"), filepath.Join(dir, "edited.xlsx")
			f := excelize.NewFile()
			defer f.Close()
			if err := f.SetWorkbookProps(&excelize.WorkbookPropsOptions{Date1904: &date1904}); err != nil {
				t.Fatal(err)
			}
			if _, err := f.NewSheet("Other"); err != nil {
				t.Fatal(err)
			}
			for _, cell := range []string{"A1", "A2"} {
				if err := f.SetCellInt("Sheet1", cell, 1); err != nil {
					t.Fatal(err)
				}
			}
			if err := f.MergeCell("Sheet1", "E1", "F1"); err != nil {
				t.Fatal(err)
			}
			for cell, formula := range map[string]string{"G1": "COUNT(A1:E1)", "G2": "YEAR(B1)", "G3": "COUNT(Other!A1)", "G4": "B1=C1"} {
				if err := f.SetCellFormula("Sheet1", cell, formula); err != nil {
					t.Fatal(err)
				}
			}
			raw, err := f.WriteToBuffer()
			if err != nil {
				t.Fatal(err)
			}
			pkg, err := opc.Read(raw.Bytes())
			if err != nil {
				t.Fatal(err)
			}
			wb, err := office.OpenWorkbook(pkg)
			if err != nil {
				t.Fatal(err)
			}
			sh := wb.Sheets["Sheet1"]
			dates := map[string]string{"A1": "2026-01-02T12:34:56", "A2": "2025-06-03"}
			for address, value := range dates {
				cell := sh.Cells[address]
				cell.CreateAttr("t", "d")
				office.RemoveAll(cell, office.NSS, "v")
				office.NewChild(cell, office.NSS, "v", nil).SetText(value)
			}
			if err := pkg.SetXML(sh.Part, sh.Doc); err != nil {
				t.Fatal(err)
			}
			if err := pkg.SaveNew(in); err != nil {
				t.Fatal(err)
			}
			original, err := os.ReadFile(in)
			if err != nil {
				t.Fatal(err)
			}
			spec := writeSpec(t, dir, "edits.json", map[string]any{"edits": []any{
				map[string]any{"operation": "copy", "sheet": "Sheet1", "range": "A1:A2", "target": "B1"},
				map[string]any{"operation": "copy-values", "sheet": "Sheet1", "range": "B1:B2", "target": "C1"},
				map[string]any{"operation": "copy-formulas", "sheet": "Sheet1", "range": "C1:C2", "target": "D1"},
				map[string]any{"operation": "copy-values", "sheet": "Sheet1", "range": "D1", "target": "F1"},
				map[string]any{"operation": "copy", "sheet": "Sheet1", "range": "D1", "target": "A1", "name": "other"},
				map[string]any{"operation": "rename-sheet", "sheet": "Sheet1", "name": "Result"},
				map[string]any{"operation": "copy", "sheet": "Result", "range": "A1:A2", "target": "A2"},
			}})
			runCLI(t, "workbook-edit", in, "--spec", spec, "--output", out)
			got, err := excelize.OpenFile(out)
			if err != nil {
				t.Fatal(err)
			}
			defer got.Close()
			for _, address := range []string{"A1", "A2", "B1", "C1", "D1", "E1", "F1", "A3", "B2", "C2", "D2"} {
				want := dates["A1"]
				if address == "A3" || address == "B2" || address == "C2" || address == "D2" {
					want = dates["A2"]
				}
				kind, err := got.GetCellType("Result", address)
				if err != nil || kind != excelize.CellTypeDate {
					t.Errorf("%s type=%v err=%v", address, kind, err)
				}
				value, err := got.GetCellValue("Result", address, excelize.Options{RawCellValue: true})
				if err != nil || value != want {
					t.Errorf("%s value=%q err=%v want=%q", address, value, err, want)
				}
			}
			result := runCLI(t, "recalculate", out, "--output", filepath.Join(dir, "calculated"))
			calculated, err := excelize.OpenFile(result["output"].(string))
			if err != nil {
				t.Fatal(err)
			}
			defer calculated.Close()
			for cell, want := range map[string]string{"G1": "5", "G2": "2026", "G3": "1", "G4": "TRUE"} {
				value, err := calculated.GetCellValue("Result", cell)
				if err != nil || value != want {
					t.Errorf("%s=%q (%v), want %s", cell, value, err, want)
				}
			}
			after, err := os.ReadFile(in)
			if err != nil || !bytes.Equal(original, after) {
				t.Fatal("input changed", err)
			}
		})
	}
}
