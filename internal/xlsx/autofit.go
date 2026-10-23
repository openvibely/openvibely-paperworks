package xlsx

import (
	"fmt"
	"math"

	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/xuri/excelize/v2"
)

func hasSavedEmptyStringResult(wb *office.Workbook, sheet, cell, formula string) bool {
	if wb == nil || wb.Sheets[sheet] == nil {
		return false
	}
	c := wb.Sheets[sheet].Cells[cell]
	if c == nil || c.SelectAttrValue("t", "") != "str" {
		return false
	}
	savedFormula := office.Child(c, office.NSS, "f")
	value := office.Child(c, office.NSS, "v")
	return savedFormula != nil && savedFormula.Text() == formula && value != nil && value.Text() == ""
}

func autofitRows(f *excelize.File, e Edit) error {
	return autofitRowsWithSourceCache(f, e, nil)
}

func autofitRowsWithSourceCache(f *excelize.File, e Edit, source *office.Workbook) error {
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
	heights := map[int]float64{}
	height := defaultSize*1.2 + 3
	err = walkCoordinates(x1, y1, x2, y2, func(col, row int) error {
		if col == x1 {
			height = defaultSize*1.2 + 3
		}
		cell, _ := excelize.CoordinatesToCellName(col, row)
		start, end := col, col
		covered := false
		for _, m := range merges {
			mx, my, _ := excelize.CellNameToCoordinates(m.GetStartAxis())
			ex, ey, _ := excelize.CellNameToCoordinates(m.GetEndAxis())
			if col >= mx && col <= ex && row >= my && row <= ey {
				if my != ey {
					return fmt.Errorf("autofit does not resize vertically merged rows")
				}
				if mx < x1 || ex > x2 {
					return fmt.Errorf("autofit range must include complete merged cells")
				}
				if col != mx {
					covered = true
				}
				start, end = mx, ex
				break
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
			if formula != "" && value == "" && !hasSavedEmptyStringResult(source, e.Sheet, cell, formula) {
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
