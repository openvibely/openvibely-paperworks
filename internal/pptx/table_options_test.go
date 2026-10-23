package pptx

import (
	"context"
	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"path/filepath"
	"testing"
)

func TestTableStyleOptionsRoundTrip(t *testing.T) {
	yes, no := true, false
	in := filepath.Join(t.TempDir(), "in.pptx")
	out := filepath.Join(t.TempDir(), "out.pptx")
	options := &TableStyleOptions{HeaderRow: &no, TotalRow: &yes, FirstColumn: &yes, LastColumn: &yes, BandedRows: &no, BandedColumns: &yes}
	if err := Create(context.Background(), Spec{Slides: []Slide{{Elements: []Element{{Type: "table", W: 4, H: 2, Rows: [][]string{{"A", "B"}, {"C", "D"}}, TableFormat: &TableFormat{StyleOptions: options}}}}}}, "", in); err != nil {
		t.Fatal(err)
	}
	check := func(path string, expected map[string]string) {
		t.Helper()
		p, err := opc.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		d, err := p.XML("ppt/slides/slide1.xml")
		if err != nil {
			t.Fatal(err)
		}
		pr := office.All(d.Root(), ans, "tblPr")[0]
		for k, v := range expected {
			if pr.SelectAttrValue(k, "") != v {
				t.Fatalf("%s not %s", k, v)
			}
		}
		if len(office.All(pr, ans, "tableStyleId")) != 1 || len(office.All(d.Root(), ans, "tc")) != 4 {
			t.Fatal("style or cells lost")
		}
	}
	expected := map[string]string{"firstRow": "0", "lastRow": "1", "firstCol": "1", "lastCol": "1", "bandRow": "0", "bandCol": "1"}
	check(in, expected)
	if _, err := EditFile(context.Background(), in, out, EditSpec{Edits: []Edit{{Operation: "table-format", Slide: 1, ShapeID: "2", TableFormat: &TableFormat{StyleOptions: &TableStyleOptions{TotalRow: &no, BandedRows: &yes}}}}}); err != nil {
		t.Fatal(err)
	}
	expected["lastRow"] = "0"
	expected["bandRow"] = "1"
	check(out, expected)
	if _, err := office.VerifyPreservation(in, out, []string{"ppt/slides/slide1.xml"}); err != nil {
		t.Fatal(err)
	}
}
