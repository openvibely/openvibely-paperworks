package office

import (
	"bytes"
	"fmt"
	"github.com/beevik/etree"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/openvibely/openvibely-paperworks/internal/opc"
)

func TestReferencePreviewIncludesHiddenSlides(t *testing.T) {
	referencePresentationRoot(t)
	p := referenceHiddenSlideFixture(t)
	src := filepath.Join(referencePresentationRoot(t), "hidden.pptx")
	if e := p.SaveNew(src); e != nil {
		t.Fatal(e)
	}
	before := previewRead(t, src)
	if needed, e := NeedsOfficePreview(src); e != nil || !needed {
		t.Fatal(needed, e)
	}
	out := filepath.Join(referencePresentationRoot(t), "preview.pptx")
	if changed, e := OfficePreviewCopy(src, out); e != nil || !changed {
		t.Fatal(changed, e)
	}
	previewUnchanged(t, src, before)
	q := previewOpen(t, out)
	slides := referencePresentationRoots(t, out)
	if len(slides) != 3 {
		t.Fatal("slide count", len(slides))
	}
	var flags, texts []string
	for _, slide := range slides {
		flags = append(flags, slide.SelectAttrValue("show", "1"))
		// Reference asks for shapes[0].text, not the first text-bearing shape.
		tree := Child(Child(slide, NSP, "cSld"), NSP, "spTree")
		var first *etree.Element
		for _, shape := range tree.ChildElements() {
			if shape.NamespaceURI() == NSP && strings.Contains("|sp|grpSp|graphicFrame|cxnSp|pic|contentPart|", "|"+shape.Tag+"|") {
				first = shape
				break
			}
		}
		if first == nil || !Is(first, NSP, "sp") {
			t.Fatal("first shape has no text property")
		}
		texts = append(texts, referenceHistogramShapeTexts(t, slide)[0])
	}
	if !reflect.DeepEqual(flags, []string{"1", "1", "1"}) {
		t.Error(flags)
	}
	if !reflect.DeepEqual(texts, []string{"Slide 1", "Slide 2", "Slide 3"}) {
		t.Error(texts)
	}
	for _, name := range p.Names() {
		if name != "ppt/slides/slide2.xml" && name != "ppt/slides/slide3.xml" && !bytes.Equal(p.Get(name), q.Get(name)) {
			t.Error("unrelated part changed", name)
		}
	}
}

// Reuse the pinned default reference presentation library template already retained in the literal
// chart fixture. Remove only its chart, then recreate the original blank-layout
// slides; no new binary template is required.
func referenceHiddenSlideFixture(t *testing.T) *opc.Package {
	t.Helper()
	p := previewOpen(t, "../../testdata/reference-mirror/literal-chart.pptx")
	for _, name := range []string{"ppt/charts/chart1.xml", "ppt/charts/_rels/chart1.xml.rels", "ppt/embeddings/Microsoft_Excel_Sheet1.xlsx"} {
		p.Remove(name)
	}
	ct, err := p.XML("[Content_Types].xml")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range ct.Root().ChildElements() {
		if strings.HasPrefix(e.SelectAttrValue("PartName", ""), "/ppt/charts/") || e.SelectAttrValue("Extension", "") == "xlsx" {
			ct.Root().RemoveChild(e)
		}
	}
	if err = p.SetXML("[Content_Types].xml", ct); err != nil {
		t.Fatal(err)
	}
	deck, err := p.XML("ppt/presentation.xml")
	if err != nil {
		t.Fatal(err)
	}
	ids := Child(deck.Root(), NSP, "sldIdLst")
	for i := 0; i < 3; i++ {
		name := fmt.Sprintf("ppt/slides/slide%d.xml", i+1)
		show := ""
		if i == 1 {
			show = ` show="0"`
		}
		if i == 2 {
			show = ` show="false"`
		}
		p.Set(name, []byte(fmt.Sprintf(`<p:sld xmlns:a="%s" xmlns:p="%s" xmlns:r="%s"%s><p:cSld><p:spTree><p:nvGrpSpPr><p:cNvPr id="1" name=""/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr><p:grpSpPr/><p:sp><p:nvSpPr><p:cNvPr id="2" name="TextBox 1"/><p:cNvSpPr txBox="1"/><p:nvPr/></p:nvSpPr><p:spPr><a:xfrm><a:off x="914400" y="914400"/><a:ext cx="5486400" cy="914400"/></a:xfrm><a:prstGeom prst="rect"><a:avLst/></a:prstGeom><a:noFill/></p:spPr><p:txBody><a:bodyPr wrap="none"><a:spAutoFit/></a:bodyPr><a:lstStyle/><a:p><a:r><a:t>Slide %d</a:t></a:r></a:p></p:txBody></p:sp></p:spTree></p:cSld><p:clrMapOvr><a:masterClrMapping/></p:clrMapOvr></p:sld>`, NSA, NSP, NSR, show, i+1)))
		p.Set(opc.RelsName(name), []byte(`<Relationships xmlns="`+opc.NSRel+`"><Relationship Id="rId1" Type="`+NSR+`/slideLayout" Target="../slideLayouts/slideLayout7.xml"/></Relationships>`))
		if i > 0 {
			rid, err := p.AddRel("ppt/presentation.xml", NSR+"/slide", fmt.Sprintf("slides/slide%d.xml", i+1), false)
			if err != nil {
				t.Fatal(err)
			}
			id := ids.CreateElement("p:sldId")
			id.CreateAttr("id", fmt.Sprint(256+i))
			id.CreateAttr("r:id", rid)
			if err = p.EnsureContentType(name, "application/vnd.openxmlformats-officedocument.presentationml.slide+xml"); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err = p.SetXML("ppt/presentation.xml", deck); err != nil {
		t.Fatal(err)
	}
	// reference presentation library writes content-type overrides sorted by part name.
	ct, err = p.XML("[Content_Types].xml")
	if err != nil {
		t.Fatal(err)
	}
	children := ct.Root().ChildElements()
	sort.SliceStable(children, func(i, j int) bool {
		if children[i].Tag != children[j].Tag {
			return children[i].Tag < children[j].Tag
		}
		return children[i].SelectAttrValue("PartName", children[i].SelectAttrValue("Extension", "")) < children[j].SelectAttrValue("PartName", children[j].SelectAttrValue("Extension", ""))
	})
	for _, e := range children {
		ct.Root().RemoveChild(e)
		ct.Root().AddChild(e)
	}
	if err = p.SetXML("[Content_Types].xml", ct); err != nil {
		t.Fatal(err)
	}
	for i, part := range p.Parts {
		if part.Name == "docProps/thumbnail.jpeg" {
			p.Parts = append(append(p.Parts[:i], p.Parts[i+1:]...), part)
			break
		}
	}
	return p
}
