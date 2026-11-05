package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestCreateDOCXEquationSchemaAndHelp(t *testing.T) {
	schema := runCLI(t, "schema", "create-docx")
	defs := schema["$defs"].(map[string]any)
	rootName := strings.TrimPrefix(schema["$ref"].(string), "#/$defs/")
	rootDef := defs[rootName].(map[string]any)
	rootProps := rootDef["properties"].(map[string]any)
	blocks := rootProps["blocks"].(map[string]any)
	blockName := strings.TrimPrefix(blocks["items"].(map[string]any)["$ref"].(string), "#/$defs/")
	blockDef := defs[blockName].(map[string]any)
	blockProps := blockDef["properties"].(map[string]any)
	mathml := blockProps["mathml"].(map[string]any)
	if mathml["type"] != "string" || !strings.Contains(mathml["description"].(string), "native Word OMML") {
		t.Fatalf("equation MathML field is not described in schema: %#v", mathml)
	}
	if len(blockDef["allOf"].([]any)) == 0 {
		t.Fatal("schema does not require mathml for equation blocks")
	}

	var stdout, stderr bytes.Buffer
	if code := Main([]string{"help", "create-docx"}, &stdout, &stderr); code != 0 {
		t.Fatalf("create-docx help failed: %s", stderr.String())
	}
	for _, want := range []string{"native Word equations", "mathml", "other MathML is rejected"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("create-docx help omits %q: %s", want, stdout.String())
		}
	}
}
