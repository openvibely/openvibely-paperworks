package pdf

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

func benchmarkFillPDF(pages, widgetsPerPage, targetBytes int) ([]byte, map[string]string) {
	widgetCount := pages * widgetsPerPage
	objs := make([]string, 2, 2+pages+widgetCount+3)
	objs[0] = ""
	objs[1] = ""
	pageStart := 3
	fieldStart := pageStart + pages
	fontObj := fieldStart + widgetCount
	appearanceObj := fontObj + 1
	contentObj := appearanceObj + 1

	fields := make([]string, widgetCount)
	values := make(map[string]string, widgetCount)
	for i := 0; i < widgetCount; i++ {
		fields[i] = fmt.Sprintf("%d 0 R", fieldStart+i)
		name := fmt.Sprintf("field_%04d", i)
		values[name] = "Updated"
	}
	objs[0] = fmt.Sprintf("<< /Type /Catalog /Pages 2 0 R /AcroForm << /Fields [%s] /DR << /Font << /Helv %d 0 R >> >> /DA (/Helv 10 Tf 0 g) >> >>", strings.Join(fields, " "), fontObj)
	pageRefs := make([]string, pages)
	for p := 0; p < pages; p++ {
		pageRefs[p] = fmt.Sprintf("%d 0 R", pageStart+p)
	}
	objs[1] = fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", strings.Join(pageRefs, " "), pages)
	for p := 0; p < pages; p++ {
		var annots []string
		for w := 0; w < widgetsPerPage; w++ {
			i := p*widgetsPerPage + w
			annots = append(annots, fmt.Sprintf("%d 0 R", fieldStart+i))
		}
		objs = append(objs, fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /Helv %d 0 R >> >> /Contents %d 0 R /Annots [%s] >>", fontObj, contentObj, strings.Join(annots, " ")))
	}
	for i := 0; i < widgetCount; i++ {
		pageObj := pageStart + i/widgetsPerPage
		name := fmt.Sprintf("field_%04d", i)
		y := 740 - (i%widgetsPerPage)*30
		objs = append(objs, fmt.Sprintf("<< /Type /Annot /Subtype /Widget /FT /Tx /T (%s) /V (Original) /DA (/Helv 10 Tf 0 g) /Rect [40 %d 240 %d] /P %d 0 R /AP << /N %d 0 R >> >>", name, y, y+20, pageObj, appearanceObj))
	}
	objs = append(objs,
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>",
		stream("/Type /XObject /Subtype /Form /BBox [0 0 200 20] /Resources << /Font << /Helv "+fmt.Sprintf("%d 0 R", fontObj)+" >> >>", "BT /Helv 10 Tf 2 5 Td (Original) Tj ET"),
		stream("", "q Q"),
	)
	data := rawPDF(objs)
	padding := targetBytes - len(data) - 1
	if padding > 0 {
		objs[len(objs)-1] = stream("", strings.Repeat("%", padding)+"\nq Q")
		data = rawPDF(objs)
	}
	return data, values
}

func BenchmarkFill(b *testing.B) {
	cases := []struct {
		name                 string
		pages, widgets, size int
	}{
		{name: "1MiB_4pages_16widgets", pages: 4, widgets: 4, size: 1 << 20},
		{name: "20MiB_64pages_256widgets", pages: 64, widgets: 4, size: 20 << 20},
	}
	for _, tc := range cases {
		data, values := benchmarkFillPDF(tc.pages, tc.widgets, tc.size)
		for _, flatten := range []bool{false, true} {
			mode := "editable"
			parses := 2.0
			serializations := 1.0
			if flatten {
				mode = "flattened"
				// The source and final output are parsed once each, with one final
				// serialization. Form consistency checks use the in-memory context.
				parses, serializations = 2, 1
			}
			b.Run(tc.name+"_"+mode, func(b *testing.B) {
				dir := b.TempDir()
				source := filepath.Join(dir, "source.pdf")
				if err := os.WriteFile(source, data, 0o600); err != nil {
					b.Fatal(err)
				}
				b.SetBytes(int64(len(data)))
				b.ReportAllocs()
				samples := make([]time.Duration, b.N)
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					dest := filepath.Join(dir, fmt.Sprintf("out-%d.pdf", i))
					started := time.Now()
					if _, err := Fill(context.Background(), source, dest, values, flatten); err != nil {
						b.Fatal(err)
					}
					samples[i] = time.Since(started)
					if err := os.Remove(dest); err != nil {
						b.Fatal(err)
					}
				}
				b.StopTimer()
				sort.Slice(samples, func(i, j int) bool { return samples[i] < samples[j] })
				median := samples[len(samples)/2]
				if len(samples)%2 == 0 {
					median = (samples[len(samples)/2-1] + samples[len(samples)/2]) / 2
				}
				b.ReportMetric(float64(median.Nanoseconds()), "median-ns/op")
				b.ReportMetric(parses, "pdf-parses/op")
				b.ReportMetric(serializations, "pdf-serializations/op")
				if peakRSS := benchmarkPeakRSSBytes(); peakRSS > 0 {
					b.ReportMetric(peakRSS/(1024*1024), "peak-rss-MiB")
				}
			})
		}
	}
}
