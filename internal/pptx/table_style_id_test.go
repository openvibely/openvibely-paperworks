package pptx

import (
	"context"
	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"path/filepath"
	"testing"
)

func TestTableStyleID(t *testing.T) {
	ctx := context.Background()
	in := filepath.Join(t.TempDir(), "in.pptx")
	out := filepath.Join(t.TempDir(), "out.pptx")
	id := "{5940675A-B579-460E-94D1-54222C63F5DA}"
	if err := Create(ctx, Spec{Slides: []Slide{{Elements: []Element{{Type: "table", W: 4, H: 2, Rows: [][]string{{"A", "B"}}, TableFormat: &TableFormat{StyleID: id}}}}}}, "", in); err != nil {
		t.Fatal(err)
	}
	check := func(path, want string) {
		t.Helper()
		p, err := opc.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		d, err := p.XML("ppt/slides/slide1.xml")
		if err != nil {
			t.Fatal(err)
		}
		ids := office.All(d.Root(), ans, "tableStyleId")
		if len(ids) != 1 || ids[0].Text() != want {
			t.Fatal("style ID missing")
		}
		if office.All(d.Root(), ans, "t")[0].Text() != "A" {
			t.Fatal("cell text lost")
		}
	}
	check(in, id)
	id = "{5C22544A-7EE6-4342-B048-85BDC9FD1C3A}"
	spec := EditSpec{Edits: []Edit{{Operation: "table-format", Slide: 1, ShapeID: "2", TableFormat: &TableFormat{StyleID: id}}}}
	if _, err := EditFile(ctx, in, out, spec); err != nil {
		t.Fatal(err)
	}
	check(out, id)
	if _, err := office.VerifyPreservation(in, out, []string{"ppt/slides/slide1.xml"}); err != nil {
		t.Fatal(err)
	}
	spec.Edits[0].TableFormat.StyleID = "bad"
	if _, err := EditFile(ctx, in, filepath.Join(t.TempDir(), "bad.pptx"), spec); err == nil {
		t.Fatal("invalid ID accepted")
	}
}
