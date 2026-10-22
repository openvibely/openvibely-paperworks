package office

import (
	"context"
	"os"
	"runtime"
	"testing"

	"github.com/beevik/etree"
)

// Optional retained-heap measurement, separate from benchmark B/op (which
// includes temporary map growth). Keep ten indexes live to dilute GC noise.
func TestTraceExtentIndexMemory(t *testing.T) {
	if os.Getenv("PAPERWORKS_EXTENT_MEMORY") == "" {
		t.Skip("optional heap measurement")
	}
	for _, rows := range []int{0, 1000, 10000} {
		var wb *Workbook
		if rows == 0 {
			d := etree.NewDocument()
			if err := d.ReadFromString(`<c xmlns="` + NSS + `"><v>1</v></c>`); err != nil {
				t.Fatal(err)
			}
			wb = &Workbook{Sheets: map[string]*Sheet{"Inputs": {Cells: map[string]*etree.Element{"A1": d.Root(), "XFD1048576": d.Root()}}}}
		} else {
			var err error
			wb, err = OpenWorkbook(extentBenchmarkPackage(t, rows, 1))
			if err != nil {
				t.Fatal(err)
			}
		}
		indexes := make([]*tracer, 10)
		var before, after runtime.MemStats
		runtime.GC()
		runtime.ReadMemStats(&before)
		for i := range indexes {
			indexes[i] = &tracer{ctx: context.Background(), wb: wb}
			if _, ok := indexes[i].savedExtent(traceExtentKey{"Inputs", true, 1, 16384}); !ok {
				t.Fatal("failed")
			}
		}
		runtime.GC()
		runtime.ReadMemStats(&after)
		t.Logf("rows=%d retained bytes/index=%d (10 live indexes; includes tracer and cache)", rows, (int64(after.HeapAlloc)-int64(before.HeapAlloc))/10)
		runtime.KeepAlive(indexes)
		runtime.KeepAlive(wb)
	}
}
