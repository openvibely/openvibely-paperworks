package cli

import (
	"context"
	"fmt"
	"github.com/openvibely/openvibely-paperworks/internal/docx"
	"github.com/openvibely/openvibely-paperworks/internal/pdf"
	"github.com/openvibely/openvibely-paperworks/internal/pptx"
	"github.com/openvibely/openvibely-paperworks/internal/xlsx"
	"reflect"
	"strings"
)

func init() {
	register(&Command{Name: "schema", Summary: "Print JSON input field schemas for document authoring and editing commands.", Usage: "paperworks schema COMMAND", Run: func(ctx context.Context, a Args) (any, error) {
		if len(a.Pos) != 1 {
			return nil, usagef("one command name required")
		}
		if a.Pos[0] == "word-edit" {
			return wordSchema(), nil
		}
		types := map[string]any{"chart-series-replace": ChartSeriesSpec{}, "create-pdf": pdf.CreateSpec{}, "pdf-layout": pdf.LayoutOptions{}, "pdf-object-edit": pdf.ObjectEditSpec{}, "create-xlsx": xlsx.Spec{}, "workbook-edit": xlsx.EditSpec{}, "create-pptx": pptx.Spec{}, "slide-edit": pptx.EditSpec{}, "deck-edit": pptx.DeckEditSpec{}, "create-docx": docx.Spec{}}
		value, ok := types[a.Pos[0]]
		if !ok {
			return nil, usagef("schema available for chart-series-replace, create-pdf, pdf-layout, pdf-object-edit, create-docx, word-edit, create-xlsx, workbook-edit, create-pptx, slide-edit and deck-edit")
		}
		g := schemaBuilder{defs: map[string]any{}}
		root := g.schema(reflect.TypeOf(value))
		root["$schema"] = "https://json-schema.org/draft/2020-12/schema"
		root["$defs"] = g.defs
		root["description"] = "Field schema derived from the Go input types. Runtime validation remains authoritative for document-dependent constraints. Indices and units are documented in docs/document-editing.md."
		if a.Pos[0] == "workbook-edit" {
			constrainWorkbookEditSchema(root, g.defs)
		}
		return root, nil
	}})
}

