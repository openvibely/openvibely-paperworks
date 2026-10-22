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

func TestTableStructureAcceptsExplicitUnmergedDefaults(t *testing.T) {
	operations := []string{"table-row-insert", "table-row-delete", "table-column-insert", "table-column-delete"}
	defaults := []struct {
		name  string
		attrs map[string]string
	}{
		{"rowSpan-one", map[string]string{"rowSpan": "1"}},
		{"gridSpan-one", map[string]string{"gridSpan": "1"}},
		{"hMerge-zero", map[string]string{"hMerge": "0"}},
		{"hMerge-false", map[string]string{"hMerge": "false"}},
		{"vMerge-zero", map[string]string{"vMerge": "0"}},
		{"vMerge-false", map[string]string{"vMerge": "false"}},
		{"combined", map[string]string{"rowSpan": "1", "gridSpan": "1", "hMerge": "0", "vMerge": "false"}},
	}
	for _, operation := range operations {
		t.Run(operation, func(t *testing.T) {
			dir := t.TempDir()
			seed := filepath.Join(dir, "seed.pptx")
			ctx := context.Background()
			if err := Create(ctx, Spec{Slides: []Slide{{Elements: []Element{{Type: "table", X: 1, Y: 1, W: 4, H: 2, Rows: [][]string{{"A", "B"}, {"C", "D"}}}}}}}, dir, seed); err != nil {
				t.Fatal(err)
			}
			baseline := filepath.Join(dir, "baseline.pptx")
			if _, err := EditFile(ctx, seed, baseline, EditSpec{Edits: []Edit{tableStructureEdit(operation)}}); err != nil {
				t.Fatal("attribute-free edit:", err)
			}
			want := normalizedTableSlide(t, baseline)
			for _, test := range defaults {
				t.Run(test.name, func(t *testing.T) {
					in := filepath.Join(dir, test.name+"-in.pptx")
					out := filepath.Join(dir, test.name+"-out.pptx")
					writeTableFixtureWithCellAttributes(t, seed, in, test.attrs)
					if _, err := EditFile(ctx, in, out, EditSpec{Edits: []Edit{tableStructureEdit(operation)}}); err != nil {
						t.Fatal("explicit defaults should allow edit:", err)
					}
					if got := normalizedTableSlide(t, out); got != want {
						t.Fatalf("edit differs from equivalent table without attributes\ngot: %s\nwant: %s", got, want)
					}
				})
			}
		})
	}
}

func TestTableStructureRejectsMergesAndMalformedSpansWithoutPublishing(t *testing.T) {
	operations := []string{"table-row-insert", "table-row-delete", "table-column-insert", "table-column-delete"}
	invalid := []struct {
		name    string
		attr    string
		value   string
		wantErr string
	}{
		{name: "rowSpan-two", attr: "rowSpan", value: "2", wantErr: "structure editing requires an unmerged table"},
		{name: "gridSpan-two", attr: "gridSpan", value: "2", wantErr: "structure editing requires an unmerged table"},
		{name: "hMerge-one", attr: "hMerge", value: "1", wantErr: "structure editing requires an unmerged table"},
		{name: "hMerge-true", attr: "hMerge", value: "true", wantErr: "structure editing requires an unmerged table"},
		{name: "vMerge-one", attr: "vMerge", value: "1", wantErr: "structure editing requires an unmerged table"},
		{name: "vMerge-true", attr: "vMerge", value: "true", wantErr: "structure editing requires an unmerged table"},
		{name: "malformed-rowSpan", attr: "rowSpan", value: "invalid", wantErr: "invalid table rowSpan"},
		{name: "malformed-gridSpan", attr: "gridSpan", value: "invalid", wantErr: "invalid table gridSpan"},
	}
	for _, operation := range operations {
		t.Run(operation, func(t *testing.T) {
			for _, test := range invalid {
				t.Run(test.name, func(t *testing.T) {
					dir := t.TempDir()
					seed, in, out := filepath.Join(dir, "seed.pptx"), filepath.Join(dir, "in.pptx"), filepath.Join(dir, "out.pptx")
					ctx := context.Background()
					if err := Create(ctx, Spec{Slides: []Slide{{Elements: []Element{{Type: "table", X: 1, Y: 1, W: 4, H: 2, Rows: [][]string{{"A", "B"}, {"C", "D"}}}}}}}, dir, seed); err != nil {
						t.Fatal(err)
					}
					writeTableFixtureWithCellAttributes(t, seed, in, map[string]string{test.attr: test.value})
					before, err := os.ReadFile(in)
					if err != nil {
						t.Fatal(err)
					}
					_, err = EditFile(ctx, in, out, EditSpec{Edits: []Edit{tableStructureEdit(operation)}})
					if err == nil || !strings.Contains(err.Error(), test.wantErr) {
						t.Fatalf("expected error containing %q, got %v", test.wantErr, err)
					}
					if _, statErr := os.Stat(out); !os.IsNotExist(statErr) {
						t.Fatal("rejected edit published output", statErr)
					}
					after, readErr := os.ReadFile(in)
					if readErr != nil || string(before) != string(after) {
						t.Fatal("rejected edit changed source", readErr)
					}
				})
			}
		})
	}
}

func tableStructureEdit(operation string) Edit {
	e := Edit{Operation: operation, Slide: 1, ShapeID: "2"}
	switch operation {
	case "table-row-insert", "table-row-delete":
		e.Row = 1
	case "table-column-insert", "table-column-delete":
		e.Column = 1
	}
	return e
}

func writeTableFixtureWithCellAttributes(t *testing.T, seed, path string, attrs map[string]string) {
	t.Helper()
	p, err := opc.Open(seed)
	if err != nil {
		t.Fatal(err)
	}
	d, err := p.XML("ppt/slides/slide1.xml")
	if err != nil {
		t.Fatal(err)
	}
	cells := office.All(d.Root(), ans, "tc")
	if len(cells) != 4 {
		t.Fatalf("fixture has %d cells, want 4", len(cells))
	}
	for key, value := range attrs {
		cells[0].CreateAttr(key, value)
	}
	if err := p.SetXML("ppt/slides/slide1.xml", d); err != nil {
		t.Fatal(err)
	}
	if err := p.SaveNew(path); err != nil {
		t.Fatal(err)
	}
}

func normalizedTableSlide(t *testing.T, path string) string {
	t.Helper()
	p, err := opc.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	d, err := p.XML("ppt/slides/slide1.xml")
	if err != nil {
		t.Fatal(err)
	}
	for _, cell := range office.All(d.Root(), ans, "tc") {
		for _, attr := range []string{"rowSpan", "gridSpan", "hMerge", "vMerge"} {
			cell.RemoveAttr(attr)
		}
	}
	got, err := d.WriteToString()
	if err != nil {
		t.Fatal(err)
	}
	return got
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
