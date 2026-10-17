package office

import (
	"bytes"
	"github.com/beevik/etree"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestModernCommentsPreservation(t *testing.T) {
	referencePresentationRoot(t)
	p := fixture(t, "reference-comments.pptx")
	before := map[string][]byte{}
	var names []string
	for _, v := range p.Parts {
		names = append(names, v.Name)
		before[v.Name] = append([]byte(nil), v.Data...)
	}
	list, e := SlideComments(p, SlideCommentSpec{})
	if e != nil || len(list) == 0 {
		t.Fatal(list, e)
	}
	id := list[0].ID
	for _, action := range []string{"reopen", "reply", "react", "resolve"} {
		s := SlideCommentSpec{Action: action, ThreadID: id}
		if action == "reply" {
			s.Author, s.Text = "Second Reviewer", "Updated"
		}
		if action == "react" {
			s.Author, s.Reaction = "Second Reviewer", "👍"
			s.ReplyID = list[len(list)-1].ID
		}
		list, e = SlideComments(p, s)
		if e != nil {
			t.Fatal(action, e)
		}
		// Reference asserts the edit's returned report, not a replacement read.
		if action == "reply" && (len(list) == 0 || list[len(list)-1].Text != "Updated") {
			t.Fatal("returned reply report", list)
		}
		if action == "react" && (len(list) == 0 || len(list[len(list)-1].Reactions) == 0 || list[len(list)-1].Reactions[0].Type != "👍") {
			t.Fatal("returned reaction report", list)
		}
		if action == "resolve" {
			for _, comment := range list {
				if comment.Status != "resolved" {
					t.Fatal("returned resolve report", list)
				}
			}
		}
		// Match the original successive output files, including serialization.
		out := referenceNextPresentation(t)
		if e = p.SaveNew(out); e != nil {
			t.Fatal(e)
		}
		p = previewOpen(t, out)
		list, e = SlideComments(p, SlideCommentSpec{})
		if e != nil {
			t.Fatal(e)
		}
		if action == "reply" && list[len(list)-1].Text != "Updated" {
			t.Fatal(list)
		}
		if action == "react" {
			r := list[len(list)-1].Reactions
			if len(r) != 1 || r[0].Type != "👍" {
				t.Fatal(r)
			}
		}
	}
	for _, v := range list {
		if v.Status != "resolved" {
			t.Fatal(v)
		}
	}
	for _, name := range names {
		data := before[name]
		if name == "ppt/authors.xml" || name == "ppt/comments/modernComment.xml" || name == "[Content_Types].xml" {
			continue
		}
		if !bytes.Equal(data, p.Get(name)) {
			t.Fatal("unrelated part changed", name)
		}
	}
	out := referenceNextPresentation(t)
	_, e = SlideComments(p, SlideCommentSpec{Action: "reply", ThreadID: id, Author: "X", Text: "Y"})
	if e == nil {
		e = p.SaveNew(out)
	}
	if e == nil || !strings.Contains(e.Error(), "Reopen") {
		t.Fatal("want Reopen error", e)
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Fatal("failed reply published output", err)
	}
}
func TestModernTextAnchorsMatchReference(t *testing.T) {
	referencePresentationRoot(t)
	for i, query := range []string{"Second", "paragraph\nSecond"} {
		t.Run(query, func(t *testing.T) { referenceTextAnchorCase(t, "reference-multiline-comments.pptx", query, i) })
	}
}

func referenceTextAnchorCase(t *testing.T, file, query string, index int) {
	t.Helper()

	p := fixture(t, file)
	old, e := SlideComments(p, SlideCommentSpec{})
	if e != nil {
		t.Fatal(e)
	}
	if file == "reference-anchors.pptx" && len(old) != 3 {
		t.Fatal(old)
	}
	readText := func(pkg *opc.Package) string {
		d, e := pkg.XML("ppt/slides/slide1.xml")
		if e != nil {
			t.Fatal(e)
		}
		tree := Child(Child(d.Root(), NSP, "cSld"), NSP, "spTree")
		shapes := Children(tree, NSP, "sp")
		if len(shapes) == 0 {
			t.Fatal("missing first shape")
		}
		return referenceCommentShapeText(shapes[0])
	}
	beforeText := readText(p)
	author, text := "Local", "Review"
	if file == "reference-anchors.pptx" {
		author, text = "Local Reviewer", "New text comment"
	}
	r, e := SlideComments(p, SlideCommentSpec{Action: "add", Slide: 1, ShapeID: 2, Match: &query, Author: author, Text: text})
	if e != nil {
		t.Fatal(e)
	}
	a, e := opc.ParseXML([]byte(r[len(r)-1].Anchors[0]))
	if e != nil {
		t.Fatal(e)
	}
	b, e := opc.ParseXML([]byte(old[index].Anchors[0]))
	if e != nil {
		t.Fatal(e)
	}
	for _, tag := range []string{"txMk", "spMk"} {
		actual := Child(a.Root(), nsOAC, tag)
		expected := Child(b.Root(), nsOAC, tag)
		if actual == nil || expected == nil {
			t.Fatal("missing moniker", tag, r[len(r)-1].Anchors, old[index].Anchors)
		}
		attrs := func(node *etree.Element) map[string]string {
			out := map[string]string{}
			for _, attr := range node.Attr {
				if attr.Space != "xmlns" && attr.Key != "xmlns" {
					out[attr.FullKey()] = attr.Value
				}
			}
			return out
		}
		if !reflect.DeepEqual(attrs(actual), attrs(expected)) {
			t.Fatal(tag, actual.Attr, expected.Attr)
		}
	}
	out := referenceNextPresentation(t)
	if e := p.SaveNew(out); e != nil {
		t.Fatal(e)
	}
	reopened := previewOpen(t, out)
	// Match the original text-preservation assertion after saving/reopening.
	if beforeText != readText(reopened) {
		t.Fatal("comment edit rewrote slide content")
	}
}

func TestCommentReactionToggle(t *testing.T) {
	p := fixture(t, "reference-comments.pptx")
	r, e := SlideComments(p, SlideCommentSpec{})
	if e != nil {
		t.Fatal(e)
	}
	s := SlideCommentSpec{Action: "react", ThreadID: r[0].ID, Author: "Toggle reviewer", Reaction: "test"}
	for range 2 {
		if _, e = SlideComments(p, s); e != nil {
			t.Fatal(e)
		}
	}
	r, e = SlideComments(p, SlideCommentSpec{})
	if e != nil {
		t.Fatal(e)
	}
	for _, v := range r[0].Reactions {
		if v.Type == "test" {
			t.Fatal("reaction did not toggle off")
		}
	}
}
