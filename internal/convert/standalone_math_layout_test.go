package convert

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/openvibely/openvibely-paperworks/internal/docx"
	"github.com/openvibely/openvibely-paperworks/internal/equation"
	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"github.com/openvibely/openvibely-paperworks/internal/pdf"
	"github.com/openvibely/openvibely-paperworks/internal/testsupport"
)

// test_document_runtime.py:262-271. Both original modes, physical heights,
// fraction text, and the inline sentence are preserved in the saved document.
func TestReferenceStandaloneRuntimeMathLayout(t *testing.T) {
	referenceRuntimeOffice(t)
	dir := t.TempDir()
	widths := map[string]float64{}
	extents := [][2]int64{}
	for _, c := range []struct {
		name   string
		inline bool
		height float64
	}{{"inline", true, .22}, {"display", false, .5}} {
		o := equation.Defaults()
		o.Inline = c.inline
		r, e := equation.Write(context.Background(), `\frac{a+b}{c+d}`, filepath.Join(dir, c.name+".png"), o)
		if e != nil {
			t.Fatal(e)
		}
		widths[c.name] = c.height * float64(r.Width) / float64(r.Height)
		// reference DOCX library Image.scaled_dimensions(height=Inches(...)) retains
		// the requested height and rounds the scaled native width to even.
		// The pinned equation helper and native PNGs both declare 300 DPI.
		nativeWidth := int64(float64(r.Width) / 300 * 914400)
		nativeHeight := int64(float64(r.Height) / 300 * 914400)
		cy := int64(c.height * 914400)
		cx := int64(math.RoundToEven(float64(nativeWidth) * (float64(cy) / float64(nativeHeight))))
		extents = append(extents, [2]int64{cx, cy})
	}
	// Reuse the Document() default package from the pinned fixture bundle. The
	// images and body below remain native outputs, not captured Reference results.
	defaults, e := opc.Open(referenceRuntimeOriginalPackageFixture(t, "runtime-word-fixtures.zip", "before", ".docx"))
	if e != nil {
		t.Fatal(e)
	}
	for _, resource := range []struct{ part, file string }{
		{"word/styles.xml", "styles.xml"}, {"word/theme/theme1.xml", "theme.xml"},
	} {
		if e := os.WriteFile(filepath.Join(dir, resource.file), defaults.Get(resource.part), 0600); e != nil {
			t.Fatal(e)
		}
	}
	src := filepath.Join(dir, "seed.docx")
	if e := docx.Create(docx.Spec{StylesFile: "styles.xml", ThemeFile: "theme.xml", CompatibilityMode: 14, Blocks: []docx.Block{
		{Type: "heading", Level: 1, Text: "Equation layouts"},
		{Type: "paragraph", Text: "Inline fraction: "},
		{Type: "image", Path: "inline.png", WidthIn: widths["inline"]},
		{Type: "paragraph", Text: " stays on the sentence line."},
		{Type: "paragraph", Text: "Display fraction:"},
		{Type: "image", Path: "display.png", WidthIn: widths["display"]},
	}}, dir, src); e != nil {
		t.Fatal(e)
	}
	// The authoring API emits image blocks. Assemble the original inline run
	// sequence in the test fixture without adding production behavior.
	p, e := opc.Open(src)
	if e != nil {
		t.Fatal(e)
	}
	d, e := p.XML("word/document.xml")
	if e != nil {
		t.Fatal(e)
	}
	// The block authoring API accepts width, while the original fixture supplies
	// height. Restore height-only sizing on both drawing extents before save.
	drawings := office.All(d.Root(), office.NSWP, "inline")
	if len(drawings) != len(extents) {
		t.Fatalf("drawings %d, want %d", len(drawings), len(extents))
	}
	for i, drawing := range drawings {
		// reference DOCX library leaves inline distances absent. Explicit zero distances
		// suppress the inherited margins in the reference Word renderer.
		for _, attr := range []string{"distT", "distB", "distL", "distR"} {
			drawing.RemoveAttr(attr)
		}
		for _, extent := range append(office.All(drawing, office.NSWP, "extent"), office.All(drawing, "http://schemas.openxmlformats.org/drawingml/2006/main", "ext")...) {
			extent.CreateAttr("cx", strconv.FormatInt(extents[i][0], 10))
			extent.CreateAttr("cy", strconv.FormatInt(extents[i][1], 10))
		}
	}
	// Match reference DOCX library CT_Inline/CT_Picture defaults for this fixture. The
	// authoring API uses its own picture metadata; preserve its generated image,
	// relationship and dimensions while applying the original constructor fields.
	for i, drawing := range drawings {
		const ans = "http://schemas.openxmlformats.org/drawingml/2006/main"
		const picns = "http://schemas.openxmlformats.org/drawingml/2006/picture"
		drawing.CreateAttr("xmlns:a", ans)
		drawing.CreateAttr("xmlns:pic", picns)
		office.Child(drawing, office.NSWP, "docPr").RemoveAttr("descr")
		graphic := office.Child(drawing, ans, "graphic")
		graphic.RemoveAttr("xmlns:a")
		picture := office.All(graphic, picns, "pic")[0]
		picture.RemoveAttr("xmlns:pic")
		nonvisual := office.All(picture, picns, "cNvPr")[0]
		nonvisual.CreateAttr("id", "0")
		nonvisual.CreateAttr("name", []string{"inline.png", "display.png"}[i])
		nonvisual.RemoveAttr("descr")
		for _, geometry := range office.All(picture, ans, "prstGeom") {
			if list := office.Child(geometry, ans, "avLst"); list != nil {
				geometry.RemoveChild(list)
			}
		}
		drawing.RemoveChild(graphic)
		frame := drawing.CreateElement("wp:cNvGraphicFramePr")
		frame.CreateElement("a:graphicFrameLocks").CreateAttr("noChangeAspect", "1")
		drawing.AddChild(graphic)
	}
	// CT_R.add_t writes xml:space only for leading/trailing whitespace.
	for _, text := range office.All(d.Root(), office.NSW, "t") {
		if strings.TrimSpace(text.Text()) == text.Text() {
			text.RemoveAttr("xml:space")
		}
	}
	body := office.Child(d.Root(), office.NSW, "body")
	defaultDocument, e := defaults.XML("word/document.xml")
	if e != nil {
		t.Fatal(e)
	}
	// Retain the original Document() root namespace order and compatibility flags.
	d.Root().Attr = append(d.Root().Attr[:0], defaultDocument.Root().Attr...)
	defaultSection := office.Child(office.Child(defaultDocument.Root(), office.NSW, "body"), office.NSW, "sectPr")
	if defaultSection == nil {
		t.Fatal("pinned Document() section missing")
	}
	body.RemoveChild(office.Child(body, office.NSW, "sectPr"))
	body.AddChild(defaultSection.Copy())
	p.Set("word/settings.xml", defaults.Get("word/settings.xml"))
	paragraphs := office.Children(body, office.NSW, "p")
	if len(paragraphs) != 6 {
		t.Fatalf("paragraphs %d, want 6", len(paragraphs))
	}
	for _, index := range []int{2, 3} {
		for _, run := range office.Children(paragraphs[index], office.NSW, "r") {
			paragraphs[1].AddChild(run.Copy())
		}
		body.RemoveChild(paragraphs[index])
	}
	// Preserve the original default package (including custom XML, font table,
	// thumbnail, numbering and metadata). Copy only the newly generated image
	// payloads; the fixture's sample document body is replaced below.
	rels, e := p.Rels("word/document.xml")
	if e != nil {
		t.Fatal(e)
	}
	for _, rel := range rels {
		if rel.Type != opc.NSOffR+"/image" {
			continue
		}
		part := opc.Resolve("word/document.xml", rel.Target)
		defaults.Set(part, p.Get(part))
		if e = defaults.EnsureContentType(filepath.Ext(part), p.ContentType(part)); e != nil {
			t.Fatal(e)
		}
		id, err := defaults.AddRel("word/document.xml", rel.Type, rel.Target, false)
		if err != nil {
			t.Fatal(err)
		}
		for _, blip := range office.All(d.Root(), "http://schemas.openxmlformats.org/drawingml/2006/main", "blip") {
			if blip.SelectAttrValue("r:embed", "") == rel.ID {
				blip.CreateAttr("r:embed", id)
			}
		}
	}
	p = defaults
	if e = p.SetXML("word/document.xml", d); e != nil {
		t.Fatal(e)
	}
	// Shared authoring now retains the template's lxml declaration directly.
	// The previous fixture-only rewrite is no longer necessary.
	const referenceDeclaration = `<?xml version='1.0' encoding='UTF-8' standalone='yes'?>`
	if !strings.HasPrefix(string(p.Get("word/document.xml")), referenceDeclaration) {
		t.Fatal("unexpected document XML declaration")
	}

	// reference DOCX library PackageWriter sorts content types and writes parts in
	// relationship depth-first order. Preserve that setup here, without
	// changing the production OPC writer's preservation contract.
	types, e := p.XML("[Content_Types].xml")
	if e != nil {
		t.Fatal(e)
	}
	entries := types.Root().ChildElements()
	sort.SliceStable(entries, func(i, j int) bool {
		a, b := entries[i], entries[j]
		if a.Tag != b.Tag {
			return a.Tag < b.Tag
		}
		key := "Extension"
		if a.Tag == "Override" {
			key = "PartName"
		}
		return a.SelectAttrValue(key, "") < b.SelectAttrValue(key, "")
	})
	for _, entry := range entries {
		types.Root().RemoveChild(entry)
		types.Root().AddChild(entry)
	}
	if e = p.SetXML("[Content_Types].xml", types); e != nil {
		t.Fatal(e)
	}
	byName := make(map[string]*opc.Part)
	for _, part := range p.Parts {
		byName[part.Name] = part
	}
	ordered := []*opc.Part{byName["[Content_Types].xml"], byName["_rels/.rels"]}
	visited := map[string]bool{}
	var walk func(string)
	walk = func(source string) {
		rels, err := p.Rels(source)
		if err != nil {
			t.Fatal(err)
		}
		for _, rel := range rels {
			if rel.TargetMode == "External" {
				continue
			}
			name := opc.Resolve(source, rel.Target)
			if visited[name] {
				continue
			}
			visited[name] = true
			part := byName[name]
			if part == nil {
				t.Fatalf("default relationship target missing: %s", name)
			}
			ordered = append(ordered, part)
			if relationship := byName[opc.RelsName(name)]; relationship != nil {
				ordered = append(ordered, relationship)
			}
			walk(name)
		}
	}
	walk("")
	if len(ordered) != len(p.Parts) {
		t.Fatalf("default package has unreachable parts: ordered %d of %d", len(ordered), len(p.Parts))
	}
	p.Parts = ordered
	out := filepath.Join(dir, "math-layout.docx")
	raw, e := testsupport.DocumentFixtureBytes(p)
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(out, raw, 0600); e != nil {
		t.Fatal(e)
	}
	pages, e := RenderFile(context.Background(), out, filepath.Join(dir, "math-layout-render"), "", "", pdf.RenderOptions{})
	if e != nil {
		t.Fatal(e)
	}
	if len(pages) == 0 {
		t.Error("saved inline/display math document produced no pages")
	}
}
