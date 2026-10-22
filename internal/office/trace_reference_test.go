package office

import (
	"context"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/beevik/etree"
	"github.com/xuri/excelize/v2"
)

// These assertions correspond to the original tests with the same suffix, not
// just analogous formula examples. The fixture has the reference's sheet order.
func TestTraceReferenceDynamicExternalStructuredAndCyclicNames(t *testing.T) {
	path := referenceTraceSavedFixture(t, func(f *excelize.File) {
		for _, c := range []struct{ cell, formula string }{{"C1", `INDIRECT("Inputs!A1")+OFFSET(A1,0,0)`}, {"C2", `'[other.xlsx]Sheet1'!A1+Table1[Amount]`}} {
			if err := f.SetCellFormula("Output", c.cell, c.formula); err != nil {
				t.Fatal(err)
			}
		}
		if err := f.SetDefinedName(&excelize.DefinedName{Name: "Loop", RefersTo: "Loop"}); err != nil {
			t.Fatal(err)
		}
		for _, c := range []struct{ cell, formula string }{{"C3", "Loop"}, {"C4", "#REF!+A1"}} {
			if err := f.SetCellFormula("Output", c.cell, c.formula); err != nil {
				t.Fatal(err)
			}
		}
		if err := f.SetDefinedName(&excelize.DefinedName{Name: "Driver", Scope: "Output", RefersTo: "'O''Brien'!A1"}); err != nil {
			t.Fatal(err)
		}
	})
	r := referenceTraceSavedRun(t, path, "Output", "C1", 200, 20)
	if !r.Complete {
		t.Fatal(r)
	}
	if len(r.Unresolved) != 0 {
		t.Fatal(r.Unresolved)
	}
	hasTraceNode(t, r, "'Inputs'!A1")
	hasTraceNode(t, r, "'Output'!A1")
	r = referenceTraceSavedRun(t, path, "Output", "C2", 200, 20)
	if r.Complete {
		t.Fatal(r)
	}
	if len(r.Unresolved) != 2 {
		t.Fatal(r.Unresolved)
	}
	if referenceTraceSavedRun(t, path, "Output", "C4", 200, 20).Complete {
		t.Fatal("lost error uncertainty")
	}
	r = referenceTraceSavedRun(t, path, "Output", "C3", 200, 20)
	if len(r.Unresolved) == 0 || !strings.Contains(r.Unresolved[0].Reason, "Cyclic") {
		t.Fatal(r)
	}
	referenceTraceEdge(t, referenceTraceSavedRun(t, path, "Output", "B1", 200, 20), "'Output'!B1", "'O''Brien'!A1")
}

