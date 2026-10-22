package pptx

import (
	"bytes"
	"context"
	"github.com/beevik/etree"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestContentSizedLayoutRejectsUnmeasuredSpacing(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "base.pptx")
	if err := Create(context.Background(), Spec{Font: "Go", Slides: []Slide{{Elements: []Element{{Type: "text", Text: "One paragraph", FontSize: 24, W: 4, H: 1}}}}}, dir, base); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, want string
		mutate     func(*etree.Element)
	}{
		{"expanded", "character spacing", func(body *etree.Element) { body.FindElement("a:p/a:r/a:rPr").CreateAttr("spc", "2000") }},
		{"condensed", "character spacing", func(body *etree.Element) { body.FindElement("a:p/a:r/a:rPr").CreateAttr("spc", "-100") }},
		{"default", "character spacing", func(body *etree.Element) {
			body.FindElement("a:p/a:pPr").CreateElement("a:defRPr").CreateAttr("spc", "2000")
		}},
		{"before", "percentage paragraph spacing", func(body *etree.Element) {
			body.FindElement("a:p/a:pPr").CreateElement("a:spcBef").CreateElement("a:spcPct").CreateAttr("val", "300000")
		}},
		{"after", "percentage paragraph spacing", func(body *etree.Element) {
			body.FindElement("a:p/a:pPr").CreateElement("a:spcAft").CreateElement("a:spcPct").CreateAttr("val", "300000")
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, err := opc.Open(base)
			if err != nil {
				t.Fatal(err)
			}
			d, err := p.XML("ppt/slides/slide1.xml")
			if err != nil {
				t.Fatal(err)
			}
			tc.mutate(d.FindElement("//p:sp/p:txBody"))
			if err = p.SetXML("ppt/slides/slide1.xml", d); err != nil {
				t.Fatal(err)
			}
			in := filepath.Join(t.TempDir(), "input.pptx")
			if err = p.SaveNew(in); err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(in)
			if err != nil {
				t.Fatal(err)
			}
			out := filepath.Join(t.TempDir(), "out.pptx")
			edit := EditSpec{Edits: []Edit{{Operation: "layout", Slide: 1, Layout: &Layout{Width: 4, Height: 6, Root: &LayoutNode{ShapeID: "2", FitText: true}}}}}
			if _, err = EditFile(context.Background(), in, out, edit); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected %s error, got %v", tc.want, err)
			}
			if _, err = os.Stat(out); !os.IsNotExist(err) {
				t.Fatal("published failed layout")
			}
			after, err := os.ReadFile(in)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatal("changed source", err)
			}
			// Explicit sizing remains available and preserves the formatting.
			edit.Edits[0].Layout.Root.FitText = false
			if _, err = EditFile(context.Background(), in, out, edit); err != nil {
				t.Fatal(err)
			}
			q, err := opc.Open(out)
			if err != nil {
				t.Fatal(err)
			}
			saved, err := q.XML("ppt/slides/slide1.xml")
			if err != nil {
				t.Fatal(err)
			}
			bodyXML := func(doc *etree.Document) string {
				body := etree.NewDocument()
				body.SetRoot(doc.FindElement("//p:sp/p:txBody").Copy())
				xml, err := body.WriteToString()
				if err != nil {
					t.Fatal(err)
				}
				return xml
			}
			if bodyXML(saved) != bodyXML(d) {
				t.Fatal("explicit sizing changed text or formatting")
			}
		})
	}
}

