package office

import (
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/beevik/etree"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"github.com/openvibely/openvibely-paperworks/internal/testsupport"
)

// Build the small blank/text/group presentations from the Reference test setup at
// runtime, retaining an existing valid presentation container instead of adding
// another binary fixture.
func referencePresentationWithShapes(t *testing.T, shapes string) *opc.Package {
	t.Helper()
	p := fixture(t, "reference-bar.pptx")
	// Remove the seed chart and its registrations to reproduce a blank deck.
	parts, err := ChartParts(p)
	if err != nil {
		t.Fatal(err)
	}
	removed := map[string]bool{}
	for _, part := range parts {
		removed[part] = true
		p.Remove(part)
		p.Remove(opc.RelsName(part))
	}
	for _, name := range p.Names() {
		if !strings.HasSuffix(name, ".rels") {
			continue
		}
		d, err := p.XML(name)
		if err != nil {
			t.Fatal(err)
		}
		for _, rel := range d.Root().ChildElements() {
			if removed[opc.Resolve(opc.SourceOf(name), rel.SelectAttrValue("Target", ""))] {
				d.Root().RemoveChild(rel)
			}
		}
		if err := p.SetXML(name, d); err != nil {
			t.Fatal(err)
		}
	}
	ct, err := p.XML("[Content_Types].xml")
	if err != nil {
		t.Fatal(err)
	}
	for _, node := range ct.Root().ChildElements() {
		if removed[strings.TrimPrefix(node.SelectAttrValue("PartName", ""), "/")] {
			ct.Root().RemoveChild(node)
		}
	}
	if err := p.SetXML("[Content_Types].xml", ct); err != nil {
		t.Fatal(err)
	}
	p.Set("ppt/slides/slide1.xml", []byte(fmt.Sprintf(`<p:sld xmlns:p="%s" xmlns:a="%s" xmlns:r="%s"><p:cSld><p:spTree><p:nvGrpSpPr><p:cNvPr id="1" name=""/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr><p:grpSpPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="0" cy="0"/><a:chOff x="0" y="0"/><a:chExt cx="0" cy="0"/></a:xfrm></p:grpSpPr>%s</p:spTree></p:cSld><p:clrMapOvr><a:masterClrMapping/></p:clrMapOvr></p:sld>`, NSP, NSA, NSR, shapes)))
	return p
}

// Retain the complete original reference presentation library package, not a chart deck with
// substituted slide XML. The bundle shares unchanged template payloads.
func referenceCommentFixture(t *testing.T, name string) *opc.Package {
	t.Helper()
	return previewOpen(t, testsupport.OriginalPackageFixture(t, "runtime-workflow-fixtures.zip", name, ".pptx"))
}

func referenceTextShape(id int, paragraph string) string {
	return fmt.Sprintf(`<p:sp><p:nvSpPr><p:cNvPr id="%d" name="TextBox"/><p:cNvSpPr txBox="1"/><p:nvPr/></p:nvSpPr><p:spPr><a:xfrm><a:off x="914400" y="914400"/><a:ext cx="2743200" cy="914400"/></a:xfrm><a:prstGeom prst="rect"><a:avLst/></a:prstGeom></p:spPr><p:txBody><a:bodyPr/><a:lstStyle/><a:p>%s</a:p></p:txBody></p:sp>`, id, paragraph)
}

func referenceSavePresentation(t *testing.T, p *opc.Package) *opc.Package {
	t.Helper()
	out := referenceNextPresentation(t)
	if e := p.SaveNew(out); e != nil {
		t.Fatal(e)
	}
	return previewOpen(t, out)
}

func referenceAnchor(t *testing.T, comments []SlideComment) *etree.Element {
	t.Helper()
	if len(comments) == 0 || len(comments[len(comments)-1].Anchors) == 0 {
		t.Fatal("missing anchor", comments)
	}
	d, e := opc.ParseXML([]byte(comments[len(comments)-1].Anchors[0]))
	if e != nil {
		t.Fatal(e)
	}
	return d.Root()
}

func referenceAnchorAttributes(t *testing.T, node *etree.Element, want map[string]string) {
	t.Helper()
	if node == nil {
		t.Fatal("missing anchor moniker")
	}
	got := map[string]string{}
	for _, a := range node.Attr {
		if a.Space != "xmlns" && a.Key != "xmlns" {
			got[a.FullKey()] = a.Value
		}
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("anchor attributes %v want %v", got, want)
	}
}

// reference presentation library TextFrame.text joins direct paragraphs and preserves ordered
// run/field text and vertical-tab breaks. Do not flatten descendant strings.
func referenceCommentShapeText(shape *etree.Element) string {
	var paragraphs []string
	for _, p := range Children(Child(shape, NSP, "txBody"), NSA, "p") {
		var text strings.Builder
		for _, item := range p.ChildElements() {
			if Is(item, NSA, "br") {
				text.WriteByte('\v')
			}
			if Is(item, NSA, "r") || Is(item, NSA, "fld") {
				if value := Child(item, NSA, "t"); value != nil {
					text.WriteString(value.Text())
				}
			}
		}
		paragraphs = append(paragraphs, text.String())
	}
	return strings.Join(paragraphs, "\n")
}

