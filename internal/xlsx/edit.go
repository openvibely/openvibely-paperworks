package xlsx

import (
	"bufio"
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"github.com/xuri/excelize/v2"
)

// EditSpec applies ordered operations to an existing workbook. Formula results
// are invalidated; callers must recalculate before relying on cached values.
type EditSpec struct {
	Edits []Edit `json:"edits"`
}
type Edit struct {
	FormulaStyle string                              `json:"formula_style,omitempty"`
	Operation    string                              `json:"operation"`
	Sheet        string                              `json:"sheet,omitempty"`
	Cell         string                              `json:"cell,omitempty"`
	Range        string                              `json:"range,omitempty"`
	Value        any                                 `json:"value,omitempty"`
	Formula      string                              `json:"formula,omitempty"`
	Name         string                              `json:"name,omitempty"`
	Target       string                              `json:"target,omitempty"`
	Index        int                                 `json:"index,omitempty"`
	Count        int                                 `json:"count,omitempty"`
	Size         float64                             `json:"size,omitempty"`
	Style        *excelize.Style                     `json:"style,omitempty"`
	Panes        *excelize.Panes                     `json:"panes,omitempty"`
	View         *excelize.ViewOptions               `json:"view,omitempty"`
	DefinedName  *excelize.DefinedName               `json:"defined_name,omitempty"`
	Table        *excelize.Table                     `json:"table,omitempty"`
	Validation   *excelize.DataValidation            `json:"validation,omitempty"`
	Conditional  []excelize.ConditionalFormatOptions `json:"conditional,omitempty"`
	Chart        *excelize.Chart                     `json:"chart,omitempty"`
	Combo        []*excelize.Chart                   `json:"combo,omitempty"`
	Picture      *Picture                            `json:"picture,omitempty"`
	Note         *excelize.Comment                   `json:"note,omitempty"`
}

