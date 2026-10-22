package cli

import "github.com/openvibely/openvibely-paperworks/internal/wordedit"

// Word edits historically use map specifications. Publish explicit field types
// for the general editor without pretending to validate legacy operation fields.
func wordSchema() map[string]any {
	props := map[string]any{}
	for _, k := range []string{"operation", "part", "old", "new", "text", "position", "font", "color", "style", "alignment", "fill", "path", "description", "kind", "type"} {
		props[k] = map[string]any{"type": "string"}
	}
	for _, k := range []string{"start", "end", "paragraph", "run", "table", "row", "column", "end_column", "occurrence", "image", "section", "width", "height", "top", "bottom", "left", "right", "header", "footer", "gutter", "after", "line"} {
		props[k] = map[string]any{"type": "integer", "minimum": 0}
	}
	for _, k := range []string{"font_size", "width_in", "height_in"} {
		props[k] = map[string]any{"type": "number", "exclusiveMinimum": 0}
	}
	for _, k := range []string{"bold", "italic", "strike", "linked"} {
		props[k] = map[string]any{"type": "boolean"}
	}
	props["before"] = map[string]any{"type": "integer", "minimum": 0, "description": "Optional zero-based destination block index for clone-component; 0 inserts before the first block. Omit to append."}
	props["rows"] = map[string]any{"type": "array", "items": map[string]any{"type": "array", "items": map[string]any{"type": "string"}}}
	props["widths_twips"] = map[string]any{"type": "array", "items": map[string]any{"type": "integer", "minimum": 1}}
	props["operation"] = map[string]any{"type": "string", "enum": wordedit.Operations}
	props["edits"] = map[string]any{"type": "array", "minItems": 1, "maxItems": 10000, "items": map[string]any{"$ref": "#"}}
	return map[string]any{"$schema": "https://json-schema.org/draft/2020-12/schema", "type": "object", "required": []string{"operation"}, "properties": props, "additionalProperties": true, "description": "Field types for general Word edits and a list of all operations. Legacy specialized operation fields remain open; see existing examples. General operation requirements and units: docs/document-editing.md. Nested batches are rejected."}
}
