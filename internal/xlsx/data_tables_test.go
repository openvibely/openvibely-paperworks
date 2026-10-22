package xlsx

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"github.com/xuri/excelize/v2"
)

func tableSpec() Spec {
	return Spec{Sheets: []Sheet{{Name: "Model", Cells: map[string]Cell{"B1": {Value: 1}, "B2": {Value: 5}, "B3": {Formula: "B1+10*B2"}, "D2": {Formula: "B3"}, "E2": {Value: 2}, "F2": {Value: 3}, "D3": {Value: 10}, "D4": {Value: 20}}, DataTables: []DataTable{{Range: "E3:F4", Input1: "B1", Input2: "B2"}}}}}
}
func TestNativeDataTableCreation(t *testing.T) {
	out := filepath.Join(t.TempDir(), "model.xlsx")
	spec := tableSpec()
	if e := Create(context.Background(), spec, ".", out); e != nil {
		t.Fatal(e)
	}
	p, e := opc.Open(out)
	if e != nil {
		t.Fatal(e)
	}
	w, e := office.OpenWorkbook(p)
	if e != nil {
		t.Fatal(e)
	}
	c := w.Sheets["Model"].Cells["E3"]
	f := office.Child(c, office.NSS, "f")
	if f == nil {
		t.Fatal("missing formula")
	}
	for key, want := range map[string]string{"t": "dataTable", "ref": "E3:F4", "r1": "B1", "r2": "B2", "dt2D": "1"} {
		if f.SelectAttrValue(key, "") != want {
			t.Fatalf("%s lost", key)
		}
	}
	if office.Child(c, office.NSS, "v") != nil {
		t.Fatal("uncalculated table has fabricated result")
	}
}
func TestDataTableRejectsDestructiveOrInvalidSpec(t *testing.T) {
	for name, edit := range map[string]func(*Sheet){
		"occupied-result":      func(s *Sheet) { s.Cells["F4"] = Cell{Value: "keep me"} },
		"formula-result":       func(s *Sheet) { s.Cells["F4"] = Cell{Formula: "B1"} },
		"input-formula":        func(s *Sheet) { s.Cells["B1"] = Cell{Formula: "B2"} },
		"merged-result":        func(s *Sheet) { s.Merges = []string{"E3:F3"} },
		"overlap":              func(s *Sheet) { s.DataTables = append(s.DataTables, s.DataTables[0]) },
		"row-and-two-input":    func(s *Sheet) { s.DataTables[0].RowOriented = true },
		"input-in-results":     func(s *Sheet) { s.DataTables[0].Input1 = "E3" },
		"missing-model-output": func(s *Sheet) { delete(s.Cells, "D2") },
		"too-large":            func(s *Sheet) { s.DataTables[0].Range = "E3:XFD1048576" },
	} {
		t.Run(name, func(t *testing.T) {
			spec := tableSpec()
			edit(&spec.Sheets[0])
			out := filepath.Join(t.TempDir(), "bad.xlsx")
			if e := Create(context.Background(), spec, ".", out); e == nil {
				t.Fatal("accepted invalid specification")
			}
			if _, e := os.Stat(out); !os.IsNotExist(e) {
				t.Fatal("left output", e)
			}
		})
	}
}

func newTableValidationWorkbook(t *testing.T) *excelize.File {
	t.Helper()
	f := excelize.NewFile()
	if err := f.SetSheetName("Sheet1", "Model"); err != nil {
		t.Fatal(err)
	}
	for cell, value := range map[string]int64{"B1": 1, "B2": 5} {
		if err := f.SetCellInt("Model", cell, value); err != nil {
			t.Fatal(err)
		}
	}
	if err := f.SetCellFormula("Model", "D2", "B1+B2"); err != nil {
		t.Fatal(err)
	}
	return f
}

