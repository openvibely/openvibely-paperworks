package office

import "testing"

func TestEmbeddedTableStyleRegions(t *testing.T) {
	styles := paintFixture(t, `<a:tblStyleLst xmlns:a="`+NSA+`" def="s"><a:tblStyle styleId="s"><a:wholeTbl><a:tcTxStyle b="1"/><a:tcStyle><a:fill><a:solidFill><a:srgbClr val="112233"/></a:solidFill></a:fill></a:tcStyle></a:wholeTbl><a:firstRow><a:tcTxStyle i="1"/><a:tcStyle><a:fill><a:solidFill><a:srgbClr val="445566"/></a:solidFill></a:fill></a:tcStyle></a:firstRow></a:tblStyle></a:tblStyleLst>`)
	table := paintFixture(t, `<a:tbl xmlns:a="`+NSA+`"><a:tblPr firstRow="1"/><a:tr><a:tc><a:txBody><a:p><a:r><a:rPr b="0"/><a:t>Title</a:t></a:r></a:p></a:txBody></a:tc></a:tr></a:tbl>`)
	regions, id, found := layoutTableRegions(table, styles, 0, 0, 2, 2)
	if !found || id != "s" || len(regions) != 2 {
		t.Fatal(id, found, regions)
	}
	cell, defs := layoutTableStyleCell(Child(Child(table, NSA, "tr"), NSA, "tc"), regions)
	f := layoutTableFormatting(cell, newLayoutPalette(nil, nil), defs...)
	if f["fill"].(map[string]any)["color"].(map[string]any)["rgb"] != "#445566" {
		t.Fatal(f)
	}
	props := f["paragraphs"].([]map[string]any)[0]["runs"].([]map[string]any)[0]["style"].(map[string]any)["properties"].(map[string]string)
	if props["b"] != "0" || props["i"] != "1" {
		t.Fatal(props)
	}
	if Child(Child(Child(table, NSA, "tr"), NSA, "tc"), NSA, "tcPr") != nil {
		t.Fatal("source mutated")
	}
}
