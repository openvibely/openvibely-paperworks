package office

import (
	"bytes"
	"context"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"github.com/xuri/excelize/v2"
)

func assertThreadNote(t *testing.T, p *opc.Package, thread Thread, fragments ...string) {
	t.Helper()
	_, _, sheets, err := WorkbookSheets(p)
	if err != nil {
		t.Fatal(err)
	}
	part, d, err := legacyComments(p, sheets[thread.Sheet])
	if err != nil || part == "" {
		t.Fatal(part, err)
	}
	if p.ContentType(part) != "application/vnd.openxmlformats-officedocument.spreadsheetml.comments+xml" {
		t.Fatal("missing comments content type")
	}
	note := legacyNoteAt(d, thread.Cell)
	if note == nil {
		t.Fatal("missing fallback")
	}
	aid, err := strconv.Atoi(note.SelectAttrValue("authorId", ""))
	if err != nil {
		t.Fatal(err)
	}
	if got := Children(Child(d.Root(), NSS, "authors"), NSS, "author")[aid].Text(); got != "tc="+thread.ID {
		t.Fatal(got)
	}
	text := Text(Child(note, NSS, "text"), NSS, "t")
	for _, fragment := range fragments {
		if !strings.Contains(text, fragment) {
			t.Fatal(text, fragment)
		}
	}
	sheet, err := p.XML(sheets[thread.Sheet])
	if err != nil {
		t.Fatal(err)
	}
	legacy := Child(sheet.Root(), NSS, "legacyDrawing")
	if legacy == nil {
		t.Fatal("missing legacyDrawing")
	}
	for _, later := range After(WorksheetOrder, "legacyDrawing") {
		if e := Child(sheet.Root(), NSS, later); e != nil && e.Index() < legacy.Index() {
			t.Fatal("legacyDrawing out of order")
		}
	}
	rels, _ := p.Rels(sheets[thread.Sheet])
	shapes := 0
	for _, r := range rels {
		if r.ID == AttrV(legacy, NSR, "id") {
			vml, err := p.XML(opc.Resolve(sheets[thread.Sheet], r.Target))
			if err != nil {
				t.Fatal(err)
			}
			col, row, _ := SplitCell(thread.Cell)
			for _, cd := range All(vml.Root(), "urn:schemas-microsoft-com:office:excel", "ClientData") {
				if Text(cd, cd.NamespaceURI(), "Row") == strconv.Itoa(row-1) && Text(cd, cd.NamespaceURI(), "Column") == strconv.Itoa(col-1) {
					shapes++
				}
			}
		}
	}
	if shapes != 1 {
		t.Fatalf("got %d matching note shapes", shapes)
	}
}

func TestThreadLegacyNotes(t *testing.T) {
	for _, existing := range []bool{false, true} {
		t.Run(strconv.FormatBool(existing), func(t *testing.T) {
			source := filepath.Join(t.TempDir(), "empty.xlsx")
			f := excelize.NewFile()
			_, _ = f.NewSheet("Second")
			if existing {
				if err := f.AddComment("Second", excelize.Comment{Cell: "A1", Author: "Original", Text: "Keep me"}); err != nil {
					t.Fatal(err)
				}
			}
			if err := f.SaveAs(source); err != nil {
				t.Fatal(err)
			}
			f.Close()
			path, items, err := referenceThreadEdit(t, source, CommentSpec{Action: "add", Sheet: "Second", Cell: "B2", Author: "Ana", Text: "Check this"})
			if err != nil {
				t.Fatal(err)
			}
			assertThreadNote(t, previewOpen(t, path), items[0], "Ana", "Check this")
			for _, action := range []string{"reply", "resolve", "reopen", "reply"} {
				before := previewOpen(t, path)
				path, items, err = referenceThreadEdit(t, path, CommentSpec{Action: action, Sheet: "Second", ThreadID: items[0].ID, Author: "Sam", Text: "Confirmed"})
				if err != nil {
					t.Fatal(err)
				}
				after := previewOpen(t, path)
				assertThreadNote(t, after, items[0], "Check this", "Confirmed")
				if action == "resolve" || action == "reopen" {
					part, _, _ := legacyComments(before, "xl/worksheets/sheet2.xml")
					b := before.Get(part)
					a := after.Get(part)
					if !bytes.Equal(a, b) {
						t.Fatal("resolution changed fallback")
					}
				}
			}
			if len(items) != 3 {
				t.Fatal(items)
			}
			f, err = excelize.OpenFile(path)
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			notes, err := f.GetComments("Second")
			want := 1
			if existing {
				want++
			}
			if err != nil || len(notes) != want {
				t.Fatal(notes, err)
			}
			if existing {
				p := previewOpen(t, path)
				before, err := p.Bytes()
				if err != nil {
					t.Fatal(err)
				}
				_, err = WorkbookComments(p, CommentSpec{Action: "add", Sheet: "Second", Cell: "A1", Author: "Ana", Text: "collision"})
				if err == nil || !strings.Contains(err.Error(), "legacy note") {
					t.Fatal(err)
				}
				after, err := p.Bytes()
				if err != nil || !bytes.Equal(before, after) {
					t.Fatal("collision changed package", err)
				}
				if notes[0].Text != "Keep me" {
					t.Fatal(notes)
				}
			}
		})
	}
}

