package office

import (
	"bytes"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
)

func TestSharedArrayConstants(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{`SUM({1,2;3,4}*A1:B2)`, `SUM({1,2;3,4}*B2:C3)`},
		{`IF(A1,{"A1","say ""Hi""";TRUE,#N/A},{-1,+2;3E-2,4})`, `IF(B2,{"A1","say ""Hi""";TRUE,#N/A},{-1,2;3E-2,4})`},
		{`SUM({1;2;3})+'O''Brien'!$A1`, `SUM({1;2;3})+'O''Brien'!$A2`},
	} {
		got, err := translateSharedFormula(c.in, 1, 1)
		if err != nil || got != c.want {
			t.Errorf("%s => %q (%v), want %q", c.in, got, err, c.want)
		}
	}
	p, file := traceFixture(t, func(f *excelize.File) {
		typ, area := "shared", "C5:D6"
		if err := f.SetCellFormula("Output", "C5", `SUM({1,2;3,4}*Inputs!A1:B2)+IF(A1="A1",1,0)`, excelize.FormulaOpts{Type: &typ, Ref: &area}); err != nil {
			t.Fatal(err)
		}
	})
	original, _ := os.ReadFile(file)
	parts := map[string][]byte{}
	for _, v := range p.Parts {
		parts[v.Name] = bytes.Clone(v.Data)
	}
	r := traceRun(t, p, "Output", "D6", 200, 20)
	if !r.Complete {
		t.Fatal(r.Unresolved)
	}
	want := `=SUM({1,2;3,4}*'Inputs'!B2:C3)+IF(B2="A1",1,0)`
	if f := r.Nodes["'Output'!D6"].Formula; f == nil || *f != want {
		t.Fatalf("formula %v", f)
	}
	expected := map[string]bool{"'Output'!D6": true, "'Inputs'!B2": true, "'Inputs'!C2": true, "'Inputs'!B3": true, "'Inputs'!C3": true, "'Output'!B2": true}
	if len(r.Nodes) != len(expected) {
		t.Fatal(r.Nodes)
	}
	for k := range r.Nodes {
		if !expected[k] {
			t.Fatal("invented dependency", k)
		}
	}
	after, _ := os.ReadFile(file)
	if !bytes.Equal(original, after) {
		t.Fatal("changed source")
	}
	for _, v := range p.Parts {
		if !bytes.Equal(parts[v.Name], v.Data) {
			t.Fatal("changed part", v.Name)
		}
	}
}
func TestInvalidArrayConstants(t *testing.T) {
	for _, s := range []string{`{1,2;3}`, `{A1,2}`, `{SUM(1),2}`, `{{1,2}}`, `{1,,2}`, `{}`, `{-TRUE}`, `{1+2,3}`, `{1,2`, "{" + strings.Repeat("1,", 2100) + "1}"} {
		if _, err := tokenizeTrace(s); err == nil {
			t.Errorf("accepted %s", s)
		}
	}
}

// Port the exact assertions in the reference tests, including duplicate edge
// removal and the node budget/depth combinations (not only similar examples).
func TestTraceReferenceExactCoreAssertions(t *testing.T) {
	p, path := traceFixture(t, func(f *excelize.File) {
		f.SetCellFormula("Output", "D1", "inputs!a1+driver")
		f.SetDefinedName(&excelize.DefinedName{Name: "Relative", RefersTo: "Inputs!A1"})
		f.SetCellFormula("Output", "D2", "Relative")
		f.SetCellFormula("Output", "C1", "C2")
		f.SetCellFormula("Output", "C2", "C1")
		f.SetCellFormula("Output", "E1", "SUM(A1:A10000)")
	})
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		after, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(before, after) {
			t.Fatal("reference trace mutated source", err)
		}
	})
	r := traceRun(t, p, "Output", "A3", 200, 20)
	if r.Nodes["'Inputs'!A1"].CachedValue != float64(7) {
		t.Fatal("reference cached input differs", r.Nodes)
	}
	if !r.Complete || len(r.Cycles) != 0 || len(r.Nodes) != 5 {
		t.Fatal(r)
	}
	for _, key := range []string{"'Output'!A3", "'Output'!A1", "'Output'!A2", "'Inputs'!A1", "'O''Brien'!A1"} {
		hasTraceNode(t, r, key)
	}
	hasEdge := func(r *TraceReport, a, b string) {
		t.Helper()
		for _, e := range r.Edges {
			if e == [2]string{a, b} {
				return
			}
		}
		t.Fatalf("missing edge %s -> %s: %v", a, b, r.Edges)
	}
	hasEdge(r, "'Output'!A1", "'Inputs'!A1")
	hasEdge(r, "'Output'!A2", "'Output'!A1")
	hasEdge(traceRun(t, p, "Output", "B1", 200, 20), "'Output'!B1", "'Inputs'!A1")
	r = traceRun(t, p, "output", "d1", 200, 20)
	if !r.Complete || !reflect.DeepEqual(r.Edges, [][2]string{{"'Output'!D1", "'Inputs'!A1"}}) {
		t.Fatal(r)
	}
	r = traceRun(t, p, "Output", "D2", 200, 20)
	if r.Complete || len(r.Unresolved) == 0 || !strings.Contains(r.Unresolved[0].Reason, "Relative defined-name") {
		t.Fatal(r)
	}
	r = traceRun(t, p, "Output", "C1", 200, 20)
	if !reflect.DeepEqual(r.Cycles, []string{"'Output'!C1"}) {
		t.Fatal(r)
	}
	for _, c := range []struct {
		cell         string
		nodes, depth int
	}{{"A3", 2, 20}, {"A3", 200, 1}, {"E1", 10, 20}} {
		r = traceRun(t, p, "Output", c.cell, c.nodes, c.depth)
		if !r.Truncated || r.Complete || len(r.Nodes) > c.nodes {
			t.Fatal(r)
		}
	}
}
