package pptx

import (
	"context"
	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"path/filepath"
	"testing"
)

func TestRichTableCells(t *testing.T) {
	yes, no := true, false
	ctx := context.Background()
	in := filepath.Join(t.TempDir(), "in.pptx")
	out := filepath.Join(t.TempDir(), "out.pptx")
	paragraphs := []Paragraph{{Runs: []Run{{Text: "Revenue:", Font: "Courier New", Bold: &yes}, {Text: " $200", Font: "Arial", Bold: &no, Italic: true, Color: "008000"}}}}
	if err := Create(ctx, Spec{Slides: []Slide{{Elements: []Element{{Type: "table", W: 4, H: 2, Rows: [][]string{{"replace", "Keep"}}, TableFormat: &TableFormat{Cells: []TableCellFormat{{Row: 0, Column: 0, Paragraphs: paragraphs}}}}}}}}, "", in); err != nil {
		t.Fatal(err)
	}
	check := func(path string) {
		t.Helper()
		p, err := opc.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		d, err := p.XML("ppt/slides/slide1.xml")
		if err != nil {
			t.Fatal(err)
		}
		cells := office.All(d.Root(), ans, "tc")
		runs := office.All(cells[0], ans, "r")
		if len(runs) != 2 || office.Child(runs[0], ans, "rPr").SelectAttrValue("b", "") != "1" || office.Child(runs[1], ans, "rPr").SelectAttrValue("b", "") != "0" {
			t.Fatal("mixed formatting lost")
		}
		for i, want := range []string{"Courier New", "Arial"} {
			if office.All(runs[i], ans, "latin")[0].SelectAttrValue("typeface", "") != want {
				t.Fatal("run font lost")
			}
		}
		if office.All(cells[1], ans, "t")[0].Text() != "Keep" {
			t.Fatal("other cell changed")
		}
	}
	check(in)
	spec := EditSpec{Edits: []Edit{{Operation: "table-cell", Slide: 1, ShapeID: "2", Row: 0, Column: 0, Paragraphs: paragraphs}}}
	if _, err := EditFile(ctx, in, out, spec); err != nil {
		t.Fatal(err)
	}
	check(out)
	if _, err := office.VerifyPreservation(in, out, []string{"ppt/slides/slide1.xml"}); err != nil {
		t.Fatal(err)
	}
	spec.Edits[0].Text = "conflict"
	if _, err := EditFile(ctx, in, filepath.Join(t.TempDir(), "bad.pptx"), spec); err == nil {
		t.Fatal("ambiguous content accepted")
	}
	spec.Edits[0].Text = ""
	spec.Edits[0].Paragraphs = []Paragraph{}
	if _, err := EditFile(ctx, in, filepath.Join(t.TempDir(), "empty.pptx"), spec); err != nil {
		t.Fatal(err)
	}
}
