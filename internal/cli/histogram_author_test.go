package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"reflect"
	"testing"

	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"github.com/openvibely/openvibely-paperworks/internal/testsupport"
)

// Public-file boundary for both original assertions, on the captured original
// blank-slide package. Extra guards preserve source and unrelated member bytes.
func TestReferenceHistogramAuthorPublicReports(t *testing.T) {
	source := testsupport.OriginalPackageFixture(t, "runtime-workflow-fixtures.zip", "categorical-author-blank", ".pptx")
	before, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	source = testsupport.PresentationOutput(t, dir)
	if err := os.WriteFile(source, before, 0600); err != nil {
		t.Fatal(err)
	}
	specDir := t.TempDir()
	output := testsupport.PresentationOutput(t, dir)
	spec := writeSpec(t, specDir, "add.json", map[string]any{
		"action": "add", "slide": 1, "type": "histogram",
		"categories": []string{"First", "Second"}, "values": []int{3, 6},
		"position_emu": []int{914400, 914400, 5486400, 3657600},
	})
	result := runCLI(t, "extended-charts", source, "--output", output, "--requirements", spec)
	charts := result["charts"].([]any)
	if len(charts) != 1 {
		t.Fatalf("authored charts: %v", charts)
	}
	dimensions := charts[0].(map[string]any)["series"].([]any)[0].(map[string]any)["dimensions"]
	if !reflect.DeepEqual(dimensions, map[string]any{"cat": []any{"First", "Second"}, "val": []any{float64(3), float64(6)}}) {
		t.Fatal(dimensions)
	}
	saved, err := opc.Open(output)
	if err != nil {
		t.Fatal(err)
	}
	verified, err := office.VerifyChartData(saved, "")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(verified)
	if err != nil {
		t.Fatal(err)
	}
	var actual []any
	if err := json.Unmarshal(raw, &actual); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(actual, charts) {
		t.Fatalf("saved reports=%v returned=%v", actual, charts)
	}
	after, err := os.ReadFile(source)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("source changed", err)
	}
	original, err := opc.Read(before)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range original.Names() {
		switch name {
		case "[Content_Types].xml", "ppt/slides/slide1.xml", "ppt/slides/_rels/slide1.xml.rels":
			continue
		}
		if !bytes.Equal(original.Get(name), saved.Get(name)) {
			t.Errorf("unrelated member changed: %s", name)
		}
	}
}
