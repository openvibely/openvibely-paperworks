package office

import (
	"bytes"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"testing"

	"github.com/xuri/excelize/v2"
)

// The Reference workbook refresh patches only explicit empty <v> string-formula
// results. Missing caches still fail, and trace must continue reporting null.
func TestWorkbookChartEmptyFormulaGap(t *testing.T) {
	for _, tc := range []struct {
		name, kind, value  string
		missing, wantError bool
	}{
		{"empty string", "str", "", false, false},
		{"absent string", "str", "", true, true},
		{"empty numeric", "n", "", false, true},
		{"zero", "n", "0", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, _ := traceFixture(t, func(f *excelize.File) {
				if err := f.SetCellFormula("Inputs", "D1", `""`); err != nil {
					t.Fatal(err)
				}
				if err := f.AddChart("Inputs", "F1", &excelize.Chart{Type: excelize.Col, Series: []excelize.ChartSeries{{Name: "Inputs!$A$1", Values: "Inputs!$D$1"}}}); err != nil {
					t.Fatal(err)
				}
			})
			sheet, address := "Inputs", "D1"
			if tc.name == "empty string" {
				var err error
				p, err = opc.Open(referenceChartCacheFixture(t, "empty-0"))
				if err != nil {
					t.Fatal(err)
				}

				sheet, address = "Sheet", "A1"
			}
			wb, err := OpenWorkbook(p)
			if err != nil {
				t.Fatal(err)
			}
			sh := wb.Sheets[sheet]
			cell := sh.Cells[address]
			if tc.name != "empty string" {
				cell.CreateAttr("t", tc.kind)
				RemoveAll(cell, NSS, "v")
				if !tc.missing {
					NewChild(cell, NSS, "v", nil).SetText(tc.value)
				}
				if err := p.SetXML(sh.Part, sh.Doc); err != nil {
					t.Fatal(err)
				}
			}

			before := bytes.Clone(p.Get(sh.Part))
			err = RefreshWorkbookChartCaches(p)
			if (err != nil) != tc.wantError {
				t.Fatalf("refresh error=%v wantError=%v", err, tc.wantError)
			}
			if !bytes.Equal(before, p.Get(sh.Part)) {
				t.Fatal("worksheet changed")
			}
			if tc.wantError {
				return
			}
			// Reopen serialized results as the original ZipFile(BytesIO(raw)) does.
			raw, err := p.Bytes()
			if err != nil {
				t.Fatal(err)
			}
			p, err = opc.Read(raw)
			if err != nil {
				t.Fatal(err)
			}
			d, err := p.XML("xl/charts/chart1.xml")
			if err != nil {
				t.Fatal(err)
			}
			refs := All(d.Root(), NSC, "numRef")
			if len(refs) != 1 {
				t.Fatal("numeric reference count", len(refs))
			}
			cache := Child(refs[0], NSC, "numCache")
			if Child(cache, NSC, "ptCount").SelectAttrValue("val", "") != "1" {
				t.Fatal("lost gap index")
			}
			pts := All(cache, NSC, "pt")
			if tc.value == "0" {
				if len(pts) != 1 || Text(pts[0], NSC, "v") != "0" {
					t.Fatal("zero lost")
				}
			} else {
				if len(pts) != 0 {
					t.Fatal("gap turned into point")
				}
				r := traceRun(t, p, sheet, address, 200, 20)
				if r.Nodes["'"+sheet+"'!"+address].CachedValue != nil {
					t.Fatal("trace null semantics changed")
				}
			}
		})
	}
}
