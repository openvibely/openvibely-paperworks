package pptx

import (
	"context"
	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"os"
	"path/filepath"
	"strings"
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

func TestFlatLayoutRejectsGroupedChildrenWithoutPublishing(t *testing.T) {
	dir := t.TempDir()
	in, out := filepath.Join(dir, "in.pptx"), filepath.Join(dir, "out.pptx")
	seed := filepath.Join(dir, "seed.pptx")
	ctx := context.Background()
	if err := Create(ctx, Spec{Slides: []Slide{{Elements: []Element{{Type: "text", Text: "Grouped", W: 1, H: 1}}}}}, dir, seed); err != nil {
		t.Fatal(err)
	}
	p, err := opc.Open(seed)
	if err != nil {
		t.Fatal(err)
	}
	d, err := p.XML("ppt/slides/slide1.xml")
	if err != nil {
		t.Fatal(err)
	}
	shape := d.FindElement("//p:sp")
	tree := shape.Parent()
	tree.RemoveChild(shape)
	group := tree.CreateElement("p:grpSp")
	nv := group.CreateElement("p:nvGrpSpPr")
	nv.CreateElement("p:cNvPr").CreateAttr("id", "10")
	nv.CreateElement("p:cNvGrpSpPr")
	nv.CreateElement("p:nvPr")
	xfrm := group.CreateElement("p:grpSpPr").CreateElement("a:xfrm")
	for _, item := range []struct{ tag, a, b, x, y string }{{"off", "x", "y", "2743200", "914400"}, {"ext", "cx", "cy", "3657600", "1828800"}, {"chOff", "x", "y", "0", "0"}, {"chExt", "cx", "cy", "1828800", "914400"}} {
		e := xfrm.CreateElement("a:" + item.tag)
		e.CreateAttr(item.a, item.x)
		e.CreateAttr(item.b, item.y)
	}
	group.AddChild(shape)
	if err := p.SetXML("ppt/slides/slide1.xml", d); err != nil {
		t.Fatal(err)
	}
	if err := p.SaveNew(in); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(in)
	if err != nil {
		t.Fatal(err)
	}
	_, err = EditFile(ctx, in, out, EditSpec{Edits: []Edit{{Operation: "layout", Slide: 1, Layout: &Layout{ShapeIDs: []string{"2"}, Mode: "row", X: 1, Y: 1, Width: 2, Height: 1}}}})
	if err == nil || !strings.Contains(err.Error(), "top-level") {
		t.Fatal("grouped child must be rejected", err)
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Fatal("failed edit published a file", err)
	}
	after, err := os.ReadFile(in)
	if err != nil || string(before) != string(after) {
		t.Fatal("source changed", err)
	}
}

func TestDeleteDetachesDrawingConnections(t *testing.T) {
	for _, target := range []string{"ppt/slides/slide1.xml", "ppt/slideMasters/slideMaster1.xml", "ppt/slideLayouts/slideLayout1.xml"} {
		for _, tc := range []struct {
			name             string
			deletes, removed []string
			group            bool
		}{
			{"start", []string{"2"}, []string{"2"}, false},
			{"end", []string{"3"}, []string{"3"}, false},
			{"both", []string{"2", "3"}, []string{"2", "3"}, false},
			{"connector", []string{"5"}, []string{"5"}, false},
			{"unconnected", []string{"4"}, []string{"4"}, false},
			{"group", []string{"10"}, []string{"10", "11", "2", "3"}, true},
			{"group-child", []string{"2"}, []string{"2"}, true},
		} {
			t.Run(target+"/"+tc.name, func(t *testing.T) {
				dir := t.TempDir()
				seed, in, out := filepath.Join(dir, "seed.pptx"), filepath.Join(dir, "in.pptx"), filepath.Join(dir, "out.pptx")
				ctx := context.Background()
				elements := []Element{{Type: "shape", Shape: "rect", X: 1, Y: 1, W: 2, H: 1}, {Type: "shape", Shape: "ellipse", X: 4, Y: 2, W: 2, H: 1}, {Type: "shape", Shape: "rect", X: 7, Y: 1, W: 1, H: 1}}
				// Shared endpoints, a loop, and an unrelated connection exercise
				// every surviving connector, including valid opposite endpoints.
				for _, pair := range [][2]string{{"2", "3"}, {"3", "2"}, {"2", "2"}, {"3", "3"}} {
					elements = append(elements, Element{Type: "connector", X: 2, Y: 2, W: 3, H: 1, DrawingStyle: &DrawingStyle{Outline: &Outline{Color: "123456", Width: 2, Dash: "dash"}}, Connector: &Connector{Kind: "elbow", Start: &Connection{ShapeID: pair[0], Site: 1}, End: &Connection{ShapeID: pair[1], Site: 2}}})
				}
				if err := Create(ctx, Spec{Slides: []Slide{{Elements: elements}, {Elements: elements}}}, dir, seed); err != nil {
					t.Fatal(err)
				}
				p, err := opc.Open(seed)
				if err != nil {
					t.Fatal(err)
				}
				d, err := p.XML("ppt/slides/slide1.xml")
				if err != nil {
					t.Fatal(err)
				}
				if tc.group {
					tree := d.FindElement("//p:spTree")
					outer := tree.CreateElement("p:grpSp")
					for i, id := range []string{"10", "11"} {
						g := outer
						if i == 1 {
							g = outer.CreateElement("p:grpSp")
						}
						nv := g.CreateElement("p:nvGrpSpPr")
						nv.CreateElement("p:cNvPr").CreateAttr("id", id)
						nv.CreateElement("p:cNvGrpSpPr")
						nv.CreateElement("p:nvPr")
						g.CreateElement("p:grpSpPr")
						shape := tree.FindElement("p:sp")
						tree.RemoveChild(shape)
						g.AddChild(shape)
					}
					// Also attach directly to the outer group from outside it.
					d.FindElement("//a:stCxn").CreateAttr("id", "10")
				}
				if target != "ppt/slides/slide1.xml" {
					dest, err := p.XML(target)
					if err != nil {
						t.Fatal(err)
					}
					old := dest.FindElement("//p:spTree")
					parent := old.Parent()
					parent.RemoveChild(old)
					parent.AddChild(d.FindElement("//p:spTree").Copy())
					d = dest
				}
				if err := p.SetXML(target, d); err != nil {
					t.Fatal(err)
				}
				if err := p.SaveNew(in); err != nil {
					t.Fatal(err)
				}
				expected, err := p.XML(target)
				if err != nil {
					t.Fatal(err)
				}
				for _, id := range tc.deletes {
					for _, pr := range office.All(expected.Root(), pns, "cNvPr") {
						if pr.SelectAttrValue("id", "") == id {
							shape := pr.Parent().Parent()
							shape.Parent().RemoveChild(shape)
							break
						}
					}
				}
				for _, tag := range []string{"stCxn", "endCxn"} {
					for _, ep := range office.All(expected.Root(), ans, tag) {
						for _, id := range tc.removed {
							if ep.SelectAttrValue("id", "") == id {
								ep.Parent().RemoveChild(ep)
								break
							}
						}
					}
				}
				var edits []Edit
				for _, id := range tc.deletes {
					e := Edit{Operation: "delete", Slide: 1, ShapeID: id}
					if target != "ppt/slides/slide1.xml" {
						e.Part = target
					}
					edits = append(edits, e)
				}
				if _, err := EditFile(ctx, in, out, EditSpec{Edits: edits}); err != nil {
					t.Fatal(err)
				}
				result, err := opc.Open(out)
				if err != nil {
					t.Fatal(err)
				}
				actual, err := result.XML(target)
				if err != nil {
					t.Fatal(err)
				}
				want, _ := expected.WriteToString()
				got, _ := actual.WriteToString()
				if got != want {
					t.Fatalf("drawing changed beyond deleted shapes and attachments:\ngot %s\nwant %s", got, want)
				}
				for _, name := range p.Names() {
					if name == target {
						continue
					}
					before := p.Get(name)
					after := result.Get(name)
					if string(before) != string(after) {
						t.Errorf("unrelated part changed: %s", name)
					}
				}
			})
		}
	}
}
