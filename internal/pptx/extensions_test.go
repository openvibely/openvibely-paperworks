package pptx

import (
	"context"
	"image"
	imgcolor "image/color"
	"image/png"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
)

func TestMergeAndUnmergeCells(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	in := filepath.Join(dir, "input.pptx")
	out := filepath.Join(dir, "merged.pptx")
	if err := Create(ctx, Spec{Slides: []Slide{{Elements: []Element{{Type: "table", W: 6, H: 3, Rows: [][]string{{"A", "B", "untouched"}, {"C", "D", "keep"}}}}}}}, dir, in); err != nil {
		t.Fatal(err)
	}
	if _, err := EditFile(ctx, in, out, EditSpec{Edits: []Edit{{Operation: "table-merge", Slide: 1, ShapeID: "2", EndRow: 1, EndColumn: 1}}}); err != nil {
		t.Fatal(err)
	}
	p, err := opc.Open(out)
	if err != nil {
		t.Fatal(err)
	}
	d, _ := p.XML("ppt/slides/slide1.xml")
	rows := d.FindElements("//a:tbl/a:tr")
	origin := rows[0].FindElement("a:tc")
	if origin.SelectAttrValue("gridSpan", "") != "2" || origin.SelectAttrValue("rowSpan", "") != "2" || office.Text(origin, ans, "t") != "ABCD" {
		t.Fatal("merge lost spans/content")
	}
	if office.Text(rows[0].ChildElements()[2], ans, "t") != "untouched" {
		t.Fatal("unrelated cell changed")
	}
	for _, op := range []Edit{{Operation: "table-cell", Slide: 1, ShapeID: "2", Column: 1, Text: "hidden"}, {Operation: "table-merge", Slide: 1, ShapeID: "2", EndColumn: 2}, {Operation: "table-unmerge", Slide: 1, ShapeID: "2", Row: 1, Column: 1}} {
		bad := filepath.Join(t.TempDir(), "bad.pptx")
		if _, err := EditFile(ctx, out, bad, EditSpec{Edits: []Edit{op}}); err == nil {
			t.Fatal("invalid edit accepted", op)
		}
		if _, err := os.Stat(bad); !os.IsNotExist(err) {
			t.Fatal("failed edit published output")
		}
	}
	split := filepath.Join(dir, "split.pptx")
	if _, err := EditFile(ctx, out, split, EditSpec{Edits: []Edit{{Operation: "table-unmerge", Slide: 1, ShapeID: "2"}}}); err != nil {
		t.Fatal(err)
	}
	p, _ = opc.Open(split)
	d, _ = p.XML("ppt/slides/slide1.xml")
	for _, c := range office.All(d.Root(), ans, "tc") {
		if mergedContinuation(c) || c.SelectAttrValue("gridSpan", "") != "" || c.SelectAttrValue("rowSpan", "") != "" {
			t.Fatal("merge retained")
		}
	}
	if got := office.Text(d.Root(), ans, "t"); got != "ABCDuntouchedkeep" {
		t.Fatal(got)
	}
}

func TestRichNotesCreateAndEdit(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	in := filepath.Join(dir, "in.pptx")
	out := filepath.Join(dir, "out.pptx")
	yes := true
	rich := []Paragraph{{Runs: []Run{{Text: "Emphasis", Bold: &yes, Italic: true, Underline: true, FontSize: 16, Color: "FF0000"}}}, {Text: "Second", Bullet: true, Level: 1, Format: &ParagraphStyle{Align: "r"}}}
	if err := Create(ctx, Spec{Slides: []Slide{{NotesParagraphs: rich, Elements: []Element{{Type: "text", Text: "Unchanged", W: 4, H: 1}}}}}, dir, in); err != nil {
		t.Fatal(err)
	}
	p, _ := opc.Open(in)
	n, _ := p.XML("ppt/notesSlides/notesSlide1.xml")
	r := n.FindElement("//a:rPr")
	if r == nil || r.SelectAttrValue("b", "") != "1" || r.SelectAttrValue("i", "") != "1" || r.SelectAttrValue("sz", "") != "1600" {
		t.Fatal("rich notes lost formatting")
	}
	if _, err := EditFile(ctx, in, out, EditSpec{Edits: []Edit{{Operation: "notes", Slide: 1, Paragraphs: []Paragraph{{Text: "Replacement", Bullet: true}}}}}); err != nil {
		t.Fatal(err)
	}
	q, _ := opc.Open(out)
	if string(q.Get("ppt/slides/slide1.xml")) != string(p.Get("ppt/slides/slide1.xml")) {
		t.Fatal("editing notes changed slide")
	}
	n, _ = q.XML("ppt/notesSlides/notesSlide1.xml")
	if office.Text(n.Root(), ans, "t") != "Replacement" || n.FindElement("//a:buChar") == nil {
		t.Fatal("replacement notes")
	}
	for _, paragraphs := range [][]Paragraph{{{Level: 9}}, {{Text: "x", Runs: []Run{{Text: "y"}}}}} {
		if _, err := EditFile(ctx, in, filepath.Join(t.TempDir(), "bad.pptx"), EditSpec{Edits: []Edit{{Operation: "notes", Slide: 1, Paragraphs: paragraphs}}}); err == nil {
			t.Fatal("invalid notes accepted")
		}
	}
}