func TestTraceReferenceDynamicInputsAndUncalculatedAddresses(t *testing.T) {
	path := referenceTraceSavedFixture(t, func(f *excelize.File) {
		for _, c := range []struct {
			cell  string
			value any
		}{{"E1", "Inputs!"}, {"E2", "A1"}, {"E3", 1}} {
			if err := f.SetCellValue("Output", c.cell, c.value); err != nil {
				t.Fatal(err)
			}
		}
		for _, c := range []struct{ cell, formula string }{{"F1", "INDIRECT(E1&E2)"}, {"F2", "SUM(OFFSET(Inputs!A1,E3,0,2,1))"}} {
			if err := f.SetCellFormula("Output", c.cell, c.formula); err != nil {
				t.Fatal(err)
			}
		}
		for _, c := range []struct {
			cell  string
			value int
		}{{"A2", 12}, {"A3", 13}} {
			if err := f.SetCellValue("Inputs", c.cell, c.value); err != nil {
				t.Fatal(err)
			}
		}
		for _, c := range []struct{ cell, formula string }{{"E4", `"Inputs!A1"`}, {"F3", "INDIRECT(E4)"}, {"F4", "OFFSET(A1,-1,0)"}, {"F5", `INDIRECT("R1C1",FALSE)`}} {
			if err := f.SetCellFormula("Output", c.cell, c.formula); err != nil {
				t.Fatal(err)
			}
		}
	})
	r := referenceTraceSavedRun(t, path, "Output", "F1", 200, 20)
	if !r.Complete {
		t.Fatal(r)
	}
	hasTraceNode(t, r, "'Inputs'!A1")
	hasTraceNode(t, r, "'Output'!E2")
	offset := referenceTraceSavedRun(t, path, "Output", "F2", 200, 20)
	if !offset.Complete {
		t.Fatal(offset)
	}
	hasTraceNode(t, offset, "'Inputs'!A2")
	hasTraceNode(t, offset, "'Inputs'!A3")
	if r := referenceTraceSavedRun(t, path, "Output", "F5", 200, 20); !r.Complete {
		t.Fatal(r)
	}
	for _, c := range []struct{ cell, reason string }{{"F3", "Formula-valued"}, {"F4", "bounds"}} {
		r := referenceTraceSavedRun(t, path, "Output", c.cell, 200, 20)
		if r.Complete {
			t.Fatal(c.cell, r)
		}
		found := false
		for _, u := range r.Unresolved {
			if strings.Contains(u.Reason, c.reason) {
				found = true
				break
			}
		}
		if !found {
			t.Fatal(c.cell, r)
		}
	}
	// Supplemental native check retained after all original assertions.
	if offset.Nodes["'Inputs'!A2"].CachedValue != float64(12) || offset.Nodes["'Inputs'!A3"].CachedValue != float64(13) {
		t.Fatal("OFFSET saved input values", offset)
	}
}

// This method has its own setup; no dynamic-input E1:E4/F1:F5 edits.
func TestReferenceTraceRelativeR1C1AndAmbiguousOffset(t *testing.T) {
	path := referenceTraceSavedFixture(t, func(f *excelize.File) {
		if err := f.SetCellValue("Output", "E9", 8); err != nil {
			t.Fatal(err)
		}
		for _, c := range []struct{ cell, formula string }{{"F10", `INDIRECT("R[-1]C[-1]",FALSE)`}, {"F11", "OFFSET(A1+A2,1,0)"}, {"F12", `INDIRECT("R[-99]C",FALSE)`}} {
			if err := f.SetCellFormula("Output", c.cell, c.formula); err != nil {
				t.Fatal(err)
			}
		}
	})
	relative := referenceTraceSavedRun(t, path, "Output", "F10", 200, 20)
	if !relative.Complete {
		t.Fatal(relative)
	}
	referenceTraceNodeSet(t, relative, "'Output'!F10", "'Output'!E9")
	for _, cell := range []string{"F11", "F12"} {
		r := referenceTraceSavedRun(t, path, "Output", cell, 200, 20)
		if r.Complete {
			t.Fatal(cell, r)
		}
		if len(r.Unresolved) == 0 {
			t.Fatal(cell, r)
		}
	}
	// Supplemental native check retained after all original assertions.
	if relative.Nodes["'Output'!E9"].CachedValue != float64(8) {
		t.Fatal("relative R1C1 saved input value", relative)
	}
}

func TestTraceReferenceStructuredAndThreeDimensionalReferences(t *testing.T) {
	path := referenceTraceSavedFixture(t, func(f *excelize.File) {
		for _, c := range []struct {
			cell  string
			value any
		}{{"A2", "Item"}, {"B2", "Amount"}, {"A3", "A"}, {"B3", 11}, {"A4", "B"}, {"B4", 22}} {
			if e := f.SetCellValue("Inputs", c.cell, c.value); e != nil {
				t.Fatal(e)
			}
		}
		if e := f.AddTable("Inputs", &excelize.Table{Range: "A2:B4", Name: "Sales"}); e != nil {
			t.Fatal(e)
		}
		for _, c := range []struct{ cell, formula string }{{"F1", "SUM(Sales[Amount])"}, {"F2", "SUM(Inputs:Output!A1)"}, {"F3", "Sales[[#Headers],[Amount]]"}} {
			if e := f.SetCellFormula("Output", c.cell, c.formula); e != nil {
				t.Fatal(e)
			}
		}
	})
	r := referenceTraceSavedRun(t, path, "Output", "F1", 200, 20)
	if !r.Complete {
		t.Fatal(r)
	}
	referenceTraceNodeSet(t, r, "'Output'!F1", "'Inputs'!B3", "'Inputs'!B4")
	r = referenceTraceSavedRun(t, path, "Output", "F2", 200, 20)
	if !r.Complete {
		t.Fatal(r)
	}
	hasTraceNode(t, r, "'O''Brien'!A1")
	r = referenceTraceSavedRun(t, path, "Output", "F3", 200, 20)
	if !r.Complete {
		t.Fatal(r)
	}
	hasTraceNode(t, r, "'Inputs'!B2")
}

