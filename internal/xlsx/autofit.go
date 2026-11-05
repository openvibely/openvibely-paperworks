package xlsx

import (
	"fmt"
	"math"
	"sort"

	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/xuri/excelize/v2"
)

func hasSavedEmptyStringResult(wb *office.Workbook, formulas *excelize.File, sourceSheet, cell, formula string, renames []formulaSheetRename) bool {
	if wb == nil || formulas == nil || wb.Sheets[sourceSheet] == nil {
		return false
	}
	c := wb.Sheets[sourceSheet].Cells[cell]
	if c == nil || c.SelectAttrValue("t", "") != "str" {
		return false
	}
	if office.Child(c, office.NSS, "f") == nil {
		return false
	}
	value := office.Child(c, office.NSS, "v")
	if value == nil || value.Text() != "" {
		return false
	}
	expectedFormula, err := formulas.GetCellFormula(sourceSheet, cell)
	if err != nil || expectedFormula == "" {
		return false
	}
	for _, rename := range renames {
		expectedFormula = renameFormulaSheet(expectedFormula, rename.from, rename.to)
	}
	return expectedFormula == formula
}

func autofitRows(f *excelize.File, e Edit) error {
	return autofitRowsWithSourceCache(f, e, nil, nil, "", nil)
}

type autofitMergeInterval struct {
	left, right, order, prefixMaxRight int
}

// autofitMergeRow stores sorted intervals for one row. prefixMaxRight lets
// containing skip earlier intervals that cannot cover the queried column.
type autofitMergeRow struct {
	intervals []autofitMergeInterval
}

type autofitMergeValidation struct {
	order    int
	vertical bool
}

type autofitMergeValidations map[int]map[int]autofitMergeValidation

func (row autofitMergeRow) containing(col int) (autofitMergeInterval, bool) {
	limit := sort.Search(len(row.intervals), func(i int) bool {
		return row.intervals[i].left > col
	}) - 1
	found := -1
	for i := limit; i >= 0 && row.intervals[i].prefixMaxRight >= col; i-- {
		candidate := row.intervals[i]
		if candidate.right >= col && (found < 0 || candidate.order < row.intervals[found].order) {
			found = i
		}
	}
	if found < 0 {
		return autofitMergeInterval{}, false
	}
	return row.intervals[found], true
}

// indexAutofitMerges parses each merge once, retaining only ranges that
// intersect the autofit selection. Validation is recorded at the first cell
// the original row-major walk would encounter it.
func indexAutofitMerges(merges []excelize.MergeCell, x1, y1, x2, y2 int) (map[int]autofitMergeRow, autofitMergeValidations) {
	if len(merges) == 0 {
		return nil, nil
	}
	byRow := make(map[int][]autofitMergeInterval)
	var validations autofitMergeValidations
	for order, merge := range merges {
		left, top, err := excelize.CellNameToCoordinates(merge.GetStartAxis())
		if err != nil {
			continue
		}
		right, bottom, err := excelize.CellNameToCoordinates(merge.GetEndAxis())
		if err != nil || left > x2 || right < x1 || top > y2 || bottom < y1 {
			continue
		}
		vertical := top != bottom
		partial := left < x1 || right > x2
		if vertical || partial {
			row, col := max(top, y1), max(left, x1)
			if validations == nil {
				validations = make(autofitMergeValidations)
			}
			columns := validations[row]
			if columns == nil {
				columns = make(map[int]autofitMergeValidation)
				validations[row] = columns
			}
			if existing, ok := columns[col]; !ok || order < existing.order {
				columns[col] = autofitMergeValidation{order: order, vertical: vertical}
			}
			continue
		}
		byRow[top] = append(byRow[top], autofitMergeInterval{left: left, right: right, order: order})
	}
	if len(byRow) == 0 {
		return nil, validations
	}

	rows := make(map[int]autofitMergeRow, len(byRow))
	for row, intervals := range byRow {
		sort.Slice(intervals, func(i, j int) bool {
			if intervals[i].left == intervals[j].left {
				return intervals[i].order < intervals[j].order
			}
			return intervals[i].left < intervals[j].left
		})
		indexed := autofitMergeRow{intervals: intervals}
		for i := range indexed.intervals {
			indexed.intervals[i].prefixMaxRight = indexed.intervals[i].right
			if i > 0 && indexed.intervals[i-1].prefixMaxRight > indexed.intervals[i].prefixMaxRight {
				indexed.intervals[i].prefixMaxRight = indexed.intervals[i-1].prefixMaxRight
			}
		}
		rows[row] = indexed
	}
	return rows, validations
}

