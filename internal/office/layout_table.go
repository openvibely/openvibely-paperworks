package office

import (
	"fmt"
	"github.com/beevik/etree"
	"strings"
)

func layoutTable(record map[string]any, table *etree.Element) {
	layoutStyledTable(record, table, newLayoutPalette(nil, nil))
}
func layoutStyledTable(record map[string]any, table *etree.Element, palette layoutPalette) {
	if table == nil {
		return
	}
	var cells []map[string]any
	var widths, heights []float64
	for _, col := range Children(Child(table, NSA, "tblGrid"), NSA, "gridCol") {
		widths = append(widths, emuAttr(col, "w", 0)/9525)
	}
	for ri, row := range Children(table, NSA, "tr") {
		heights = append(heights, emuAttr(row, "h", 0)/9525)
		for ci, cell := range Children(row, NSA, "tc") {
			id := fmt.Sprintf("%s/cell/%d/%d", record["id"], ri+1, ci+1)
			var paragraphs []string
			for _, p := range Children(Child(cell, NSA, "txBody"), NSA, "p") {
				paragraphs = append(paragraphs, paraText(p))
			}
			c := map[string]any{"id": id, "table_id": record["id"], "row": ri + 1, "column": ci + 1, "text": strings.Join(paragraphs, "\n"), "row_span": emuAttr(cell, "rowSpan", 1), "column_span": emuAttr(cell, "gridSpan", 1), "horizontal_merge": Truthy(AttrOr(cell, "hMerge", "0"), false), "vertical_merge": Truthy(AttrOr(cell, "vMerge", "0"), false), "properties_xml": layoutXML(Child(cell, NSA, "tcPr")), "text_xml": layoutXML(Child(cell, NSA, "txBody")), "owned_element_ids": []string{}}
			regions, styleID, found := layoutTableRegions(table, palette.tableStyles, ri, ci, len(Children(table, NSA, "tr")), len(widths))
			effective, defaults := layoutTableStyleCell(cell, regions)
			formatting := layoutTableFormatting(effective, palette, defaults...)
			formatting["table_style_id"] = styleID
			formatting["table_style_found"] = found
			formatting["style_scope"] = "explicit cell, local text defaults, and embedded table-style regions; renderer defaults excluded"
			c["formatting"] = formatting
			cells = append(cells, c)
		}
	}
	record["table_properties"] = layoutAttributes(Child(table, NSA, "tblPr"))
	record["table_style_id"] = Text(Child(table, NSA, "tblPr"), NSA, "tableStyleId")
	record["cells"] = cells
	record["column_widths"] = widths
	record["row_heights"] = heights
}
