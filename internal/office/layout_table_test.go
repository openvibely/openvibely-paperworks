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

func TestTableStructuredFormatting(t *testing.T) {
	cell := paintFixture(t, `<a:tc xmlns:a="`+NSA+`"><a:txBody><a:lstStyle><a:lvl1pPr><a:defRPr sz="1800" b="1"/></a:lvl1pPr></a:lstStyle><a:p><a:pPr algn="ctr"><a:defRPr i="1"/></a:pPr><a:r><a:rPr b="0"><a:solidFill><a:srgbClr val="ABCDEF"/></a:solidFill></a:rPr><a:t>Hi</a:t></a:r><a:br/></a:p></a:txBody><a:tcPr marL="91440"><a:lnL w="12700"><a:solidFill><a:srgbClr val="123456"/></a:solidFill></a:lnL></a:tcPr></a:tc>`)
	f := layoutTableFormatting(cell, newLayoutPalette(nil, nil))
	p := f["paragraphs"].([]map[string]any)[0]
	r := p["runs"].([]map[string]any)
	props := r[0]["style"].(map[string]any)["properties"].(map[string]string)
	if props["b"] != "0" || props["i"] != "1" || props["sz"] != "1800" || r[1]["text"] != "\n" {
		t.Fatal(f)
	}
	if f["properties"].(map[string]string)["marL"] != "91440" || p["properties"].(map[string]string)["algn"] != "ctr" {
		t.Fatal(f)
	}
	if f["borders"].(map[string]any)["lnL"] == nil {
		t.Fatal(f)
	}
}
