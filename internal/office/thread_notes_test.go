package office

import (
	"bytes"
	"context"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
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
	source := "../../testdata/regression/comment-identity-source.xlsx"
	p := previewOpen(t, source)
	items, err := WorkbookComments(p, CommentSpec{})
	if err != nil || len(items) == 0 {
		t.Fatal(items, err)
	}
	root := items[0]
	if root.Resolved {
		_, err = WorkbookComments(p, CommentSpec{Action: "reopen", Sheet: root.Sheet, ThreadID: root.ID})
		if err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 2; i++ {
		items, err = WorkbookComments(p, CommentSpec{Action: "reply", Sheet: root.Sheet, ThreadID: root.ID, Author: "Compatibility reviewer", Text: "Paired reply"})
		if err != nil {
			t.Fatal(err)
		}
		assertThreadNote(t, p, root, root.Text, "Paired reply")
		out := filepath.Join(t.TempDir(), "paired.xlsx")
		if err = p.SaveNew(out); err != nil {
			t.Fatal(err)
		}
		p = previewOpen(t, out)
	}
	_, _, sheets, _ := WorkbookSheets(p)
	_, d, _ := legacyComments(p, sheets[root.Sheet])
	count := 0
	for _, n := range Children(Child(d.Root(), NSS, "commentList"), NSS, "comment") {
		if n.SelectAttrValue("ref", "") == root.Cell {
			count++
		}
	}
	if count != 1 {
		t.Fatal("duplicate fallback", count)
	}
	items, err = WorkbookComments(p, CommentSpec{Action: "add", Sheet: root.Sheet, Cell: "Z99", Author: "Ana", Text: "New Excel thread"})
	if err != nil {
		t.Fatal(err)
	}
	assertThreadNote(t, p, items[len(items)-1], "New Excel thread")
	assertThreadNote(t, p, root, root.Text, "Paired reply")
}

func TestThreadLegacyLibreOffice(t *testing.T) {
	soffice := os.Getenv("DOCUMENT_SOFFICE")
	if soffice == "" {
		t.Fatal("DOCUMENT_SOFFICE required")
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
