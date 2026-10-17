package convert

import (
	"encoding/json"
	"reflect"
	"testing"
)

// Reference compare_files includes bounds for matched pages (null if identical),
// and omits both image and bounds for unmatched added/removed pages.
func TestReferencePageComparisonReports(t *testing.T) {
	for _, tc := range []struct {
		page PageChange
		want string
	}{
		{PageChange{Page: 1, Status: "same", Image: "diff-0001.png"}, `{"page":1,"status":"same","bounds":null,"image":"diff-0001.png"}`},
		{PageChange{Page: 1, Status: "changed", Bounds: []int{0, 1, 2, 3}, Image: "diff-0001.png"}, `{"page":1,"status":"changed","bounds":[0,1,2,3],"image":"diff-0001.png"}`},
		{PageChange{Page: 2, Status: "added"}, `{"page":2,"status":"added"}`},
		{PageChange{Page: 2, Status: "removed"}, `{"page":2,"status":"removed"}`},
	} {
		t.Run(tc.page.Status, func(t *testing.T) {
			raw, err := json.Marshal(tc.page)
			if err != nil {
				t.Fatal(err)
			}
			var got, want any
			if err = json.Unmarshal(raw, &got); err != nil {
				t.Fatal(err)
			}
			if err = json.Unmarshal([]byte(tc.want), &want); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("got %s; want %s", raw, tc.want)
			}
		})
	}
}