func TestTraceReferenceWholeRanges(t *testing.T) {
	f := excelize.NewFile()
	defer f.Close()
	must := func(e error) {
		t.Helper()
		if e != nil {
			t.Fatal(e)
		}
	}
	must(f.SetSheetName("Sheet1", "Inputs"))
	for _, c := range []struct {
		cell  string
		value int
	}{{"A1", 10}, {"A2", 20}, {"A3", 30}, {"D1", 5}} {
		must(f.SetCellValue("Inputs", c.cell, c.value))
	}
	_, e := f.NewSheet("Output")
	must(e)
	must(f.SetCellFormula("Output", "A1", "SUM(Inputs!$A:$A)"))
	must(f.SetCellFormula("Output", "A2", "SUM(Inputs!$1:$1)"))
	source := filepath.Join(t.TempDir(), "source.xlsx")
	must(f.SaveAs(source))
	before := previewRead(t, source)
	column := referenceTraceSavedRun(t, source, "Output", "A1", 200, 20)
	row := referenceTraceSavedRun(t, source, "Output", "A2", 200, 20)
	if !column.Complete {
		t.Fatal(column)
	}
	referenceTraceNodeSet(t, column, "'Output'!A1", "'Inputs'!A1", "'Inputs'!A2", "'Inputs'!A3")
	if !row.Complete {
		t.Fatal(row)
	}
	referenceTraceNodeSet(t, row, "'Output'!A2", "'Inputs'!A1", "'Inputs'!B1", "'Inputs'!C1", "'Inputs'!D1")
	if row.Nodes["'Inputs'!C1"].CachedValue != nil {
		t.Fatal("empty cell not null")
	}
	previewUnchanged(t, source, before)
	must(f.SetDefinedName(&excelize.DefinedName{Name: "AllValues", RefersTo: "'Inputs'!$A:$A"}))
	must(f.SetCellFormula("Output", "A1", "SUM(AllValues)"))
	must(f.SaveAs(source))
	named := referenceTraceSavedRun(t, source, "Output", "A1", 200, 20)
	if !named.Complete {
		t.Fatal(named)
	}
	keys := make([]string, 0, len(column.Nodes))
	for key := range column.Nodes {
		keys = append(keys, key)
	}
	referenceTraceNodeSet(t, named, keys...)
	sparse := excelize.NewFile()
	defer sparse.Close()
	must(sparse.SetSheetName("Sheet1", "Inputs"))
	must(sparse.SetCellValue("Inputs", "A3", 2))
	must(sparse.SetCellValue("Inputs", "Z20", 8))
	must(sparse.SetCellFormula("Inputs", "D10", "SUM(A:A)"))
	must(sparse.SetCellFormula("Inputs", "D11", "SUM(3:3)"))
	must(sparse.SaveAs(source))
	for _, cell := range []string{"D10", "D11"} {
		r := referenceTraceSavedRun(t, source, "Inputs", cell, 200, 20)
		referenceTraceNodeSet(t, r, "'Inputs'!"+cell, "'Inputs'!A3")
	}
}

// Cancel deterministically inside the worksheet scan, rather than relying on
// scheduler timing or a pre-cancelled context at TraceWorkbook's entry.
type extentCancelContext struct {
	context.Context
	checks int
	cancel context.CancelFunc
	after  int
}

