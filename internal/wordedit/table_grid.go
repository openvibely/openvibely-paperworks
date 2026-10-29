package wordedit

import (
	"fmt"
	"strconv"

	"github.com/beevik/etree"
	"github.com/openvibely/openvibely-paperworks/internal/office"
)

// tableGridColumnCount returns the number of columns declared by tblGrid, or
// zero when the table has no grid.
func tableGridColumnCount(table *etree.Element) int {
	grid := child(table, "tblGrid")
	if grid == nil {
		return 0
	}
	return len(office.Children(grid, nsW, "gridCol"))
}

func rowGridOffset(row *etree.Element, name string) (int, error) {
	trPr := child(row, "trPr")
	e := child(trPr, name)
	if e == nil {
		return 0, nil
	}
	value, err := strconv.Atoi(getW(e, "val"))
	if err != nil || value < 0 {
		return 0, fmt.Errorf("invalid table %s", name)
	}
	return value, nil
}

func tableCellGridSpan(cell *etree.Element) (int, error) {
	span := child(child(cell, "tcPr"), "gridSpan")
	if span == nil {
		return 1, nil
	}
	value, err := strconv.Atoi(getW(span, "val"))
	if err != nil || value < 1 {
		return 0, fmt.Errorf("invalid table gridSpan")
	}
	return value, nil
}

// resolveTableGridColumn maps a zero-based table grid column to the physical
// cell that covers it. A column inside a gridSpan consistently resolves to
// that spanning cell. The returned index is the cell's physical index in row.
func resolveTableGridColumn(table, row *etree.Element, column int) (*etree.Element, int, int, error) {
	if column < 0 {
		return nil, -1, 0, fmt.Errorf("column index out of range")
	}
	gridColumns := tableGridColumnCount(table)
	if gridColumns > 0 && column >= gridColumns {
		return nil, -1, 0, fmt.Errorf("column index out of range")
	}

	before, err := rowGridOffset(row, "gridBefore")
	if err != nil {
		return nil, -1, 0, err
	}
	if column < before {
		return nil, -1, 0, fmt.Errorf("column %d is omitted from the row by gridBefore=%d", column, before)
	}

	cells := office.Children(row, nsW, "tc")
	gridPosition := before
	for index, cell := range cells {
		span, err := tableCellGridSpan(cell)
		if err != nil {
			return nil, -1, 0, err
		}
		if column-gridPosition < span && column >= gridPosition {
			return cell, index, gridPosition, nil
		}
		gridPosition += span
	}

	after, err := rowGridOffset(row, "gridAfter")
	if err != nil {
		return nil, -1, 0, err
	}
	if column < gridPosition+after || (gridColumns > 0 && column < gridColumns) {
		return nil, -1, 0, fmt.Errorf("column %d is omitted from the row grid", column)
	}
	return nil, -1, 0, fmt.Errorf("column index out of range")
}

func rowIsRectangularUnmerged(row *etree.Element, columns int) bool {
	before, err := rowGridOffset(row, "gridBefore")
	if err != nil || before != 0 {
		return false
	}
	after, err := rowGridOffset(row, "gridAfter")
	if err != nil || after != 0 {
		return false
	}
	if len(office.Children(row, nsW, "tc")) != columns || len(all(row, "gridSpan"))+len(all(row, "vMerge")) > 0 {
		return false
	}
	return true
}
