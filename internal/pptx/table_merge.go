package pptx

import (
	"fmt"
	"github.com/beevik/etree"
	"github.com/openvibely/openvibely-paperworks/internal/office"
	"strconv"
)

func mergedContinuation(c *etree.Element) bool {
	for _, k := range []string{"hMerge", "vMerge"} {
		v := c.SelectAttrValue(k, "")
		if v == "1" || v == "true" {
			return true
		}
	}
	return false
}
func cellSpan(c *etree.Element, k string) (int, error) {
	n, err := strconv.Atoi(c.SelectAttrValue(k, "1"))
	if err != nil || n < 1 {
		return 0, fmt.Errorf("invalid table %s", k)
	}
	return n, nil
}
func mergeTableCells(shape *etree.Element, e Edit) error {
	ts := office.All(shape, ans, "tbl")
	if len(ts) != 1 {
		return fmt.Errorf("shape must contain one table")
	}
	rows := office.Children(ts[0], ans, "tr")
	grid := office.Child(ts[0], ans, "tblGrid")
	if grid == nil {
		return fmt.Errorf("missing table grid")
	}
	cols := len(office.Children(grid, ans, "gridCol"))
	cells := make([][]*etree.Element, len(rows))
	for i, r := range rows {
		cells[i] = office.Children(r, ans, "tc")
		if len(cells[i]) != cols {
			return fmt.Errorf("nonrectangular table")
		}
	}
	if e.Row < 0 || e.Row >= len(rows) || e.Column < 0 || e.Column >= cols {
		return fmt.Errorf("cell out of range")
	}
	anchor := cells[e.Row][e.Column]
	endRow, endCol := e.EndRow, e.EndColumn
	unmerge := e.Operation == "table-unmerge"
	if unmerge {
		if mergedContinuation(anchor) {
			return fmt.Errorf("unmerge requires the top-left merge origin")
		}
		h, err := cellSpan(anchor, "rowSpan")
		if err != nil {
			return err
		}
		w, err := cellSpan(anchor, "gridSpan")
		if err != nil {
			return err
		}
		if h == 1 && w == 1 {
			return fmt.Errorf("cell is not merged")
		}
		endRow, endCol = e.Row+h-1, e.Column+w-1
		if e.EndRow != 0 || e.EndColumn != 0 {
			return fmt.Errorf("unmerge takes only row and column, not end coordinates")
		}
	}
	if endRow < e.Row || endCol < e.Column || endRow >= len(rows) || endCol >= cols || endRow == e.Row && endCol == e.Column {
		return fmt.Errorf("invalid merge rectangle")
	}
	// Validate the complete region before moving any paragraphs.
	for r := e.Row; r <= endRow; r++ {
		for c := e.Column; c <= endCol; c++ {
			cell := cells[r][c]
			h, err := cellSpan(cell, "rowSpan")
			if err != nil {
				return err
			}
			w, err := cellSpan(cell, "gridSpan")
			if err != nil {
				return err
			}
			if office.Child(cell, ans, "txBody") == nil {
				return fmt.Errorf("cell has no text body")
			}
			if !unmerge && (h > 1 || w > 1 || mergedContinuation(cell)) {
				return fmt.Errorf("merge overlaps an existing merged region")
			}
			if unmerge {
				wantH, wantW := 1, 1
				if r == e.Row {
					wantH = endRow - e.Row + 1
				}
				if c == e.Column {
					wantW = endCol - e.Column + 1
				}
				hm := cell.SelectAttrValue("hMerge", "false")
				vm := cell.SelectAttrValue("vMerge", "false")
				if h != wantH || w != wantW || (hm == "true" || hm == "1") != (c > e.Column) || (vm == "true" || vm == "1") != (r > e.Row) {
					return fmt.Errorf("inconsistent merge rectangle")
				}
			}
		}
	}
	originBody := office.Child(anchor, ans, "txBody")
	for r := e.Row; r <= endRow; r++ {
		for c := e.Column; c <= endCol; c++ {
			cell := cells[r][c]
			for _, k := range []string{"rowSpan", "gridSpan", "hMerge", "vMerge"} {
				cell.RemoveAttr(k)
			}
			if unmerge {
				continue
			}
			if r == e.Row && endRow > e.Row {
				cell.CreateAttr("rowSpan", strconv.Itoa(endRow-e.Row+1))
			}
			if c == e.Column && endCol > e.Column {
				cell.CreateAttr("gridSpan", strconv.Itoa(endCol-e.Column+1))
			}
			if c > e.Column {
				cell.CreateAttr("hMerge", "1")
			}
			if r > e.Row {
				cell.CreateAttr("vMerge", "1")
			}
			if cell != anchor {
				body := office.Child(cell, ans, "txBody")
				for _, p := range office.Children(body, ans, "p") {
					originBody.AddChild(p.Copy())
				}
				replaceBody(body, "")
			}
		}
	}
	return nil
}
