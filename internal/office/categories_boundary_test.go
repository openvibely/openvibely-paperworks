package office

import (
	"bytes"
	"reflect"
	"strings"
	"testing"

	"github.com/openvibely/openvibely-paperworks/internal/opc"
)

// The typed API cannot represent numeric category entries. That original
// rejection is tested separately at the public JSON boundary in cli.
func TestReferenceCategoricalTypedBoundary(t *testing.T) {
	for _, tc := range []struct {
		name    string
		labels  []string
		present bool
		values  []float64
		want    []string
	}{
		{"omitted-resize", nil, false, []float64{2, 4}, nil},
		{"short", []string{"Only one"}, false, []float64{2, 4}, nil},
		{"explicit-null", nil, true, []float64{2, 4}, nil},
		{"valid", []string{"X", "Y"}, false, []float64{2, 4}, []string{"X", "Y"}},
		{"typed-nil-retains", nil, false, []float64{2, 4, 6}, []string{"A", "B", "C"}},
		{"empty-not-omitted", []string{}, false, []float64{2, 4, 6}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, err := opc.Open("../../testdata/fixtures/reference-categorical-histogram.pptx")
			if err != nil {
				t.Fatal(err)
			}
			reports, err := chartExReports(p)
			if err != nil || len(reports) != 1 {
				t.Fatal(reports, err)
			}
			part := reports[0].Chart
			before := append([]byte(nil), p.Get(part)...)
			got, err := ExtendedCharts(p, ExtendedChartSpec{Action: "replace-data", Part: part, Values: tc.values, Categories: tc.labels, CategoriesPresent: tc.present})
			if tc.want == nil {
				if err == nil || !strings.Contains(err.Error(), "categories") {
					t.Fatalf("want categories error, got %v", err)
				}
				if !bytes.Equal(before, p.Get(part)) {
					t.Fatal("rejected edit changed chart")
				}
				return
			}
			if err != nil || len(got) != 1 {
				t.Fatal(got, err)
			}
			dims := got[0].Series[0].Dimensions
			if !reflect.DeepEqual(dims["cat"], tc.want) || !reflect.DeepEqual(dims["val"], tc.values) {
				t.Fatal(dims)
			}
		})
	}
}
