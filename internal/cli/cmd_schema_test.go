package cli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/openvibely/openvibely-paperworks/internal/xlsx"
)

func TestWorkbookEditSchemaValidation(t *testing.T) {
	schema := runCLI(t, "schema", "workbook-edit")
	defs := schema["$defs"].(map[string]any)
	editDef := defs[schemaDefinitionName(reflect.TypeOf(xlsx.Edit{}))].(map[string]any)
	editProperties := editDef["properties"].(map[string]any)
	if ref, ok := editProperties["style"].(map[string]any)["$ref"]; !ok || ref == "" {
		t.Fatal("workbook edit schema omitted nested Excel style options")
	}

	valid := []map[string]any{
		{"edits": []any{map[string]any{"operation": "value", "sheet": "Sheet1", "cell": "A1", "value": "ok"}}},
		{"edits": []any{map[string]any{"operation": "add-sheet", "name": "New sheet"}}},
	}
	for _, document := range valid {
		if err := validateJSONSchema(schema, document); err != nil {
			t.Errorf("valid workbook edit rejected: %v", err)
		}
	}

	tooManyEdits := make([]any, 10001)
	for i := range tooManyEdits {
		tooManyEdits[i] = map[string]any{"operation": "add-sheet", "name": "New sheet"}
	}
	invalid := []struct {
		name     string
		document map[string]any
	}{
		{name: "missing operation", document: map[string]any{"edits": []any{map[string]any{"sheet": "Sheet1", "cell": "A1", "value": "x"}}}},
		{name: "unsupported operation", document: map[string]any{"edits": []any{map[string]any{"operation": "invented"}}}},
		{name: "missing edits", document: map[string]any{}},
		{name: "empty edits", document: map[string]any{"edits": []any{}}},
		{name: "too many edits", document: map[string]any{"edits": tooManyEdits}},
		{name: "missing operation field", document: map[string]any{"edits": []any{map[string]any{"operation": "value", "sheet": "Sheet1", "cell": "A1"}}}},
		{name: "invalid insertion count", document: map[string]any{"edits": []any{map[string]any{"operation": "insert-rows", "sheet": "Sheet1", "index": 1, "count": 0}}}},
	}
	for _, tc := range invalid {
		t.Run(tc.name, func(t *testing.T) {
			if err := validateJSONSchema(schema, tc.document); err == nil {
				t.Fatalf("invalid workbook edit accepted: %#v", tc.document)
			}
		})
	}
}

func TestWorkbookEditRuntimeDocumentValidationDoesNotPublish(t *testing.T) {
	dir := t.TempDir()
	csvPath := filepath.Join(dir, "input.csv")
	if err := os.WriteFile(csvPath, []byte("value\n1\n"), 0600); err != nil {
		t.Fatal(err)
	}
	input := filepath.Join(dir, "input.xlsx")
	runCLI(t, "import-csv", csvPath, "--output", input)
	spec := writeSpec(t, dir, "edit.json", map[string]any{"edits": []any{
		map[string]any{"operation": "delete-sheet", "sheet": "Sheet1"},
	}})
	output := filepath.Join(dir, "output.xlsx")
	var stdout, stderr bytes.Buffer
	if code := Main([]string{"workbook-edit", input, "--spec", spec, "--output", output}, &stdout, &stderr); code == 0 {
		t.Fatal("deleting the only sheet unexpectedly succeeded")
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Fatalf("failed document-dependent edit published output: stat error %v", err)
	}
}

// validateJSONSchema evaluates the generated schema keywords exercised by the
// workbook-edit contract, including refs and conditional required fields.
func validateJSONSchema(root map[string]any, document any) error {
	return validateSchemaNode(root, root, document, "$")
}

