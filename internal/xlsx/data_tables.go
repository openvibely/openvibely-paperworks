package xlsx

import (
	"context"
	"fmt"
	"strings"

	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"github.com/xuri/excelize/v2"
)

// DataTable describes a native Excel What-If table. Range covers results only.
// Input1 is the sole input, or the row-input cell for a two-variable table.
type DataTable struct {
	Range       string `json:"range"`
	Input1      string `json:"input1"`
	Input2      string `json:"input2,omitempty"`
	RowOriented bool   `json:"row_oriented,omitempty"`
}

func hasDataTables(spec Spec) bool {
	for _, s := range spec.Sheets {
		if len(s.DataTables) > 0 {
			return true
		}
	}
	return false
}
func prepareDataTables(ctx context.Context, f *excelize.File, spec Spec) error {
	total := 0
	for _, s := range spec.Sheets {
		occupied := map[string]bool{}
		for _, d := range s.DataTables {
			if e := ctx.Err(); e != nil {
				return e
			}
			a, b, e := bounds(d.Range)
			if e != nil {
				return e
			}
			x, y, _ := excelize.CellNameToCoordinates(a)
			xx, yy, _ := excelize.CellNameToCoordinates(b)
			if x <= 1 || y <= 1 {
				return fmt.Errorf("data table needs a preceding header row and column")
			}
			if d.Input2 != "" && d.RowOriented {
				return fmt.Errorf("two-input data table cannot be row_oriented")
			}
			if d.Input2 == "" && ((!d.RowOriented && x != xx) || (d.RowOriented && y != yy)) {
				return fmt.Errorf("single-input table results must be one column or row")
			}
			if d.Input1 == "" || (d.Input2 != "" && strings.EqualFold(d.Input1, d.Input2)) {
				return fmt.Errorf("data table needs distinct input cells")
			}
			for _, ref := range []string{d.Input1, d.Input2} {
				if ref == "" {
					continue
				}
				cx, cy, e := excelize.CellNameToCoordinates(ref)
				if e != nil {
					return e
				}
				if cx >= x && cx <= xx && cy >= y && cy <= yy {
					return fmt.Errorf("input %s lies in result range", ref)
				}
				formula, e := f.GetCellFormula(s.Name, ref)
				if e != nil {
					return e
				}
				if formula != "" {
					return fmt.Errorf("data table input %s must be literal", ref)
				}
				value, e := f.GetCellValue(s.Name, ref)
				if e != nil {
					return e
				}
				if value == "" {
					return fmt.Errorf("data table input %s is empty", ref)
				}
			}
			// A formula immediately above/left is the model output, not the master.
			fx, fy := x, y-1
			if d.Input2 != "" {
				fx = x - 1
			} else if d.RowOriented {
				fx, fy = x-1, y
			}
			output, _ := excelize.CoordinatesToCellName(fx, fy)
			formula, e := f.GetCellFormula(s.Name, output)
			if e != nil {
				return e
			}
			if formula == "" {
				return fmt.Errorf("data table requires model-output formula at %s", output)
			}
			merges, e := f.GetMergeCells(s.Name)
			if e != nil {
				return e
			}
			for _, m := range merges {
				mx, my, _ := excelize.CellNameToCoordinates(m.GetStartAxis())
				ex, ey, _ := excelize.CellNameToCoordinates(m.GetEndAxis())
				if mx <= xx && ex >= x && my <= yy && ey >= y {
					return fmt.Errorf("data table overlaps merged cells")
				}
			}
			tables, e := f.GetTables(s.Name)
			if e != nil {
				return e
			}
			for _, table := range tables {
				start, end, e := bounds(table.Range)
				if e != nil {
					return e
				}
				tx, ty, _ := excelize.CellNameToCoordinates(start)
				ex, ey, _ := excelize.CellNameToCoordinates(end)
				if tx <= xx && ex >= x && ty <= yy && ey >= y {
					return fmt.Errorf("data table overlaps structured table %s", table.Name)
				}
			}
			e = walkCoordinates(x, y, xx, yy, func(col, row int) error {
				if err := ctx.Err(); err != nil {
					return err
				}
				total++
				if total > 100000 {
					return fmt.Errorf("data tables exceed 100000 result cells")
				}
				ref, _ := excelize.CoordinatesToCellName(col, row)
				if occupied[ref] {
					return fmt.Errorf("overlapping data tables at %s", ref)
				}
				occupied[ref] = true
				value, err := f.GetCellValue(s.Name, ref)
				if err != nil {
					return err
				}
				formula, err := f.GetCellFormula(s.Name, ref)
				if err != nil {
					return err
				}
				if value != "" || formula != "" {
					return fmt.Errorf("data table would overwrite %s", ref)
				}
				return nil
			})
			if e != nil {
				return e
			}
			// Materialize a blank master cell; the patch replaces this placeholder and
			// removes its value. Zero is never advertised as a calculated result.
			if e := f.SetCellValue(s.Name, a, 0); e != nil {
				return e
			}
		}
		for _, d := range s.DataTables {
			for _, ref := range []string{d.Input1, d.Input2} {
				if occupied[strings.ToUpper(strings.ReplaceAll(ref, "$", ""))] {
					return fmt.Errorf("data table input overlaps another result range")
				}
			}
		}
	}
	return nil
}
func patchDataTables(p *opc.Package, spec Spec) error {
	wb, e := office.OpenWorkbook(p)
	if e != nil {
		return e
	}
	for _, s := range spec.Sheets {
		if len(s.DataTables) == 0 {
			continue
		}
		sheet := wb.Sheets[s.Name]
		for _, d := range s.DataTables {
			a, _, e := bounds(d.Range)
			if e != nil {
				return e
			}
			c := sheet.Cells[strings.ToUpper(a)]
			if c == nil {
				return fmt.Errorf("missing data table master %s", a)
			}
			c.RemoveAttr("t")
			for _, child := range c.ChildElements() {
				if child.Tag == "v" || child.Tag == "f" || child.Tag == "is" {
					c.RemoveChild(child)
				}
			}
			formula := c.CreateElement("f")
			formula.CreateAttr("t", "dataTable")
			formula.CreateAttr("ref", strings.ToUpper(d.Range))
			formula.CreateAttr("r1", strings.ToUpper(d.Input1))
			if d.Input2 != "" {
				formula.CreateAttr("dt2D", "1")
				formula.CreateAttr("r2", strings.ToUpper(d.Input2))
			}
			if d.RowOriented {
				formula.CreateAttr("dtr", "1")
			}
		}
		if e := p.SetXML(sheet.Part, sheet.Doc); e != nil {
			return e
		}
	}
	return nil
}
