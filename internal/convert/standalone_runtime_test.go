package convert

import (
	"bytes"
	"context"
	"fmt"
	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/testsupport"
	"math"
	"path/filepath"
	"reflect"
	"strconv"
	"testing"

	"github.com/openvibely/openvibely-paperworks/internal/lo"
	"github.com/openvibely/openvibely-paperworks/internal/pdf"
	"github.com/xuri/excelize/v2"
)

func referenceRuntimeOffice(t *testing.T) {
	t.Helper()
	if _, e := lo.Find(); e != nil {
		t.Skip("LibreOffice unavailable; standalone runtime assertions not verified")
	}
}
func referenceRuntimeBook(t *testing.T, name string, rows [][]any) *excelize.File {
	t.Helper()
	f := excelize.NewFile()
	t.Cleanup(func() { f.Close() })
	if e := f.SetSheetName("Sheet1", name); e != nil {
		t.Fatal(e)
	}
	for i, row := range rows {
		for j, v := range row {
			cell, _ := excelize.CoordinatesToCellName(j+1, i+1)
			var e error
			if s, ok := v.(string); ok && len(s) > 0 && s[0] == '=' {
				e = f.SetCellFormula(name, cell, s[1:])
			} else {
				e = f.SetCellValue(name, cell, v)
			}
			if e != nil {
				t.Fatal(e)
			}
		}
	}
	return f
}
func referenceRuntimeCleanReport(t *testing.T, r map[string]any) {
	t.Helper()
	for _, key := range []string{"errors", "issues"} {
		v := reflect.ValueOf(r[key])
		if !v.IsValid() {
			t.Fatalf("missing %s in report", key)
		}
		if v.Len() != 0 {
			t.Errorf("%s: %v", key, r[key])
		}
	}
}
func referenceRuntimeRead(t *testing.T, path string) *excelize.File {
	t.Helper()
	f, e := excelize.OpenFile(path)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { f.Close() })
	return f
}
func referenceRuntimeValue(t *testing.T, f *excelize.File, sheet, cell string) float64 {
	t.Helper()
	kind, e := f.GetCellType(sheet, cell)
	if e != nil || (kind != excelize.CellTypeUnset && kind != excelize.CellTypeNumber) {
		t.Fatalf("%s!%s expected numeric cell, got type %v: %v", sheet, cell, kind, e)
	}
	s, e := f.GetCellValue(sheet, cell, excelize.Options{RawCellValue: true})
	if e != nil {
		t.Fatal(e)
	}
	v, e := strconv.ParseFloat(s, 64)
	if e != nil {
		t.Fatal(e)
	}
	return v
}

// Cached string values must not be produced by numeric display formatting.
func referenceRuntimeStringValue(t *testing.T, f *excelize.File, sheet, cell string) string {
	t.Helper()
	kind, err := f.GetCellType(sheet, cell)
	if err != nil || (kind != excelize.CellTypeSharedString && kind != excelize.CellTypeInlineString && kind != excelize.CellTypeFormula) {
		t.Fatalf("%s!%s expected cached string, got %v: %v", sheet, cell, kind, err)
	}
	value, err := f.GetCellValue(sheet, cell, excelize.Options{RawCellValue: true})
	if err != nil {
		t.Fatal(err)
	}
	return value
}

