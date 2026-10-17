package convert

import (
	"context"
	"math"
	"path/filepath"
	"testing"

	"github.com/openvibely/openvibely-paperworks/internal/testsupport"
)

func referenceStandaloneInputEdit(t *testing.T, src, dst, sheet string, values map[string]any) string {
	t.Helper()
	_, e := WorkbookInputs(src, dst, sheet, values)
	if e != nil {
		t.Fatal(e)
	}
	return dst
}
func referenceStandaloneCalculate(t *testing.T, src, dir string) string {
	t.Helper()
	r, e := Convert(context.Background(), src, dir, Options{Recalculate: true})
	if e != nil {
		t.Fatal(e)
	}
	return r["output"].(string)
}
func TestReferenceStandaloneRuntimeIteration(t *testing.T) {
	referenceRuntimeOffice(t)
	dir := t.TempDir()
	src := referenceRuntimeOriginalWorkbookFixture(t, "runtime-workflow-fixtures.zip", "iteration")
	calculated := referenceStandaloneCalculate(t, src, filepath.Join(dir, "calculated"))
	t.Run("initial", func(t *testing.T) {
		book := referenceRuntimeRead(t, calculated)
		v := referenceRuntimeValue(t, book, book.GetSheetName(book.GetActiveSheetIndex()), "A2")
		if !(math.Abs(v-10) < 1e-7) {
			t.Error(v)
		}
	})
	edited := referenceStandaloneInputEdit(t, calculated, filepath.Join(dir, "edited.xlsx"), "Iteration", map[string]any{"A1": float64(20)})
	updated := referenceStandaloneCalculate(t, edited, filepath.Join(dir, "updated"))
	t.Run("updated", func(t *testing.T) {
		book := referenceRuntimeRead(t, updated)
		v := referenceRuntimeValue(t, book, book.GetSheetName(book.GetActiveSheetIndex()), "A2")
		if !(math.Abs(v-20) < 1e-7) {
			t.Error(v)
		}
	})
	t.Run("settings", func(t *testing.T) {
		props, e := referenceRuntimeRead(t, updated).GetCalcProps()
		if e != nil {
			t.Fatal(e)
		}
		if props.IterateCount == nil || *props.IterateCount != 1000 {
			t.Error("iteration count", props.IterateCount)
		}
		if props.IterateDelta == nil || *props.IterateDelta != 1e-8 {
			t.Error("iteration delta", props.IterateDelta)
		}

	})
}
func TestReferenceStandaloneRuntimeSnapshots(t *testing.T) {
	referenceRuntimeOffice(t)
	dir := t.TempDir()
	source := referenceRuntimeOriginalWorkbookFixture(t, "runtime-workflow-fixtures.zip", "capture")
	for _, step := range []struct {
		name   string
		inputs map[string]any
		want   [2]float64
	}{{"initialize", nil, [2]float64{0, 0}}, {"first", map[string]any{"B3": float64(0), "B2": float64(1)}, [2]float64{100, 0}}, {"second", map[string]any{"B1": float64(2)}, [2]float64{100, 200}}, {"retain", map[string]any{"B2": float64(0), "B4": float64(150)}, [2]float64{100, 200}}} {
		if step.inputs != nil {
			source = referenceStandaloneInputEdit(t, source, filepath.Join(dir, step.name+".xlsx"), "Capture", step.inputs)
		}
		source = referenceStandaloneCalculate(t, source, filepath.Join(dir, step.name))
		t.Run(step.name, func(t *testing.T) {
			book := referenceRuntimeRead(t, source)
			for i, cell := range []string{"D1", "E1"} {
				if v := referenceRuntimeValue(t, book, book.GetSheetName(book.GetActiveSheetIndex()), cell); v != step.want[i] {
					t.Errorf("%s=%v want %v", cell, v, step.want[i])
				}
			}
		})
	}
	t.Run("stale-state", func(t *testing.T) {
		book := referenceRuntimeRead(t, source)
		for cell, want := range map[string]string{"D2": "Stale snapshot", "E2": "Current"} {
			got := referenceRuntimeStringValue(t, book, book.GetSheetName(book.GetActiveSheetIndex()), cell)
			if got != want {
				t.Errorf("%s=%q want %q", cell, got, want)
			}
		}
		if v := referenceRuntimeValue(t, book, book.GetSheetName(book.GetActiveSheetIndex()), "B6"); v != 200 {
			t.Error(v)
		}
	})
}
func TestReferenceStandaloneRuntimeNativeArray(t *testing.T) {
	referenceRuntimeOffice(t)
	dir := t.TempDir()
	src := referenceRuntimeOriginalWorkbookFixture(t, "runtime-feature-array-fixtures.zip", "array")
	calculated := referenceStandaloneCalculate(t, src, filepath.Join(dir, "calculated"))
	check := func(t *testing.T, path string, want []float64) {
		t.Helper()
		book := referenceRuntimeRead(t, path)
		// Reference reads load_workbook(..., data_only=True).active.
		sheet := book.GetSheetName(book.GetActiveSheetIndex())
		if sheet == "" {
			t.Fatal("no active worksheet")
		}
		for i, cell := range []string{"B1", "B2", "B3"} {
			if v := referenceRuntimeValue(t, book, sheet, cell); v != want[i] {
				t.Errorf("%s=%v want %v", cell, v, want[i])
			}
		}
	}
	t.Run("initial", func(t *testing.T) { check(t, calculated, []float64{2, 4, 6}) })
	t.Run("array-definition", func(t *testing.T) {
		found, e := testsupport.ActiveWorksheetArrayFormula(contents(t, calculated), "B1")
		if e != nil {
			t.Fatal(e)
		}
		if !found {
			t.Error("B1 lost native array formula")
		}
	})
	edited := referenceStandaloneInputEdit(t, calculated, filepath.Join(dir, "edited.xlsx"), "Array", map[string]any{"A2": float64(7)})
	updated := referenceStandaloneCalculate(t, edited, filepath.Join(dir, "updated"))
	t.Run("updated", func(t *testing.T) { check(t, updated, []float64{2, 14, 6}) })
}