func TestContentSizedLayoutMovesFollowingText(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "input.pptx")
	text := "A long title with many words that needs several lines in a narrow column."
	err := Create(context.Background(), Spec{Font: "Go", Slides: []Slide{{Elements: []Element{{Type: "text", Text: text, FontSize: 24, W: 2, H: 1}, {Type: "text", Text: "Following paragraph", FontSize: 12, W: 2, H: 1}}}}}, dir, in)
	if err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(in)
	heights := []int64{}
	for _, width := range []float64{2, 5} {
		out := filepath.Join(t.TempDir(), "out.pptx")
		_, err = EditFile(context.Background(), in, out, EditSpec{Edits: []Edit{{Operation: "layout", Slide: 1, Layout: &Layout{X: 1, Y: 1, Width: width, Height: 6, Root: &LayoutNode{Mode: "column", Gap: .2, Children: []*LayoutNode{{ShapeID: "2", FitText: true}, {ShapeID: "3", FitText: true}}}}}}})
		if err != nil {
			t.Fatal(err)
		}
		p, _ := opc.Open(out)
		d, _ := p.XML("ppt/slides/slide1.xml")
		sh := d.FindElements("//p:sp")
		height, _ := strconv.ParseInt(sh[0].FindElement("p:spPr/a:xfrm/a:ext").SelectAttrValue("cy", ""), 10, 64)
		y, _ := strconv.ParseInt(sh[1].FindElement("p:spPr/a:xfrm/a:off").SelectAttrValue("y", ""), 10, 64)
		if y < 914400+height+182879 {
			t.Fatalf("following paragraph overlaps: height=%d y=%d", height, y)
		}
		heights = append(heights, height)
		if !strings.Contains(string(p.Get("ppt/slides/slide1.xml")), text) {
			t.Fatal("lost native text")
		}
	}
	if heights[0] <= heights[1] {
		t.Fatalf("narrow text did not grow: %v", heights)
	}
	after, _ := os.ReadFile(in)
	if string(before) != string(after) {
		t.Fatal("changed source")
	}
	out := filepath.Join(dir, "overflow.pptx")
	_, err = EditFile(context.Background(), in, out, EditSpec{Edits: []Edit{{Operation: "layout", Slide: 1, Layout: &Layout{Width: 1, Height: .1, Root: &LayoutNode{ShapeID: "2", FitText: true}}}}})
	if err == nil {
		t.Fatal("accepted overflowing text")
	}
	if _, err = os.Stat(out); !os.IsNotExist(err) {
		t.Fatal("published failed layout")
	}
}

func TestGridSpanLayoutSavedGeometry(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "grid.pptx")
	elements := []Element{{Type: "text", Text: "Wide", W: 1, H: 1}, {Type: "text", Text: "Left", W: 1, H: 1}, {Type: "text", Text: "Right", W: 1, H: 1}}
	layout := &Layout{Width: 8, Height: 4, Root: &LayoutNode{Mode: "grid", Columns: 2, Children: []*LayoutNode{{ShapeID: "2", ColumnSpan: 2}, {ShapeID: "3"}, {ShapeID: "4"}}}}
	err := Create(context.Background(), Spec{Slides: []Slide{{Elements: elements, Layout: layout}}}, dir, out)
	if err != nil {
		t.Fatal(err)
	}
	p, _ := opc.Open(out)
	d, _ := p.XML("ppt/slides/slide1.xml")
	sh := d.FindElements("//p:sp")
	for i, want := range [][4]int{{0, 0, 8, 2}, {0, 2, 4, 2}, {4, 2, 4, 2}} {
		xf := sh[i].FindElement("p:spPr/a:xfrm")
		for j, k := range []string{"x", "y", "cx", "cy"} {
			tag := "a:off"
			if j > 1 {
				tag = "a:ext"
			}
			v, _ := strconv.Atoi(xf.FindElement(tag).SelectAttrValue(k, ""))
			if v != want[j]*914400 {
				t.Fatalf("shape %d %s=%d", i, k, v)
			}
		}
	}
	cells, rows, err := gridCells(&LayoutNode{Columns: 2, Children: []*LayoutNode{{RowSpan: 2}, {}, {}}})
	if err != nil || rows != 2 || cells[2].row != 1 || cells[2].col != 1 {
		t.Fatal(cells, rows, err)
	}
	for _, n := range []*LayoutNode{{Columns: 2, Children: []*LayoutNode{{ColumnSpan: 3}}}, {Columns: 2, Children: []*LayoutNode{nil}}, {Columns: 2, Children: []*LayoutNode{{RowSpan: -1}}}} {
		if _, _, err := gridCells(n); err == nil {
			t.Fatal("accepted invalid grid")
		}
	}
}
