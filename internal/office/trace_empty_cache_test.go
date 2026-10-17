package office

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"github.com/xuri/excelize/v2"
)

// openpyxl 3.1.5 WorkSheetParser.parse_cell uses findtext(VALUE_TAG, None)
// or None before converting numeric/date/bool/shared-string values.
func TestTraceEmptySavedCachesAreNull(t *testing.T) {
	for _, c := range []struct {
		name, kind string
		format     int
	}{
		{"numeric", "n", 0}, {"date", "n", 22}, {"duration", "n", 46},
		{"boolean", "b", 0}, {"shared string", "s", 0}, {"string", "str", 0},
		{"ISO date", "d", 0}, {"error", "e", 0},
	} {
		for _, missing := range []bool{false, true} {
			t.Run(c.name+map[bool]string{true: "/absent", false: "/empty"}[missing], func(t *testing.T) {
				p, _ := traceFixture(t, func(f *excelize.File) {
					if err := f.SetCellFormula("Inputs", "D1", "A1+1"); err != nil {
						t.Fatal(err)
					}
					style, err := f.NewStyle(&excelize.Style{NumFmt: c.format})
					if err != nil {
						t.Fatal(err)
					}
					if err = f.SetCellStyle("Inputs", "D1", "D1", style); err != nil {
						t.Fatal(err)
					}
				})
				wb, err := OpenWorkbook(p)
				if err != nil {
					t.Fatal(err)
				}
				sh := wb.Sheets["Inputs"]
				cell := sh.Cells["D1"]
				cell.CreateAttr("t", c.kind)
				if v := Child(cell, NSS, "v"); v != nil {
					cell.RemoveChild(v)
				}
				if !missing {
					cell.CreateElement("v")
				}
				if err = p.SetXML(sh.Part, sh.Doc); err != nil {
					t.Fatal(err)
				}
				file := filepath.Join(t.TempDir(), "empty-cache.xlsx")
				if err = p.SaveNew(file); err != nil {
					t.Fatal(err)
				}
				before, err := os.ReadFile(file)
				if err != nil {
					t.Fatal(err)
				}
				p, err = opc.Open(file)
				if err != nil {
					t.Fatal(err)
				}
				parts := map[string][]byte{}
				for _, part := range p.Parts {
					parts[part.Name] = bytes.Clone(part.Data)
				}
				report := traceRun(t, p, "Inputs", "D1", 200, 20)
				root := report.Nodes["'Inputs'!D1"]
				if !report.Complete || len(report.Nodes) != 2 || len(report.Edges) != 1 || root.CachedValue != nil || root.Formula == nil || *root.Formula != "=A1+1" {
					t.Fatalf("%+v", report)
				}
				if report.Nodes["'Inputs'!A1"].CachedValue != float64(7) {
					t.Fatal("precedent changed")
				}
				data, err := json.Marshal(root)
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Contains(data, []byte(`"cached_value":null`)) {
					t.Fatal(string(data))
				}
				after, err := os.ReadFile(file)
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(before, after) {
					t.Fatal("source modified")
				}
				for _, part := range p.Parts {
					if !bytes.Equal(parts[part.Name], part.Data) {
						t.Fatal("part modified", part.Name)
					}
				}
			})
		}
	}
}

func TestEmptyInlineStringRemainsAString(t *testing.T) {
	p, _ := traceFixture(t, nil)
	wb, err := OpenWorkbook(p)
	if err != nil {
		t.Fatal(err)
	}
	cell := wb.Sheets["Inputs"].Cells["A1"]
	cell.CreateAttr("t", "inlineStr")
	if v := Child(cell, NSS, "v"); v != nil {
		cell.RemoveChild(v)
	}
	cell.CreateElement("is").CreateElement("t").SetText("")
	if err = p.SetXML(wb.Sheets["Inputs"].Part, wb.Sheets["Inputs"].Doc); err != nil {
		t.Fatal(err)
	}
	r := traceRun(t, p, "Inputs", "A1", 200, 20)
	if !r.Complete || r.Nodes[r.Root].CachedValue != "" {
		t.Fatalf("%+v", r)
	}
}
