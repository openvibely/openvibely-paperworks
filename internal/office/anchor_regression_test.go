package office

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestReferenceCommentAnchorReportRegression(t *testing.T) {
	raw, err := os.ReadFile("../../testdata/inspection-reports/comment-anchors.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Input, SHA256 string
		Comments      []SlideComment
	}
	if err = json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	for _, c := range cases {
		t.Run(c.Input, func(t *testing.T) {
			path := filepath.Join("../../testdata", c.Input)
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if fmt.Sprintf("%x", sha256.Sum256(data)) != c.SHA256 {
				t.Fatal("baseline hash mismatch")
			}
			p, err := opc.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			got, err := SlideComments(p, SlideCommentSpec{})
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, c.Comments) {
				t.Fatalf("comment report mismatch\ngot: %+v\nwant: %+v", got, c.Comments)
			}
		})
	}
}
