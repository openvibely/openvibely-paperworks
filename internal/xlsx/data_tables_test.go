package xlsx

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
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