// test_document_runtime.py: the original 3*7+4*8 model, then quantity 5.
func TestReferenceStandaloneRuntimeFormulaChart(t *testing.T) {
	referenceRuntimeOffice(t)
	ctx := context.Background()
	dir := t.TempDir()
	source := referenceRuntimeOriginalWorkbookFixture(t, "runtime-workflow-fixtures.zip", "formula-model")
	report, e := Convert(ctx, source, filepath.Join(dir, "calculated"), Options{Recalculate: true})
	if e != nil {
		t.Fatal(e)
	}
	referenceRuntimeCleanReport(t, report)
	calculated := report["output"].(string)
	saved := referenceRuntimeRead(t, calculated)
	if got := referenceRuntimeValue(t, saved, "Model", "C4"); got != 53 {
		t.Error("initial total", got)
	}
	if formula, e := saved.GetCellFormula("Model", "C4"); e != nil || formula == "" {
		t.Error("formula flattened", formula, e)
	}
	scenario := filepath.Join(dir, "scenario.xlsx")
	referenceFormulaModelSecondSave(t, calculated, scenario)
	result, e := Convert(ctx, scenario, filepath.Join(dir, "scenario"), Options{Recalculate: true})
	if e != nil {
		t.Fatal(e)
	}
	if got := referenceRuntimeValue(t, referenceRuntimeRead(t, result["output"].(string)), "Model", "C4"); got != 67 {
		t.Error("scenario total", got)
	}
	rendered, e := Convert(ctx, calculated, filepath.Join(dir, "sheet-render"), Options{})
	if e != nil {
		t.Fatal(e)
	}
	path := rendered["output"].(string)
	if count := len(referenceRuntimePDFDimensions(t, contents(t, path))); count != 1 {
		t.Error("PDF page count", count)
	}
	pages, e := pdf.RenderPages(ctx, path, filepath.Join(dir, "sheet-render"), pdf.RenderOptions{})
	if e != nil || len(pages) == 0 {
		t.Error("rendered pages", pages, e)
	}
}

// test_document_runtime.py: both selector values and all history/forecast checks.
func TestReferenceStandaloneRuntimeForecastCases(t *testing.T) {
	referenceRuntimeOffice(t)
	f := referenceRuntimeBook(t, "Forecast", [][]any{{"Case", "Base"}, {"Historical revenue", 100}, {"Base growth", .1}, {"Downside growth", -.2}, {"Selected growth", `=IF(B1="Base",B3,B4)`}, {"Forecast revenue", "=B2*(1+B5)"}, {"Growth amount", "=B6-B2"}})
	if e := f.SetColWidth("Forecast", "A", "A", 25); e != nil {
		t.Fatal(e)
	}
	if e := f.SetColWidth("Forecast", "B", "B", 20); e != nil {
		t.Fatal(e)
	}
	format := "0.0%"
	style, e := f.NewStyle(&excelize.Style{CustomNumFmt: &format})
	if e != nil {
		t.Fatal(e)
	}
	if e := f.SetCellStyle("Forecast", "B3", "B5", style); e != nil {
		t.Fatal(e)
	}
	if e := f.SetDefinedName(&excelize.DefinedName{Name: "_xlnm.Print_Area", RefersTo: "Forecast!$A$1:$B$7", Scope: "Forecast"}); e != nil {
		t.Fatal(e)
	}
	for _, c := range []struct {
		name     string
		expected float64
	}{{"Base", 110}, {"Downside", 80}} {
		t.Run(c.name, func(t *testing.T) {
			if e := f.SetCellValue("Forecast", "B1", c.name); e != nil {
				t.Fatal(e)
			}
			dir := t.TempDir()
			src := filepath.Join(dir, "forecast.xlsx")
			if e := f.SaveAs(src); e != nil {
				t.Fatal(e)
			}
			report, e := Convert(context.Background(), src, filepath.Join(dir, "calculated"), Options{Recalculate: true})
			if e != nil {
				t.Fatal(e)
			}
			referenceRuntimeCleanReport(t, report)
			result := referenceRuntimeRead(t, report["output"].(string))
			if got := referenceRuntimeValue(t, result, "Forecast", "B2"); got != 100 {
				t.Error("history changed", got)
			}
			if got := referenceRuntimeValue(t, result, "Forecast", "B6"); !(math.Abs(got-c.expected) < 1e-9) {
				t.Error("forecast", got, c.expected)
			}
			if got := referenceRuntimeValue(t, result, "Forecast", "B7"); !(math.Abs(got-(c.expected-100)) < 1e-9) {
				t.Error("growth", got, c.expected-100)
			}
			if formula, e := result.GetCellFormula("Forecast", "B6"); e != nil || formula == "" {
				t.Error("formula flattened", formula, e)
			}
		})
	}
}

