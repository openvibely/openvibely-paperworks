package office

import (
	"crypto/sha256"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"github.com/xuri/excelize/v2"
)

// Matches SpreadsheetThreadsTrace.setUp, including its formula graph and legacy note.
func referenceTraceFixture(t *testing.T, change func(*excelize.File)) (*opc.Package, string) {
	t.Helper()
	return traceFixture(t, func(f *excelize.File) {
		for cell, formula := range map[string]string{"A1": "Inputs!$A$1+'O''Brien'!A1", "A2": "A1*2", "B1": "Driver+A1"} {
			if e := f.SetCellFormula("Output", cell, formula); e != nil {
				t.Fatal(e)
			}
		}
		if e := f.AddComment("Inputs", excelize.Comment{Cell: "A1", Author: "Original", Text: "Keep this note"}); e != nil {
			t.Fatal(e)
		}
		if change != nil {
			change(f)
		}
	})
}
func referenceTraceNodeSet(t *testing.T, r *TraceReport, want ...string) {
	t.Helper()
	if len(r.Nodes) != len(want) {
		t.Fatalf("nodes: %v; want %v", r.Nodes, want)
	}
	for _, n := range want {
		hasTraceNode(t, r, n)
	}
}
func referenceTraceEdge(t *testing.T, r *TraceReport, from, to string) {
	t.Helper()
	for _, e := range r.Edges {
		if e == [2]string{from, to} {
			return
		}
	}
	t.Fatalf("missing edge %s -> %s: %v", from, to, r.Edges)
}

func TestReferenceTraceCrossSheetRangesAndReadOnly(t *testing.T) {
	_, path := referenceTraceFixture(t, nil)
	before := sha256.Sum256(previewRead(t, path))
	r := referenceTraceSavedRun(t, path, "Output", "A3", 200, 20)
	if !r.Complete {
		t.Fatal(r)
	}
	if len(r.Cycles) != 0 {
		t.Fatal(r.Cycles)
	}
	referenceTraceNodeSet(t, r, "'Output'!A3", "'Output'!A1", "'Output'!A2", "'Inputs'!A1", "'O''Brien'!A1")
	referenceTraceEdge(t, r, "'Output'!A1", "'Inputs'!A1")
	referenceTraceEdge(t, r, "'Output'!A2", "'Output'!A1")
	if r.Nodes["'Inputs'!A1"].CachedValue != float64(7) {
		t.Fatal("input cache", r.Nodes)
	}
	referenceTraceEdge(t, referenceTraceSavedRun(t, path, "Output", "B1", 200, 20), "'Output'!B1", "'Inputs'!A1")
	if sha256.Sum256(previewRead(t, path)) != before {
		t.Fatal("source hash changed")
	}
}
func TestReferenceTraceCaseAndRelativeName(t *testing.T) {
	path := referenceTraceSavedFixture(t, func(f *excelize.File) {
		if e := f.SetCellFormula("Output", "D1", "inputs!a1+driver"); e != nil {
			t.Fatal(e)
		}
		if e := f.SetDefinedName(&excelize.DefinedName{Name: "Relative", RefersTo: "Inputs!A1"}); e != nil {
			t.Fatal(e)
		}
		if e := f.SetCellFormula("Output", "D2", "Relative"); e != nil {
			t.Fatal(e)
		}
	})
	r := referenceTraceSavedRun(t, path, "output", "d1", 200, 20)
	if !r.Complete {
		t.Fatal(r)
	}
	if !reflect.DeepEqual(r.Edges, [][2]string{{"'Output'!D1", "'Inputs'!A1"}}) {
		t.Fatal(r.Edges)
	}
	r = referenceTraceSavedRun(t, path, "Output", "D2", 200, 20)
	if r.Complete {
		t.Fatal(r)
	}
	if len(r.Unresolved) == 0 || !strings.Contains(r.Unresolved[0].Reason, "Relative defined-name") {
		t.Fatal(r.Unresolved)
	}
}
func TestReferenceTraceCyclesAndBudgets(t *testing.T) {
	path := referenceTraceSavedFixture(t, func(f *excelize.File) {
		for _, c := range []struct{ cell, formula string }{{"C1", "C2"}, {"C2", "C1"}, {"D1", "SUM(A1:A10000)"}} {
			if e := f.SetCellFormula("Output", c.cell, c.formula); e != nil {
				t.Fatal(e)
			}
		}
	})
	r := referenceTraceSavedRun(t, path, "Output", "C1", 200, 20)
	if !reflect.DeepEqual(r.Cycles, []string{"'Output'!C1"}) {
		t.Fatal(r.Cycles)
	}
	for _, c := range []struct {
		root         string
		nodes, depth int
	}{{"A3", 2, 20}, {"A3", 200, 1}, {"D1", 10, 20}} {
		r := referenceTraceSavedRun(t, path, "Output", c.root, c.nodes, c.depth)
		if !r.Truncated {
			t.Fatal(r)
		}
		if r.Complete {
			t.Fatal(r)
		}
		if len(r.Nodes) > c.nodes {
			t.Fatalf("nodes: %d; budget: %d", len(r.Nodes), c.nodes)
		}
	}
}

