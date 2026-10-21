package xlsx

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"github.com/xuri/excelize/v2"
)

func TestCopyMergedErrorValues(t *testing.T) {
	for _, tc := range []struct {
		name, operation, source, target string
		mergeSource                     bool
		errors                          map[string]string
		trailingValue                   any
		trailingFormula                 string
		want                            string
		wantType                        excelize.CellType
	}{
		{name: "matching merged values", operation: "copy-values", source: "A1:B1", target: "C1", mergeSource: true, errors: map[string]string{"A1": "#N/A"}, want: "#N/A", wantType: excelize.CellTypeError},
		{name: "matching merged copy", operation: "copy", source: "A1:B1", target: "C1", mergeSource: true, errors: map[string]string{"A1": "#N/A"}, want: "#N/A", wantType: excelize.CellTypeError},
		{name: "matching merged formulas", operation: "copy-formulas", source: "A1:B1", target: "C1", mergeSource: true, errors: map[string]string{"A1": "#N/A"}, want: "#N/A", wantType: excelize.CellTypeError},
		{name: "interior destination", operation: "copy-values", source: "A1", target: "D1", errors: map[string]string{"A1": "#N/A"}, want: "#N/A", wantType: excelize.CellTypeError},
		{name: "later text replaces error", operation: "copy-values", source: "A1:B1", target: "C1", errors: map[string]string{"A1": "#N/A"}, trailingValue: "recovered", want: "recovered", wantType: excelize.CellTypeSharedString},
		{name: "later blank clears error", operation: "copy-values", source: "A1:B1", target: "C1", errors: map[string]string{"A1": "#N/A"}, want: "", wantType: excelize.CellTypeUnset},
		{name: "later error wins", operation: "copy-values", source: "A1:B1", target: "C1", errors: map[string]string{"A1": "#N/A", "B1": "#DIV/0!"}, want: "#DIV/0!", wantType: excelize.CellTypeError},
		{name: "later formula replaces error", operation: "copy", source: "A1:B1", target: "C1", errors: map[string]string{"A1": "#N/A"}, trailingFormula: "1+1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			in, out := filepath.Join(dir, "in.xlsx"), filepath.Join(dir, "out.xlsx")
			f := excelize.NewFile()
			defer f.Close()
			if err := f.MergeCell("Sheet1", "C1", "D1"); err != nil {
				t.Fatal(err)
			}
			if tc.mergeSource {
				if err := f.MergeCell("Sheet1", "A1", "B1"); err != nil {
					t.Fatal(err)
				}
			}
			if tc.trailingValue != nil {
				if err := f.SetCellValue("Sheet1", "B1", tc.trailingValue); err != nil {
					t.Fatal(err)
				}
			}
			if tc.trailingFormula != "" {
				if err := f.SetCellFormula("Sheet1", "B1", tc.trailingFormula); err != nil {
					t.Fatal(err)
				}
			}
			for address, value := range tc.errors {
				if err := f.SetCellStr("Sheet1", address, value); err != nil {
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
			for address, value := range tc.errors {
				c := sh.Cells[address]
				c.CreateAttr("t", "e")
				office.RemoveAll(c, office.NSS, "v")
				office.NewChild(c, office.NSS, "v", nil).SetText(value)
			}
			if err := pkg.SetXML(sh.Part, sh.Doc); err != nil {
				t.Fatal(err)
			}
			if err := pkg.SaveNew(in); err != nil {
				t.Fatal(err)
			}
			if err := EditFile(context.Background(), in, out, dir, EditSpec{Edits: []Edit{{Operation: tc.operation, Sheet: "Sheet1", Range: tc.source, Target: tc.target}}}); err != nil {
				t.Fatal(err)
			}
			got, err := excelize.OpenFile(out)
			if err != nil {
				t.Fatal(err)
			}
			defer got.Close()
			if tc.trailingFormula != "" {
				formula, err := got.GetCellFormula("Sheet1", "C1")
				if err != nil || formula != tc.trailingFormula {
					t.Fatalf("formula=%q err=%v", formula, err)
				}
				kind, err := got.GetCellType("Sheet1", "C1")
				if err != nil || kind == excelize.CellTypeError {
					t.Fatalf("formula retained error type: %v %v", kind, err)
				}
			} else {
				for _, address := range []string{"C1", "D1"} {
					kind, err := got.GetCellType("Sheet1", address)
					if err != nil || kind != tc.wantType {
						t.Fatalf("%s type=%v err=%v want=%v", address, kind, err, tc.wantType)
					}
					value, err := got.GetCellValue("Sheet1", address)
					if err != nil || value != tc.want {
						t.Fatalf("%s value=%q err=%v want=%q", address, value, err, tc.want)
					}
				}
			}
			merges, err := got.GetMergeCells("Sheet1", true)
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, merge := range merges {
				if merge.GetStartAxis() == "C1" && merge.GetEndAxis() == "D1" {
					found = true
				}
			}
			if !found {
				t.Fatal("destination merge lost")
			}
		})
	}
}
