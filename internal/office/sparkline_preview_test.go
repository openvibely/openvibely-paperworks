package office

import (
	"bytes"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"testing"

	"github.com/beevik/etree"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"github.com/xuri/excelize/v2"
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

func TestSparklinePreviewEmptyCellModesPreserveSourcePositions(t *testing.T) {
	for _, mode := range []string{"span", "gap", "zero"} {
		t.Run(mode, func(t *testing.T) {
			source := sparklineEmptyCellModeBook(t, mode)
			out := filepath.Join(t.TempDir(), "preview.xlsx")
			if changed, err := OfficePreviewCopy(source, out); err != nil || !changed {
				t.Fatalf("OfficePreviewCopy changed=%v err=%v", changed, err)
			}

			const xdr = "http://schemas.openxmlformats.org/drawingml/2006/spreadsheetDrawing"
			drawing := previewXML(t, previewOpen(t, out), "xl/drawings/preview1.xml")
			shapes := All(drawing, xdr, "sp")
			wantIndexes := map[string][][]int{
				"span": {{0, 2, 3}, {2, 5}, {0, 3, 4}},
				"gap":  {{2, 3}, {3, 4}},
				"zero": {{0, 1, 2, 3}, {0, 1, 2, 3, 4, 5}, {0, 1, 2, 3, 4}},
			}[mode]
			sourceCounts := map[string][]int{"span": {4, 6, 5}, "gap": {4, 5}, "zero": {4, 6, 5}}[mode]
			if len(shapes) != len(wantIndexes) {
				t.Fatalf("line shapes=%d want %d", len(shapes), len(wantIndexes))
			}
			for i, shape := range shapes {
				points := All(shape, NSA, "pt")
				if len(points) != len(wantIndexes[i]) {
					t.Fatalf("shape %d points=%d want %d", i, len(points), len(wantIndexes[i]))
				}
				anchor := shape.Parent()
				ext := Child(anchor, xdr, "ext")
				width, err := strconv.ParseFloat(ext.SelectAttrValue("cx", ""), 64)
				if err != nil {
					t.Fatal(err)
				}
				height, err := strconv.ParseFloat(ext.SelectAttrValue("cy", ""), 64)
				if err != nil {
					t.Fatal(err)
				}
				sourceCount := sourceCounts[i]
				var gotX []int
				for _, point := range points {
					x, err := strconv.Atoi(point.SelectAttrValue("x", ""))
					if err != nil {
						t.Fatal(err)
					}
					gotX = append(gotX, x)
				}
				wantX := make([]int, len(wantIndexes[i]))
				pad := math.Min(28575, math.Min(width/4, height/4))
				for j, index := range wantIndexes[i] {
					wantX[j] = int(math.RoundToEven(pad + float64(index)/float64(sourceCount-1)*(width-2*pad)))
				}
				if !reflect.DeepEqual(gotX, wantX) {
					t.Fatalf("shape %d x coordinates=%v want source positions %v (%v)", i, gotX, wantX, wantIndexes[i])
				}
				if mode == "zero" && i == 0 {
					firstY, err := strconv.Atoi(points[0].SelectAttrValue("y", ""))
					if err != nil {
						t.Fatal(err)
					}
					blankY, err := strconv.Atoi(points[1].SelectAttrValue("y", ""))
					if err != nil {
						t.Fatal(err)
					}
					if blankY <= firstY {
						t.Fatalf("zero-mode blank y=%d should plot below value 10 at y=%d", blankY, firstY)
					}
				}
			}
		})
	}
}

func sparklineEmptyCellModeBook(t *testing.T, mode string) string {
	t.Helper()
	f := excelize.NewFile()
	defer f.Close()
	for cell, value := range map[string]int{"A1": 10, "C1": 20, "D1": 30, "C2": 10, "F2": 20, "A3": 10, "D3": 20, "E3": 30} {
		if err := f.SetCellValue("Sheet1", cell, value); err != nil {
			t.Fatal(err)
		}
	}
	buffer, err := f.WriteToBuffer()
	if err != nil {
		t.Fatal(err)
	}
	p, err := opc.Read(buffer.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	var spec SparklineSpec
	if err := json.Unmarshal([]byte(`{"sheet":"Sheet1","sparklines":[{"cell":"H1","range":"Sheet1!A1:D1"},{"cell":"H2","range":"Sheet1!A2:F2"},{"cell":"H3","range":"Sheet1!A3:E3"}],"options":{"displayEmptyCellsAs":"`+mode+`"}}`), &spec); err != nil {
		t.Fatal(err)
	}
	if _, err := EditSparklines(p, spec); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "source.xlsx")
	if err := p.SaveNew(path); err != nil {
		t.Fatal(err)
	}
	return path
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
