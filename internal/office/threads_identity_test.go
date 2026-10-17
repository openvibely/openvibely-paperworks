package office

import (
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/openvibely/openvibely-paperworks/internal/opc"
)

func TestAmbiguousWorkbookAuthorSelection(t *testing.T) {
	source := "../../testdata/regression/comment-ambiguous-baseline/source.xlsx"
	before, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	for _, selectPerson := range []int{-1, 0, 1} {
		p, err := opc.Open(source)
		if err != nil {
			t.Fatal(err)
		}
		original, err := WorkbookComments(p, CommentSpec{})
		if err != nil {
			t.Fatal(err)
		}
		if len(original) != 2 || original[0].Author != original[1].Author || original[0].AuthorID == original[1].AuthorID {
			t.Fatal("invalid ambiguity fixture")
		}
		spec := CommentSpec{Action: "reply", Sheet: original[0].Sheet, ThreadID: original[0].ID, Author: original[0].Author, Text: "Disambiguated reviewer"}
		if selectPerson >= 0 {
			spec.AuthorID = &original[selectPerson].AuthorID
		}
		got, err := WorkbookComments(p, spec)
		if selectPerson < 0 {
			if err == nil || !strings.Contains(err.Error(), "ambiguous author") {
				t.Fatalf("expected ambiguity rejection: %v", err)
			}
			after, listErr := WorkbookComments(p, CommentSpec{})
			if listErr != nil || !reflect.DeepEqual(after, original) {
				t.Fatalf("rejection changed comments: %v", listErr)
			}
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 3 || !reflect.DeepEqual(got[:2], original) {
			t.Fatal("existing identities changed")
		}
		added := got[2]
		if added.AuthorID != *spec.AuthorID || added.ParentID == nil || *added.ParentID != original[0].ID || added.Text != spec.Text || added.Date == "" || added.ID == "" || added.ID == original[0].ID || added.ID == original[1].ID {
			t.Fatalf("incorrect reply: %+v", added)
		}
	}
	after, err := os.ReadFile(source)
	if err != nil || string(after) != string(before) {
		t.Fatal("source changed")
	}
}

func TestWorkbookInvalidThreadParents(t *testing.T) {
	for _, name := range []string{"missing-parent", "self-parent", "different-cell", "valid"} {
		t.Run(name, func(t *testing.T) {
			source := "../../testdata/regression/comment-parent-baseline/" + name + ".xlsx"
			before, err := os.ReadFile(source)
			if err != nil {
				t.Fatal(err)
			}
			p, err := opc.Open(source)
			if err != nil {
				t.Fatal(err)
			}
			comments, err := WorkbookComments(p, CommentSpec{})
			if name != "valid" {
				if err == nil {
					t.Fatal("invalid parent accepted")
				}
			} else {
				if err != nil || len(comments) != 2 {
					t.Fatalf("control: %v", err)
				}
				got, err := WorkbookComments(p, CommentSpec{Action: "resolve", Sheet: comments[0].Sheet, ThreadID: comments[0].ID})
				if err != nil {
					t.Fatal(err)
				}
				comments[0].Resolved = true
				if !reflect.DeepEqual(got, comments) {
					t.Fatal("resolve changed identity or content")
				}
			}
			after, err := os.ReadFile(source)
			if err != nil || string(after) != string(before) {
				t.Fatal("source changed")
			}
		})
	}
}
