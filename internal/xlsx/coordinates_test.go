package xlsx

import (
	"errors"
	"testing"

	"github.com/xuri/excelize/v2"
)

func TestWalkCoordinatesVisitsInclusiveBoundsInRowMajorOrder(t *testing.T) {
	tests := []struct {
		name          string
		left, top     int
		right, bottom int
		want          [][2]int
	}{
		{name: "single cell", left: 3, top: 4, right: 3, bottom: 4, want: [][2]int{{3, 4}}},
		{name: "2 by 2", left: 2, top: 5, right: 3, bottom: 6, want: [][2]int{{2, 5}, {3, 5}, {2, 6}, {3, 6}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var got [][2]int
			if err := walkCoordinates(tc.left, tc.top, tc.right, tc.bottom, func(col, row int) error {
				got = append(got, [2]int{col, row})
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("visited %v, want %v", got, tc.want)
			}
			for i := range tc.want {
				if got[i] != tc.want[i] {
					t.Fatalf("visited %v, want %v", got, tc.want)
				}
			}
		})
	}
}

func TestWalkCoordinatesStopsOnCallbackError(t *testing.T) {
	wantErr := errors.New("stop traversal")
	var got [][2]int
	err := walkCoordinates(2, 5, 3, 6, func(col, row int) error {
		got = append(got, [2]int{col, row})
		if len(got) == 2 {
			return wantErr
		}
		return nil
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("got error %v, want %v", err, wantErr)
	}
	want := [][2]int{{2, 5}, {3, 5}}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("visited %v, want %v", got, want)
	}
}

func TestBoundsRejectsReversedAndOversizedRanges(t *testing.T) {
	for _, r := range []string{"B2:A1", "A1:XFD1048576"} {
		t.Run(r, func(t *testing.T) {
			if _, _, err := bounds(r); err == nil {
				t.Fatalf("bounds(%q) succeeded", r)
			}
		})
	}
}

func TestRangeClearAndFillIncludeSingleCellAndAllCorners(t *testing.T) {
	f := excelize.NewFile()
	defer f.Close()
	for cell, value := range map[string]string{
		"A1": "single",
		"B2": "top-left",
		"C2": "top-right",
		"B3": "bottom-left",
		"C3": "bottom-right",
	} {
		if err := f.SetCellValue("Sheet1", cell, value); err != nil {
			t.Fatal(err)
		}
	}
	if err := f.SetCellFormula("Sheet1", "C3", "1+1"); err != nil {
		t.Fatal(err)
	}
	for _, r := range []string{"A1", "B2:C3"} {
		if err := applyEdit(f, "", Edit{Operation: "clear-values", Sheet: "Sheet1", Range: r}); err != nil {
			t.Fatal(err)
		}
	}
	for _, cell := range []string{"A1", "B2", "C2", "B3", "C3"} {
		value, err := f.GetCellValue("Sheet1", cell)
		if err != nil {
			t.Fatal(err)
		}
		formula, err := f.GetCellFormula("Sheet1", cell)
		if err != nil {
			t.Fatal(err)
		}
		if value != "" || formula != "" {
			t.Fatalf("%s was not cleared: value %q formula %q", cell, value, formula)
		}
	}

	for _, tc := range []struct{ r, firstFormula string }{
		{r: "H8", firstFormula: "A1+1"},
		{r: "D5:E6", firstFormula: "A1+1"},
	} {
		if err := applyEdit(f, "", Edit{Operation: "fill-formula", Sheet: "Sheet1", Range: tc.r, Formula: tc.firstFormula}); err != nil {
			t.Fatal(err)
		}
	}
	for cell, want := range map[string]string{
		"H8": "A1+1",
		"D5": "A1+1",
		"E5": "B1+1",
		"D6": "A2+1",
		"E6": "B2+1",
	} {
		got, err := f.GetCellFormula("Sheet1", cell)
		if err != nil || got != want {
			t.Fatalf("formula at %s = %q, %v; want %q", cell, got, err, want)
		}
	}
}

func TestCopyRangePreservesSingleCellAndRectangleCoordinateMapping(t *testing.T) {
	f := excelize.NewFile()
	defer f.Close()
	for cell, value := range map[string]string{
		"A1": "single",
		"B2": "top-left",
		"C2": "top-right",
		"B3": "bottom-left",
		"C3": "bottom-right",
	} {
		if err := f.SetCellValue("Sheet1", cell, value); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct{ source, target string }{
		{source: "A1", target: "H8"},
		{source: "B2:C3", target: "E5"},
	} {
		if _, err := copyRange(f, Edit{Operation: "copy", Sheet: "Sheet1", Range: tc.source, Target: tc.target}); err != nil {
			t.Fatal(err)
		}
	}
	for cell, want := range map[string]string{
		"H8": "single",
		"E5": "top-left",
		"F5": "top-right",
		"E6": "bottom-left",
		"F6": "bottom-right",
	} {
		got, err := f.GetCellValue("Sheet1", cell)
		if err != nil || got != want {
			t.Fatalf("value at %s = %q, %v; want %q", cell, got, err, want)
		}
	}
}