func TestShapePatternAndImageFills(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	img := image.NewRGBA(image.Rect(0, 0, 20, 10))
	for y := 0; y < 10; y++ {
		for x := 0; x < 20; x++ {
			img.Set(x, y, imgcolor.RGBA{uint8(x * 12), uint8(y * 24), 80, 255})
		}
	}
	imagePath := filepath.Join(dir, "fill.png")
	f, _ := os.Create(imagePath)
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	f.Close()
	in := filepath.Join(dir, "in.pptx")
	out := filepath.Join(dir, "out.pptx")
	if err := Create(ctx, Spec{Slides: []Slide{{Elements: []Element{{Type: "shape", Shape: "rect", W: 2, H: 2, DrawingStyle: &DrawingStyle{Pattern: &PatternFill{Preset: "diagCross", Foreground: "112233", Background: "FFFFFF"}}}, {Type: "shape", Shape: "ellipse", X: 3, W: 2, H: 2, DrawingStyle: &DrawingStyle{Image: &ImageFill{Path: "fill.png", Fit: "cover"}}}}}}}, dir, in); err != nil {
		t.Fatal(err)
	}
	p, _ := opc.Open(in)
	d, _ := p.XML("ppt/slides/slide1.xml")
	if d.FindElement("//a:pattFill") == nil {
		t.Fatal("missing pattern")
	}
	rect := d.FindElement("//a:blipFill/a:srcRect")
	if rect == nil || rect.SelectAttrValue("l", "") != "25000" || rect.SelectAttrValue("r", "") != "25000" {
		t.Fatal("cover crop", rect)
	}
	blip := d.FindElement("//a:blipFill/a:blip")
	id := blip.SelectAttrValue("r:embed", "")
	rels, _ := p.Rels("ppt/slides/slide1.xml")
	found := false
	for _, r := range rels {
		if r.ID == id {
			found = true
			if !p.Has(opc.Resolve("ppt/slides/slide1.xml", r.Target)) {
				t.Fatal("missing image part")
			}
		}
	}
	if !found {
		t.Fatal("missing fill relationship")
	}
	if _, err := EditFile(ctx, in, out, EditSpec{Edits: []Edit{{Operation: "drawing-style", Slide: 1, ShapeID: "2", DrawingStyle: &DrawingStyle{Image: &ImageFill{Path: imagePath, Fit: "tile"}}}}}); err != nil {
		t.Fatal(err)
	}
	p, _ = opc.Open(out)
	d, _ = p.XML("ppt/slides/slide1.xml")
	if d.FindElement("//a:tile") == nil || d.FindElement("//a:pattFill") != nil {
		t.Fatal("fill replacement failed")
	}
	if _, err := EditFile(ctx, in, filepath.Join(dir, "bad.pptx"), EditSpec{Edits: []Edit{{Operation: "drawing-style", Slide: 1, ShapeID: "2", DrawingStyle: &DrawingStyle{Fill: "000000", Image: &ImageFill{Path: imagePath}}}}}); err == nil {
		t.Fatal("multiple fills accepted")
	}
}

