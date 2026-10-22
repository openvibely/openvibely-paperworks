package office

import (
	"bytes"
	"crypto/sha256"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
)

func referenceThreadEdit(t *testing.T, source string, spec CommentSpec) (string, []Thread, error) {
	t.Helper()
	if spec.Sheet == "" {
		spec.Sheet = "Output"
	}
	p := previewOpen(t, source)
	_, err := WorkbookComments(p, spec)
	if err != nil {
		return "", nil, err
	}
	out := filepath.Join(t.TempDir(), "comments.xlsx")
	if err := p.SaveNew(out); err != nil {
		return "", nil, err
	}
	items, err := WorkbookComments(previewOpen(t, out), CommentSpec{})
	return out, items, err
}
func TestReferenceThreadLifecycleAndPreservation(t *testing.T) {
	_, source := referenceTraceFixture(t, nil)
	before := sha256.Sum256(previewRead(t, source))
	path, items, e := referenceThreadEdit(t, source, CommentSpec{Action: "add", Cell: "B2", Author: "Alex", Text: "Check assumption"})
	if e != nil || len(items) != 1 {
		t.Fatal(items, e)
	}
	id := items[0].ID
	path, items, e = referenceThreadEdit(t, path, CommentSpec{Action: "reply", ThreadID: id, Author: "Sam", Text: "Confirmed"})
	if e != nil || len(items) != 2 {
		t.Fatal(items, e)
	}
	if !reflect.DeepEqual([]string{items[0].Author, items[1].Author}, []string{"Alex", "Sam"}) {
		t.Fatal(items)
	}
	if items[1].ParentID == nil || *items[1].ParentID != id {
		t.Fatal("reply parent", items)
	}
	path, items, e = referenceThreadEdit(t, path, CommentSpec{Action: "resolve", ThreadID: id})
	if e != nil || !items[0].Resolved {
		t.Fatal(items, e)
	}
	if _, _, e := referenceThreadEdit(t, path, CommentSpec{Action: "reply", ThreadID: id, Author: "Alex", Text: "More"}); e == nil || !strings.Contains(e.Error(), "Reopen") {
		t.Fatal("resolved reply accepted", e)
	}
	path, items, e = referenceThreadEdit(t, path, CommentSpec{Action: "reopen", ThreadID: id})
	if e != nil || items[0].Resolved {
		t.Fatal(items, e)
	}
	path, items, e = referenceThreadEdit(t, path, CommentSpec{Action: "reply", ThreadID: id, Author: "Alex", Text: "Thanks"})
	if e != nil || len(items) != 3 {
		t.Fatal(items, e)
	}
	if items[0].AuthorID != items[2].AuthorID {
		t.Fatal("author not reused")
	}
	old, updated := referencePreviewZIP(t, source), referencePreviewZIP(t, path)
	for _, part := range old.File {
		n := part.Name
		if n != "xl/worksheets/sheet3.xml" && n != "[Content_Types].xml" && n != "xl/_rels/workbook.xml.rels" && !bytes.Equal(referencePreviewZIPRead(t, old, n), referencePreviewZIPRead(t, updated, n)) {
			t.Fatal("changed part", n)
		}
	}
	fresh := previewOpen(t, path)
	people := previewXML(t, fresh, "xl/persons/person1.xml")
	if !Is(people, NSTC, "personList") {
		t.Fatal("persons root")
	}
	if len(people.ChildElements()) != 2 {
		t.Fatal("persons root/count")
	}
	comments := previewXML(t, fresh, "xl/threadedComments/threadedComment1.xml")
	if !Is(comments, NSTC, "ThreadedComments") {
		t.Fatal("comments root")
	}
	children := comments.ChildElements()
	if len(children) != 3 {
		t.Fatal("serialized thread count", len(children))
	}
	parent, rootID := children[1].SelectAttr("parentId"), children[0].SelectAttr("id")
	if (parent == nil) != (rootID == nil) || (parent != nil && rootID != nil && parent.Value != rootID.Value) {
		t.Fatal("serialized parent")
	}
	if children[0].SelectAttrValue("done", "") != "0" {
		t.Fatal("serialized resolution")
	}
	rels := previewXML(t, fresh, "xl/worksheets/_rels/sheet3.xml.rels")
	found := false
	for _, r := range rels.ChildElements() {
		found = found || r.SelectAttrValue("Type", "") == relTR+"threadedComment"
	}
	if !found {
		t.Fatal("thread relationship missing")
	}
	found = false
	for _, c := range previewXML(t, fresh, "[Content_Types].xml").ChildElements() {
		found = found || c.SelectAttrValue("ContentType", "") == "application/vnd.ms-excel.person+xml"
	}
	if !found {
		t.Fatal("person content type missing")
	}
	f, e := excelize.OpenFile(path)
	if e != nil {
		t.Fatal(e)
	}
	defer f.Close()
	notes, e := f.GetComments("Inputs")
	if e != nil || len(notes) != 1 || notes[0].Cell != "A1" || notes[0].Text != "Keep this note" {
		t.Fatal(notes, e)
	}
	if sha256.Sum256(previewRead(t, source)) != before {
		t.Fatal("source hash changed")
	}
}
func TestReferenceThreadValidation(t *testing.T) {
	_, source := referenceTraceFixture(t, nil)
	for _, kind := range []string{"author", "XFE1", "A0", "text"} {
		s := CommentSpec{Action: "add", Cell: "A1", Author: "Alex", Text: "Text"}
		switch kind {
		case "author":
			s.Author = ""
		case "text":
			s.Text = ""
		default:
			s.Cell = kind
		}
		if _, _, e := referenceThreadEdit(t, source, s); e == nil {
			t.Fatal("invalid comment accepted")
		}
	}
	path, _, e := referenceThreadEdit(t, source, CommentSpec{Action: "add", Cell: "A1", Author: "Alex", Text: "Text"})
	if e != nil {
		t.Fatal(e)
	}
	for _, spec := range []CommentSpec{{Action: "add", Cell: "A1", Author: "Alex", Text: "duplicate"}, {Action: "reply", ThreadID: "missing", Author: "Alex", Text: "orphan"}} {
		if _, _, e := referenceThreadEdit(t, path, spec); e == nil {
			t.Fatal("invalid comment accepted")
		}
	}
}
func TestReferenceThreadsAcrossSheetsReuseAuthor(t *testing.T) {
	_, source := referenceTraceFixture(t, nil)
	path, _, e := referenceThreadEdit(t, source, CommentSpec{Action: "add", Cell: "A1", Author: "Alex", Text: "one"})
	if e != nil {
		t.Fatal(e)
	}
	_, items, e := referenceThreadEdit(t, path, CommentSpec{Action: "add", Sheet: "Inputs", Cell: "B1", Author: "Alex", Text: "two"})
	if e != nil || len(items) != 2 {
		t.Fatal(items, e)
	}
	if items[0].AuthorID != items[1].AuthorID {
		t.Fatal("author IDs differ")
	}
	sheets := map[string]bool{}
	for _, item := range items {
		sheets[item.Sheet] = true
	}
	if !reflect.DeepEqual(sheets, map[string]bool{"Inputs": true, "Output": true}) {
		t.Fatal(sheets)
	}
}
