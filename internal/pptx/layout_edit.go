package pptx

import (
	"fmt"
	"github.com/beevik/etree"
	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"strconv"
)

type Layout struct {
	Root     *LayoutNode `json:"root,omitempty"`
	ShapeIDs []string    `json:"shape_ids"`
	Mode     string      `json:"mode"`
	X        float64     `json:"x"`
	Y        float64     `json:"y"`
	Width    float64     `json:"width"`
	Height   float64     `json:"height"`
	Gap      float64     `json:"gap"`
	Columns  int         `json:"columns,omitempty"`
}

func layoutObjects(pkg *opc.Package, part string, root *etree.Element, l *Layout) error {
	if l != nil && l.Root != nil {
		return layoutTree(pkg, part, root, l)
	}
	if l == nil || len(l.ShapeIDs) == 0 || len(l.ShapeIDs) > 10000 {
		return fmt.Errorf("layout needs 1..10000 shape_ids")
	}
	for _, v := range []float64{l.X, l.Y, l.Width, l.Height, l.Gap} {
		if !finite(v) {
			return fmt.Errorf("invalid layout geometry")
		}
	}
	if l.Width <= 0 || l.Height <= 0 || l.Gap < 0 {
		return fmt.Errorf("positive layout dimensions and nonnegative gap required")
	}
	cols := l.Columns
	switch l.Mode {
	case "row":
		cols = len(l.ShapeIDs)
	case "column":
		cols = 1
	case "grid":
		if cols < 1 || cols > len(l.ShapeIDs) {
			return fmt.Errorf("grid columns out of range")
		}
	default:
		return fmt.Errorf("mode must be row, column or grid")
	}
	rows := (len(l.ShapeIDs) + cols - 1) / cols
	w := (l.Width - float64(cols-1)*l.Gap) / float64(cols)
	h := (l.Height - float64(rows-1)*l.Gap) / float64(rows)
	if w <= 0 || h <= 0 {
		return fmt.Errorf("gaps exceed layout dimensions")
	}
	seen := map[string]bool{}
	for i, id := range l.ShapeIDs {
		if seen[id] {
			return fmt.Errorf("duplicate shape_id")
		}
		seen[id] = true
		var shape *etree.Element
		for _, p := range office.All(root, pns, "cNvPr") {
			if p.SelectAttrValue("id", "") == id {
				if shape != nil {
					return fmt.Errorf("ambiguous shape_id")
				}
				shape = p.Parent().Parent()
			}
		}
		if shape == nil {
			return fmt.Errorf("shape_id %s not found", id)
		}
		x := l.X + float64(i%cols)*(w+l.Gap)
		y := l.Y + float64(i/cols)*(h+l.Gap)
		if err := editObject(pkg, part, shape, Edit{Operation: "geometry", X: &x, Y: &y, W: &w, H: &h}); err != nil {
			return err
		}
	}
	return nil
}
func tableObject(shape *etree.Element, e Edit) error {
	tables := office.All(shape, ans, "tbl")
	if len(tables) != 1 {
		return fmt.Errorf("shape must contain one table")
	}
	table := tables[0]
	rows := office.Children(table, ans, "tr")
	if e.Row < 0 || e.Row >= len(rows) {
		return fmt.Errorf("row out of range")
	}
	row := rows[e.Row]
	cells := office.Children(row, ans, "tc")
	if e.Column < 0 || e.Column >= len(cells) {
		return fmt.Errorf("column out of range")
	}
	cell := cells[e.Column]
	if e.Operation != "table-cell-format" {
		for _, r := range rows {
			for _, c := range office.Children(r, ans, "tc") {
				for _, a := range []string{"rowSpan", "gridSpan", "hMerge", "vMerge"} {
					if c.SelectAttrValue(a, "") != "" {
						return fmt.Errorf("structure editing requires an unmerged table")
					}
				}
			}
		}
	}
	switch e.Operation {
	case "table-row-insert":
		copy := row.Copy()
		table.InsertChildAt(row.Index(), copy)
		for _, c := range office.Children(copy, ans, "tc") {
			body := office.Child(c, ans, "txBody")
			if body != nil {
				replaceBody(body, "")
			}
		}
	case "table-row-delete":
		if len(rows) <= 1 {
			return fmt.Errorf("cannot delete last row")
		}
		table.RemoveChild(row)
	case "table-column-insert", "table-column-delete":
		grid := office.Child(table, ans, "tblGrid")
		if grid == nil {
			return fmt.Errorf("missing grid")
		}
		cols := office.Children(grid, ans, "gridCol")
		if len(cols) != len(cells) {
			return fmt.Errorf("nonrectangular table")
		}
		if e.Operation == "table-column-delete" && len(cols) <= 1 {
			return fmt.Errorf("cannot delete last column")
		}
		for _, r := range rows {
			if len(office.Children(r, ans, "tc")) != len(cols) {
				return fmt.Errorf("nonrectangular table")
			}
		}
		if e.Operation == "table-column-insert" {
			grid.InsertChildAt(cols[e.Column].Index(), cols[e.Column].Copy())
		} else {
			grid.RemoveChild(cols[e.Column])
		}
		for _, r := range rows {
			c := office.Children(r, ans, "tc")[e.Column]
			if e.Operation == "table-column-insert" {
				copy := c.Copy()
				r.InsertChildAt(c.Index(), copy)
				if body := office.Child(copy, ans, "txBody"); body != nil {
					replaceBody(body, "")
				}
			} else {
				r.RemoveChild(c)
			}
		}
	case "table-cell-format":
		pr := drawingChild(cell, "tcPr")
		if e.Fill != "" {
			if _, err := color(e.Fill, ""); err != nil {
				return err
			}
			for _, c := range pr.ChildElements() {
				if c.Tag == "solidFill" || c.Tag == "noFill" {
					pr.RemoveChild(c)
				}
			}
			drawingChild(pr, "solidFill").CreateElement("a:srgbClr").CreateAttr("val", e.Fill)
		}
		body := office.Child(cell, ans, "txBody")
		if body != nil {
			for _, p := range office.Children(body, ans, "p") {
				for _, r := range office.Children(p, ans, "r") {
					rp := office.Child(r, ans, "rPr")
					if rp == nil {
						rp = etree.NewElement("a:rPr")
						r.InsertChildAt(0, rp)
					}
					if e.FontSize != nil {
						if !finite(*e.FontSize) || *e.FontSize <= 0 || *e.FontSize > 400 {
							return fmt.Errorf("invalid font size")
						}
						rp.CreateAttr("sz", strconv.Itoa(int(*e.FontSize*100)))
					}
					if e.Bold != nil {
						v := "0"
						if *e.Bold {
							v = "1"
						}
						rp.CreateAttr("b", v)
					}
					if e.Color != "" {
						if _, err := color(e.Color, ""); err != nil {
							return err
						}
						for _, f := range rp.ChildElements() {
							if f.Tag == "solidFill" {
								rp.RemoveChild(f)
							}
						}
						drawingChild(rp, "solidFill").CreateElement("a:srgbClr").CreateAttr("val", e.Color)
					}
				}
			}
		}
	default:
		return fmt.Errorf("unsupported table edit")
	}
	if e.Operation != "table-cell-format" {
		var width, height int64
		for _, c := range office.Children(office.Child(table, ans, "tblGrid"), ans, "gridCol") {
			v, err := strconv.ParseInt(c.SelectAttrValue("w", ""), 10, 64)
			if err != nil {
				return err
			}
			width += v
		}
		for _, r := range office.Children(table, ans, "tr") {
			v, err := strconv.ParseInt(r.SelectAttrValue("h", ""), 10, 64)
			if err != nil {
				return err
			}
			height += v
		}
		transform := office.Child(shape, pns, "xfrm")
		if transform == nil {
			return fmt.Errorf("table has no frame transform")
		}
		extent := drawingChild(transform, "ext")
		extent.CreateAttr("cx", strconv.FormatInt(width, 10))
		extent.CreateAttr("cy", strconv.FormatInt(height, 10))
	}
	return nil
}
