package office

import (
	"path/filepath"
	"testing"

	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"github.com/xuri/excelize/v2"
)

func TestPreserveFormulaCachesEmptyString(t *testing.T) {
	for _, tc := range []struct {
		name, kind         string
		missing, wantError bool
	}{
		{"empty string", "str", false, false},
		{"missing string cache", "str", true, true},
		{"empty numeric cache", "n", false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := filepath.Join(t.TempDir(), "source.xlsx")
			f := excelize.NewFile()
			defer f.Close()
			if err := f.SetCellFormula("Sheet1", "A1", `""`); err != nil {
				t.Fatal(err)
			}
			if err := f.SaveAs(source); err != nil {
				t.Fatal(err)
			}
			pkg, err := opc.Open(source)
			if err != nil {
				t.Fatal(err)
			}
			report, err := PackageReportOf(pkg)
			if err != nil {
				t.Fatal(err)
			}
			wb, err := OpenWorkbook(pkg)
			if err != nil {
				t.Fatal(err)
			}
			sh := wb.Sheets["Sheet1"]
			cell := sh.Cells["A1"]
			cell.CreateAttr("t", tc.kind)
			RemoveAll(cell, NSS, "v")
			if !tc.missing {
				NewChild(cell, NSS, "v", nil)
			}
			if err := pkg.SetXML(sh.Part, sh.Doc); err != nil {
				t.Fatal(err)
			}
			calculated, err := pkg.Bytes()
			if err != nil {
				t.Fatal(err)
			}
			result, err := PreserveFormulaCaches(source, calculated, report["formulas"].([]Formula))
			if (err != nil) != tc.wantError {
				t.Fatalf("err=%v wantError=%v", err, tc.wantError)
			}
			if tc.wantError {
				return
			}
			saved, err := opc.Read(result)
			if err != nil {
				t.Fatal(err)
			}
			got, err := OpenWorkbook(saved)
			if err != nil {
				t.Fatal(err)
			}
			c := got.Sheets["Sheet1"].Cells["A1"]
			v := Child(c, NSS, "v")
			if c.SelectAttrValue("t", "") != "str" || v == nil || v.Text() != "" || Child(c, NSS, "f").Text() != `""` {
				t.Fatal("empty-string formula result not preserved")
			}
		})
	}
}