// Original feature-preservation runtime block: inspect the reopened file as well
// as every package part outside the one permitted calculation sheet.
func TestReferenceStandaloneRuntimeFeaturePreservation(t *testing.T) {
	referenceRuntimeOffice(t)
	dir := t.TempDir()
	src := referenceRuntimeOriginalWorkbookFixture(t, "runtime-feature-array-fixtures.zip", "feature-preservation")
	must := func(e error) {
		t.Helper()
		if e != nil {
			t.Fatal(e)
		}
	}
	before := contents(t, src)
	r, e := Convert(context.Background(), src, filepath.Join(dir, "calculated"), Options{Recalculate: true})
	must(e)
	if r["original_package_preserved_except_calculated_results"] != true {
		t.Error(r)
	}
	output := r["output"].(string)
	saved := referenceRuntimeRead(t, output)
	if got := referenceRuntimeValue(t, saved, "Inputs", "C3"); got != 12 {
		t.Error("C3", got)
	}
	formula, e := saved.GetCellFormula("Inputs", "C3")
	must(e)
	if formula != "B3*3" {
		t.Error("formula", formula)
	}
	tables, e := testsupport.NamedWorksheetTables(contents(t, output), "Inputs")
	must(e)
	if !reflect.DeepEqual(tables, []string{"InputsTable"}) {
		t.Error("tables", tables)
	}
	validations, e := saved.GetDataValidations("Inputs")
	must(e)
	if len(validations) != 1 {
		t.Error("validations", validations)
	}
	formats, e := saved.GetConditionalFormats("Inputs")
	must(e)
	if len(formats) != 1 {
		t.Error("conditional formats", formats)
	}
	comment, found, e := testsupport.NamedWorksheetCommentText(contents(t, output), "Inputs", "B2")
	must(e)
	if !found || comment != "Preserve this source" {
		t.Error("source comment", comment, found)
	}

	rawState, globalRate, e := testsupport.WorksheetStateAndGlobalName(contents(t, output), "Raw", "Rate")
	must(e)
	if rawState != "hidden" || !globalRate {
		t.Error("Raw sheet state / global Rate", rawState, globalRate)
	}
	if _, e := office.VerifyPreservation(src, output, []string{"xl/worksheets/sheet1.xml"}); e != nil {
		t.Error(e)
	}
	if !bytes.Equal(before, contents(t, src)) {
		t.Error("source changed")
	}
}

// Every original selector/blank/zero/date/duplicate-key loop case is retained.
func TestReferenceStandaloneRuntimeCaseBoundaries(t *testing.T) {
	referenceRuntimeOffice(t)
	// These are the three consecutive saves of the SAME original openpyxl
	// workbook. Independent Excelize construction loses its retained cell/style state.
	for _, c := range []struct {
		name string
		want []string
	}{
		{"base", []string{"100", "1", "0"}},
		{"missing", []string{"MISSING", "0", "0"}},
		{"zero", []string{"0", "1", "0"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			src := referenceRuntimeOriginalWorkbookFixture(t, "runtime-case-fixtures.zip", "case-"+c.name)
			dir := t.TempDir()
			r, e := Convert(context.Background(), src, filepath.Join(dir, "calculated"), Options{Recalculate: true})
			if e != nil {
				t.Fatal(e)
			}
			saved := referenceRuntimeRead(t, r["output"].(string))
			var got []string
			for _, cell := range []string{"B6", "B10", "B13"} {
				v, e := saved.GetCellValue("Cases", cell, excelize.Options{RawCellValue: true})
				if e != nil {
					t.Fatal(e)
				}
				got = append(got, v)
			}
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("got %v want %v", got, c.want)
			}
			if c.name != "missing" {
				want, _ := strconv.ParseFloat(c.want[0], 64)
				if value := referenceRuntimeValue(t, saved, "Cases", "B6"); value != want {
					t.Errorf("B6 numeric value %v, want %v", value, want)
				}
			} else {
				kind, err := saved.GetCellType("Cases", "B6")
				if err != nil || (kind != excelize.CellTypeSharedString && kind != excelize.CellTypeInlineString && kind != excelize.CellTypeFormula) {
					t.Errorf("B6 string type %v: %v", kind, err)
				}
			}
			for _, cell := range []string{"B10", "B13"} {
				kind, e := saved.GetCellType("Cases", cell)
				if e != nil || kind != excelize.CellTypeBool {
					t.Errorf("%s boolean type: %v %v", cell, kind, e)
				}
			}
		})
	}
}

