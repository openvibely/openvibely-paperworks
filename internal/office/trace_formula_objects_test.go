package office

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"github.com/xuri/excelize/v2"
)

// Expected results were checked against document_native.trace_workbook with
// openpyxl 3.1.5. Tests require neither Reference nor the reference worktree.
func TestTraceReferenceFormulaObjectsAndColonRange(t *testing.T) {
	for _, tc := range []struct {
		name, kind, text, cache string
		refs                    []string
		reason, unresolved      string
	}{
		{"chain", "", `SUM(A1:B2:C3)+D1`, "", []string{"D1"}, "Unresolved name, structured, external, or spill reference", "A1:B2:C3"},
		{"array", "array", `SUM(A1:A2)+D1`, "", []string{"A1", "A2", "D1"}, "", ""},
		{"data table", "dataTable", "", "", nil, "Unsupported formula object", ""},
		{"data table stale cache", "dataTable", "", "99", nil, "Unsupported formula object", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, _ := traceFixture(t, func(f *excelize.File) {
				for cell, v := range map[string]int{"A1": 7, "A2": 11, "D1": 17} {
					if err := f.SetCellValue("Inputs", cell, v); err != nil {
						t.Fatal(err)
					}
				}
				if err := f.SetCellFormula("Inputs", "F1", "A1"); err != nil {
					t.Fatal(err)
				}
			})
			wb, err := OpenWorkbook(p)
			if err != nil {
				t.Fatal(err)
			}
			sheet := wb.Sheets["Inputs"]
			cell := sheet.Cells["F1"]
			cell.RemoveAttr("t")
			f := Child(cell, NSS, "f")
			f.SetText(tc.text)
			if tc.kind != "" {
				f.CreateAttr("t", tc.kind)
				f.CreateAttr("ref", "F1:G2")
			}
			if tc.kind == "dataTable" {
				f.CreateAttr("r1", "A1")
			}
			v := Child(cell, NSS, "v")
			if v == nil {
				v = cell.CreateElement("v")
			}
			v.SetText(tc.cache)
			if err = p.SetXML(sheet.Part, sheet.Doc); err != nil {
				t.Fatal(err)
			}
			file := filepath.Join(t.TempDir(), "objects.xlsx")
			if err = p.SaveNew(file); err != nil {
				t.Fatal(err)
			}
			p, err = opc.Open(file)
			if err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			parts := map[string][]byte{}
			for _, part := range p.Parts {
				parts[part.Name] = bytes.Clone(part.Data)
			}
			r := traceRun(t, p, "Inputs", "F1", 200, 20)
			root := "'Inputs'!F1"
			n := r.Nodes[root]
			if tc.kind == "dataTable" {
				if n.Formula != nil {
					t.Fatal("invented formula", *n.Formula)
				}
			} else if n.Formula == nil || *n.Formula != "="+tc.text {
				t.Fatal(n)
			}
			var wantCache any
			if tc.cache != "" {
				wantCache = float64(99)
			}
			if n.CachedValue != wantCache {
				t.Fatal("cache changed", n)
			}
			edges := [][2]string{}
			for _, c := range tc.refs {
				edges = append(edges, [2]string{root, traceLabel("Inputs", c)})
				want := map[string]float64{"A1": 7, "A2": 11, "D1": 17}[c]
				if r.Nodes[traceLabel("Inputs", c)].CachedValue != want {
					t.Fatal(r.Nodes)
				}
			}
			if len(r.Nodes) != 1+len(tc.refs) || !reflect.DeepEqual(r.Edges, edges) || r.Truncated || len(r.Cycles) != 0 {
				t.Fatal(r)
			}
			issues := []TraceIssue{}
			if tc.reason != "" {
				issues = append(issues, TraceIssue{Cell: root, Reference: tc.unresolved, Reason: tc.reason})
			}
			if !reflect.DeepEqual(r.Unresolved, issues) || r.Complete != (tc.reason == "") {
				t.Fatal(r)
			}
			after, err := os.ReadFile(file)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatal("source changed", err)
			}
			for _, part := range p.Parts {
				if !bytes.Equal(parts[part.Name], part.Data) {
					t.Fatal("part changed", part.Name)
				}
			}
		})
	}
}
