package wordedit

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestExistingCommentEditPreservesIdentityAndAnchors(t *testing.T) {
	source := "../../testdata/regression/word-comment-anchor-baseline/source.docx"
	before, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	identities := func(raw string) [][]xml.Attr {
		t.Helper()
		dec := xml.NewDecoder(strings.NewReader(raw))
		var attrs [][]xml.Attr
		for {
			token, err := dec.Token()
			if err != nil {
				break
			}
			if start, ok := token.(xml.StartElement); ok && start.Name.Local == "comment" {
				attrs = append(attrs, start.Attr)
			}
		}
		return attrs
	}
	for id, old := range []string{"Body review", "Table review"} {
		t.Run(fmt.Sprint(id), func(t *testing.T) {
			replacement := "Revised review"
			out := mustEdit(t, source, map[string]any{"operation": "comment-edit", "id": float64(id), "text": replacement})
			original := part(t, source, "word/comments.xml")
			updated := part(t, out, "word/comments.xml")
			if !reflect.DeepEqual(identities(original), identities(updated)) {
				t.Fatal("comment identity metadata changed")
			}
			if strings.Contains(updated, old) || strings.Count(updated, replacement) != 1 {
				t.Fatal("target comment not replaced exactly once")
			}
			other := []string{"Table review", "Body review"}[id]
			contains(t, updated, other)
			zr, err := zip.NewReader(bytes.NewReader(before), int64(len(before)))
			if err != nil {
				t.Fatal(err)
			}
			afterRaw, err := os.ReadFile(out)
			if err != nil {
				t.Fatal(err)
			}
			afterZip, err := zip.NewReader(bytes.NewReader(afterRaw), int64(len(afterRaw)))
			if err != nil {
				t.Fatal(err)
			}
			if len(zr.File) != len(afterZip.File) {
				t.Fatal("package part count changed")
			}
			for _, f := range zr.File {
				if f.Name != "word/comments.xml" && part(t, source, f.Name) != part(t, out, f.Name) {
					t.Fatalf("part changed: %s", f.Name)
				}
			}
		})
	}
	after, err := os.ReadFile(source)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("source modified")
	}
}
