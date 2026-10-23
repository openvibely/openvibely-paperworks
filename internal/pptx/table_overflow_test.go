package pptx

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
)

func TestTableHorizontalOverflow(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	in := filepath.Join(dir, "in.pptx")
	format := &TableFormat{Cells: []TableCellFormat{{Row: 0, Column: 0, HorizontalOverflow: "clip"}, {Row: 0, Column: 1, HorizontalOverflow: "overflow"}}}
	spec := Spec{Slides: []Slide{{Elements: []Element{{Type: "table", W: 4, H: 2, Rows: [][]string{{"Long text", "Keep"}}, TableFormat: format}}}}}
	if err := Create(ctx, spec, "", in); err != nil {
		t.Fatal(err)
	}
	check := func(path, first string) {
		t.Helper()
		pkg, err := opc.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		doc, err := pkg.XML("ppt/slides/slide1.xml")
		if err != nil {
			t.Fatal(err)
		}
		cells := office.All(doc.Root(), ans, "tc")
		if len(cells) != 2 {
			t.Fatal("cell count changed")
		}
		for i, want := range []string{first, "overflow"} {
			if office.Child(cells[i], ans, "tcPr").SelectAttrValue("horzOverflow", "") != want {
				t.Fatalf("cell %d overflow not %s", i, want)
			}
		}
		for i, want := range []string{"Long text", "Keep"} {
			if office.All(cells[i], ans, "t")[0].Text() != want {
				t.Fatal("text changed")
			}
		}
	}
	check(in, "clip")
	previous := in
	for i, cell := range []TableCellFormat{{HorizontalOverflow: "overflow"}, {VerticalAlign: "bottom"}, {HorizontalOverflow: "clip"}} {
		out := filepath.Join(dir, []string{"changed.pptx", "omitted.pptx", "clip.pptx"}[i])
		if _, err := EditFile(ctx, previous, out, EditSpec{Edits: []Edit{{Operation: "table-format", Slide: 1, ShapeID: "2", TableFormat: &TableFormat{Cells: []TableCellFormat{cell}}}}}); err != nil {
			t.Fatal(err)
		}
		want := "overflow"
		if i == 2 {
			want = "clip"
		}
		check(out, want)
		if _, err := office.VerifyPreservation(previous, out, []string{"ppt/slides/slide1.xml"}); err != nil {
			t.Fatal(err)
		}
		previous = out
	}
	format.Cells[0].HorizontalOverflow = "invalid"
	bad := filepath.Join(dir, "bad.pptx")
	if err := Create(ctx, spec, "", bad); err == nil {
		t.Fatal("invalid overflow accepted")
	}
	if _, err := os.Stat(bad); !os.IsNotExist(err) {
		t.Fatal("failed creation published output")
	}
	if _, err := EditFile(ctx, in, bad, EditSpec{Edits: []Edit{{Operation: "table-format", Slide: 1, ShapeID: "2", TableFormat: format}}}); err == nil {
		t.Fatal("invalid edit accepted")
	}
	if _, err := os.Stat(bad); !os.IsNotExist(err) {
		t.Fatal("failed edit published output")
	}
}