func validateSchemaNode(root, schema map[string]any, value any, path string) error {
	if ref, ok := schema["$ref"].(string); ok {
		if ref == "#" {
			return validateSchemaNode(root, root, value, path)
		}
		const prefix = "#/$defs/"
		if !strings.HasPrefix(ref, prefix) {
			return fmt.Errorf("unsupported schema reference %q", ref)
		}
		defs, ok := root["$defs"].(map[string]any)
		if !ok {
			return fmt.Errorf("schema has no definitions")
		}
		definition, ok := defs[strings.TrimPrefix(ref, prefix)].(map[string]any)
		if !ok {
			return fmt.Errorf("schema reference %q is unresolved", ref)
		}
		if err := validateSchemaNode(root, definition, value, path); err != nil {
			return err
		}
	}
	if allOf, ok := schema["allOf"].([]any); ok {
		for _, child := range allOf {
			if err := validateSchemaNode(root, child.(map[string]any), value, path); err != nil {
				return err
			}
		}
	}
	if condition, ok := schema["if"].(map[string]any); ok {
		if validateSchemaNode(root, condition, value, path) == nil {
			if thenSchema, ok := schema["then"].(map[string]any); ok {
				if err := validateSchemaNode(root, thenSchema, value, path); err != nil {
					return err
				}
			}
		}
	}
	if enum, ok := schema["enum"]; ok && !containsSchemaEnum(enum, value) {
		return fmt.Errorf("%s: value %v is outside enum", path, value)
	}
	if kind, ok := schema["type"].(string); ok && !schemaTypeMatches(kind, value) {
		return fmt.Errorf("%s: expected %s", path, kind)
	}
	if object, ok := value.(map[string]any); ok {
		for _, name := range schemaStringList(schema["required"]) {
			if _, exists := object[name]; !exists {
				return fmt.Errorf("%s: missing required property %q", path, name)
			}
		}
		properties, _ := schema["properties"].(map[string]any)
		for name, child := range properties {
			if v, exists := object[name]; exists {
				if err := validateSchemaNode(root, child.(map[string]any), v, path+name+"."); err != nil {
					return err
				}
			}
		}
		if schema["additionalProperties"] == false {
			for name := range object {
				if _, exists := properties[name]; !exists {
					return fmt.Errorf("%s: unexpected property %q", path, name)
				}
			}
		}
	}
	if array, ok := value.([]any); ok {
		if min, exists := schemaNumber(schema["minItems"]); exists && float64(len(array)) < min {
			return fmt.Errorf("%s: fewer than %v items", path, min)
		}
		if max, exists := schemaNumber(schema["maxItems"]); exists && float64(len(array)) > max {
			return fmt.Errorf("%s: more than %v items", path, max)
		}
		if itemSchema, ok := schema["items"].(map[string]any); ok {
			for i, item := range array {
				if err := validateSchemaNode(root, itemSchema, item, fmt.Sprintf("%s%d.", path, i)); err != nil {
					return err
				}
			}
		}
	}
	if number, ok := schemaNumber(value); ok {
		if min, exists := schemaNumber(schema["minimum"]); exists && number < min {
			return fmt.Errorf("%s: value %v is below minimum %v", path, number, min)
		}
	}
	return nil
}

func schemaStringList(value any) []string {
	switch values := value.(type) {
	case []string:
		return values
	case []any:
		out := make([]string, 0, len(values))
		for _, value := range values {
			if name, ok := value.(string); ok {
				out = append(out, name)
			}
		}
		return out
	default:
		return nil
	}
}

func schemaTypeMatches(kind string, value any) bool {
	switch kind {
	case "object":
		_, ok := value.(map[string]any)
		return ok
	case "array":
		_, ok := value.([]any)
		return ok
	case "string":
		_, ok := value.(string)
		return ok
	case "boolean":
		_, ok := value.(bool)
		return ok
	case "integer":
		n, ok := schemaNumber(value)
		return ok && n == float64(int64(n))
	case "number":
		_, ok := schemaNumber(value)
		return ok
	default:
		return true
	}
}

func schemaNumber(value any) (float64, bool) {
	switch n := value.(type) {
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case float64:
		return n, true
	default:
		return 0, false
	}
}

func containsSchemaEnum(enum, value any) bool {
	switch values := enum.(type) {
	case []string:
		for _, item := range values {
			if item == value {
				return true
			}
		}
	case []any:
		for _, item := range values {
			if reflect.DeepEqual(item, value) {
				return true
			}
		}
	}
	return false
}
