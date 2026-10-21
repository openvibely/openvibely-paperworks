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
		root["description"] = "Field schema derived from the Go input types. Runtime validation enforces operation-specific required fields and constraints. Indices and units are documented in docs/document-editing.md."
		return root, nil
	}})
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