func EditFile(ctx context.Context, input, output, base string, spec EditSpec) error {
	if len(spec.Edits) == 0 || len(spec.Edits) > 10000 {
		return fmt.Errorf("edits must contain 1..10000 operations")
	}
	f, err := excelize.OpenFile(input)
	if err != nil {
		return err
	}
	defer func() { f.Close() }()
	for i, op := range spec.Edits {
		if err = ctx.Err(); err != nil {
			return err
		}
		switch op.Operation {
		case "rename-sheet", "copy", "copy-values", "copy-formulas", "copy-formats":
			var next *excelize.File
			if op.Operation == "rename-sheet" {
				next, err = renameSheet(f, op.Sheet, op.Name)
			} else {
				next, err = copyRange(f, op)
			}
			if err != nil {
				return fmt.Errorf("edit %d (%s): %w", i, op.Operation, err)
			}
			if next != f {
				f.Close()
				f = next
			}
			continue
		}
		if err = applyEdit(f, base, op); err != nil {
			return fmt.Errorf("edit %d (%s): %w", i, op.Operation, err)
		}
	}
	yes := true
	if err = f.SetCalcProps(&excelize.CalcPropsOptions{FullCalcOnLoad: &yes, ForceFullCalc: &yes}); err != nil {
		return err
	}
	b, err := f.WriteToBuffer()
	if err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	return opc.WriteNew(output, b.Bytes())
}
func applyEdit(f *excelize.File, base string, e Edit) error {
	// Sheet-independent operations are explicitly enumerated, avoiding silent
	// sheet creation on misspelled targets.
	switch e.Operation {
	case "add-sheet":
		_, err := f.NewSheet(e.Name)
		return err
	case "set-name":
		if e.DefinedName == nil {
			return fmt.Errorf("defined_name required")
		}
		return f.SetDefinedName(e.DefinedName)
	case "delete-name":
		if e.DefinedName == nil {
			return fmt.Errorf("defined_name required")
		}
		return f.DeleteDefinedName(e.DefinedName)
	}
	idx, err := f.GetSheetIndex(e.Sheet)
	if err != nil {
		return err
	}
	if idx < 0 {
		return fmt.Errorf("sheet %q not found", e.Sheet)
	}
	switch e.Operation {
	case "fill-formula":
		if e.FormulaStyle != "" && e.FormulaStyle != "a1" && e.FormulaStyle != "r1c1" {
			return fmt.Errorf("formula_style must be a1 or r1c1")
		}
		if strings.TrimSpace(e.Formula) == "" {
			return fmt.Errorf("formula required")
		}
		start, _, err := bounds(e.Range)
		if err != nil {
			return err
		}
		x, y, _ := excelize.CellNameToCoordinates(start)
		return eachCell(e.Range, func(cell string) error {
			cx, cy, _ := excelize.CellNameToCoordinates(cell)
			formula, err := TranslateFormula(strings.TrimPrefix(e.Formula, "="), cx-x, cy-y)
			if e.FormulaStyle == "r1c1" {
				formula, err = r1c1Formula(strings.TrimPrefix(e.Formula, "="), cell)
			}
			if err != nil {
				return err
			}
			return f.SetCellFormula(e.Sheet, cell, formula)
		})
	case "value":
		if err = literal(e.Value); err != nil {
			return err
		}
		if err = f.SetCellFormula(e.Sheet, e.Cell, ""); err != nil {
			return err
		}
		return f.SetCellValue(e.Sheet, e.Cell, e.Value)
	case "formula":
		if strings.TrimSpace(e.Formula) == "" {
			return fmt.Errorf("formula required")
		}
		if e.FormulaStyle != "" && e.FormulaStyle != "a1" && e.FormulaStyle != "r1c1" {
			return fmt.Errorf("formula_style must be a1 or r1c1")
		}
		if e.FormulaStyle == "r1c1" {
			var err error
			e.Formula, err = r1c1Formula(e.Formula, e.Cell)
			if err != nil {
				return err
			}
		}
		return f.SetCellFormula(e.Sheet, e.Cell, strings.TrimPrefix(e.Formula, "="))
	case "clear-values", "clear-formats", "clear-all":
		return eachCell(e.Range, func(cell string) error {
			if e.Operation != "clear-formats" {
				if err := f.SetCellFormula(e.Sheet, cell, ""); err != nil {
					return err
				}
				if err := f.SetCellValue(e.Sheet, cell, nil); err != nil {
					return err
				}
			}
			if e.Operation != "clear-values" {
				return f.SetCellStyle(e.Sheet, cell, cell, 0)
			}
			return nil
		})
	case "style":
		if e.Style == nil {
			return fmt.Errorf("style required")
		}
		a, b, err := bounds(e.Range)
		if err != nil {
			return err
		}
		id, err := f.NewStyle(e.Style)
		if err != nil {
			return err
		}
		return f.SetCellStyle(e.Sheet, a, b, id)
	case "autofit-rows":
		return autofitRows(f, e)
	case "autofit-columns":
		return f.AutoFitColWidth(e.Sheet, e.Range)
	case "column-width":
		a, b, _ := strings.Cut(e.Range, ":")
		if b == "" {
			b = a
		}
		return f.SetColWidth(e.Sheet, a, b, e.Size)
	case "row-height":
		return f.SetRowHeight(e.Sheet, e.Index, e.Size)
	case "panes":
		if e.Panes == nil {
			return fmt.Errorf("panes required")
		}
		return f.SetPanes(e.Sheet, e.Panes)
	case "view":
		if e.View == nil {
			return fmt.Errorf("view required")
		}
		return f.SetSheetView(e.Sheet, 0, e.View)
	case "delete-sheet":
		if len(f.GetSheetList()) <= 1 {
			return fmt.Errorf("cannot delete last sheet")
		}
		return f.DeleteSheet(e.Sheet)
	case "move-sheet":
		return f.MoveSheet(e.Sheet, e.Target)
	case "merge", "unmerge":
		a, b, err := bounds(e.Range)
		if err != nil {
			return err
		}
		if e.Operation == "merge" {
			return f.MergeCell(e.Sheet, a, b)
		}
		return f.UnmergeCell(e.Sheet, a, b)
	case "insert-rows":
		if e.Count < 1 {
			return fmt.Errorf("positive count required")
		}
		return f.InsertRows(e.Sheet, e.Index, e.Count)
	case "delete-row":
		return f.RemoveRow(e.Sheet, e.Index)
	case "insert-columns":
		if e.Count < 1 {
			return fmt.Errorf("positive count required")
		}
		return f.InsertCols(e.Sheet, e.Cell, e.Count)
	case "delete-column":
		return f.RemoveCol(e.Sheet, e.Cell)
	case "add-table":
		if e.Table == nil {
			return fmt.Errorf("table required")
		}
		return f.AddTable(e.Sheet, e.Table)
	case "delete-table":
		return f.DeleteTable(e.Name)
	case "set-table":
		if e.Table == nil {
			return fmt.Errorf("table required")
		}
		tables, err := f.GetTables(e.Sheet)
		if err != nil {
			return err
		}
		found := false
		for _, t := range tables {
			if t.Name == e.Name {
				found = true
			}
		}
		if !found {
			return fmt.Errorf("table not found")
		}
		if err = f.DeleteTable(e.Name); err != nil {
			return err
		}
		return f.AddTable(e.Sheet, e.Table)
	case "validation":
		if e.Validation == nil {
			return fmt.Errorf("validation required")
		}
		return f.AddDataValidation(e.Sheet, e.Validation)
	case "delete-validation":
		if e.Range == "" {
			return fmt.Errorf("range required")
		}
		return f.DeleteDataValidation(e.Sheet, e.Range)
	case "conditional-format":
		if len(e.Conditional) == 0 {
			return fmt.Errorf("conditional required")
		}
		if e.Style != nil {
			id, err := f.NewConditionalStyle(e.Style)
			if err != nil {
				return err
			}
			for i := range e.Conditional {
				e.Conditional[i].Format = &id
			}
		}
		return f.SetConditionalFormat(e.Sheet, e.Range, e.Conditional)
	case "delete-conditional-format":
		return f.UnsetConditionalFormat(e.Sheet, e.Range)
	case "add-chart":
		if e.Chart == nil {
			return fmt.Errorf("chart required")
		}
		return f.AddChart(e.Sheet, e.Cell, e.Chart, e.Combo...)
	case "replace-chart":
		if e.Chart == nil {
			return fmt.Errorf("chart required")
		}
		if err = f.DeleteChart(e.Sheet, e.Cell); err != nil {
			return err
		}
		return f.AddChart(e.Sheet, e.Cell, e.Chart, e.Combo...)
	case "delete-chart":
		return f.DeleteChart(e.Sheet, e.Cell)
	case "add-picture", "replace-picture":
		if e.Picture == nil {
			return fmt.Errorf("picture required")
		}
		if e.Operation == "replace-picture" {
			if err = f.DeletePicture(e.Sheet, e.Cell); err != nil {
				return err
			}
		}
		p := e.Picture.Path
		if !filepath.IsAbs(p) {
			p = filepath.Join(base, p)
		}
		return f.AddPicture(e.Sheet, e.Cell, p, e.Picture.Options)
	case "delete-picture":
		return f.DeletePicture(e.Sheet, e.Cell)
	case "note":
		if e.Note == nil {
			return fmt.Errorf("note required")
		}
		return f.AddComment(e.Sheet, *e.Note)
	case "delete-note":
		return f.DeleteComment(e.Sheet, e.Cell)
	default:
		return fmt.Errorf("unsupported operation %q", e.Operation)
	}
}
func eachCell(r string, fn func(string) error) error {
	a, b, err := bounds(r)
	if err != nil {
		return err
	}
	x, y, _ := excelize.CellNameToCoordinates(a)
	xx, yy, _ := excelize.CellNameToCoordinates(b)
	return walkCoordinates(x, y, xx, yy, func(col, row int) error {
		c, _ := excelize.CoordinatesToCellName(col, row)
		return fn(c)
	})
}

