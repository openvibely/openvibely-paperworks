package office

import (
	"archive/zip"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/beevik/etree"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"github.com/xuri/excelize/v2"
)

func previewBenchmarkChartXML() string {
	var xml strings.Builder
	xml.WriteString(`<c:chartSpace xmlns:c="http://schemas.openxmlformats.org/drawingml/2006/chart"><c:chart><c:plotArea><c:pieChart>`)
	for series := 0; series < 8; series++ {
		_, _ = fmt.Fprintf(&xml, `<c:ser><c:idx val="%d"/><c:order val="%d"/><c:tx><c:v>Series %d</c:v></c:tx><c:cat><c:strRef><c:f>Sheet1!$A$2:$A$17</c:f><c:strCache><c:ptCount val="16"/>`, series, series, series)
		for point := 0; point < 16; point++ {
			_, _ = fmt.Fprintf(&xml, `<c:pt idx="%d"><c:v>Category %d</c:v></c:pt>`, point, point)
		}
		xml.WriteString(`</c:strCache></c:strRef></c:cat><c:val><c:numRef><c:f>Sheet1!$B$2:$B$17</c:f><c:numCache><c:formatCode>General</c:formatCode><c:ptCount val="16"/>`)
		for point := 0; point < 16; point++ {
			_, _ = fmt.Fprintf(&xml, `<c:pt idx="%d"><c:v>%d</c:v></c:pt>`, point, (series+1)*(point+1))
		}
		xml.WriteString(`</c:numCache></c:numRef></c:val></c:ser>`)
	}
	xml.WriteString(`</c:pieChart></c:plotArea></c:chart></c:chartSpace>`)
	return xml.String()
}

func writePreviewPerfPackage(dir, extension string, chartCount, unrelatedCount int, chartXML string) (string, error) {
	if chartXML == "" {
		chartXML = previewBenchmarkChartXML()
	}
	name := "preview" + extension
	file, err := os.Create(filepath.Join(dir, name))
	if err != nil {
		return "", err
	}
	zw := zip.NewWriter(file)
	write := func(part, content string) error {
		w, err := zw.Create(part)
		if err != nil {
			return err
		}
		_, err = w.Write([]byte(content))
		return err
	}
	var types strings.Builder
	_, _ = fmt.Fprintf(&types, `<Types xmlns="%s"><Default Extension="xml" ContentType="application/xml"/>`, opc.NSCT)
	for i := 1; i <= chartCount; i++ {
		_, _ = fmt.Fprintf(&types, `<Override PartName="/xl/charts/chart%d.xml" ContentType="%s"/>`, i, chartMimes[0])
	}
	types.WriteString(`</Types>`)
	if err = write("[Content_Types].xml", types.String()); err == nil {
		switch extension {
		case ".pptx":
			err = write("ppt/presentation.xml", `<p:presentation xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main"><p:sldIdLst/></p:presentation>`)
		case ".xlsx":
			err = write("xl/workbook.xml", `<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheets/></workbook>`)
		}
	}
	for i := 1; err == nil && i <= chartCount; i++ {
		err = write(fmt.Sprintf("xl/charts/chart%d.xml", i), chartXML)
	}
	for i := 0; err == nil && i < unrelatedCount; i++ {
		// Distinct payloads keep the package representative without making the
		// fixture's repeated contents collapse to a trivial ZIP stream.
		payload := fmt.Sprintf("part=%04d;", i) + strings.Repeat("unrelated office package data ", 12)
		err = write(fmt.Sprintf("custom/part%04d.bin", i), payload)
	}
	closeErr := zw.Close()
	fileErr := file.Close()
	if err != nil {
		return "", err
	}
	if closeErr != nil {
		return "", closeErr
	}
	if fileErr != nil {
		return "", fileErr
	}
	return filepath.Join(dir, name), nil
}

func TestOfficePreviewGeneratedPackageBehavior(t *testing.T) {
	for _, ext := range []string{".xlsx", ".pptx"} {
		t.Run(ext, func(t *testing.T) {
			dir := t.TempDir()
			source, err := writePreviewPerfPackage(dir, ext, 1, 12, "")
			if err != nil {
				t.Fatal(err)
			}
			if need, err := NeedsOfficePreview(source); err != nil || !need {
				t.Fatalf("NeedsOfficePreview = %v, %v; want true", need, err)
			}
			output := filepath.Join(dir, "fallback"+ext)
			if changed, err := OfficePreviewCopy(source, output); err != nil || !changed {
				t.Fatalf("OfficePreviewCopy = %v, %v; want changed", changed, err)
			}
			p, err := opc.Open(output)
			if err != nil {
				t.Fatal(err)
			}
			parts, err := ChartParts(p)
			if err != nil || len(parts) != 1 {
				t.Fatalf("output charts = %v, %v", parts, err)
			}
			doc, err := p.XML(parts[0])
			if err != nil {
				t.Fatal(err)
			}
			if Child(Child(doc.Root(), NSC, "chart"), NSC, "plotArea") == nil || len(All(doc.Root(), NSC, "varyColors")) != 1 {
				t.Fatal("preview chart styling missing")
			}
		})
	}
}

func TestOfficePreviewGeneratedNoFallbackDoesNotWrite(t *testing.T) {
	for _, ext := range []string{".xlsx", ".pptx"} {
		t.Run(ext, func(t *testing.T) {
			dir := t.TempDir()
			source, err := writePreviewPerfPackage(dir, ext, 0, 12, "")
			if err != nil {
				t.Fatal(err)
			}
			if need, err := NeedsOfficePreview(source); err != nil || need {
				t.Fatalf("NeedsOfficePreview = %v, %v; want false", need, err)
			}
			output := filepath.Join(dir, "must-not-exist"+ext)
			if changed, err := OfficePreviewCopy(source, output); err != nil || changed {
				t.Fatalf("OfficePreviewCopy = %v, %v; want unchanged", changed, err)
			}
			if _, err := os.Stat(output); !os.IsNotExist(err) {
				t.Fatalf("no-fallback output exists or stat failed: %v", err)
			}
		})
	}
}

