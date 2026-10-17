package convert

import (
	"bytes"
	"fmt"
	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"strconv"
	"testing"

	"github.com/xuri/excelize/v2"
)

// Numeric equality for runtime81: openpyxl returns a number, not its XML spelling.
// Retain the numeric-type guard so a text cell containing "200" cannot pass.
func referenceSkillNumericCell(f *excelize.File, sheet, cell string) (float64, error) {
	kind, err := f.GetCellType(sheet, cell)
	if err != nil {
		return 0, err
	}
	if kind != excelize.CellTypeNumber && kind != excelize.CellTypeUnset {
		return 0, fmt.Errorf("%s!%s must be numeric, got type %v", sheet, cell, kind)
	}
	raw, err := f.GetCellValue(sheet, cell, excelize.Options{RawCellValue: true})
	if err != nil {
		return 0, err
	}
	return strconv.ParseFloat(raw, 64)
}

func TestReferenceSkillNumericCellEquality(t *testing.T) {
	for _, tc := range []struct {
		name, kind, raw string
		want            bool
	}{
		{"integer", "n", "200", true}, {"decimal", "n", "200.0", true},
		{"exponent", "n", "2e2", true}, {"other", "n", "201", false},
		{"string", "str", "200", false}, {"boolean", "b", "1", false},
		{"error", "e", "#VALUE!", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := excelize.NewFile()
			defer f.Close()
			if err := f.SetCellValue("Sheet1", "D4", 200); err != nil {
				t.Fatal(err)
			}
			raw, err := f.WriteToBuffer()
			if err != nil {
				t.Fatal(err)
			}
			p, err := opc.Read(raw.Bytes())
			if err != nil {
				t.Fatal(err)
			}
			d, err := p.XML("xl/worksheets/sheet1.xml")
			if err != nil {
				t.Fatal(err)
			}
			for _, c := range office.All(d.Root(), office.NSS, "c") {
				if c.SelectAttrValue("r", "") == "D4" {
					c.CreateAttr("t", tc.kind)
					office.Child(c, office.NSS, "v").SetText(tc.raw)
				}
			}
			if err = p.SetXML("xl/worksheets/sheet1.xml", d); err != nil {
				t.Fatal(err)
			}
			data, err := p.Bytes()
			if err != nil {
				t.Fatal(err)
			}
			saved, err := excelize.OpenReader(bytes.NewReader(data))
			if err != nil {
				t.Fatal(err)
			}
			defer saved.Close()
			got, err := referenceSkillNumericCell(saved, "Sheet1", "D4")
			if pass := err == nil && got == 200; pass != tc.want {
				t.Fatalf("value=%v error=%v equal200=%v want %v", got, err, pass, tc.want)
			}
		})
	}
}