func autofitRowsWithSourceCache(f *excelize.File, e Edit, source *office.Workbook, formulas *excelize.File, sourceSheet string, renames []formulaSheetRename) error {
	a, b, err := bounds(e.Range)
	if err != nil {
		return err
	}
	x1, y1, _ := excelize.CellNameToCoordinates(a)
	x2, y2, _ := excelize.CellNameToCoordinates(b)
	if (x2-x1+1)*(y2-y1+1) > 10000 {
		return fmt.Errorf("autofit range exceeds 10000 cells")
	}
	defaultName, err := f.GetDefaultFont()
	if err != nil {
		return err
	}
	base, err := f.GetStyle(0)
	if err != nil {
		return err
	}
	defaultSize := 11.0
	if base.Font != nil && base.Font.Size > 0 {
		defaultSize = base.Font.Size
	}
	face := office.FindFont(defaultName, false, false)
	if face == nil {
		return fmt.Errorf("default font unavailable for measurement: %s", defaultName)
	}
	digit := office.Advance(face, "0", defaultSize) * 96 / 72
	merges, err := f.GetMergeCells(e.Sheet)
	if err != nil {
		return err
	}
	mergeRows, mergeValidations := indexAutofitMerges(merges, x1, y1, x2, y2)
	heights := map[int]float64{}
	height := defaultSize*1.2 + 3
	var rowMerges autofitMergeRow
	var rowValidations map[int]autofitMergeValidation
	err = walkCoordinates(x1, y1, x2, y2, func(col, row int) error {
		if col == x1 {
			height = defaultSize*1.2 + 3
			rowMerges = mergeRows[row]
			rowValidations = mergeValidations[row]
		}
		if len(rowValidations) > 0 {
			if validation, ok := rowValidations[col]; ok {
				if validation.vertical {
					return fmt.Errorf("autofit does not resize vertically merged rows")
				}
				return fmt.Errorf("autofit range must include complete merged cells")
			}
		}
		cell, _ := excelize.CoordinatesToCellName(col, row)
		start, end := col, col
		covered := false
		if len(rowMerges.intervals) > 0 {
			if merge, ok := rowMerges.containing(col); ok {
				if col != merge.left {
					covered = true
				}
				start, end = merge.left, merge.right
			}
		}
		if !covered {
			id, err := f.GetCellStyle(e.Sheet, cell)
			if err != nil {
				return err
			}
			style, err := f.GetStyle(id)
			if err != nil {
				return err
			}
			name, size, bold, italic := defaultName, defaultSize, false, false
			if style.Font != nil {
				if style.Font.Family != "" {
					name = style.Font.Family
				}
				if style.Font.Size > 0 {
					size = style.Font.Size
				}
				bold = style.Font.Bold
				italic = style.Font.Italic
			}
			width := 0.0
			for c := start; c <= end; c++ {
				letter, _ := excelize.ColumnNumberToName(c)
				w, err := f.GetColWidth(e.Sheet, letter)
				if err != nil {
					return err
				}
				px := math.Floor(w*digit + 5)
				if w < 1 {
					px = math.Floor(w * (digit + 5))
				}
				width += px * 72 / 96
			}
			width -= 5 * 72 / 96
			wrap := false
			if al := style.Alignment; al != nil {
				if al.TextRotation != 0 || al.ShrinkToFit {
					return fmt.Errorf("autofit requires unrotated text without shrink-to-fit")
				}
				wrap = al.WrapText
				width -= float64(al.Indent) * 3 * digit * 72 / 96
			}
			value, err := f.GetCellValue(e.Sheet, cell)
			if err != nil {
				return err
			}
			formula, err := f.GetCellFormula(e.Sheet, cell)
			if err != nil {
				return err
			}
			if formula != "" && value == "" && !hasSavedEmptyStringResult(source, formulas, sourceSheet, cell, formula, renames) {
				return fmt.Errorf("%s needs recalculation before autofit", cell)
			}
			runs := []office.TextRun{{Text: value, Font: name, Size: size, Bold: bold, Italic: italic}}
			rich, err := f.GetCellRichText(e.Sheet, cell)
			if err != nil {
				return err
			}
			if len(rich) > 0 {
				runs = nil
				for _, r := range rich {
					rr := office.TextRun{Text: r.Text, Font: name, Size: size, Bold: bold, Italic: italic}
					if r.Font != nil {
						if r.Font.Family != "" {
							rr.Font = r.Font.Family
						}
						if r.Font.Size > 0 {
							rr.Size = r.Font.Size
						}
						rr.Bold = r.Font.Bold
						rr.Italic = r.Font.Italic
					}
					runs = append(runs, rr)
				}
			}
			h, err := office.TextHeight(runs, width, wrap)
			if err != nil {
				return fmt.Errorf("%s: %w", cell, err)
			}
			height = math.Max(height, h+3)
		}
		if col == x2 {
			if height > 409.5 {
				return fmt.Errorf("row %d needs more than Excel's 409.5-point limit", row)
			}
			heights[row] = math.Ceil(height*20) / 20
		}
		return nil
	})
	if err != nil {
		return err
	}
	for row, h := range heights {
		if err := f.SetRowHeight(e.Sheet, row, h); err != nil {
			return err
		}
	}
	return nil
}