func (c *extentCancelContext) Err() error {
	c.checks++
	if c.checks == c.after || (c.after == 0 && c.checks == 10) {
		c.cancel()
	}
	return c.Context.Err()
}
func TestTraceExtentScanCancellationAndCache(t *testing.T) {
	p, _ := referenceTraceFixture(t, func(f *excelize.File) {
		for i := 1; i <= 200; i++ {
			f.SetCellValue("Inputs", fmt.Sprintf("A%d", i), i)
		}
	})
	wb, e := OpenWorkbook(p)
	if e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	checked := &extentCancelContext{Context: ctx, cancel: cancel}
	tr := &tracer{ctx: checked, wb: wb}
	key := traceExtentKey{sheet: "Inputs", columns: true, first: 1, last: 1}
	if _, ok := tr.savedExtent(key); ok || checked.checks != 10 || len(tr.extents) != 0 {
		t.Fatal("cancelled scan cached partial extent", checked.checks)
	}
	tr.ctx = context.Background()
	got, ok := tr.savedExtent(key)
	if !ok || got != [2]int{1, 200} {
		t.Fatal(got, ok)
	}
	if len(tr.extents) != 1 {
		t.Fatal("extent not cached")
	}
	// A cached result must still honor cancellation.
	tr.ctx = ctx
	if _, ok := tr.savedExtent(key); ok {
		t.Fatal("cached lookup ignored cancellation")
	}
}

func TestTraceReferenceWholeRangeBoundsAndCycles(t *testing.T) {
	// Sparse XML retains the millionth-row case without allocating intermediate rows.
	makeSource := func(formula, tail string) string {
		t.Helper()
		return referencePreviewBook(t, []string{"Inputs"}, "", []string{`<sheetData><row r="1"><c r="A1"><f>SUM(A:A)</f></c><c r="D1"><f>` + formula + `</f></c></row><row r="3"><c r="A3"><v>2</v></c></row>` + tail + `</sheetData>`})
	}
	source := makeSource("SUM(2:2)", `<row r="1048576"><c r="A1048576"><v>1</v></c></row>`)
	limited := referenceTraceSavedRun(t, source, "Inputs", "A1", 20, 20)
	if !limited.Truncated {
		t.Fatal(limited)
	}
	if limited.Complete {
		t.Fatal(limited)
	}
	if len(limited.Nodes) > 20 {
		t.Fatal(limited)
	}
	source = makeSource("SUM(2:2)", "")
	cycle := referenceTraceSavedRun(t, source, "Inputs", "A1", 200, 20)
	if !reflect.DeepEqual(cycle.Cycles, []string{"'Inputs'!A1"}) {
		t.Fatal(cycle)
	}
	row := referenceTraceSavedRun(t, source, "Inputs", "D1", 200, 20)
	if len(row.Nodes) != 1 {
		t.Fatal(row)
	}
	for _, formula := range []string{"SUM(XFE:XFE)", "SUM(1048577:1048577)", "SUM(C:A)", "SUM(3:1)", "SUM(0:0)", "SUM(A0:A1)"} {
		source = makeSource(formula, "")
		report := referenceTraceSavedRun(t, source, "Inputs", "D1", 200, 20)
		if report.Complete {
			t.Fatal(formula, report)
		}
	}
}