func TestExcelPairedThreadReply(t *testing.T) {
	// This static fixture is paired before WorkbookComments performs any edit.
	source := "../../testdata/regression/comment-paired-source.xlsx"
	original := previewRead(t, source)
	p := previewOpen(t, source)
	initial, err := WorkbookComments(p, CommentSpec{})
	if err != nil || len(initial) != 2 {
		t.Fatal(initial, err)
	}
	root := initial[0]
	if root.ParentID != nil || root.Resolved {
		t.Fatal("fixture needs an open root", root)
	}
	_, _, sheets, err := WorkbookSheets(p)
	if err != nil {
		t.Fatal(err)
	}
	owner := sheets[root.Sheet]
	const commentsPart = "xl/comments/comment7.xml"
	const vmlPart = "xl/drawings/vmlDrawing7.vml"
	originalVML := append([]byte(nil), p.Get(vmlPart)...)
	if len(originalVML) == 0 {
		t.Fatal("fixture lacks pre-existing VML")
	}
	originalRels := append([]byte(nil), p.Get(opc.RelsName(owner))...)
	originalSheet := append([]byte(nil), p.Get(owner)...)
	originalNames := p.Names()

	// Check the input as well as every edited and reopened package. A fixture
	// missing a legacy note or shape must fail before the first reply can repair it.
	assertPair := func(wantCount int, fragments ...string) {
		t.Helper()
		listed, err := WorkbookComments(p, CommentSpec{})
		if err != nil || len(listed) != wantCount {
			t.Fatal("thread count", listed, err)
		}
		if !reflect.DeepEqual(listed[:len(initial)], initial) {
			t.Fatal("original threads changed")
		}
		part, d, err := legacyComments(p, owner)
		if err != nil || part != commentsPart || d == nil {
			t.Fatal("pre-existing comments part not reused", part, err)
		}
		notes := Children(Child(d.Root(), NSS, "commentList"), NSS, "comment")
		authors := Children(Child(d.Root(), NSS, "authors"), NSS, "author")
		if len(notes) != 1 || notes[0].SelectAttrValue("ref", "") != root.Cell || notes[0].SelectAttrValue("authorId", "") != "0" || len(authors) != 1 || authors[0].Text() != "tc="+root.ID {
			t.Fatal("missing or duplicated paired note/author")
		}
		var text strings.Builder
		for _, e := range All(notes[0], NSS, "t") {
			text.WriteString(e.Text())
		}
		for _, fragment := range fragments {
			if strings.Count(text.String(), fragment) != 1 {
				t.Fatalf("fallback must contain %q exactly once: %q", fragment, text.String())
			}
		}
		if !bytes.Equal(p.Get(vmlPart), originalVML) || !bytes.Equal(p.Get(opc.RelsName(owner)), originalRels) || !bytes.Equal(p.Get(owner), originalSheet) || !reflect.DeepEqual(p.Names(), originalNames) {
			t.Fatal("reply replaced or duplicated existing parts, relationships, or VML")
		}
		sheet, err := p.XML(owner)
		if err != nil {
			t.Fatal(err)
		}
		legacy := Child(sheet.Root(), NSS, "legacyDrawing")
		if legacy == nil || AttrV(legacy, NSR, "id") != "rIdVML7" {
			t.Fatal("fixture lacks legacyDrawing link")
		}
		rels, err := p.Rels(owner)
		if err != nil {
			t.Fatal(err)
		}
		counts := map[string]int{}
		for _, r := range rels {
			if r.Type == NSR+"/comments" || r.Type == NSR+"/vmlDrawing" {
				counts[r.Type]++
				want := commentsPart
				if r.Type == NSR+"/vmlDrawing" {
					want = vmlPart
					if r.ID != "rIdVML7" {
						t.Fatal("VML relationship ID", r.ID)
					}
				}
				if r.TargetMode == "External" || opc.Resolve(owner, r.Target) != want {
					t.Fatal("incorrect pair relationship", r)
				}
			}
		}
		if counts[NSR+"/comments"] != 1 || counts[NSR+"/vmlDrawing"] != 1 {
			t.Fatal("missing or duplicate pair relationships", counts)
		}
		if p.ContentType(commentsPart) != "application/vnd.openxmlformats-officedocument.spreadsheetml.comments+xml" || p.ContentType(vmlPart) != "application/vnd.openxmlformats-officedocument.vmlDrawing" {
			t.Fatal("missing pair content types")
		}
		vml, err := p.XML(vmlPart)
		if err != nil {
			t.Fatal(err)
		}
		shapes := All(vml.Root(), NSV, "shape")
		if len(shapes) != 1 {
			t.Fatal("missing or duplicate VML shapes", len(shapes))
		}
		const nsExcel = "urn:schemas-microsoft-com:office:excel"
		cd := Child(shapes[0], nsExcel, "ClientData")
		col, row, _ := SplitCell(root.Cell)
		if cd == nil || cd.SelectAttrValue("ObjectType", "") != "Note" || Text(cd, nsExcel, "Row") != strconv.Itoa(row-1) || Text(cd, nsExcel, "Column") != strconv.Itoa(col-1) {
			t.Fatal("fixture note shape is not anchored at thread cell")
		}
	}
	fragments := []string{root.Text, initial[1].Text}
	assertPair(len(initial), fragments...)
	for i, text := range []string{"First paired reply", "Second paired reply"} {
		// Reuse the existing author so no unrelated person parts need changing.
		_, err = WorkbookComments(p, CommentSpec{Action: "reply", Sheet: root.Sheet, ThreadID: root.ID, Author: root.Author, AuthorID: &root.AuthorID, Text: text})
		if err != nil {
			t.Fatal(err)
		}
		fragments = append(fragments, text)
		assertPair(len(initial)+i+1, fragments...)
		assertThreadNote(t, p, root, fragments...)
		out := filepath.Join(t.TempDir(), "paired.xlsx")
		if err = p.SaveNew(out); err != nil {
			t.Fatal(err)
		}
		p = previewOpen(t, out)
		assertPair(len(initial)+i+1, fragments...)
		assertThreadNote(t, p, root, fragments...)
	}
	items, err := WorkbookComments(p, CommentSpec{Action: "add", Sheet: root.Sheet, Cell: "Z99", Author: "Ana", Text: "New Excel thread"})
	if err != nil {
		t.Fatal(err)
	}
	assertThreadNote(t, p, items[len(items)-1], "New Excel thread")
	assertThreadNote(t, p, root, fragments...)
	if !bytes.Equal(original, previewRead(t, source)) {
		t.Fatal("source fixture changed")
	}
}

