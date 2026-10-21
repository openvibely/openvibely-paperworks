package pptx

import (
	"context"
	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"path/filepath"
	"testing"
)

func TestObjectGeometryStyleAndMissingNotes(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "in.pptx")
	out := filepath.Join(dir, "out.pptx")
	ctx := context.Background()
	if err := Create(ctx, Spec{Slides: []Slide{{Elements: []Element{{Type: "text", Text: "keep", X: 1, Y: 1, W: 2, H: 1}}}}}, dir, in); err != nil {
		t.Fatal(err)
	}
	x := 2.0
	size := 24.0
	yes := true
	_, err := EditFile(ctx, in, out, EditSpec{Edits: []Edit{{Operation: "geometry", Slide: 1, ShapeID: "2", X: &x, FlipH: &yes}, {Operation: "run-format", Slide: 1, ShapeID: "2", FontSize: &size, Bold: &yes}, {Operation: "notes", Slide: 1, Text: "New speaker notes"}}})
	if err != nil {
		t.Fatal(err)
	}
	p, err := opc.Open(out)
	if err != nil {
		t.Fatal(err)
	}
	d, _ := p.XML("ppt/slides/slide1.xml")
	off := d.FindElement("//p:sp/p:spPr/a:xfrm/a:off")
	if off.SelectAttrValue("x", "") != "1828800" {
		t.Fatal(off)
	}
	if d.FindElement("//a:rPr").SelectAttrValue("sz", "") != "2400" {
		t.Fatal("font size")
	}
	if d.FindElement("//a:t").Text() != "keep" {
		t.Fatal("text changed")
	}
	rels, _ := p.Rels("ppt/slides/slide1.xml")
	found := false
	for _, r := range rels {
		if r.Type == rns+"/notesSlide" {
			found = true
			n, _ := p.XML(opc.Resolve("ppt/slides/slide1.xml", r.Target))
			texts := office.All(n.Root(), ans, "t")
			if len(texts) == 0 || texts[0].Text() != "New speaker notes" {
				t.Fatal("notes", texts)
			}
		}
	}
	if !found {
		t.Fatal("missing notes relation")
	}
}

func TestTableStructureUpdatesFrameAndPreservesCells(t *testing.T) {
	dir := t.TempDir()
	in, out := filepath.Join(dir, "in.pptx"), filepath.Join(dir, "out.pptx")
	ctx := context.Background()
	if err := Create(ctx, Spec{Slides: []Slide{{Elements: []Element{{Type: "table", X: 1, Y: 1, W: 4, H: 2, Rows: [][]string{{"A", "B"}, {"C", "D"}}}}}}}, dir, in); err != nil {
		t.Fatal(err)
	}
	_, err := EditFile(ctx, in, out, EditSpec{Edits: []Edit{{Operation: "table-row-insert", Slide: 1, ShapeID: "2", Row: 1}, {Operation: "table-column-insert", Slide: 1, ShapeID: "2", Column: 1}}})
	if err != nil {
		t.Fatal(err)
	}
	p, err := opc.Open(out)
	if err != nil {
		t.Fatal(err)
	}
	d, err := p.XML("ppt/slides/slide1.xml")
	if err != nil {
		t.Fatal(err)
	}
	rows := d.FindElements("//a:tbl/a:tr")
	if len(rows) != 3 {
		t.Fatal("row count", len(rows))
	}
	for _, r := range rows {
		if len(office.Children(r, ans, "tc")) != 3 {
			t.Fatal("column count")
		}
	}
	if got := office.Text(d.Root(), ans, "t"); got != "ABCD" {
		t.Fatal("original cell text changed", got)
	}
	extent := d.FindElement("//p:graphicFrame/p:xfrm/a:ext")
	if extent == nil || extent.SelectAttrValue("cx", "") != "5486400" || extent.SelectAttrValue("cy", "") != "2743200" {
		t.Fatal("frame does not match table dimensions", extent)
	}
}

func TestUniformObjectLayout(t *testing.T) {
	dir := t.TempDir()
	in, out := filepath.Join(dir, "in.pptx"), filepath.Join(dir, "out.pptx")
	ctx := context.Background()
	if err := Create(ctx, Spec{Slides: []Slide{{Elements: []Element{{Type: "text", Text: "Left", X: 1, Y: 1, W: 2, H: 1}, {Type: "text", Text: "Right", X: 1, Y: 4, W: 2, H: 1}}}}}, dir, in); err != nil {
		t.Fatal(err)
	}
	_, err := EditFile(ctx, in, out, EditSpec{Edits: []Edit{{Operation: "layout", Slide: 1, Layout: &Layout{ShapeIDs: []string{"2", "3"}, Mode: "row", X: 1, Y: 2, Width: 5, Height: 1, Gap: 1}}}})
	if err != nil {
		t.Fatal(err)
	}
	p, _ := opc.Open(out)
	d, _ := p.XML("ppt/slides/slide1.xml")
	offsets := d.FindElements("//p:sp/p:spPr/a:xfrm/a:off")
	if len(offsets) != 2 || offsets[0].SelectAttrValue("x", "") != "914400" || offsets[1].SelectAttrValue("x", "") != "3657600" {
		t.Fatal(offsets)
	}
	if office.Text(d.Root(), ans, "t") != "LeftRight" {
		t.Fatal("text changed")
	}
}

func TestParagraphAndHyperlinkFormattingPreservesText(t *testing.T) {
	dir := t.TempDir()
	in, out := filepath.Join(dir, "in.pptx"), filepath.Join(dir, "out.pptx")
	ctx := context.Background()
	if err := Create(ctx, Spec{Slides: []Slide{{Elements: []Element{{Type: "text", Text: "Review this", X: 1, Y: 1, W: 4, H: 2}}}}}, dir, in); err != nil {
		t.Fatal(err)
	}
	after := 12.0
	level := 1
	yes := true
	link := "https://example.com/review"
	_, err := EditFile(ctx, in, out, EditSpec{Edits: []Edit{{Operation: "paragraph-format", Slide: 1, ShapeID: "2", ParagraphStyle: &ParagraphStyle{After: &after, Level: &level, Bullet: &yes, BulletChar: "→"}}, {Operation: "run-format", Slide: 1, ShapeID: "2", Underline: &yes, Hyperlink: &link}}})
	if err != nil {
		t.Fatal(err)
	}
	p, _ := opc.Open(out)
	d, _ := p.XML("ppt/slides/slide1.xml")
	if office.Text(d.Root(), ans, "t") != "Review this" {
		t.Fatal("text changed")
	}
	if bullet := d.FindElement("//a:pPr/a:buChar"); bullet == nil || bullet.SelectAttrValue("char", "") != "→" {
		t.Fatal("bullet missing")
	}
	if spacing := d.FindElement("//a:pPr/a:spcAft/a:spcPts"); spacing == nil || spacing.SelectAttrValue("val", "") != "1200" {
		t.Fatal("paragraph spacing missing")
	}
	refs, err := p.Rels("ppt/slides/slide1.xml")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, ref := range refs {
		if ref.Type == rns+"/hyperlink" && ref.Target == link && ref.TargetMode == "External" {
			found = true
		}
	}
	if !found {
		t.Fatal("hyperlink relationship missing")
	}
}
