package office

import (
	"encoding/json"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"strings"
	"testing"
)

func TestExtendedCategoryJSONValidationBoundary(t *testing.T) {
	for _, labels := range []string{`[1,2]`, `[null,"Y"]`, `["X",null]`, `null`, `42`, `"XY"`} {
		t.Run(labels, func(t *testing.T) {
			p, err := opc.Open("../../testdata/fixtures/reference-categorical-histogram.pptx")
			if err != nil {
				t.Fatal(err)
			}
			reports, err := chartExReports(p)
			if err != nil || len(reports) != 1 {
				t.Fatal(reports, err)
			}
			raw := `{"action":"replace-data","values":[2,4],"categories":` + labels + `}`
			var s ExtendedChartSpec
			if err = json.Unmarshal([]byte(raw), &s); err != nil {
				t.Fatalf("decoder rejected category shape: %v", err)
			}
			s.Part = reports[0].Chart
			if s.Categories != nil {
				t.Fatal("invalid labels entered typed slice")
			}
			_, err = ExtendedCharts(p, s)
			if err == nil || !strings.Contains(err.Error(), "categories") {
				t.Fatal(err)
			}
		})
	}
}