// Supplied domain examples: preserve raw records and independently calculated
// expectations, including tolerances, boolean types, and all five rendered sheets.
func TestReferenceStandaloneRuntimeDomainExamples(t *testing.T) {
	referenceRuntimeOffice(t)
	raw := [][]any{{"Source facts", "Synthetic acceptance data only"}, {"Revenue recognized January", 120}, {"Cash collected February", 120}, {"Monthly salary", 3100}, {"Protocol upper bound (supplied)", 10}, {"Observation 001", 8}, {"Observation 002", 12}, {"Campaign A impressions", 1000}, {"Campaign A clicks", 100}, {"Campaign B impressions", 10}, {"Campaign B clicks", 2}, {"Measurement 1 (mm)", 1}, {"Measurement 2 (mm)", 2}, {"Measurement 3 (mm)", 3}, {"Source as-of", "2026-01-31"}, {"Analysis date", "2026-02-01"}}
	dir := t.TempDir()
	src := referenceRuntimeOriginalWorkbookFixture(t, "runtime-workflow-fixtures.zip", "domain-examples")
	must := func(e error) {
		t.Helper()
		if e != nil {
			t.Fatal(e)
		}
	}
	before := contents(t, src)
	r, e := Convert(context.Background(), src, filepath.Join(dir, "calculated"), Options{Recalculate: true})
	must(e)
	output := r["output"].(string)
	saved := referenceRuntimeRead(t, output)
	wantRows := make([][]any, len(raw))
	for i, row := range raw {
		wantRows[i] = append([]any(nil), row...)
		for j, value := range row {
			if n, ok := value.(int); ok {
				wantRows[i][j] = float64(n)
			}
		}
	}
	if got := referenceLibraryTypedRows(t, saved, "Raw"); !reflect.DeepEqual(got, wantRows) {
		t.Errorf("raw records changed: %v", got)
	}
	for i, want := range []float64{120, 0, 0, 120, 1600} {
		if got := referenceRuntimeValue(t, saved, "Finance", fmt.Sprintf("B%d", i+1)); got != want {
			t.Errorf("finance row %d: %v want %v", i+1, got, want)
		}
	}
	closeNumber := func(sheet, cell string, want, tolerance float64) {
		t.Helper()
		got := referenceRuntimeValue(t, saved, sheet, cell)
		if !(math.Abs(got-want) < tolerance) {
			t.Errorf("%s %s: %v want %v", sheet, cell, got, want)
		}
	}
	closeNumber("Finance", "B6", 60/1.1+60/math.Pow(1.1, 2)-100, 1e-8)
	for cell, want := range map[string]string{"C2": "0", "C3": "1"} {
		got, e := saved.GetCellValue("Protocol review", cell, excelize.Options{RawCellValue: true})
		must(e)
		kind, e := saved.GetCellType("Protocol review", cell)
		must(e)
		if got != want || kind != excelize.CellTypeBool {
			t.Error("protocol boolean", cell, got, kind)
		}
	}
	if got := referenceRuntimeValue(t, saved, "Protocol review", "C4"); got != 1 {
		t.Error("review count", got)
	}
	closeNumber("Campaigns", "B1", 102.0/1010, 1e-10)
	for cell, want := range map[string]string{"B2": "2026-01-31", "B3": "2026-02-01"} {
		got := referenceRuntimeStringValue(t, saved, "Campaigns", cell)
		if got != want {
			t.Error("date", cell, got)
		}
	}
	for cell, want := range map[string]float64{"B1": 2, "B2": 1} {
		if got := referenceRuntimeValue(t, saved, "Measurements", cell); got != want {
			t.Error("measurement", cell, got)
		}
	}
	closeNumber("Measurements", "B3", 1/math.Sqrt(3), 1e-10)
	if !bytes.Equal(before, contents(t, src)) {
		t.Error("source changed")
	}
	pages, e := RenderFile(context.Background(), output, filepath.Join(dir, "render"), "", "", pdf.RenderOptions{})
	must(e)
	if len(pages) != 5 {
		t.Error("rendered page count", len(pages))
	}
}
