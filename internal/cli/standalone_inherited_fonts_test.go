package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/openvibely/openvibely-paperworks/internal/lo"
	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"github.com/openvibely/openvibely-paperworks/internal/testsupport"
)

// test_document_runtime.py:635-653: fonts inherited from the theme must be
// checked against actual rendering, including the wrong-family exception text.
func TestReferenceStandaloneRuntimeInheritedFonts(t *testing.T) {
	if _, err := lo.Find(); err != nil {
		t.Skip("LibreOffice required for inherited font contract")
	}
	dir := t.TempDir()
	// Reference uses the title/content layout, including its inherited Arial bullet.
	// Reuse the already-retained reference presentation library template, removing its chart payload.
	p, err := opc.Open("../../testdata/reference-mirror/literal-chart.pptx")
	if err != nil {
		t.Fatal(err)
	}
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
	if err := p.SetXML("[Content_Types].xml", ct); err != nil {
		t.Fatal(err)
	}
	p.Set("ppt/slides/slide1.xml", []byte(`<?xml version='1.0' encoding='UTF-8' standalone='yes'?>
<p:sld xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><p:cSld><p:spTree><p:nvGrpSpPr><p:cNvPr id="1" name=""/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr><p:grpSpPr/><p:sp><p:nvSpPr><p:cNvPr id="2" name="Title 1"/><p:cNvSpPr><a:spLocks noGrp="1"/></p:cNvSpPr><p:nvPr><p:ph type="title"/></p:nvPr></p:nvSpPr><p:spPr/><p:txBody><a:bodyPr/><a:lstStyle/><a:p><a:r><a:t>Inherited font check</a:t></a:r></a:p></p:txBody></p:sp><p:sp><p:nvSpPr><p:cNvPr id="3" name="Content Placeholder 2"/><p:cNvSpPr><a:spLocks noGrp="1"/></p:cNvSpPr><p:nvPr><p:ph idx="1"/></p:nvPr></p:nvSpPr><p:spPr/><p:txBody><a:bodyPr/><a:lstStyle/><a:p><a:r><a:t>Rendered with the requested family</a:t></a:r></a:p></p:txBody></p:sp></p:spTree></p:cSld><p:clrMapOvr><a:masterClrMapping/></p:clrMapOvr></p:sld>`))
	p.Set("ppt/slides/_rels/slide1.xml.rels", []byte(`<?xml version='1.0' encoding='UTF-8' standalone='yes'?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/slideLayout" Target="../slideLayouts/slideLayout2.xml"/></Relationships>`))
	for _, name := range p.Names() {
		if !strings.HasSuffix(name, ".xml") {
			continue
		}
		if !strings.HasPrefix(name, "ppt/theme/") {
			continue
		}
		d, err := p.XML(name)
		if err != nil {
			t.Fatal(err)
		}
		for _, n := range office.All(d.Root(), office.NSA, "latin") {
			n.CreateAttr("typeface", "Liberation Sans")
		}
		if err := p.SetXML(name, d); err != nil {
			t.Fatal(err)
		}
		// The original lxml.tostring uses encoding='utf-8' and omits standalone.
		p.Set(name, bytes.Replace(p.Get(name), []byte(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`), []byte(`<?xml version='1.0' encoding='utf-8'?>`), 1))
	}
	src := filepath.Join(dir, "font-inherited.pptx")
	// The original ZipFile(..., 'w') rewrites ordered members using ZIP_STORED.
	// Preserve all payloads, including the inherited Arial bullet declaration.
	stored := &testsupport.OrderedZIP{}
	for _, part := range p.Parts {
		stored.Set(part.Name, part.Data)
	}
	if err := testsupport.SaveStoredZIP(src, stored); err != nil {
		t.Fatal(err)
	}
	// Retain the original defect as a rejection check before correcting the
	// fixture. The theme-only text check must not include template bullets.
	badReq := writeSpec(t, dir, "original-fonts.json", map[string]any{"fonts": []string{"Liberation Sans"}, "render_fonts": true})
	var badOut, badErr bytes.Buffer
	if code := Main([]string{"verify-slides", src, "--requirements", badReq}, &badOut, &badErr); code != 4 || !strings.Contains(badErr.String(), "Unapproved rendered fonts") {
		t.Fatalf("original Arial bullet: code %d, error %q", code, badErr.String())
	}
	// This checks inherited text, not a template bullet's independent font.
	// Disable bullets on the test paragraphs; keep all text runs unformatted.
	slide, err := p.XML("ppt/slides/slide1.xml")
	if err != nil {
		t.Fatal(err)
	}
	for _, paragraph := range office.All(slide.Root(), office.NSA, "p") {
		props := paragraph.CreateElement("a:pPr")
		paragraph.RemoveChild(props)
		paragraph.InsertChildAt(0, props)
		props.CreateElement("a:buNone")
	}
	if err := p.SetXML("ppt/slides/slide1.xml", slide); err != nil {
		t.Fatal(err)
	}
	corrected := &testsupport.OrderedZIP{}
	for _, part := range p.Parts {
		corrected.Set(part.Name, part.Data)
	}
	if err := testsupport.SaveStoredZIP(src, corrected); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	if !t.Run("rendered-family", func(t *testing.T) {
		req := writeSpec(t, dir, "approved.json", map[string]any{"fonts": []string{"Liberation Sans"}, "render_fonts": true})
		r := runCLI(t, "verify-slides", src, "--requirements", req)
		if fonts, ok := r["explicit_fonts"].([]any); !ok || len(fonts) != 0 {
			t.Errorf("explicit fonts: %v", r["explicit_fonts"])
		}
		want := []any{map[string]any{"family": "Liberation Sans", "pages": []any{float64(1)}}}
		if !reflect.DeepEqual(r["rendered_fonts"], want) {
			t.Errorf("rendered fonts = %#v, want %#v", r["rendered_fonts"], want)
		}
	}) {
		// The original sequential Reference block stops at this failed prerequisite.
		return
	}
	t.Run("wrong-family", func(t *testing.T) {
		req := writeSpec(t, dir, "rejected.json", map[string]any{"fonts": []string{"Arial"}, "render_fonts": true})
		var out, errs bytes.Buffer
		code := Main([]string{"verify-slides", src, "--requirements", req}, &out, &errs)
		// Reference catches ValueError specifically; the CLI maps verification failures to 4.
		if code != 4 || !strings.Contains(errs.String(), "Unapproved rendered fonts") {
			t.Errorf("code %d, error %q: want Unapproved rendered fonts", code, errs.String())
		}
	})
	t.Run("source-preserved", func(t *testing.T) {
		after, err := os.ReadFile(src)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(before, after) {
			t.Error("font verification modified source")
		}
	})
}