func TestTraceExtentIndexMatchesSavedCells(t *testing.T) {
	cells := map[string]*etree.Element{}
	for coord, content := range map[string]string{
		"A1": "<v/>", "A4": "<f/>", "C2": "<is/>", "D8": "<v>1</v>",
		"B20": "", "XFD1048576": "<v/>", "XFE1": "<v/>", "A1048577": "<v/>",
		"A0": "<v/>", "A01": "<v/>", "bad": "<f/>", "$E$9": "<v/>",
		"a7": "<v/>", "F5": `<v xmlns="urn:foreign"/>`,
	} {
		d := etree.NewDocument()
		if err := d.ReadFromString(`<c xmlns="` + NSS + `" s="1">` + content + `</c>`); err != nil {
			t.Fatal(err)
		}
		cells[coord] = d.Root()
	}
	wb := &Workbook{Sheets: map[string]*Sheet{"Inputs": {Cells: cells}, "Empty": {Cells: map[string]*etree.Element{}}}}
	tr := &tracer{ctx: context.Background(), wb: wb}
	for _, sheet := range []string{"Inputs", "Empty"} {
		for _, columns := range []bool{true, false} {
			for _, bounds := range [][2]int{{1, 1}, {1, 3}, {2, 4}, {2, 2}, {4, 1}, {0, 0}, {5, 9}, {1, 1048576}, {16384, 16384}, {1048576, 1048576}} {
				key := traceExtentKey{sheet, columns, bounds[0], bounds[1]}
				want, _ := legacyTraceExtent(tr, key)
				got, ok := tr.savedExtent(key)
				if !ok || got != want {
					t.Fatalf("%+v: got %v want %v", key, got, want)
				}
			}
		}
	}
	index := tr.extents["Inputs"]
	if len(index.columns) != 5 || len(index.rows) != 7 {
		t.Fatalf("style-only or invalid cells indexed: %+v", index)
	}
	// An unrelated workbook must not reuse the first workbook's index.
	other := &tracer{ctx: context.Background(), wb: &Workbook{Sheets: map[string]*Sheet{"Inputs": {Cells: map[string]*etree.Element{}}}}}
	if got, ok := other.savedExtent(traceExtentKey{"Inputs", true, 1, 1}); !ok || got != [2]int{} {
		t.Fatal(got, ok)
	}
}

type extentCountContext struct {
	context.Context
	checks int
}

func (c *extentCountContext) Err() error { c.checks++; return c.Context.Err() }

func TestTraceExtentEnumeration(t *testing.T) {
	p := extentBenchmarkPackage(t, 1000, 100)
	wb, err := OpenWorkbook(p)
	if err != nil {
		t.Fatal(err)
	}
	ctx := &extentCountContext{Context: context.Background()}
	tr := &tracer{ctx: ctx, wb: wb}
	query := func() {
		for col := 1; col <= 100; col++ {
			if _, ok := tr.savedExtent(traceExtentKey{"Inputs", true, col, col}); !ok {
				t.Fatal("query failed")
			}
		}
	}
	query()
	buildAndQuery := ctx.checks
	ctx.checks = 0
	query()
	// Every enumerated cell checks cancellation. Subtract the identical cached
	// query workload and the one check before publishing the completed index.
	visits := buildAndQuery - ctx.checks - 1
	if visits != 100000 {
		t.Fatalf("enumerated %d cells, want 100000", visits)
	}
	ctx.checks = 0
	for col := 1; col <= 100; col++ {
		legacyTraceExtent(tr, traceExtentKey{"Inputs", true, col, col})
	}
	if ctx.checks != 10000000 {
		t.Fatal(ctx.checks)
	}
	t.Logf("cell visits: legacy=%d indexed=%d", ctx.checks, visits)
}

func TestTraceExtentCancellationAtPublishAndQuery(t *testing.T) {
	p := extentBenchmarkPackage(t, 1, 1)
	wb, err := OpenWorkbook(p)
	if err != nil {
		t.Fatal(err)
	}
	key := traceExtentKey{"Inputs", true, 1, 100}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	checked := &extentCancelContext{Context: ctx, cancel: cancel, after: 102}
	tr := &tracer{ctx: checked, wb: wb}
	if _, ok := tr.savedExtent(key); ok || len(tr.extents) != 0 {
		t.Fatal("published cancelled index")
	}
	tr.ctx = context.Background()
	if _, ok := tr.savedExtent(key); !ok {
		t.Fatal("retry failed")
	}
	queryCtx, queryCancel := context.WithCancel(context.Background())
	defer queryCancel()
	tr.ctx = &extentCancelContext{Context: queryCtx, cancel: queryCancel, after: 10}
	if _, ok := tr.savedExtent(key); ok {
		t.Fatal("index query ignored cancellation")
	}
	if len(tr.extents["Inputs"].columns) != 100 {
		t.Fatal("completed index was corrupted")
	}
}