func TestThreadLegacyLibreOffice(t *testing.T) {
	soffice := os.Getenv("DOCUMENT_SOFFICE")
	if soffice == "" {
		t.Fatal("LibreOffice test dependency was not initialized")
	}
	source := "../../testdata/reference-mirror/stale-sparkline.xlsx"
	if _, err := os.Stat(source); err != nil {
		t.Fatal(err)
	}
	path, items, err := referenceThreadEdit(t, source, CommentSpec{Action: "add", Sheet: "Inputs", Cell: "B2", Author: "Ana", Text: "Check this"})
	if err != nil {
		t.Fatal(err)
	}
	path, _, err = referenceThreadEdit(t, path, CommentSpec{Action: "reply", Sheet: "Inputs", ThreadID: items[0].ID, Author: "Sam", Text: "Confirmed reply"})
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	profile := (&url.URL{Scheme: "file", Path: filepath.Join(t.TempDir(), "profile")}).String()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	raw, err := exec.CommandContext(ctx, soffice, "-env:UserInstallation="+profile, "--headless", "--convert-to", "ods", "--outdir", dir, path).CombinedOutput()
	if err != nil {
		t.Fatal(err, string(raw))
	}
	z := referencePreviewZIP(t, filepath.Join(dir, "comments.ods"))
	content := string(referencePreviewZIPRead(t, z, "content.xml"))
	if !strings.Contains(content, "office:annotation") || !strings.Contains(content, "Check this") || !strings.Contains(content, "Confirmed reply") {
		t.Fatal("ODS lost comment text", string(raw))
	}
}
