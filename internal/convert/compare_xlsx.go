package convert

import (
	"fmt"
	"sort"

	"github.com/xuri/excelize/v2"
)

// CellContent describes stored workbook cell content. Formula text and its
// stored cached result are separate so a comparison never suggests that a
// formula was recalculated.
type CellContent struct {
	Value           *string `json:"value,omitempty"`
	ValueType       string  `json:"value_type,omitempty"`
	Formula         *string `json:"formula,omitempty"`
	CachedValue     *string `json:"cached_value,omitempty"`
	CachedValueType string  `json:"cached_value_type,omitempty"`
}

// CellChange reports a changed, added, or removed populated XLSX cell.
type CellChange struct {
	Sheet   string       `json:"sheet"`
	Address string       `json:"address"`
	Status  string       `json:"status"`
	Before  *CellContent `json:"before,omitempty"`
	After   *CellContent `json:"after,omitempty"`
}

type xlsxCellKey struct {
	sheet   string
	address string
}

type indexedXLSXCell struct {
	content CellContent
	row     int
	column  int
}

type xlsxCellIndex struct {
	cells      map[xlsxCellKey]indexedXLSXCell
	sheetOrder map[string]int
}

func cellTypeName(kind excelize.CellType) string {
	switch kind {
	case excelize.CellTypeUnset, excelize.CellTypeNumber:
		// OOXML's default cell type is numeric when the type attribute is absent.
		return "number"
	case excelize.CellTypeBool:
		return "boolean"
	case excelize.CellTypeDate:
		return "date"
	case excelize.CellTypeError:
		return "error"
	case excelize.CellTypeFormula:
		return "string"
	case excelize.CellTypeInlineString, excelize.CellTypeSharedString:
		return "string"
	default:
		return "unknown"
	}
}

func readXLSXCellIndex(path string) (xlsxCellIndex, error) {
	book, err := excelize.OpenFile(path)
	if err != nil {
		return xlsxCellIndex{}, err
	}
	defer book.Close()

	index := xlsxCellIndex{
		cells:      make(map[xlsxCellKey]indexedXLSXCell),
		sheetOrder: make(map[string]int),
	}
	for sheetOrder, sheet := range book.GetSheetList() {
		index.sheetOrder[sheet] = sheetOrder
		rows, err := book.GetRows(sheet, excelize.Options{RawCellValue: true})
		if err != nil {
			return xlsxCellIndex{}, fmt.Errorf("read XLSX sheet %q: %w", sheet, err)
		}
		for rowOffset, values := range rows {
			for columnOffset, value := range values {
				address, err := excelize.CoordinatesToCellName(columnOffset+1, rowOffset+1)
				if err != nil {
					return xlsxCellIndex{}, fmt.Errorf("address XLSX cell in sheet %q: %w", sheet, err)
				}
				formula, err := book.GetCellFormula(sheet, address)
				if err != nil {
					return xlsxCellIndex{}, fmt.Errorf("read XLSX formula at %s!%s: %w", sheet, address, err)
				}
				if value == "" && formula == "" {
					continue
				}
				kind, err := book.GetCellType(sheet, address)
				if err != nil {
					return xlsxCellIndex{}, fmt.Errorf("read XLSX cell type at %s!%s: %w", sheet, address, err)
				}
				content := CellContent{}
				if formula != "" {
					content.Formula = stringPointer(formula)
					content.CachedValue = stringPointer(value)
					content.CachedValueType = cellTypeName(kind)
				} else {
					content.Value = stringPointer(value)
					content.ValueType = cellTypeName(kind)
				}
				column, row, err := excelize.CellNameToCoordinates(address)
				if err != nil {
					return xlsxCellIndex{}, fmt.Errorf("parse XLSX cell address %q: %w", address, err)
				}
				index.cells[xlsxCellKey{sheet: sheet, address: address}] = indexedXLSXCell{
					content: content,
					row:     row,
					column:  column,
				}
			}
		}
	}
	return index, nil
}

func stringPointer(value string) *string { return &value }

func equalCellContent(a, b CellContent) bool {
	return equalStringPointer(a.Value, b.Value) &&
		a.ValueType == b.ValueType &&
		equalStringPointer(a.Formula, b.Formula) &&
		equalStringPointer(a.CachedValue, b.CachedValue) &&
		a.CachedValueType == b.CachedValueType
}

func equalStringPointer(a, b *string) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func compareXLSXContents(beforePath, afterPath string) ([]CellChange, error) {
	before, err := readXLSXCellIndex(beforePath)
	if err != nil {
		return nil, fmt.Errorf("index before XLSX workbook: %w", err)
	}
	after, err := readXLSXCellIndex(afterPath)
	if err != nil {
		return nil, fmt.Errorf("index after XLSX workbook: %w", err)
	}

	keys := make(map[xlsxCellKey]struct{}, len(before.cells)+len(after.cells))
	for key := range before.cells {
		keys[key] = struct{}{}
	}
	for key := range after.cells {
		keys[key] = struct{}{}
	}
	ordered := make([]xlsxCellKey, 0, len(keys))
	for key := range keys {
		ordered = append(ordered, key)
	}
	sheetOrder := func(sheet string) int {
		if order, ok := before.sheetOrder[sheet]; ok {
			return order
		}
		return len(before.sheetOrder) + after.sheetOrder[sheet]
	}
	sort.Slice(ordered, func(i, j int) bool {
		a, aBefore := before.cells[ordered[i]]
		if !aBefore {
			a = after.cells[ordered[i]]
		}
		b, bBefore := before.cells[ordered[j]]
		if !bBefore {
			b = after.cells[ordered[j]]
		}
		if orderA, orderB := sheetOrder(ordered[i].sheet), sheetOrder(ordered[j].sheet); orderA != orderB {
			return orderA < orderB
		}
		if a.row != b.row {
			return a.row < b.row
		}
		if a.column != b.column {
			return a.column < b.column
		}
		return ordered[i].sheet < ordered[j].sheet
	})

	changes := make([]CellChange, 0)
	for _, key := range ordered {
		beforeCell, hadBefore := before.cells[key]
		afterCell, hasAfter := after.cells[key]
		if hadBefore && hasAfter && equalCellContent(beforeCell.content, afterCell.content) {
			continue
		}
		change := CellChange{Sheet: key.sheet, Address: key.address}
		switch {
		case !hadBefore:
			change.Status = "added"
			change.After = &afterCell.content
		case !hasAfter:
			change.Status = "removed"
			change.Before = &beforeCell.content
		default:
			change.Status = "changed"
			change.Before = &beforeCell.content
			change.After = &afterCell.content
		}
		changes = append(changes, change)
	}
	return changes, nil
}
