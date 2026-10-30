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

func TestSparklineColumnGeometryIndex(t *testing.T) {
	doc := etree.NewDocument()
	if err := doc.ReadFromString(`<worksheet xmlns="` + NSS + `"><sheetFormatPr defaultColWidth="12.5"/><cols><col min="2" max="4" width="10"/><col min="3" max="5" width="20" hidden="1"/><col min="5" max="6" width="30" hidden="0"/></cols></worksheet>`); err != nil {
		t.Fatal(err)
	}
	g := newGeometry(doc.Root())
	for _, tc := range []struct {
		column int
		pixels float64
		hidden bool
	}{
		{2, 70, false},
		{3, 70, false}, // The first range wins over the overlapping hidden range.
		{4, 70, false},
		{5, 0, true}, // The second range wins over the third at column 5.
		{6, 210, false},
		{7, 87, false}, // Unspecified columns use defaultColWidth.
	} {
		if got := g.colPixels(tc.column); got != tc.pixels {
			t.Errorf("column %d pixels=%v want %v", tc.column, got, tc.pixels)
		}
		if got := g.colHidden(tc.column); got != tc.hidden {
			t.Errorf("column %d hidden=%v want %v", tc.column, got, tc.hidden)
		}
	}
}

func TestSparklineColumnGeometryWithoutCols(t *testing.T) {
	doc := etree.NewDocument()
	if err := doc.ReadFromString(`<worksheet xmlns="` + NSS + `"/>`); err != nil {
		t.Fatal(err)
	}
	g := newGeometry(doc.Root())
	if got := g.colPixels(16384); got != 59 {
		t.Fatalf("default column pixels=%v want 59", got)
	}
	if g.colHidden(16384) {
		t.Fatal("unspecified column should not be hidden")
	}
}

func TestSparklinePreviewColumnGeometryBySheetAndSourceOrientation(t *testing.T) {
	book := excelize.NewFile()
	defer book.Close()
	if _, err := book.NewSheet("Second"); err != nil {
		t.Fatal(err)
	}
	if err := book.SetSheetRow("Sheet1", "A1", &[]interface{}{1, 2, 3, 4}); err != nil {
		t.Fatal(err)
	}
	for row := 1; row <= 4; row++ {
		if err := book.SetCellValue("Second", "A"+strconv.Itoa(row), row); err != nil {
			t.Fatal(err)
		}
	}
	buffer, err := book.WriteToBuffer()
	if err != nil {
		t.Fatal(err)
	}
	p, err := opc.Read(buffer.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := EditSparklines(p, SparklineSpec{Sheet: "Sheet1", Sparklines: []SparklineItem{
		{Cell: "F1", Range: "Sheet1!A1:D1"},
		{Cell: "F2", Range: "Second!A1:A4"},
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := EditSparklines(p, SparklineSpec{Sheet: "Second", Sparklines: []SparklineItem{
		{Cell: "C1", Range: "Second!A1:A4"},
	}}); err != nil {
		t.Fatal(err)
	}
	setSparklineTestColumns(t, p, "Sheet1", []sparklineTestColumn{
		{min: 1, max: 2, width: "10"},
		{min: 3, max: 4, width: "12", hidden: "1"},
		{min: 6, max: 6, width: "20"},
	})
	setSparklineTestColumns(t, p, "Second", []sparklineTestColumn{
		{min: 1, max: 1, width: "11"},
		{min: 3, max: 3, width: "30"},
	})
	source := filepath.Join(t.TempDir(), "two-sheets.xlsx")
	if err := p.SaveNew(source); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "two-sheets-preview.xlsx")
	if changed, err := OfficePreviewCopy(source, output); err != nil || !changed {
		t.Fatalf("OfficePreviewCopy changed=%v err=%v", changed, err)
	}
	preview := previewOpen(t, output)
	for _, tc := range []struct {
		drawing string
		counts  []int
		widths  []int
	}{
		{"xl/drawings/preview1.xml", []int{2, 4}, []int{140 * emuPerPixel, 140 * emuPerPixel}},
		{"xl/drawings/preview2.xml", []int{4}, []int{210 * emuPerPixel}},
	} {
		drawing := previewXML(t, preview, tc.drawing)
		shapes := All(drawing, NSXDR, "sp")
		if len(shapes) != len(tc.counts) {
			t.Fatalf("%s shapes=%d want %d", tc.drawing, len(shapes), len(tc.counts))
		}
		for i, shape := range shapes {
			if points := len(All(shape, NSA, "pt")); points != tc.counts[i] {
				t.Errorf("%s shape %d points=%d want %d", tc.drawing, i, points, tc.counts[i])
			}
			ext := Child(shape.Parent(), NSXDR, "ext")
			width, err := strconv.Atoi(ext.SelectAttrValue("cx", ""))
			if err != nil || width != tc.widths[i] {
				t.Errorf("%s shape %d width=%d err=%v want %d", tc.drawing, i, width, err, tc.widths[i])
			}
		}
	}
}

type sparklineTestColumn struct {
	min, max int
	width    string
	hidden   string
}

func setSparklineTestColumns(t *testing.T, p *opc.Package, sheet string, definitions []sparklineTestColumn) {
	t.Helper()
	_, _, byName, err := WorkbookSheets(p)
	if err != nil {
		t.Fatal(err)
	}
	part := byName[sheet]
	doc, err := p.XML(part)
	if err != nil {
		t.Fatal(err)
	}
	root := doc.Root()
	if existing := Child(root, NSS, "cols"); existing != nil {
		root.RemoveChild(existing)
	}
	cols := newNS(root, NSS, "cols")
	for _, definition := range definitions {
		col := newNS(cols, NSS, "col")
		col.CreateAttr("min", strconv.Itoa(definition.min))
		col.CreateAttr("max", strconv.Itoa(definition.max))
		col.CreateAttr("width", definition.width)
		col.CreateAttr("customWidth", "1")
		if definition.hidden != "" {
			col.CreateAttr("hidden", definition.hidden)
		}
		cols.AddChild(col)
	}
	if sheetData := Child(root, NSS, "sheetData"); sheetData != nil {
		root.InsertChildAt(sheetData.Index(), cols)
	} else {
		root.AddChild(cols)
	}
	if err := p.SetXML(part, doc); err != nil {
		t.Fatal(err)
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
