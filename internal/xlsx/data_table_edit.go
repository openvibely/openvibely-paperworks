package xlsx

import (
	"bytes"
	"context"
	"fmt"

	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"github.com/xuri/excelize/v2"
)

// Add to the current ordered-edit state, preserving native table definitions.
func addDataTable(ctx context.Context, f *excelize.File, name string, table *DataTable) (*excelize.File, error) {
	if table == nil {
		return nil, fmt.Errorf("data_table required")
	}
	idx, err := f.GetSheetIndex(name)
	if err != nil {
		return nil, err
	}
	if idx < 0 {
		return nil, fmt.Errorf("sheet %q not found", name)
	}
	name = f.GetSheetName(idx)
	a, b, err := bounds(table.Range)
	if err != nil {
		return nil, err
	}
	x, y, _ := excelize.CellNameToCoordinates(a)
	xx, yy, _ := excelize.CellNameToCoordinates(b)
	raw, err := f.WriteToBuffer()
	if err != nil {
		return nil, err
	}
	p, err := opc.Read(raw.Bytes())
	if err != nil {
		return nil, err
	}
	wb, err := office.OpenWorkbook(p)
	if err != nil {
		return nil, err
	}
	sheet := wb.Sheets[name]
	if sheet == nil {
		return nil, fmt.Errorf("worksheet not found")
	}
	// A table/array's empty result cells still belong to its formula range.
	for _, formula := range office.All(sheet.Doc.Root(), office.NSS, "f") {
		ref := formula.SelectAttrValue("ref", "")
		if ref == "" {
			continue
		}
		start, end, e := bounds(ref)
		if e != nil {
			return nil, e
		}
		left, top, _ := excelize.CellNameToCoordinates(start)
		right, bottom, _ := excelize.CellNameToCoordinates(end)
		if left <= xx && right >= x && top <= yy && bottom >= y {
			return nil, fmt.Errorf("data table overlaps formula range %s", ref)
		}
	}
	spec := Spec{Sheets: []Sheet{{Name: name, DataTables: []DataTable{*table}}}}
	if err = prepareDataTables(ctx, f, spec); err != nil {
		return nil, err
	}
	raw, err = f.WriteToBuffer()
	if err != nil {
		return nil, err
	}
	p, err = opc.Read(raw.Bytes())
	if err != nil {
		return nil, err
	}
	if err = patchDataTables(p, spec); err != nil {
		return nil, err
	}
	data, err := p.Bytes()
	if err != nil {
		return nil, err
	}
	return excelize.OpenReader(bytes.NewReader(data))
}
