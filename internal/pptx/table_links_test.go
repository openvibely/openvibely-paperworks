package pptx

import (
	"context"
	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"path/filepath"
	"testing"
)

func TestTableRunHyperlinks(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	in := filepath.Join(dir, "in.pptx")
	paras := []Paragraph{{Runs: []Run{{Text: "Visit", Hyperlink: "https://example.com", Font: "Courier New", UnderlineStyle: "dbl"}, {Text: " plain"}}}}
	format := &TableFormat{Cells: []TableCellFormat{{Row: 0, Column: 0, Paragraphs: paras}}}
	spec := Spec{Slides: []Slide{{Elements: []Element{{Type: "table", W: 4, H: 2, Rows: [][]string{{"old", "Keep"}}, TableFormat: format}}}}}
	if err := Create(ctx, spec, "", in); err != nil {
		t.Fatal(err)
	}
	check := func(path string) {
		t.Helper()
		pkg, err := opc.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		d, err := pkg.XML("ppt/slides/slide1.xml")
		if err != nil {
			t.Fatal(err)
		}
		cells := office.All(d.Root(), ans, "tc")
		links := office.All(cells[0], ans, "hlinkClick")
		if len(links) != 1 {
			t.Fatalf("expected one link, got %d", len(links))
		}
		id := links[0].SelectAttrValue("r:id", "")
		rels, err := pkg.Rels("ppt/slides/slide1.xml")
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, r := range rels {
			if r.ID == id && r.Type == rns+"/hyperlink" && r.Target == "https://example.com" && r.TargetMode == "External" {
				found = true
			}
		}
		if !found {
			t.Fatal("missing external hyperlink relationship")
		}
		if office.All(cells[1], ans, "t")[0].Text() != "Keep" {
			t.Fatal("unrelated cell changed")
		}
	}
	check(in)
	for _, op := range []Edit{{Operation: "table-cell", Slide: 1, ShapeID: "2", Paragraphs: paras}, {Operation: "table-format", Slide: 1, ShapeID: "2", TableFormat: format}} {
		out := filepath.Join(dir, op.Operation+".pptx")
		if _, err := EditFile(ctx, in, out, EditSpec{Edits: []Edit{op}}); err != nil {
			t.Fatal(err)
		}
		check(out)
	}
	paras[0].Runs[0].Hyperlink = ""
	cleared := filepath.Join(dir, "cleared.pptx")
	if _, err := EditFile(ctx, in, cleared, EditSpec{Edits: []Edit{{Operation: "table-cell", Slide: 1, ShapeID: "2", Paragraphs: paras}}}); err != nil {
		t.Fatal(err)
	}
	clearedPkg, err := opc.Open(cleared)
	if err != nil {
		t.Fatal(err)
	}
	clearedDoc, err := clearedPkg.XML("ppt/slides/slide1.xml")
	if err != nil {
		t.Fatal(err)
	}
	if len(office.All(clearedDoc.Root(), ans, "hlinkClick")) != 0 {
		t.Fatal("replaced text retained hyperlink")
	}
	paras[0].Runs[0].Hyperlink = "javascript:alert(1)"
	if err := Create(ctx, spec, "", filepath.Join(dir, "invalid.pptx")); err == nil {
		t.Fatal("invalid link accepted")
	}
}

func TestTableRunUnderlineValidation(t *testing.T) {
	for _, run := range []Run{{Text: "bad", UnderlineStyle: "bogus"}, {Text: "conflict", Underline: true, UnderlineStyle: "dbl"}} {
		spec := Spec{Slides: []Slide{{Elements: []Element{{Type: "table", W: 4, H: 2, Rows: [][]string{{"old"}}, TableFormat: &TableFormat{Cells: []TableCellFormat{{Paragraphs: []Paragraph{{Runs: []Run{run}}}}}}}}}}}
		if err := Create(context.Background(), spec, "", filepath.Join(t.TempDir(), "bad.pptx")); err == nil {
			t.Fatal("invalid underline accepted")
		}
	}
}
