package pptx

import (
	"context"
	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"path/filepath"
	"testing"
)

func TestTableFormattingAndSizing(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "in.pptx")
	margin := 8.0
	style := &TableFormat{RowHeights: []float64{30, 40}, ColumnWidths: []float64{120, 180}, Cells: []TableCellFormat{{Row: 0, Column: 0, Margins: &TextInsets{Left: &margin}, VerticalAlign: "middle", Borders: map[string]TextOutline{"bottom": {Width: 2, Fill: &DrawingStyle{Fill: "FF0000"}}}, Fill: &DrawingStyle{Fill: "EEEEEE"}, TextPaint: &TextPaint{Fill: &DrawingStyle{Fill: "0000FF"}}}}}
	if err := Create(context.Background(), Spec{Slides: []Slide{{Elements: []Element{{Type: "table", W: 5, H: 2, Rows: [][]string{{"A", "B"}, {"C", "D"}}, TableFormat: style}}}}}, dir, in); err != nil {
		t.Fatal(err)
	}
	p, err := opc.Open(in)
	if err != nil {
		t.Fatal(err)
	}
	d, err := p.XML("ppt/slides/slide1.xml")
	if err != nil {
		t.Fatal(err)
	}
	cell := office.All(d.Root(), ans, "tc")[0]
	pr := office.Child(cell, ans, "tcPr")
	if pr.SelectAttrValue("anchor", "") != "ctr" || pr.SelectAttrValue("marL", "") != "101600" || office.Child(pr, ans, "lnB") == nil {
		t.Fatal("cell formatting missing")
	}
	xf := office.All(d.Root(), pns, "xfrm")[0]
	ext := office.Child(xf, ans, "ext")
	if ext.SelectAttrValue("cx", "") != "3810000" || ext.SelectAttrValue("cy", "") != "889000" {
		t.Fatal("frame not synchronized", ext.Attr)
	}
	out := filepath.Join(dir, "out.pptx")
	if _, err := EditFile(context.Background(), in, out, EditSpec{Edits: []Edit{{Operation: "table-format", Slide: 1, ShapeID: "2", TableFormat: &TableFormat{Cells: []TableCellFormat{{Row: 1, Column: 1, VerticalAlign: "bottom"}}}}}}); err != nil {
		t.Fatal(err)
	}
	p, _ = opc.Open(out)
	d, _ = p.XML("ppt/slides/slide1.xml")
	texts := office.All(d.Root(), ans, "t")
	for i, want := range []string{"A", "B", "C", "D"} {
		if texts[i].Text() != want {
			t.Fatal("cell contents changed")
		}
	}
	style.ColumnWidths = []float64{10}
	if _, err := EditFile(context.Background(), in, filepath.Join(dir, "bad.pptx"), EditSpec{Edits: []Edit{{Operation: "table-format", Slide: 1, ShapeID: "2", TableFormat: style}}}); err == nil {
		t.Fatal("invalid sizes accepted")
	}
}
