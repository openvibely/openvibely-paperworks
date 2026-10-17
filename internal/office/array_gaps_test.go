package office

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"github.com/xuri/excelize/v2"
)

// Generates the same array-formula cells as DocumentGapChecks.formula_workbook.
func referenceArrayBook(t *testing.T, end int, narrow, blocked bool) string {
	t.Helper()
	name := fmt.Sprintf("array-%d", end)
	if narrow || blocked {
		if end != 2 || narrow && blocked {
			t.Fatal("unsupported original array fixture combination")
		}
		if narrow {
			name = "array-narrow"
		} else {
			name = "array-blocked"
		}
	}
	// The collision input is saved by openpyxl *after* rewrite, restoring
	// Deflate and clearing the formula cache. Its captured members retain that save.
	if blocked {
		return referenceOriginalWorkbookFixture(t, "chart-cache-fixtures.zip", name)
	}
	return referenceChartCacheFixture(t, name)
}

// Match the file API: successful cache preservation replaces the engine result.
// No reader may inspect only the returned buffer in place of the saved output.
func referenceMergeArrays(t *testing.T, src, result string) error {
	t.Helper()
	r, e := PackageReport(src)
	if e != nil {
		t.Fatal(e)
	}
	b, e := os.ReadFile(result)
	if e != nil {
		t.Fatal(e)
	}
	raw, e := PreserveFormulaCaches(src, b, r["formulas"].([]Formula))
	if e != nil {
		return e
	}
	if e := os.WriteFile(result, raw, 0600); e != nil {
		t.Fatal(e)
	}
	return nil
}
func TestReferenceArrayGrowthAndShrink(t *testing.T) {
	for _, c := range [][2]int{{2, 4}, {4, 2}} {
		src := referenceArrayBook(t, c[0], false, false)
		result := referenceArrayBook(t, c[1], false, false)
		before, e := os.ReadFile(src)
		if e != nil {
			t.Fatal(e)
		}
		if e := referenceMergeArrays(t, src, result); e != nil {
			t.Fatal(e)
		}
		f, e := excelize.OpenFile(result)
		if e != nil {
			t.Fatal(e)
		}
		t.Cleanup(func() { f.Close() })
		for i := 1; i <= 4; i++ {
			var want any
			if i <= c[1] {
				want = float64(i)
			}
			if got := referenceCachedScalar(t, result, "xl/worksheets/sheet1.xml", fmt.Sprint("B", i)); got != want {
				t.Fatalf("B%d cached value %#v (%T), want %#v", i, got, got, want)
			}
		}

		if got, e := f.GetCellValue("Sheet", "B5"); e != nil || got != "Unrelated" {
			t.Fatal(got, e)
		}
		p, e := opc.Open(result)
		if e != nil {
			t.Fatal(e)
		}
		d, e := p.XML("xl/worksheets/sheet1.xml")
		if e != nil {
			t.Fatal(e)
		}
		// Select B1 explicitly; an unrelated formula must not satisfy this check.
		var formulaRef string
		for _, cell := range All(d.Root(), NSS, "c") {
			if cell.SelectAttrValue("r", "") == "B1" {
				f := Child(cell, NSS, "f")
				if f == nil || f.SelectAttrValue("t", "") != "array" {
					t.Fatal("B1 is not an array formula")
				}
				formulaRef = f.SelectAttrValue("ref", "")
				break
			}
		}
		if formulaRef != fmt.Sprintf("B1:B%d", c[1]) {
			t.Fatal(formulaRef)
		}
		after, e := os.ReadFile(src)
		if e != nil || !bytes.Equal(before, after) {
			t.Fatal("source changed", e)
		}
	}
}
func TestReferenceSpillDimensionsAndCollisions(t *testing.T) {
	src := referenceArrayBook(t, 2, true, false)
	result := referenceArrayBook(t, 4, false, false)
	if e := referenceMergeArrays(t, src, result); e != nil {
		t.Fatal(e)
	}
	p, e := opc.Open(result)
	if e != nil {
		t.Fatal(e)
	}
	d, e := p.XML("xl/worksheets/sheet1.xml")
	if e != nil {
		t.Fatal(e)
	}
	if got := Child(d.Root(), NSS, "dimension").SelectAttrValue("ref", ""); got != "A1:B4" {
		t.Fatal("streaming dimensions", got)
	}
	f, e := excelize.OpenFile(result)
	if e != nil {
		t.Fatal(e)
	}
	defer f.Close()
	rows, e := f.Rows("Sheet")
	if e != nil {
		t.Fatal(e)
	}
	defer rows.Close()
	last := 0
	var fourth []string
	for rows.Next() {
		last++
		cols, e := rows.Columns(excelize.Options{RawCellValue: true})
		if e != nil {
			t.Fatal(e)
		}
		if last == 4 {
			fourth = append([]string(nil), cols...)
		}
	}
	if e = rows.Error(); e != nil {
		t.Fatal(e)
	}
	if last != 4 {
		t.Fatal("streaming row count", last)
	}
	if got := referenceCachedScalar(t, result, "xl/worksheets/sheet1.xml", "B4"); got != float64(4) {
		t.Fatalf("B4 value %#v, want numeric 4", got)
	}
	if len(fourth) < 2 || fourth[1] != "4" {
		t.Fatal(fourth)
	}
	if e := rows.Close(); e != nil {
		t.Fatal(e)
	}
	if e := f.Close(); e != nil {
		t.Fatal(e)
	}
	blocked := referenceArrayBook(t, 2, false, true)
	// Reuse the saved first merge result, as the original does.
	if e := referenceMergeArrays(t, blocked, result); e == nil || !strings.Contains(e.Error(), "overwrite") {
		t.Fatal("collision not rejected", e)
	}
}

// Openpyxl replaces the B5 follower and expands the saved dimension; it does
// not append a second row 5. This only constructs and reads a saved fixture.
func TestReferenceArrayFixtureLongExtent(t *testing.T) {
	for _, end := range []int{5, 6, 50} {
		t.Run(fmt.Sprint(end), func(t *testing.T) {
			path := referenceArrayBook(t, end, false, false)
			p, err := opc.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			d, err := p.XML("xl/worksheets/sheet1.xml")
			if err != nil {
				t.Fatal(err)
			}
			if got := Child(d.Root(), NSS, "dimension").SelectAttrValue("ref", ""); got != fmt.Sprintf("A1:B%d", end) {
				t.Fatal(got)
			}
			seen := map[string]bool{}
			for _, row := range Children(Child(d.Root(), NSS, "sheetData"), NSS, "row") {
				id := row.SelectAttrValue("r", "")
				if seen[id] {
					t.Fatal("duplicate row", id)
				}
				seen[id] = true
			}
			if got := referenceCachedScalar(t, path, "xl/worksheets/sheet1.xml", "B5"); got != "Unrelated" {
				t.Fatal(got)
			}
		})
	}
}