func TestOfficePreviewMalformedChartStillReturnsError(t *testing.T) {
	for _, ext := range []string{".xlsx", ".pptx"} {
		t.Run(ext, func(t *testing.T) {
			dir := t.TempDir()
			source, err := writePreviewPerfPackage(dir, ext, 1, 0, `<c:chartSpace xmlns:c="http://schemas.openxmlformats.org/drawingml/2006/chart"><broken></c:chartSpace>`)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := NeedsOfficePreview(source); err == nil || !strings.Contains(err.Error(), "chart1.xml") {
				t.Fatalf("NeedsOfficePreview error = %v; want malformed chart error", err)
			}
			output := filepath.Join(dir, "invalid-output"+ext)
			if _, err := OfficePreviewCopy(source, output); err == nil || !strings.Contains(err.Error(), "chart1.xml") {
				t.Fatalf("OfficePreviewCopy error = %v; want malformed chart error", err)
			}
			if _, err := os.Stat(output); !os.IsNotExist(err) {
				t.Fatalf("malformed input output exists or stat failed: %v", err)
			}
		})
	}
}

func BenchmarkNeedsOfficePreview(b *testing.B) {
	benchmarkOfficePreview(b, false)
}

func BenchmarkOfficePreviewCopy(b *testing.B) {
	benchmarkOfficePreview(b, true)
}

func benchmarkOfficePreview(b *testing.B, copyPreview bool) {
	for _, ext := range []string{".xlsx", ".pptx"} {
		for _, charts := range []int{0, 1, 10, 20} {
			label := fmt.Sprintf("%s/charts-%02d/unrelated-128", strings.TrimPrefix(ext, "."), charts)
			b.Run(label, func(b *testing.B) {
				dir := b.TempDir()
				source, err := writePreviewPerfPackage(dir, ext, charts, 128, "")
				if err != nil {
					b.Fatal(err)
				}
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					if !copyPreview {
						if _, err := NeedsOfficePreview(source); err != nil {
							b.Fatal(err)
						}
						continue
					}
					output := filepath.Join(dir, fmt.Sprintf("out-%d%s", i, ext))
					changed, err := OfficePreviewCopy(source, output)
					if err != nil {
						b.Fatal(err)
					}
					if (charts > 0) != changed {
						b.Fatalf("OfficePreviewCopy changed=%v for %d charts", changed, charts)
					}
					if changed {
						b.StopTimer()
						if err := os.Remove(output); err != nil {
							b.Fatal(err)
						}
						b.StartTimer()
					}
				}
			})
		}
	}
}

func BenchmarkOfficePreviewCopySparklineColumnRanges(b *testing.B) {
	for _, ranges := range []int{1, 32, 128} {
		b.Run(fmt.Sprintf("ranges-%03d/points-10000", ranges), func(b *testing.B) {
			dir := b.TempDir()
			source, err := sparklineColumnBenchmarkBook(dir, 10000, ranges)
			if err != nil {
				b.Fatal(err)
			}
			output := filepath.Join(dir, "preview.xlsx")
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				changed, err := OfficePreviewCopy(source, output)
				if err != nil || !changed {
					b.Fatalf("OfficePreviewCopy = %v, %v; want changed", changed, err)
				}
				b.StopTimer()
				if err := os.Remove(output); err != nil {
					b.Fatal(err)
				}
				b.StartTimer()
			}
		})
	}
}

func sparklineColumnBenchmarkBook(dir string, points, ranges int) (string, error) {
	book := excelize.NewFile()
	defer book.Close()
	row := make([]interface{}, points)
	for i := range row {
		row[i] = i + 1
	}
	if err := book.SetSheetRow("Sheet1", "A1", &row); err != nil {
		return "", err
	}
	buffer, err := book.WriteToBuffer()
	if err != nil {
		return "", err
	}
	p, err := opc.Read(buffer.Bytes())
	if err != nil {
		return "", err
	}
	item := SparklineItem{Cell: "A2", Range: "Sheet1!A1:" + ColumnName(points) + "1"}
	if _, err := EditSparklines(p, SparklineSpec{Sheet: "Sheet1", Sparklines: []SparklineItem{item}}); err != nil {
		return "", err
	}
	_, _, byName, err := WorkbookSheets(p)
	if err != nil {
		return "", err
	}
	part := byName["Sheet1"]
	doc, err := p.XML(part)
	if err != nil {
		return "", err
	}
	root := doc.Root()
	if existing := Child(root, NSS, "cols"); existing != nil {
		root.RemoveChild(existing)
	}
	cols := etree.NewElement("cols")
	for i := 0; i < ranges; i++ {
		lo := i*points/ranges + 1
		hi := (i + 1) * points / ranges
		col := cols.CreateElement("col")
		col.CreateAttr("min", fmt.Sprint(lo))
		col.CreateAttr("max", fmt.Sprint(hi))
		col.CreateAttr("width", fmt.Sprint(8.43+float64(i%3)/10))
		col.CreateAttr("customWidth", "1")
	}
	if sheetData := Child(root, NSS, "sheetData"); sheetData != nil {
		root.InsertChildAt(sheetData.Index(), cols)
	} else {
		root.AddChild(cols)
	}
	if err := p.SetXML(part, doc); err != nil {
		return "", err
	}
	path := filepath.Join(dir, "sparkline-columns.xlsx")
	if err := p.SaveNew(path); err != nil {
		return "", err
	}
	return path, nil
}
