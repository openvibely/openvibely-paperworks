package pptx

import (
	"fmt"
	"math"
	"strconv"

	"github.com/beevik/etree"
	"github.com/openvibely/openvibely-paperworks/internal/office"
)

// A table's grid, rather than its graphic frame alone, determines its rendered
// size. Scale the cells proportionally whenever geometry changes the frame.
func resizeTableGrid(shape *etree.Element, width, height *float64) error {
	if width == nil && height == nil {
		return nil
	}
	tables := office.All(shape, ans, "tbl")
	if len(tables) == 0 {
		return nil
	} // Charts also use graphic frames.
	if len(tables) != 1 {
		return fmt.Errorf("geometry requires one table per frame")
	}
	table := tables[0]
	grid := office.Child(table, ans, "tblGrid")
	if grid == nil {
		return fmt.Errorf("missing table grid")
	}
	columns := office.Children(grid, ans, "gridCol")
	rows := office.Children(table, ans, "tr")
	if len(columns) == 0 || len(rows) == 0 {
		return fmt.Errorf("empty table grid")
	}
	for _, r := range rows {
		if len(office.Children(r, ans, "tc")) != len(columns) {
			return fmt.Errorf("nonrectangular table")
		}
	}
	type dimension struct {
		elements []*etree.Element
		attr     string
		values   []int64
	}
	var changes []dimension
	for _, axis := range []struct {
		elements []*etree.Element
		attr     string
		size     *float64
	}{{columns, "w", width}, {rows, "h", height}} {
		if axis.size == nil {
			continue
		}
		if !finite(*axis.size) || *axis.size <= 0 || *axis.size*914400 >= float64(math.MaxInt64) {
			return fmt.Errorf("invalid table size")
		}
		target := emu(*axis.size)
		if target < int64(len(axis.elements)) {
			return fmt.Errorf("table size is too small for its grid")
		}
		var total int64
		old := make([]int64, len(axis.elements))
		for i, e := range axis.elements {
			n, err := strconv.ParseInt(e.SelectAttrValue(axis.attr, ""), 10, 64)
			if err != nil || n <= 0 || n > math.MaxInt64-total {
				return fmt.Errorf("invalid table grid dimensions")
			}
			old[i] = n
			total += n
		}
		values := make([]int64, len(old))
		var cumulative, previous int64
		for i, n := range old {
			cumulative += n
			edge := target
			if i < len(old)-1 {
				edge = int64(math.Round(float64(cumulative) / float64(total) * float64(target)))
			}
			values[i] = edge - previous
			previous = edge
			if values[i] <= 0 {
				return fmt.Errorf("table resize collapses a row or column")
			}
		}
		changes = append(changes, dimension{axis.elements, axis.attr, values})
	}
	for _, c := range changes {
		for i, e := range c.elements {
			e.CreateAttr(c.attr, strconv.FormatInt(c.values[i], 10))
		}
	}
	return nil
}