// ImportCSV keeps all fields literal (including leading '=' and leading zeros).
func ImportCSV(ctx context.Context, input, output, sheet string, delimiter rune) error {
	in, err := os.Open(input)
	if err != nil {
		return err
	}
	defer in.Close()
	br := bufio.NewReader(in)
	if bom, _ := br.Peek(3); bytes.Equal(bom, []byte{0xEF, 0xBB, 0xBF}) {
		br.Discard(3)
	}
	r := csv.NewReader(br)
	r.Comma = delimiter
	r.FieldsPerRecord = -1
	f := excelize.NewFile()
	defer f.Close()
	if sheet == "" {
		sheet = "Sheet1"
	}
	if err = f.SetSheetName("Sheet1", sheet); err != nil {
		return err
	}
	total := 0
	for row := 1; ; row++ {
		if err = ctx.Err(); err != nil {
			return err
		}
		record, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		total += len(record)
		if total > 1000000 {
			return fmt.Errorf("CSV exceeds one million cells")
		}
		for col, v := range record {
			cell, err := excelize.CoordinatesToCellName(col+1, row)
			if err != nil {
				return err
			}
			if err = f.SetCellStr(sheet, cell, v); err != nil {
				return err
			}
		}
	}
	b, err := f.WriteToBuffer()
	if err != nil {
		return err
	}
	return opc.WriteNew(output, b.Bytes())
}
