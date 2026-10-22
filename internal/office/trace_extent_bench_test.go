package office

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/beevik/etree"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"github.com/xuri/excelize/v2"
)

// Construction and ZIP loading are deliberately outside the timed regions.
func extentBenchmarkPackage(b testing.TB, rows, k int) *opc.Package {
	b.Helper()
	f := excelize.NewFile()
	defer f.Close()
	if err := f.SetSheetName("Sheet1", "Inputs"); err != nil {
		b.Fatal(err)
	}
	if _, err := f.NewSheet("Output"); err != nil {
		b.Fatal(err)
	}
	raw, err := f.WriteToBuffer()
	if err != nil {
		b.Fatal(err)
	}
	p, err := opc.Read(raw.Bytes())
	if err != nil {
		b.Fatal(err)
	}
	var xml strings.Builder
	xml.WriteString(`<worksheet xmlns="` + NSS + `"><sheetData>`)
	for row := 1; row <= rows; row++ {
		fmt.Fprintf(&xml, `<row r="%d">`, row)
		for col := 1; col <= 100; col++ {
			fmt.Fprintf(&xml, `<c r="%s%d"><v>1</v></c>`, ColumnName(col), row)
		}
		xml.WriteString(`</row>`)
	}
	xml.WriteString(`</sheetData></worksheet>`)
	p.Set("xl/worksheets/sheet1.xml", []byte(xml.String()))
	refs := make([]string, k)
	for i := range refs {
		col := ColumnName(i + 1)
		refs[i] = "Inputs!" + col + ":" + col
	}
	p.Set("xl/worksheets/sheet2.xml", []byte(`<worksheet xmlns="`+NSS+`"><sheetData><row r="1"><c r="A1"><f>SUM(`+strings.Join(refs, ",")+`)</f></c></row></sheetData></worksheet>`))
	return p
}

func BenchmarkTraceExtent(b *testing.B) {
	for _, rows := range []int{1000, 10000} {
		for _, k := range []int{1, 10, 100} {
			b.Run(fmt.Sprintf("rows=%d/K=%d", rows, k), func(b *testing.B) {
				p := extentBenchmarkPackage(b, rows, k)
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					if _, err := TraceWorkbook(context.Background(), p, "Output", "A1", 200, 20); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}

func BenchmarkSavedExtent(b *testing.B) {
	for _, rows := range []int{1000, 10000} {
		for _, k := range []int{1, 10, 100} {
			b.Run(fmt.Sprintf("rows=%d/K=%d", rows, k), func(b *testing.B) {
				p := extentBenchmarkPackage(b, rows, k)
				wb, err := OpenWorkbook(p)
				if err != nil {
					b.Fatal(err)
				}
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					tr := &tracer{ctx: context.Background(), wb: wb}
					for col := 1; col <= k; col++ {
						if v, ok := tr.savedExtent(traceExtentKey{"Inputs", true, col, col}); !ok || v != [2]int{1, rows} {
							b.Fatal(v, ok)
						}
					}
				}
			})
		}
	}
}

// Optional fixture export allows identical CLI inputs before and after changes.
func TestExportExtentBenchmark(t *testing.T) {
	dir := os.Getenv("PAPERWORKS_EXTENT_FIXTURES")
	if dir == "" {
		t.Skip("benchmark fixture export only")
	}
	for _, rows := range []int{1000, 10000} {
		for _, k := range []int{1, 10, 100} {
			p := extentBenchmarkPackage(t, rows, k)
			if err := p.SaveNew(filepath.Join(dir, fmt.Sprintf("rows-%d-k-%d.xlsx", rows, k))); err != nil {
				t.Fatal(err)
			}
		}
	}
}

// Sparse coordinates at Excel's limits must not allocate dense axis arrays.
func BenchmarkSavedExtentSparse(b *testing.B) {
	d := etree.NewDocument()
	if err := d.ReadFromString(`<c xmlns="` + NSS + `"><v>1</v></c>`); err != nil {
		b.Fatal(err)
	}
	wb := &Workbook{Sheets: map[string]*Sheet{"Inputs": {Cells: map[string]*etree.Element{"A1": d.Root(), "XFD1048576": d.Root()}}}}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tr := &tracer{ctx: context.Background(), wb: wb}
		if _, ok := tr.savedExtent(traceExtentKey{"Inputs", true, 1, 16384}); !ok {
			b.Fatal("failed")
		}
	}
}