// Preserve Reference's setup save, load_workbook/edit/save, then fresh read per query.
// Each query opens the saved package independently.
func referenceTraceSavedFixture(t *testing.T, change func(*excelize.File)) string {
	t.Helper()
	_, path := referenceTraceFixture(t, nil)
	f, err := excelize.OpenFile(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })
	change(f)
	if err := f.Save(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

func referenceTraceSavedRun(t *testing.T, path, sheet, cell string, nodes, depth int) *TraceReport {
	t.Helper()
	p, err := opc.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	return traceRun(t, p, sheet, cell, nodes, depth)
}

func TestReferenceTraceQualifiedNames(t *testing.T) {
	cases := []struct{ cell, formula, target string }{{"C1", "Inputs!Rate*2", "'Inputs'!A2"}, {"C2", "'O''Brien'!Rate", "'O''Brien'!A1"}, {"C3", "'iNpUtS'!rAtE", "'Inputs'!A2"}, {"C4", "Inputs!Alias", "'Inputs'!A2"}, {"C5", "Rate", "'Output'!D1"}, {"C6", "Inputs!Driver", "'Inputs'!A1"}}
	path := referenceTraceSavedFixture(t, func(f *excelize.File) {
		for _, n := range []excelize.DefinedName{{Name: "Rate", RefersTo: "Inputs!$A$1"}, {Name: "Rate", Scope: "Output", RefersTo: "Output!$D$1"}, {Name: "Rate", Scope: "Inputs", RefersTo: "Inputs!$A$2"}, {Name: "Rate", Scope: "O'Brien", RefersTo: "'O''Brien'!$A$1"}, {Name: "Alias", Scope: "Inputs", RefersTo: "Rate"}} {
			if e := f.SetDefinedName(&n); e != nil {
				t.Fatal(e)
			}
		}
		if e := f.SetCellValue("Output", "D1", 99); e != nil {
			t.Fatal(e)
		}
		if e := f.SetCellValue("Inputs", "A2", 20); e != nil {
			t.Fatal(e)
		}
		for _, c := range cases {
			if e := f.SetCellFormula("Output", c.cell, c.formula); e != nil {
				t.Fatal(e)
			}
		}
	})
	before := previewRead(t, path)
	for _, c := range cases {
		t.Run(c.cell, func(t *testing.T) {
			r := referenceTraceSavedRun(t, path, "Output", c.cell, 200, 20)
			if !r.Complete {
				t.Fatal(r)
			}
			if !reflect.DeepEqual(r.Edges, [][2]string{{"'Output'!" + c.cell, c.target}}) {
				t.Fatal(r.Edges)
			}
			referenceTraceNodeSet(t, r, "'Output'!"+c.cell, c.target)
		})
	}
	previewUnchanged(t, path, before)
}
func TestReferenceTraceQualifiedNameFailures(t *testing.T) {
	path := referenceTraceSavedFixture(t, func(f *excelize.File) {
		for _, n := range []excelize.DefinedName{{Name: "Loop", Scope: "Inputs", RefersTo: "'O''Brien'!Loop"}, {Name: "Loop", Scope: "O'Brien", RefersTo: "Inputs!Loop"}} {
			if e := f.SetDefinedName(&n); e != nil {
				t.Fatal(e)
			}
		}
		for i, formula := range []string{"Inputs!Loop", "Missing!Driver", "'[other.xlsx]Inputs'!Driver", "Inputs!Absent"} {
			if e := f.SetCellFormula("Output", fmt.Sprintf("C%d", i+1), formula); e != nil {
				t.Fatal(e)
			}
		}
		for i := 0; i < 6; i++ {
			ref := fmt.Sprintf("Chain%d", i+1)
			if i == 5 {
				ref = "Inputs!$A$1"
			}
			if e := f.SetDefinedName(&excelize.DefinedName{Name: fmt.Sprintf("Chain%d", i), Scope: "Inputs", RefersTo: ref}); e != nil {
				t.Fatal(e)
			}
		}
		if e := f.SetCellFormula("Output", "C5", "Inputs!Chain0"); e != nil {
			t.Fatal(e)
		}
	})
	for i := 1; i <= 4; i++ {
		r := referenceTraceSavedRun(t, path, "Output", fmt.Sprintf("C%d", i), 200, 20)
		if r.Complete {
			t.Fatal("expected incomplete report", r)
		}
		if len(r.Unresolved) == 0 {
			t.Fatal("expected unresolved reference", r)
		}
		if len(r.Edges) != 0 {
			t.Fatal("expected empty edges", r.Edges)
		}
		if i == 1 && (len(r.Unresolved) == 0 || !strings.Contains(r.Unresolved[0].Reason, "Cyclic")) {
			t.Fatal(r)
		}
	}
	r := referenceTraceSavedRun(t, path, "Output", "C5", 200, 3)
	if !r.Truncated {
		t.Fatal("expected truncated report", r)
	}
	if r.Complete {
		t.Fatal("expected incomplete report", r)
	}
	if len(r.Edges) != 0 {
		t.Fatal("expected empty edges", r.Edges)
	}
	r = referenceTraceSavedRun(t, path, "Output", "C5", 200, 20)
	if !r.Complete {
		t.Fatal("expected complete report", r)
	}
	if !reflect.DeepEqual(r.Edges, [][2]string{{"'Output'!C5", "'Inputs'!A1"}}) {
		t.Fatal(r.Edges)
	}
}