func constrainWorkbookEditSchema(root map[string]any, defs map[string]any) {
	editSpecName := schemaDefinitionName(reflect.TypeOf(xlsx.EditSpec{}))
	editSpec := defs[editSpecName].(map[string]any)
	editSpec["required"] = []string{"edits"}
	specProperties := editSpec["properties"].(map[string]any)
	edits := specProperties["edits"].(map[string]any)
	edits["minItems"] = 1
	edits["maxItems"] = 10000

	editName := schemaDefinitionName(reflect.TypeOf(xlsx.Edit{}))
	edit := defs[editName].(map[string]any)
	edit["required"] = []string{"operation"}
	properties := edit["properties"].(map[string]any)
	operation := properties["operation"].(map[string]any)
	operation["enum"] = xlsx.SupportedEditOperations()

	formulaStyle := properties["formula_style"].(map[string]any)
	formulaStyle["enum"] = []string{"", "a1", "r1c1"}

	// Runtime checks the count before applying either insertion operation.
	var allOf []any
	allOf = append(allOf, map[string]any{
		"if":   map[string]any{"required": []string{"operation"}, "properties": map[string]any{"operation": map[string]any{"enum": []string{"insert-rows", "insert-columns"}}}},
		"then": map[string]any{"properties": map[string]any{"count": map[string]any{"minimum": 1}}},
	})
	allOf = append(allOf, map[string]any{
		"if":   map[string]any{"required": []string{"operation"}, "properties": map[string]any{"operation": map[string]any{"enum": []string{"insert-rows", "delete-row", "row-height"}}}},
		"then": map[string]any{"properties": map[string]any{"index": map[string]any{"minimum": 1}}},
	})
	edit["allOf"] = allOf

	requiredByOperation := map[string][]string{
		"add-chart":                 {"sheet", "cell", "chart"},
		"add-data-table":            {"sheet", "data_table"},
		"add-picture":               {"sheet", "cell", "picture"},
		"add-sheet":                 {"name"},
		"add-table":                 {"sheet", "table"},
		"autofit-columns":           {"sheet", "range"},
		"autofit-rows":              {"sheet", "range"},
		"clear-all":                 {"sheet", "range"},
		"clear-formats":             {"sheet", "range"},
		"clear-values":              {"sheet", "range"},
		"column-width":              {"sheet", "range", "size"},
		"conditional-format":        {"sheet", "range", "conditional"},
		"copy":                      {"sheet", "range", "target"},
		"copy-formats":              {"sheet", "range", "target"},
		"copy-formulas":             {"sheet", "range", "target"},
		"copy-values":               {"sheet", "range", "target"},
		"delete-chart":              {"sheet", "cell"},
		"delete-column":             {"sheet", "cell"},
		"delete-conditional-format": {"sheet", "range"},
		"delete-name":               {"defined_name"},
		"delete-note":               {"sheet", "cell"},
		"delete-row":                {"sheet", "index"},
		"delete-sheet":              {"sheet"},
		"delete-table":              {"name"},
		"delete-validation":         {"sheet", "range"},
		"fill-formula":              {"sheet", "range", "formula"},
		"formula":                   {"sheet", "cell", "formula"},
		"insert-columns":            {"sheet", "cell", "count"},
		"insert-rows":               {"sheet", "index", "count"},
		"merge":                     {"sheet", "range"},
		"move-sheet":                {"sheet", "target"},
		"note":                      {"sheet", "note"},
		"panes":                     {"sheet", "panes"},
		"rename-sheet":              {"sheet", "name"},
		"replace-chart":             {"sheet", "cell", "chart"},
		"replace-picture":           {"sheet", "cell", "picture"},
		"row-height":                {"sheet", "index", "size"},
		"set-name":                  {"defined_name"},
		"set-table":                 {"sheet", "name", "table"},
		"style":                     {"sheet", "range", "style"},
		"tab-color":                 {"sheet", "color"},
		"unmerge":                   {"sheet", "range"},
		"validation":                {"sheet", "validation"},
		"value":                     {"sheet", "cell", "value"},
		"view":                      {"sheet", "view"},
	}
	groups := make([]map[string]any, 0)
	groupIndex := map[string]int{}
	for _, op := range xlsx.SupportedEditOperations() {
		required, ok := requiredByOperation[op]
		if !ok || len(required) == 0 {
			panic("workbook-edit schema is missing required fields for operation " + op)
		}
		key := strings.Join(required, "\x00")
		idx, ok := groupIndex[key]
		if !ok {
			idx = len(groups)
			groupIndex[key] = idx
			groups = append(groups, map[string]any{"operations": []string{op}, "required": required})
		} else {
			ops := groups[idx]["operations"].([]string)
			groups[idx]["operations"] = append(ops, op)
		}
	}
	for _, group := range groups {
		condition := map[string]any{
			"required":   []string{"operation"},
			"properties": map[string]any{"operation": map[string]any{"enum": group["operations"]}},
		}
		allOf = append(allOf, map[string]any{
			"if":   condition,
			"then": map[string]any{"required": group["required"]},
		})
	}
	edit["allOf"] = allOf

	dataTableName := schemaDefinitionName(reflect.TypeOf(xlsx.DataTable{}))
	if dataTable, ok := defs[dataTableName].(map[string]any); ok {
		dataTable["required"] = []string{"range", "input1"}
	}
	root["description"] = "Workbook edit field schema with operation-specific required fields and stable constraints. Runtime validation remains authoritative for document-dependent constraints."
}

func schemaDefinitionName(t reflect.Type) string {
	return strings.ReplaceAll(t.PkgPath()+"."+t.Name(), "/", "_")
}

type schemaBuilder struct{ defs map[string]any }

func (g *schemaBuilder) schema(t reflect.Type) map[string]any {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	switch t.Kind() {
	case reflect.Struct:
		name := strings.ReplaceAll(t.PkgPath()+"."+t.Name(), "/", "_")
		if _, ok := g.defs[name]; !ok {
			g.defs[name] = map[string]any{}
			props := map[string]any{}
			for i := 0; i < t.NumField(); i++ {
				f := t.Field(i)
				if !f.IsExported() {
					continue
				}
				tag := strings.Split(f.Tag.Get("json"), ",")[0]
				if tag == "-" {
					continue
				}
				if tag == "" {
					tag = f.Name
				}
				props[tag] = g.schema(f.Type)
			}
			g.defs[name] = map[string]any{"type": "object", "properties": props, "additionalProperties": false}
		}
		return map[string]any{"$ref": "#/$defs/" + name}
	case reflect.Map:
		return map[string]any{"type": "object", "additionalProperties": g.schema(t.Elem())}
	case reflect.Slice, reflect.Array:
		r := map[string]any{"type": "array", "items": g.schema(t.Elem())}
		if t.Kind() == reflect.Array {
			r["minItems"] = t.Len()
			r["maxItems"] = t.Len()
		}
		return r
	case reflect.String:
		return map[string]any{"type": "string"}
	case reflect.Bool:
		return map[string]any{"type": "boolean"}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return map[string]any{"type": "integer"}
	case reflect.Float32, reflect.Float64:
		return map[string]any{"type": "number"}
	case reflect.Interface:
		return map[string]any{}
	default:
		return map[string]any{"description": fmt.Sprintf("Go type %s", t)}
	}
}
