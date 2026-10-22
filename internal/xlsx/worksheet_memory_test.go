package xlsx

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"github.com/xuri/excelize/v2"
)

// Exercise the adapter against actual Excelize types and a nonstandard sheet
// part, including absolute relationship targets. The ZIP writer trap makes a
// regression to per-copy workbook serialization fail independently of timing.
func TestCopyUsesLiveWorksheetWithoutSerialization(t *testing.T) {
	for _, target := range []string{"worksheets/custom.xml", "/xl/worksheets/custom.xml"} {
		t.Run(target, func(t *testing.T) {
			seed := excelize.NewFile()
			raw, err := seed.WriteToBuffer()
			seed.Close()
			if err != nil {
				t.Fatal(err)
			}
			pkg, err := opc.Read(raw.Bytes())
			if err != nil {
				t.Fatal(err)
			}
			pkg.Set("xl/worksheets/custom.xml", []byte(`<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData><row r="1"><c r="A1" t="e"><v>#N/A</v></c><c r="B1" t="d"><v>2026-01-02T03:04:05Z</v></c><c r="C1"><f t="array" ref="C1:C2">A1:A2*2</f></c></row><row r="2"><c r="C2"><v>4</v></c></row></sheetData></worksheet>`))
			rels := "xl/_rels/workbook.xml.rels"
			pkg.Set(rels, bytes.ReplaceAll(pkg.Get(rels), []byte("worksheets/sheet1.xml"), []byte(target)))
			data, err := pkg.Bytes()
			if err != nil {
				t.Fatal(err)
			}
			f, err := excelize.OpenReader(bytes.NewReader(data))
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			f.ZipWriter = func(io.Writer) excelize.ZipWriter { t.Fatal("copy serialized the workbook"); return nil }
			for _, e := range []Edit{
				{Operation: "copy", Sheet: "sheet1", Range: "A1:B1", Target: "E1"},
				{Operation: "copy-values", Sheet: "Sheet1", Range: "E1:F1", Target: "G1"},
				{Operation: "copy-formulas", Sheet: "Sheet1", Range: "C1:C2", Target: "J1"},
			} {
				next, err := copyRange(f, e)
				if err != nil {
					t.Fatal(err)
				}
				if next != f {
					t.Fatal("copy replaced the workbook")
				}
			}
			for _, c := range []struct {
				address string
				kind    excelize.CellType
				value   string
			}{
				{"E1", excelize.CellTypeError, "#N/A"}, {"F1", excelize.CellTypeDate, "2026-01-02T03:04:05Z"},
				{"G1", excelize.CellTypeError, "#N/A"}, {"H1", excelize.CellTypeDate, "2026-01-02T03:04:05Z"},
			} {
				kind, err := f.GetCellType("Sheet1", c.address)
				if err != nil || kind != c.kind {
					t.Fatalf("%s kind=%v err=%v", c.address, kind, err)
				}
				value, err := f.GetCellValue("Sheet1", c.address, excelize.Options{RawCellValue: true})
				if err != nil || value != c.value {
					t.Fatalf("%s value=%q err=%v", c.address, value, err)
				}
			}
			// Both source and destination checks must observe the newly copied array.
			for _, e := range []Edit{
				{Operation: "copy", Sheet: "Sheet1", Range: "J2", Target: "L1"},
				{Operation: "copy-values", Sheet: "Sheet1", Range: "A1", Target: "J2"},
			} {
				if _, err := copyRange(f, e); err == nil || !strings.Contains(err.Error(), "part of array formula") {
					t.Fatalf("partial copy: %v", err)
				}
			}
			// Clearing the master must be visible immediately, without a stale cache.
			if err := applyEdit(f, "", Edit{Operation: "clear-values", Sheet: "Sheet1", Range: "J1:J2"}); err != nil {
				t.Fatal(err)
			}
			if _, err := copyRange(f, Edit{Operation: "copy-values", Sheet: "Sheet1", Range: "A1", Target: "J2"}); err != nil {
				t.Fatal(err)
			}
			// New sheets resolve through live relationships, even after deletion/recreation.
			if _, err := f.NewSheet("Other"); err != nil {
				t.Fatal(err)
			}
			if err := f.DeleteSheet("Other"); err != nil {
				t.Fatal(err)
			}
			if _, err := f.NewSheet("Other"); err != nil {
				t.Fatal(err)
			}
			if _, err := copyRange(f, Edit{Operation: "copy", Sheet: "Sheet1", Range: "C1:C2", Name: "other", Target: "C1"}); err != nil {
				t.Fatal(err)
			}
			if _, err := copyRange(f, Edit{Operation: "copy-values", Sheet: "Sheet1", Range: "A1", Name: "Other", Target: "C2"}); err == nil || !strings.Contains(err.Error(), "part of array formula") {
				t.Fatalf("new sheet partial copy: %v", err)
			}
		})
	}
}
