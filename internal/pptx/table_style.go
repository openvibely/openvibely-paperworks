package pptx

import (
	"fmt"
	"github.com/beevik/etree"
	"github.com/openvibely/openvibely-paperworks/internal/office"
	"math"
	"strconv"
)

type TableFormat struct {
	// Sizes are points. Omit an array to retain that axis.
	RowHeights   []float64         `json:"row_heights,omitempty"`
	ColumnWidths []float64         `json:"column_widths,omitempty"`
	Cells        []TableCellFormat `json:"cells,omitempty"`
}
type TableCellFormat struct {
	Row           int                    `json:"row"`
	Column        int                    `json:"column"`
	Margins       *TextInsets            `json:"margins,omitempty"`
	VerticalAlign string                 `json:"vertical_align,omitempty"`
	Fill          *DrawingStyle          `json:"fill,omitempty"`
	Borders       map[string]TextOutline `json:"borders,omitempty"`
	TextPaint     *TextPaint             `json:"text_paint,omitempty"`
}

func formatTable(shape *etree.Element, s *TableFormat) error {
	if s == nil {
		return fmt.Errorf("table_format required")
	}
	tables := office.All(shape, ans, "tbl")
	if len(tables) != 1 {
		return fmt.Errorf("one table required")
	}
	table := tables[0]
	rows := office.Children(table, ans, "tr")
	cols := office.Children(office.Child(table, ans, "tblGrid"), ans, "gridCol")
	for _, axis := range []struct {
		els    []*etree.Element
		values []float64
		attr   string
	}{{rows, s.RowHeights, "h"}, {cols, s.ColumnWidths, "w"}} {
		if axis.values == nil {
			continue
		}
		if len(axis.values) != len(axis.els) {
			return fmt.Errorf("table sizes must cover the complete axis")
		}
		for i, v := range axis.values {
			if !finite(v) || v <= 0 || v > 100000 || math.Round(v*12700) < 1 {
				return fmt.Errorf("invalid table size in points")
			}
			axis.els[i].CreateAttr(axis.attr, strconv.FormatInt(int64(math.Round(v*12700)), 10))
		}
	}
	for _, c := range s.Cells {
		if c.Row < 0 || c.Row >= len(rows) {
			return fmt.Errorf("cell row out of range")
		}
		cells := office.Children(rows[c.Row], ans, "tc")
		if c.Column < 0 || c.Column >= len(cells) {
			return fmt.Errorf("cell column out of range")
		}
		cell := cells[c.Column]
		if mergedContinuation(cell) {
			return fmt.Errorf("format merged cells through their origin")
		}
		pr := office.Child(cell, ans, "tcPr")
		if pr == nil {
			pr = cell.CreateElement("a:tcPr")
		}
		if c.VerticalAlign != "" {
			anchor := map[string]string{"top": "t", "middle": "ctr", "bottom": "b"}[c.VerticalAlign]
			if anchor == "" {
				return fmt.Errorf("invalid cell vertical alignment")
			}
			pr.CreateAttr("anchor", anchor)
		}
		if m := c.Margins; m != nil {
			for _, v := range []struct {
				key   string
				value *float64
			}{{"marL", m.Left}, {"marR", m.Right}, {"marT", m.Top}, {"marB", m.Bottom}} {
				if v.value != nil {
					if !finite(*v.value) || *v.value < 0 || *v.value > 1000 {
						return fmt.Errorf("invalid cell margin")
					}
					pr.CreateAttr(v.key, strconv.FormatInt(int64(*v.value*12700), 10))
				}
			}
		}
		if c.Fill != nil {
			fill, err := paintFill(c.Fill)
			if err != nil {
				return err
			}
			for _, old := range pr.ChildElements() {
				switch old.Tag {
				case "noFill", "solidFill", "gradFill", "blipFill", "pattFill", "grpFill":
					pr.RemoveChild(old)
				}
			}
			insertCellProperty(pr, fill)
		}
		for _, side := range []string{"left", "right", "top", "bottom"} {
			border, ok := c.Borders[side]
			if !ok {
				continue
			}
			temp := etree.NewElement("a:rPr")
			temp.CreateAttr("xmlns:a", ans)
			if err := paintTextProperties(temp, &TextPaint{Outline: &border}); err != nil {
				return err
			}
			line := office.Child(temp, ans, "ln").Copy()
			line.Tag = map[string]string{"left": "lnL", "right": "lnR", "top": "lnT", "bottom": "lnB"}[side]
			for _, old := range office.Children(pr, ans, line.Tag) {
				pr.RemoveChild(old)
			}
			insertCellProperty(pr, line)
		}
		for side := range c.Borders {
			if side != "left" && side != "right" && side != "top" && side != "bottom" {
				return fmt.Errorf("invalid cell border side %s", side)
			}
		}
		if c.TextPaint != nil {
			body := office.Child(cell, ans, "txBody")
			if body == nil {
				return fmt.Errorf("missing cell text")
			}
			if err := paintBody(body, c.TextPaint); err != nil {
				return err
			}
		}
	}
	// Keep the frame consistent with the native table grid.
	if s.RowHeights != nil || s.ColumnWidths != nil {
		xf := office.Child(shape, pns, "xfrm")
		if xf == nil {
			return fmt.Errorf("table transform missing")
		}
		ext := office.Child(xf, ans, "ext")
		if ext == nil {
			return fmt.Errorf("table extent missing")
		}
		for _, axis := range []struct {
			els       []*etree.Element
			attr, key string
		}{{rows, "h", "cy"}, {cols, "w", "cx"}} {
			var total int64
			for _, e := range axis.els {
				v, err := strconv.ParseInt(e.SelectAttrValue(axis.attr, ""), 10, 64)
				if err != nil || v <= 0 || v > math.MaxInt64-total {
					return fmt.Errorf("invalid table grid")
				}
				total += v
			}
			ext.CreateAttr(axis.key, strconv.FormatInt(total, 10))
		}
	}
	return nil
}
func insertCellProperty(pr, e *etree.Element) {
	order := map[string]int{"lnL": 1, "lnR": 2, "lnT": 3, "lnB": 4, "lnTlToBr": 5, "lnBlToTr": 6, "cell3D": 7, "noFill": 8, "solidFill": 8, "gradFill": 8, "blipFill": 8, "pattFill": 8, "grpFill": 8, "headers": 9, "extLst": 10}
	for _, old := range pr.ChildElements() {
		if order[old.Tag] > order[e.Tag] {
			pr.InsertChildAt(old.Index(), e)
			return
		}
	}
	pr.AddChild(e)
}
