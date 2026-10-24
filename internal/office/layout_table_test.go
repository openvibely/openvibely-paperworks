package office

import (
	"github.com/beevik/etree"
	"testing"
)

func TestLayoutTableCells(t *testing.T) {
	d := etree.NewDocument()
	if err := d.ReadFromString(`<a:tbl xmlns:a="` + NSA + `"><a:tblGrid><a:gridCol w="952500"/></a:tblGrid><a:tr h="190500"><a:tc gridSpan="2"><a:txBody><a:p><a:r><a:t>Hello</a:t></a:r></a:p></a:txBody></a:tc><a:tc hMerge="1"/></a:tr></a:tbl>`); err != nil {
		t.Fatal(err)
	}
	r := map[string]any{"id": "slide#2"}
	layoutTable(r, d.Root())
	c := r["cells"].([]map[string]any)
	if len(c) != 2 || c[0]["text"] != "Hello" || c[0]["table_id"] != "slide#2" || c[1]["horizontal_merge"] != true || c[0]["column_span"] != float64(2) {
		t.Fatal(r)
	}
	if r["column_widths"].([]float64)[0] != 100 || r["row_heights"].([]float64)[0] != 20 {
		t.Fatal(r)
	}
}
