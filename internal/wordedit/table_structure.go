package wordedit

import (
	"fmt"
	"github.com/beevik/etree"
	"github.com/openvibely/openvibely-paperworks/internal/office"
	"strconv"
)

func tableStructure(table *etree.Element, s map[string]any) error {
	rows := office.Children(table, nsW, "tr")
	op := str(s, "operation")
	col, ok := intArg(s["column"])
	if !ok || col < 0 {
		return fmt.Errorf("column required")
	}
	grid := child(table, "tblGrid")
	if grid == nil {
		return fmt.Errorf("table has no grid")
	}
	cols := office.Children(grid, nsW, "gridCol")
	if col >= len(cols) {
		return fmt.Errorf("column out of range")
	}
	if op == "table-merge" || op == "table-unmerge" {
		row, ok := intArg(s["row"])
		if !ok || row < 0 || row >= len(rows) {
			return fmt.Errorf("row out of range")
		}
		cells := office.Children(rows[row], nsW, "tc")
		if col >= len(cells) {
			return fmt.Errorf("cell index out of range")
		}
		cell := cells[col]
		if len(all(rows[row], "vMerge")) > 0 {
			return fmt.Errorf("vertical merges require explicit OOXML editing")
		}
		if op == "table-unmerge" {
			span := child(child(cell, "tcPr"), "gridSpan")
			if span == nil {
				return fmt.Errorf("cell is not horizontally merged")
			}
			n, err := strconv.Atoi(getW(span, "val"))
			if err != nil || n < 2 || n > len(cols) {
				return fmt.Errorf("invalid grid span")
			}
			remove(span)
			for i := 1; i < n; i++ {
				c := W("tc")
				if pr := child(cell, "tcPr"); pr != nil {
					c.AddChild(pr.Copy())
				}
				c.AddChild(W("p"))
				rows[row].InsertChildAt(cell.Index()+i, c)
			}
			return nil
		}
		end, ok := intArg(s["end_column"])
		if !ok || end <= col || end >= len(cells) {
			return fmt.Errorf("end_column must follow column")
		}
		if len(all(rows[row], "gridSpan")) > 0 {
			return fmt.Errorf("unmerge row cells before merging")
		}
		pr := property(cell, "tcPr")
		span := W("gridSpan")
		setW(span, "val", strconv.Itoa(end-col+1))
		insertWordProperty(pr, span)
		for i := col + 1; i <= end; i++ {
			for _, c := range cells[i].ChildElements() {
				if !is(c, "tcPr") {
					cell.AddChild(c.Copy())
				}
			}
			remove(cells[i])
		}
		return nil
	}
	// Inserting or deleting a physical grid column in merged tables requires
	// restructuring the spans; fail rather than silently corrupting the grid.
	for _, r := range rows {
		if len(office.Children(r, nsW, "tc")) != len(cols) || len(all(r, "gridSpan"))+len(all(r, "vMerge")) > 0 {
			return fmt.Errorf("column structure edits require an unmerged rectangular table")
		}
	}
	switch op {
	case "table-column-insert":
		grid.InsertChildAt(cols[col].Index(), cols[col].Copy())
		for _, r := range rows {
			cells := office.Children(r, nsW, "tc")
			c := W("tc")
			if pr := child(cells[col], "tcPr"); pr != nil {
				c.AddChild(pr.Copy())
			}
			c.AddChild(W("p"))
			r.InsertChildAt(cells[col].Index(), c)
		}
	case "table-column-delete":
		if len(cols) <= 1 {
			return fmt.Errorf("cannot delete last column")
		}
		remove(cols[col])
		for _, r := range rows {
			remove(office.Children(r, nsW, "tc")[col])
		}
	default:
		return fmt.Errorf("unsupported table structure operation")
	}
	return nil
}
