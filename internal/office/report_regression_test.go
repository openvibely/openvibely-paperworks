package office

import (
	"context"
	"encoding/json"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// Exact JSON oracle from the retained original Reference replay, without file path.
// No field normalization or sorting is allowed here.
func TestReferenceInspectionReportRegression(t *testing.T) {
	root := "../../testdata/inspection-reports"
	data, err := os.ReadFile(filepath.Join(root, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Cases []struct {
			Name, Input, Kind string
			Arguments         struct {
				Sheet, Cell string
				MaxNodes    int `json:"max_nodes"`
				MaxDepth    int `json:"max_depth"`
			}
		}
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	for _, c := range manifest.Cases {
		t.Run(c.Name, func(t *testing.T) {
			report, err := PackageReport(filepath.Join(root, c.Input))
			if err != nil {
				t.Fatal(err)
			}
			delete(report, "file")
			var value any = report
			if c.Kind == "trace" {
				p, e := opc.Open(filepath.Join(root, c.Input))
				if e != nil {
					t.Fatal(e)
				}
				value, err = TraceWorkbook(context.Background(), p, c.Arguments.Sheet, c.Arguments.Cell, c.Arguments.MaxNodes, c.Arguments.MaxDepth)
				if err != nil {
					t.Fatal(err)
				}
			}
			raw, err := json.Marshal(value)
			if err != nil {
				t.Fatal(err)
			}
			var got, want map[string]any
			if err := json.Unmarshal(raw, &got); err != nil {
				t.Fatal(err)
			}
			expected, err := os.ReadFile(filepath.Join(root, c.Name+".json"))
			if err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(expected, &want); err != nil {
				t.Fatal(err)
			}
			for k, v := range want {
				if !reflect.DeepEqual(v, got[k]) {
					t.Errorf("field %s: got %v; want %v", k, got[k], v)
				}
			}
			for k := range got {
				if _, ok := want[k]; !ok {
					t.Errorf("unexpected field %s", k)
				}
			}
		})
	}
}
