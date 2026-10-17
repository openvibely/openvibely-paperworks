package office

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"github.com/xuri/excelize/v2"
)

func traceFixture(t *testing.T, change func(*excelize.File)) (*opc.Package, string) {
	t.Helper()
	f := excelize.NewFile()
	defer f.Close()
	f.SetSheetName("Sheet1", "Inputs")
	f.NewSheet("O'Brien")
	f.NewSheet("Output")
	f.SetCellValue("Inputs", "A1", 7)
	f.SetCellValue("O'Brien", "A1", 3)
	for c, v := range map[string]string{"A1": "Inputs!A1*2", "A2": "A1+'O''Brien'!A1", "A3": "SUM(A1:A2)", "B1": "Driver"} {
		if e := f.SetCellFormula("Output", c, v); e != nil {
			t.Fatal(e)
		}
	}
	if e := f.SetDefinedName(&excelize.DefinedName{Name: "Driver", RefersTo: "Inputs!$A$1"}); e != nil {
		t.Fatal(e)
	}
	if change != nil {
		change(f)
	}
	path := filepath.Join(t.TempDir(), "trace.xlsx")
	if e := f.SaveAs(path); e != nil {
		t.Fatal(e)
	}
	p, e := opc.Open(path)
	if e != nil {
		t.Fatal(e)
	}
	return p, path
}
func traceRun(t *testing.T, p *opc.Package, sheet, cell string, nodes, depth int) *TraceReport {
	t.Helper()
	r, e := TraceWorkbook(context.Background(), p, sheet, cell, nodes, depth)
	if e != nil {
		t.Fatal(e)
	}
	return r
}
func hasTraceNode(t *testing.T, r *TraceReport, label string) {
	t.Helper()
	if _, ok := r.Nodes[label]; !ok {
		t.Fatalf("missing %s: %+v", label, r)
	}
}
func TestTraceCrossSheetNamesReadOnly(t *testing.T) {
	p, path := traceFixture(t, nil)
	before, _ := os.ReadFile(path)
	parts := map[string][]byte{}
	for _, v := range p.Parts {
		parts[v.Name] = append([]byte{}, v.Data...)
	}
	r := traceRun(t, p, "output", "a3", 200, 20)
	if !r.Complete || len(r.Nodes) != 5 || len(r.Cycles) != 0 {
		t.Fatalf("%+v", r)
	}
	if r.Nodes["'Inputs'!A1"].CachedValue != float64(7) {
		t.Fatal(r.Nodes)
	}
	hasTraceNode(t, traceRun(t, p, "Output", "B1", 200, 20), "'Inputs'!A1")
	after, _ := os.ReadFile(path)
	if !bytes.Equal(before, after) {
		t.Fatal("source mutated")
	}
	for _, v := range p.Parts {
		if !bytes.Equal(parts[v.Name], v.Data) {
			t.Fatal("part mutated", v.Name)
		}
	}
}
func TestTraceNamesCyclesLimits(t *testing.T) {
	p, _ := traceFixture(t, func(f *excelize.File) {
		for _, n := range []excelize.DefinedName{{Name: "Driver", Scope: "Output", RefersTo: "'O''Brien'!$A$1"}, {Name: "Relative", RefersTo: "Inputs!A1"}, {Name: "Loop", RefersTo: "Loop"}, {Name: "Alias", Scope: "Inputs", RefersTo: "Driver"}, {Name: "Driver", Scope: "Inputs", RefersTo: "Inputs!$A$1"}} {
			if e := f.SetDefinedName(&n); e != nil {
				t.Fatal(e)
			}
		}
		for c, v := range map[string]string{"C1": "C2", "C2": "C1", "C3": "Loop", "C4": "#REF!+A1", "D1": "SUM(A1:A10000)", "D2": "Relative", "D3": "inputs!Alias", "D4": "Missing!Alias"} {
			f.SetCellFormula("Output", c, v)
		}
	})
	hasTraceNode(t, traceRun(t, p, "Output", "B1", 200, 20), "'O''Brien'!A1")
	r := traceRun(t, p, "Output", "C1", 200, 20)
	if len(r.Cycles) != 1 || r.Cycles[0] != "'Output'!C1" {
		t.Fatal(r)
	}
	for _, c := range []string{"C3", "C4", "D2", "D4"} {
		r = traceRun(t, p, "Output", c, 200, 20)
		if r.Complete || len(r.Unresolved) == 0 {
			t.Fatal(c, r)
		}
	}
	r = traceRun(t, p, "Output", "D3", 200, 20)
	if !r.Complete {
		t.Fatal(r)
	}
	hasTraceNode(t, r, "'Inputs'!A1")
	for _, v := range []struct {
		cell string
		n, d int
	}{{"A3", 2, 20}, {"A3", 200, 1}, {"D1", 10, 20}} {
		r = traceRun(t, p, "Output", v.cell, v.n, v.d)
		if !r.Truncated || r.Complete || len(r.Nodes) > v.n {
			t.Fatal(r)
		}
	}
}
func TestTraceDynamicInputsAndUncertainty(t *testing.T) {
	p, _ := traceFixture(t, func(f *excelize.File) {
		f.SetCellValue("Output", "E1", "Inputs!")
		f.SetCellValue("Output", "E2", "A1")
		f.SetCellValue("Output", "E3", 1)
		f.SetCellFormula("Output", "E4", `"Inputs!A1"`)
		f.SetCellValue("Output", "E9", 8)
		f.SetCellValue("Inputs", "A2", 12)
		f.SetCellValue("Inputs", "A3", 13)
		for c, v := range map[string]string{"F1": "INDIRECT(E1&E2)", "F2": "SUM(OFFSET(Inputs!A1,E3,0,2,1))", "F3": "INDIRECT(E4)", "F4": "OFFSET(A1,-1,0)", "F5": `INDIRECT("R1C1",FALSE)`, "F10": `INDIRECT("R[-1]C[-1]",FALSE)`, "F11": "OFFSET(A1+A2,1,0)", "F12": `INDIRECT("R[-99]C",FALSE)`, "F6": `INDIRECT("Inputs!A1")+OFFSET(A1,0,0)`} {
			f.SetCellFormula("Output", c, v)
		}
	})
	for _, c := range []string{"F1", "F2", "F5", "F6", "F10"} {
		r := traceRun(t, p, "Output", c, 200, 20)
		if !r.Complete {
			t.Fatalf("%s: %+v", c, r)
		}
	}
	r := traceRun(t, p, "Output", "F1", 200, 20)
	hasTraceNode(t, r, "'Inputs'!A1")
	hasTraceNode(t, r, "'Output'!E2")
	r = traceRun(t, p, "Output", "F2", 200, 20)
	hasTraceNode(t, r, "'Inputs'!A2")
	hasTraceNode(t, r, "'Inputs'!A3")
	r = traceRun(t, p, "Output", "F10", 200, 20)
	hasTraceNode(t, r, "'Output'!E9")
	if len(r.Nodes) != 2 {
		t.Fatal(r)
	}
	for _, c := range []string{"F3", "F4", "F11", "F12"} {
		r = traceRun(t, p, "Output", c, 200, 20)
		if r.Complete || len(r.Unresolved) == 0 {
			t.Fatal(c, r)
		}
	}
	r = traceRun(t, p, "Output", "F3", 200, 20)
	if !strings.Contains(r.Unresolved[0].Reason, "Formula-valued") {
		t.Fatal(r)
	}
}
func TestTraceWholeRangesAnd3D(t *testing.T) {
	p, _ := traceFixture(t, func(f *excelize.File) {
		f.SetCellValue("Inputs", "B3", 10)
		f.SetCellValue("Inputs", "B5", 20)
		f.SetCellValue("Inputs", "Z99", 99)
		for c, v := range map[string]string{"F1": "SUM(Inputs!B:B)", "F2": "SUM(Inputs!3:3)", "F3": "SUM(Inputs:Output!A1)", "F4": "SUM(Inputs!XFE:XFE)"} {
			f.SetCellFormula("Output", c, v)
		}
	})
	r := traceRun(t, p, "Output", "F1", 200, 20)
	if !r.Complete || len(r.Nodes) != 4 {
		t.Fatal(r)
	}
	hasTraceNode(t, r, "'Inputs'!B4")
	r = traceRun(t, p, "Output", "F2", 200, 20)
	if !r.Complete || len(r.Nodes) != 2 {
		t.Fatal(r)
	}
	r = traceRun(t, p, "Output", "F3", 200, 20)
	if !r.Complete {
		t.Fatal(r)
	}
	hasTraceNode(t, r, "'O''Brien'!A1")
	if traceRun(t, p, "Output", "F4", 200, 20).Complete {
		t.Fatal("invalid column accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e := TraceWorkbook(ctx, p, "Output", "A1", 200, 20); e != context.Canceled {
		t.Fatal(e)
	}
	for _, cell := range []string{"A0", "XFE1", "A1048577", "A1:A2"} {
		if _, e := TraceWorkbook(context.Background(), p, "Output", cell, 200, 20); e == nil {
			t.Fatal(cell)
		}
	}
}

func TestTraceStructuredReferences(t *testing.T) {
	p, _ := traceFixture(t, func(f *excelize.File) {
		for c, v := range map[string]any{"A2": "Item", "B2": "Amount", "A3": "A", "B3": 11, "A4": "B", "B4": 22} {
			f.SetCellValue("Inputs", c, v)
		}
		if e := f.AddTable("Inputs", &excelize.Table{Range: "A2:B4", Name: "Sales"}); e != nil {
			t.Fatal(e)
		}
		for c, v := range map[string]string{"F1": "SUM(Sales[Amount])", "F2": "SUM(Inputs:Output!A1)", "F3": "Sales[[#Headers],[Amount]]", "F4": "Sales[@Amount]", "F5": "SUM(Sales[[Item]:[Amount]])", "F6": "SUM(Sales[#Totals])"} {
			f.SetCellFormula("Output", c, v)
		}
		f.SetCellFormula("Inputs", "D3", "Sales[@Amount]")
	})
	r := traceRun(t, p, "Output", "F1", 200, 20)
	if !r.Complete || len(r.Nodes) != 3 {
		t.Fatal(r)
	}
	hasTraceNode(t, r, "'Inputs'!B3")
	hasTraceNode(t, r, "'Inputs'!B4")
	r = traceRun(t, p, "Output", "F3", 200, 20)
	if !r.Complete {
		t.Fatal(r)
	}
	hasTraceNode(t, r, "'Inputs'!B2")
	if traceRun(t, p, "Output", "F4", 200, 20).Complete {
		t.Fatal("accepted out-of-table row")
	}
	r = traceRun(t, p, "Inputs", "D3", 200, 20)
	if !r.Complete {
		t.Fatal(r)
	}
	hasTraceNode(t, r, "'Inputs'!B3")
	r = traceRun(t, p, "Output", "F5", 200, 20)
	if !r.Complete || len(r.Nodes) != 5 {
		t.Fatal(r)
	}
	r = traceRun(t, p, "Output", "F6", 200, 20)
	if !r.Complete || len(r.Nodes) != 1 {
		t.Fatal(r)
	}
}

func TestTraceQualifiedNameCorpus(t *testing.T) {
	cases := map[string]struct{ formula, target string }{"C1": {"Inputs!Rate*2", "'Inputs'!A2"}, "C2": {"'O''Brien'!Rate", "'O''Brien'!A1"}, "C3": {"'iNpUtS'!rAtE", "'Inputs'!A2"}, "C4": {"Inputs!Alias", "'Inputs'!A2"}, "C5": {"Rate", "'Output'!D1"}, "C6": {"Inputs!Driver", "'Inputs'!A1"}}
	p, source := traceFixture(t, func(f *excelize.File) {
		f.SetCellValue("Output", "D1", 99)
		f.SetCellValue("Inputs", "A2", 20)
		for _, n := range []excelize.DefinedName{{Name: "Rate", RefersTo: "Inputs!$A$1"}, {Name: "Rate", Scope: "Output", RefersTo: "Output!$D$1"}, {Name: "Rate", Scope: "Inputs", RefersTo: "Inputs!$A$2"}, {Name: "Rate", Scope: "O'Brien", RefersTo: "'O''Brien'!$A$1"}, {Name: "Alias", Scope: "Inputs", RefersTo: "Rate"}, {Name: "Loop", Scope: "Inputs", RefersTo: "'O''Brien'!Loop"}, {Name: "Loop", Scope: "O'Brien", RefersTo: "Inputs!Loop"}} {
			if e := f.SetDefinedName(&n); e != nil {
				t.Fatal(e)
			}
		}
		for c, v := range cases {
			f.SetCellFormula("Output", c, v.formula)
		}
		f.SetCellFormula("Output", "E1", "Inputs!Loop")
		f.SetCellFormula("Output", "E3", "Missing!Driver")
		f.SetCellFormula("Output", "E4", "'[other.xlsx]Inputs'!Driver")
		f.SetCellFormula("Output", "E5", "Inputs!Absent")
		for i := 0; i < 6; i++ {
			ref := fmt.Sprintf("Chain%d", i+1)
			if i == 5 {
				ref = "Inputs!$A$1"
			}
			if e := f.SetDefinedName(&excelize.DefinedName{Name: fmt.Sprintf("Chain%d", i), Scope: "Inputs", RefersTo: ref}); e != nil {
				t.Fatal(e)
			}
		}
		f.SetCellFormula("Output", "E2", "Inputs!Chain0")
	})
	before, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	for c, v := range cases {
		r := traceRun(t, p, "Output", c, 200, 20)
		if !r.Complete || len(r.Nodes) != 2 || !reflect.DeepEqual(r.Edges, [][2]string{{traceLabel("Output", c), v.target}}) {
			t.Fatalf("%s: %+v", c, r)
		}
		hasTraceNode(t, r, traceLabel("Output", c))
		hasTraceNode(t, r, v.target)
	}
	r := traceRun(t, p, "Output", "E1", 200, 20)
	if r.Complete || len(r.Edges) != 0 || !strings.Contains(r.Unresolved[0].Reason, "Cyclic") {
		t.Fatal(r)
	}
	r = traceRun(t, p, "Output", "E2", 200, 3)
	if !r.Truncated || r.Complete || len(r.Edges) != 0 {
		t.Fatal(r)
	}
	r = traceRun(t, p, "Output", "E2", 200, 20)
	if !r.Complete || !reflect.DeepEqual(r.Edges, [][2]string{{"'Output'!E2", "'Inputs'!A1"}}) {
		t.Fatal(r)
	}
	for _, cell := range []string{"E3", "E4", "E5"} {
		report := traceRun(t, p, "Output", cell, 200, 20)
		if report.Complete || len(report.Unresolved) == 0 || len(report.Edges) != 0 {
			t.Fatal(cell, report)
		}
	}
	after, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("qualified-name tracing modified source")
	}

}
func TestTraceMalformedAndExternal(t *testing.T) {
	p, _ := traceFixture(t, func(f *excelize.File) {
		for c, v := range map[string]string{"E1": "SUM(A1", "E2": "'Missing!A1", "E3": "'Missing'!A1", "E4": "'[other.xlsx]Sheet1'!A1+Table1[Amount]"} {
			f.SetCellFormula("Output", c, v)
		}
	})
	for _, c := range []string{"E1", "E2", "E3", "E4"} {
		r := traceRun(t, p, "Output", c, 200, 20)
		if r.Complete || len(r.Unresolved) == 0 {
			t.Fatal(c, r)
		}
	}
}
