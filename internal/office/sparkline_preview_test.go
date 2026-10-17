package office

import (
	"bytes"
	"encoding/json"
	"github.com/beevik/etree"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"testing"
)

func referenceSparkSave(t *testing.T, source, raw string) string {
	t.Helper()
	var spec SparklineSpec
	if e := json.Unmarshal([]byte(raw), &spec); e != nil {
		t.Fatal(e)
	}
	p := previewOpen(t, source)
	if _, e := EditSparklines(p, spec); e != nil {
		t.Fatal(e)
	}
	out := filepath.Join(t.TempDir(), "authored.xlsx")
	if e := p.SaveNew(out); e != nil {
		t.Fatal(e)
	}
	return out
}
func TestReferenceSparklinePreviewExistingDrawing(t *testing.T) {
	source := referenceOriginalWorkbookFixture(t, "sparkline-fixtures.zip", "drawing")
	authored := referenceSparkSave(t, source, `{"sheet":"Inputs","sparklines":[{"cell":"E2","range":"Inputs!A1:D1"}],"options":{"lineWeight":2,"markers":true,"colorSeries":"245A81"}}`)
	before := previewRead(t, authored)
	out := filepath.Join(t.TempDir(), "preview.xlsx")
	if changed, e := OfficePreviewCopy(authored, out); e != nil || !changed {
		t.Fatal(changed, e)
	}
	previewUnchanged(t, authored, before)
	p := previewOpen(t, out)
	if n := len(All(previewXML(t, p, "xl/worksheets/sheet1.xml"), NSX14, "sparklineGroup")); n != 0 {
		t.Fatalf("sparkline groups: %d", n)
	}
	const ns = "http://schemas.openxmlformats.org/drawingml/2006/spreadsheetDrawing"
	d := previewXML(t, p, "xl/drawings/drawing1.xml")
	if n := len(All(d, ns, "graphicFrame")); n != 1 {
		t.Fatalf("existing charts: %d", n)
	}
	shapes := All(d, ns, "sp")
	if len(shapes) != 5 {
		t.Fatalf("shapes: %d", len(shapes))
	}
	props := Child(shapes[len(shapes)-1], ns, "spPr")
	line := Child(props, NSA, "ln")
	if line == nil || line.SelectAttrValue("w", "") != "25400" {
		t.Fatal("line width must be 25400")
	}
	pts := All(props, NSA, "pt")
	if len(pts) != 4 {
		t.Fatalf("points: %d", len(pts))
	}
	y := func(i int) int {
		v, e := strconv.Atoi(pts[i].SelectAttrValue("y", ""))
		if e != nil {
			t.Fatal(e)
		}
		return v
	}
	if y(2) <= y(1) {
		t.Fatal("negative point must be below positive point")
	}
	if !bytes.Equal(referencePreviewZIPRead(t, referencePreviewZIP(t, authored), "xl/charts/chart1.xml"), referencePreviewZIPRead(t, referencePreviewZIP(t, out), "xl/charts/chart1.xml")) {
		t.Fatal("existing chart changed")
	}
}
func TestReferenceSparklinePreviewGapScaleDeletion(t *testing.T) {
	source := referenceOriginalWorkbookFixture(t, "sparkline-fixtures.zip", "gap")
	authored := referenceSparkSave(t, source, `{"sheet":"Inputs","sparklines":[{"cell":"E1","range":"Inputs!A1:D1"},{"cell":"E2","range":"Inputs!A2:D2"}],"options":{"minAxisType":"group","maxAxisType":"group","displayEmptyCellsAs":"gap"}}`)
	out := filepath.Join(t.TempDir(), "preview.xlsx")
	if _, e := OfficePreviewCopy(authored, out); e != nil {
		t.Fatal(e)
	}
	d := previewXML(t, previewOpen(t, out), "xl/drawings/preview1.xml")
	shapes := All(d, "http://schemas.openxmlformats.org/drawingml/2006/spreadsheetDrawing", "sp")
	if len(shapes) != 2 {
		t.Fatalf("shapes: %d", len(shapes))
	}
	counts := []int{}
	for _, s := range shapes {
		counts = append(counts, len(All(s, NSA, "pt")))
	}
	if !reflect.DeepEqual(counts, []int{2, 4}) {
		t.Fatal(counts)
	}
	cleared := referenceSparkSave(t, authored, `{"sheet":"Inputs","action":"clear"}`)
	unused := filepath.Join(t.TempDir(), "unused.xlsx")
	if changed, e := OfficePreviewCopy(cleared, unused); e != nil || changed {
		t.Fatal(changed, e)
	}
	if _, e := os.Stat(unused); !os.IsNotExist(e) {
		t.Fatal("unexpected preview output", e)
	}
}

func TestSparklineRowPixelRounding(t *testing.T) {
	for _, c := range []struct {
		height, hidden string
		want           float64
	}{
		{"40", "", 54}, {"15", "", 20}, {"12.2", "", 17}, {"0", "", 20}, {"40", "1", 0},
	} {
		t.Run(c.height+"-"+c.hidden, func(t *testing.T) {
			d := etree.NewDocument()
			if err := d.ReadFromString(`<worksheet xmlns="` + NSS + `"><sheetFormatPr defaultRowHeight="15"/><sheetData><row r="1" ht="` + c.height + `" hidden="` + c.hidden + `"/></sheetData></worksheet>`); err != nil {
				t.Fatal(err)
			}
			if got := newGeometry(d.Root()).rowPixels(1); got != c.want {
				t.Fatalf("pixels=%v want %v", got, c.want)
			}
		})
	}
}

func TestSparklineEMUTiesToEven(t *testing.T) {
	for _, c := range []struct {
		v    float64
		want string
	}{{61912.5, "61912"}, {61913.5, "61914"}, {-2.5, "-2"}, {-3.5, "-4"}} {
		if got := sparklineRoundedEMU(c.v); got != c.want {
			t.Fatalf("%v => %s want %s", c.v, got, c.want)
		}
	}
}