func TestDataTableValidationIncludesSingleCellAndAllRectangleCorners(t *testing.T) {
	for _, tc := range []struct{ name, r string }{
		{name: "single cell", r: "E3"},
		{name: "2 by 2", r: "E3:F4"},
	} {
		t.Run(tc.name+" empty results", func(t *testing.T) {
			f := newTableValidationWorkbook(t)
			defer f.Close()
			spec := Spec{Sheets: []Sheet{{Name: "Model", DataTables: []DataTable{{Range: tc.r, Input1: "B1", Input2: "B2"}}}}}
			if err := prepareDataTables(context.Background(), f, spec); err != nil {
				t.Fatal(err)
			}
		})
	}

	for _, cell := range []string{"E3", "F3", "E4", "F4"} {
		t.Run("occupied "+cell, func(t *testing.T) {
			f := newTableValidationWorkbook(t)
			defer f.Close()
			if err := f.SetCellValue("Model", cell, "occupied"); err != nil {
				t.Fatal(err)
			}
			spec := Spec{Sheets: []Sheet{{Name: "Model", DataTables: []DataTable{{Range: "E3:F4", Input1: "B1", Input2: "B2"}}}}}
			err := prepareDataTables(context.Background(), f, spec)
			if err == nil || !strings.Contains(err.Error(), "data table would overwrite "+cell) {
				t.Fatalf("got %v, want occupied result error for %s", err, cell)
			}
		})
	}

	t.Run("row-major first occupied result", func(t *testing.T) {
		f := newTableValidationWorkbook(t)
		defer f.Close()
		for _, cell := range []string{"F3", "E4"} {
			if err := f.SetCellValue("Model", cell, "occupied"); err != nil {
				t.Fatal(err)
			}
		}
		spec := Spec{Sheets: []Sheet{{Name: "Model", DataTables: []DataTable{{Range: "E3:F4", Input1: "B1", Input2: "B2"}}}}}
		err := prepareDataTables(context.Background(), f, spec)
		if err == nil || !strings.Contains(err.Error(), "data table would overwrite F3") {
			t.Fatalf("got %v, want row-major error at F3", err)
		}
	})
}

type cancelAfterErrChecks struct {
	checks   int
	cancelAt int
}

func (c *cancelAfterErrChecks) Deadline() (time.Time, bool) { return time.Time{}, false }
func (c *cancelAfterErrChecks) Done() <-chan struct{}       { return nil }
func (c *cancelAfterErrChecks) Value(any) any               { return nil }
func (c *cancelAfterErrChecks) Err() error {
	c.checks++
	if c.checks >= c.cancelAt {
		return context.Canceled
	}
	return nil
}

func TestDataTableValidationStopsPromptlyOnCancellation(t *testing.T) {
	f := newTableValidationWorkbook(t)
	defer f.Close()
	spec := Spec{Sheets: []Sheet{{Name: "Model", DataTables: []DataTable{{Range: "E3:F4", Input1: "B1", Input2: "B2"}}}}}
	ctx := &cancelAfterErrChecks{cancelAt: 3} // Initial check, E3, then cancellation before F3.
	err := prepareDataTables(ctx, f, spec)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v, want context cancellation", err)
	}
	if ctx.checks != 3 {
		t.Fatalf("made %d context checks, want cancellation at the second result cell", ctx.checks)
	}
}

func TestDataTableValidationEnforcesTotalCellLimit(t *testing.T) {
	f := excelize.NewFile()
	defer f.Close()
	if err := f.SetSheetName("Sheet1", "Model"); err != nil {
		t.Fatal(err)
	}
	for cell, value := range map[string]int64{"A2": 1, "A3": 2} {
		if err := f.SetCellInt("Model", cell, value); err != nil {
			t.Fatal(err)
		}
	}
	if err := f.SetCellFormula("Model", "A1", "A2+A3"); err != nil {
		t.Fatal(err)
	}
	dataTable := DataTable{Range: "B2:CW1001", Input1: "A2", Input2: "A3"} // Exactly 100000 cells.
	spec := Spec{Sheets: []Sheet{{Name: "Model", DataTables: []DataTable{dataTable, dataTable}}}}
	err := prepareDataTables(context.Background(), f, spec)
	if err == nil || !strings.Contains(err.Error(), "data tables exceed 100000 result cells") {
		t.Fatalf("got %v, want total result cell limit error", err)
	}
}
