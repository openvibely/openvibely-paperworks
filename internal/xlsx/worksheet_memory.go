package xlsx

import (
	"encoding/xml"
	"fmt"
	"path"
	"reflect"
	"strings"

	"github.com/xuri/excelize/v2"
)

// Excelize exposes File.Sheet but its worksheet type is unexported. This adapter
// accesses only exported fields, using checked reflection (never unsafe). Keep
// this compatibility boundary covered when upgrading Excelize. Callers own the
// File exclusively, as EditFile does; do not use it alongside concurrent setters.
// Reading the live rows avoids stale XML in File.Pkg and sees structural edits,
// new array masters and typed copies immediately, without cache invalidation.
type memoryWorksheet struct {
	rows                                                                       reflect.Value
	cellsIndex, formulaIndex, addressIndex, typeIndex, valueIndex, inlineIndex int
}

func memorySheet(f *excelize.File, sheet string) (memoryWorksheet, error) {
	var result memoryWorksheet
	// Force Excelize to load and normalize this worksheet through its public API.
	if _, err := f.GetCellType(sheet, "A1"); err != nil {
		return result, err
	}
	idx, err := f.GetSheetIndex(sheet)
	if err != nil {
		return result, err
	}
	if idx < 0 {
		return result, fmt.Errorf("sheet %q missing", sheet)
	}
	name := f.GetSheetName(idx)
	var id string
	for _, sh := range f.WorkBook.Sheets.Sheet {
		if sh.Name == name {
			id = sh.ID
			break
		}
	}
	roots, err := memoryRelationships(f, "_rels/.rels")
	if err != nil {
		return result, err
	}
	var workbook string
	for _, rel := range roots {
		if rel.Type == excelize.SourceRelationshipOfficeDocument {
			workbook = strings.TrimPrefix(rel.Target, "/")
			break
		}
	}
	if workbook == "" {
		return result, fmt.Errorf("workbook relationship missing")
	}
	rels, err := memoryRelationships(f, path.Join(path.Dir(workbook), "_rels", path.Base(workbook)+".rels"))
	if err != nil {
		return result, err
	}
	var part string
	for _, rel := range rels {
		if rel.ID == id {
			target := strings.ReplaceAll(rel.Target, "\\", "/")
			part = path.Join(path.Dir(workbook), target)
			if strings.HasPrefix(target, "/") {
				part = strings.TrimPrefix(path.Clean(target), "/")
			}
			break
		}
	}
	ws, ok := f.Sheet.Load(part)
	if !ok {
		return result, fmt.Errorf("loaded worksheet %q missing", sheet)
	}
	v := reflect.ValueOf(ws)
	if v.Kind() != reflect.Pointer || v.IsNil() || v.Elem().Kind() != reflect.Struct {
		return result, fmt.Errorf("unsupported Excelize worksheet representation")
	}
	data := v.Elem().FieldByName("SheetData")
	if data.Kind() != reflect.Struct {
		return result, fmt.Errorf("unsupported Excelize SheetData representation")
	}
	result.rows = data.FieldByName("Row")
	if result.rows.Kind() != reflect.Slice || result.rows.Type().Elem().Kind() != reflect.Struct {
		return result, fmt.Errorf("unsupported Excelize row representation")
	}
	cells, ok := result.rows.Type().Elem().FieldByName("C")
	if !ok || cells.Type.Kind() != reflect.Slice || cells.Type.Elem().Kind() != reflect.Struct {
		return result, fmt.Errorf("unsupported Excelize cell representation")
	}
	result.cellsIndex = cells.Index[0]
	ct := cells.Type.Elem()
	for _, field := range []struct {
		name  string
		kind  reflect.Kind
		index *int
	}{
		{"F", reflect.Pointer, &result.formulaIndex}, {"R", reflect.String, &result.addressIndex},
		{"T", reflect.String, &result.typeIndex}, {"V", reflect.String, &result.valueIndex}, {"IS", reflect.Pointer, &result.inlineIndex},
	} {
		sf, ok := ct.FieldByName(field.name)
		if !ok || sf.Type.Kind() != field.kind || sf.PkgPath != "" {
			return result, fmt.Errorf("unsupported Excelize cell field %s", field.name)
		}
		*field.index = sf.Index[0]
	}
	ft := ct.Field(result.formulaIndex).Type.Elem()
	if ft.Kind() != reflect.Struct {
		return result, fmt.Errorf("unsupported Excelize formula representation")
	}
	for _, name := range []string{"T", "Ref"} {
		sf, ok := ft.FieldByName(name)
		if !ok || sf.Type.Kind() != reflect.String {
			return result, fmt.Errorf("unsupported Excelize formula field %s", name)
		}
	}
	return result, nil
}

type memoryRelationship struct {
	ID     string `xml:"Id,attr"`
	Type   string `xml:"Type,attr"`
	Target string `xml:"Target,attr"`
}

// Relationships may have changed after add/delete-sheet, so prefer live data.
func memoryRelationships(f *excelize.File, part string) ([]memoryRelationship, error) {
	var data []byte
	if live, ok := f.Relationships.Load(part); ok {
		var err error
		data, err = xml.Marshal(live)
		if err != nil {
			return nil, err
		}
	} else if raw, ok := f.Pkg.Load(part); ok {
		data, _ = raw.([]byte)
	}
	var rels struct {
		Items []memoryRelationship `xml:"Relationship"`
	}
	if err := xml.Unmarshal(data, &rels); err != nil {
		return nil, err
	}
	return rels.Items, nil
}