func TestNestedWeightedLayout(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	in := filepath.Join(dir, "in.pptx")
	one := 1.0
	layout := &Layout{X: 1, Y: 1, Width: 8, Height: 4, Root: &LayoutNode{Mode: "row", Gap: 1, Children: []*LayoutNode{{ShapeID: "2", Width: &one}, {Mode: "column", Gap: 1, Children: []*LayoutNode{{ShapeID: "3", Weight: 1}, {ShapeID: "4", Weight: 2}}}}}}
	elements := []Element{{Type: "text", Text: "fixed", W: 1, H: 1}, {Type: "text", Text: "one", W: 1, H: 1}, {Type: "text", Text: "two", W: 1, H: 1}}
	if err := Create(ctx, Spec{Slides: []Slide{{Layout: layout, Elements: elements}}}, dir, in); err != nil {
		t.Fatal(err)
	}
	p, _ := opc.Open(in)
	d, _ := p.XML("ppt/slides/slide1.xml")
	shapes := d.FindElements("//p:sp")
	for i, want := range [][4]int{{1, 1, 1, 4}, {3, 1, 6, 1}, {3, 3, 6, 2}} {
		xf := shapes[i].FindElement("p:spPr/a:xfrm")
		for j, key := range []string{"x", "y", "cx", "cy"} {
			tag := "a:off"
			if j > 1 {
				tag = "a:ext"
			}
			got, _ := strconv.ParseInt(xf.FindElement(tag).SelectAttrValue(key, ""), 10, 64)
			if got != int64(want[j])*914400 {
				t.Fatalf("shape %d %s=%d", i, key, got)
			}
		}
	}
	for _, root := range []*LayoutNode{{Mode: "row", Children: []*LayoutNode{{ShapeID: "2"}, {ShapeID: "2"}}}, {Mode: "row", Gap: 20, Children: []*LayoutNode{{ShapeID: "2"}, {ShapeID: "3"}}}, {Mode: "row", Children: []*LayoutNode{nil}}} {
		_, err := EditFile(ctx, in, filepath.Join(t.TempDir(), "bad.pptx"), EditSpec{Edits: []Edit{{Operation: "layout", Slide: 1, Layout: &Layout{Width: 4, Height: 2, Root: root}}}})
		if err == nil {
			t.Fatal("invalid layout accepted")
		}
	}
	// Saved layout still has native, editable text.
	if !strings.Contains(office.Text(d.Root(), ans, "t"), "fixedonetwo") {
		t.Fatal("layout lost text")
	}
}

func TestLayoutResizesTableGridAndPreservesMergedContent(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	in := filepath.Join(dir, "in.pptx")
	if err := Create(ctx, Spec{Slides: []Slide{{Elements: []Element{{Type: "table", W: 8, H: 4, Rows: [][]string{{"A", "B"}, {"C", "D"}}}, {Type: "text", Text: "Neighbor", W: 2, H: 1}}}}}, dir, in); err != nil {
		t.Fatal(err)
	}
	merged := filepath.Join(dir, "merged.pptx")
	if _, err := EditFile(ctx, in, merged, EditSpec{Edits: []Edit{{Operation: "table-merge", Slide: 1, ShapeID: "2", EndColumn: 1}}}); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "out.pptx")
	if _, err := EditFile(ctx, merged, out, EditSpec{Edits: []Edit{{Operation: "layout", Slide: 1, Layout: &Layout{X: 1, Y: 1, Width: 6, Height: 2, Root: &LayoutNode{Mode: "row", Children: []*LayoutNode{{ShapeID: "2"}, {ShapeID: "3"}}}}}}}); err != nil {
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
	ext := d.FindElement("//p:graphicFrame/p:xfrm/a:ext")
	for _, axis := range []struct {
		tag, attr, frame string
		want             int64
	}{{"gridCol", "w", "cx", 3 * 914400}, {"tr", "h", "cy", 2 * 914400}} {
		var total int64
		for _, e := range office.All(d.Root(), ans, axis.tag) {
			n, err := strconv.ParseInt(e.SelectAttrValue(axis.attr, ""), 10, 64)
			if err != nil || n <= 0 {
				t.Fatal("invalid dimension", err)
			}
			total += n
		}
		frame, _ := strconv.ParseInt(ext.SelectAttrValue(axis.frame, ""), 10, 64)
		if total != axis.want || frame != total {
			t.Fatalf("%s grid=%d frame=%d want=%d", axis.tag, total, frame, axis.want)
		}
	}
	if office.Text(d.Root(), ans, "t") != "ABCDNeighbor" {
		t.Fatal("layout changed text")
	}
	first := d.FindElement("//a:tbl/a:tr/a:tc")
	if first.SelectAttrValue("gridSpan", "") != "2" {
		t.Fatal("layout removed merge")
	}
	// Resizing only one axis must preserve the other, including noninteger sizes.
	width := 3.333333
	next := filepath.Join(dir, "width.pptx")
	if _, err := EditFile(ctx, out, next, EditSpec{Edits: []Edit{{Operation: "geometry", Slide: 1, ShapeID: "2", W: &width}}}); err != nil {
		t.Fatal(err)
	}
	p, _ = opc.Open(next)
	d, _ = p.XML("ppt/slides/slide1.xml")
	var sum int64
	for _, c := range office.All(d.Root(), ans, "gridCol") {
		v, _ := strconv.ParseInt(c.SelectAttrValue("w", ""), 10, 64)
		sum += v
	}
	if sum != emu(width) {
		t.Fatal("column rounding does not match frame")
	}
	for _, r := range office.All(d.Root(), ans, "tr") {
		if r.SelectAttrValue("h", "") != "914400" {
			t.Fatal("width-only resize changed row height")
		}
	}
}