func TestReferenceSlideLevelComment(t *testing.T) {
	referencePresentationRoot(t)
	referenceTextAnchorCase(t, "reference-anchors.pptx", "four", 1)
	p := referenceCommentFixture(t, "comment-blank")
	base := filepath.Join(referencePresentationRoot(t), "plain.pptx")
	if err := p.SaveNew(base); err != nil {
		t.Fatal(err)
	}
	p = previewOpen(t, base)
	r, e := SlideComments(p, SlideCommentSpec{Action: "add", Slide: 1, Author: "Reviewer", Text: "Slide comment"})
	if e != nil {
		t.Fatal(e)
	}
	if len(r) == 0 || r[0].Slide != 1 {
		t.Fatal(r)
	}
	out := referenceNextPresentation(t)
	if e := p.SaveNew(out); e != nil {
		t.Fatal(e)
	}
	if slides := referencePresentationRoots(t, out); len(slides) != 1 {
		t.Fatal("saved slide count", len(slides))
	}
}

func TestReferenceGroupedComments(t *testing.T) {
	referencePresentationRoot(t)
	source := referenceSavePresentation(t, referenceCommentFixture(t, "comment-group"))
	raw, err := source.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name  string
		id    int
		query *string
		kind  string
	}{{"box", 3, func() *string { s := "one\nLine"; return &s }(), "spMk"}, {"group", 2, nil, "grpSpMk"}} {
		t.Run(c.name, func(t *testing.T) {
			p, err := opc.Read(raw)
			if err != nil {
				t.Fatal(err)
			}
			r, e := SlideComments(p, SlideCommentSpec{Action: "add", Slide: 1, ShapeID: c.id, Match: c.query, Author: "Local", Text: "Grouped"})
			if e != nil {
				t.Fatal(e)
			}
			anchor := referenceAnchor(t, r)
			node := Child(anchor, nsOAC, c.kind)
			if node == nil || node.SelectAttrValue("id", "") != fmt.Sprint(c.id) {
				t.Fatal("incorrect grouped anchor")
			}
			p = referenceSavePresentation(t, p)
			d, e := p.XML("ppt/slides/slide1.xml")
			if e != nil {
				t.Fatal(e)
			}
			groups := All(d.Root(), NSP, "grpSp")
			if len(groups) != 1 || len(Children(groups[0], NSP, "sp")) != 1 {
				t.Fatal("group children changed")
			}
			shape := Child(groups[0], NSP, "sp")
			if text := referenceCommentShapeText(shape); text != "Line one\vLine two" {
				t.Fatal("grouped text changed", text)
			}
			if c.query != nil {
				referenceAnchorAttributes(t, Child(anchor, nsOAC, "txMk"), map[string]string{"cp": "5", "len": "8"})
			}
		})
	}
}

func TestReferenceFieldCommentOffset(t *testing.T) {
	referencePresentationRoot(t)
	p := referenceSavePresentation(t, referenceCommentFixture(t, "comment-field"))
	match := "7"
	r, e := SlideComments(p, SlideCommentSpec{Action: "add", Slide: 1, ShapeID: 2, Match: &match, Author: "Reviewer", Text: "Page number"})
	if e != nil {
		t.Fatal(e)
	}
	if len(r) == 0 {
		t.Fatal("missing returned comment")
	}
	referenceAnchorAttributes(t, Child(referenceAnchor(t, r[:1]), nsOAC, "txMk"), map[string]string{"cp": "5", "len": "1"})
	p = referenceSavePresentation(t, p)
	d, e := p.XML("ppt/slides/slide1.xml")
	if e != nil {
		t.Fatal(e)
	}
	fields := All(d.Root(), NSA, "fld")
	if len(fields) != 1 || Child(fields[0], NSA, "t").Text() != "7" {
		t.Fatal("field text changed")
	}
}

func TestReferenceGraphicFrameComment(t *testing.T) {
	referencePresentationRoot(t)
	p := referenceSavePresentation(t, referenceCommentFixture(t, "comment-table"))
	r, e := SlideComments(p, SlideCommentSpec{Action: "add", Slide: 1, ShapeID: 2, Author: "Local", Text: "Table"})
	if e != nil {
		t.Fatal(e)
	}

	if len(r) == 0 {
		t.Fatal("missing returned comment")
	}
	anchor := referenceAnchor(t, r[:1])
	if Child(anchor, nsOAC, "graphicFrameMk") == nil || Child(anchor, nsOAC, "spMk") != nil {
		t.Fatal("wrong graphic-frame moniker")
	}
	referenceSavePresentation(t, p)
}
