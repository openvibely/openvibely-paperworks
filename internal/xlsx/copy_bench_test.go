package xlsx

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/xuri/excelize/v2"
)

func largeCopyWorkbook(b *testing.B, dir string) string {
	b.Helper()
	f := excelize.NewFile()
	defer f.Close()
	if _, err := f.NewSheet("Sheet2"); err != nil {
		b.Fatal(err)
	}
	for _, sheet := range []string{"Sheet1", "Sheet2"} {
		sw, err := f.NewStreamWriter(sheet)
		if err != nil {
			b.Fatal(err)
		}
		for row := 1; row <= 50000; row++ {
			cell, _ := excelize.CoordinatesToCellName(1, row)
			if err = sw.SetRow(cell, []any{row, row * 2, "text", float64(row) / 3}); err != nil {
				b.Fatal(err)
			}
		}
		if err = sw.Flush(); err != nil {
			b.Fatal(err)
		}
	}
	path := filepath.Join(dir, "large.xlsx")
	if err := f.SaveAs(path); err != nil {
		b.Fatal(err)
	}
	return path
}

func BenchmarkEditFileCopyLargeWorkbook(b *testing.B) {
	dir := b.TempDir()
	in := largeCopyWorkbook(b, dir)
	out := filepath.Join(dir, "out.xlsx")
	for _, operation := range []string{"value", "copy", "copy-values", "copy-formulas", "copy-formats"} {
		for _, n := range []int{1, 10} {
			edits := make([]Edit, n)
			for i := range edits {
				target, _ := excelize.CoordinatesToCellName(6, 2*i+1)
				edits[i] = Edit{Operation: operation, Sheet: "Sheet1", Range: "A1:B2", Target: target}
				if operation == "value" {
					edits[i] = Edit{Operation: operation, Sheet: "Sheet1", Cell: target, Value: i}
				}
			}
			b.Run(fmt.Sprintf("%s/%d", operation, n), func(b *testing.B) {
				b.ReportAllocs()
				for range b.N {
					b.StopTimer()
					_ = os.Remove(out)
					b.StartTimer()
					if err := EditFile(context.Background(), in, out, dir, EditSpec{Edits: edits}); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}
