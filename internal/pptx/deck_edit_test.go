package pptx

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/beevik/etree"
	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
)

func TestDeckEditingPreservesNativeSlides(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	in := filepath.Join(dir, "in.pptx")
	out := filepath.Join(dir, "out.pptx")
	slide := func(s string) Slide {
		return Slide{Notes: "notes " + s, Elements: []Element{{Type: "text", Text: s, X: 1, Y: 1, W: 5, H: 1}}}
	}
	if err := Create(ctx, Spec{Slides: []Slide{slide("First"), slide("Second")}}, dir, in); err != nil {
		t.Fatal(err)
	}
	third := slide("Third")
	extra := Element{Type: "shape", Text: "Added", Shape: "ellipse", X: 2, Y: 3, W: 2, H: 1}
	err := DeckEditFile(ctx, in, out, dir, DeckEditSpec{Edits: []DeckEdit{{Operation: "duplicate-slide", Slide: 1, Position: 3}, {Operation: "move-slide", Slide: 3, Position: 1}, {Operation: "delete-slide", Slide: 2}, {Operation: "add-slide", Content: &third}, {Operation: "add-element", Slide: 2, Element: &extra}}})
	if err != nil {
		t.Fatal(err)
	}
	p, err := opc.Open(out)
	if err != nil {
		t.Fatal(err)
	}
	slides, err := office.PresentationSlides(p)
	if err != nil || len(slides) != 3 {
		t.Fatal(slides, err)
	}
	for i, want := range []string{"First", "Second", "Third"} {
		d, err := p.XML(slides[i])
		if err != nil {
			t.Fatal(err)
		}

		seenIDs := map[string]bool{}
		for _, pr := range office.All(d.Root(), pns, "cNvPr") {
			id := pr.SelectAttrValue("id", "")
			if seenIDs[id] {
				t.Fatal("duplicate object id", id)
			}
			seenIDs[id] = true
		}
		texts := office.All(d.Root(), ans, "t")
		if len(texts) == 0 || texts[0].Text() != want {
			t.Fatal(i, texts)
		}
		if i == 1 && len(texts) != 2 {
			t.Fatal("missing added shape")
		}

		notesCount := 0
		rels, err := p.Rels(slides[i])
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range rels {
			if r.Type == rns+"/notesSlide" {
				notesCount++
				notes, err := p.XML(opc.Resolve(slides[i], r.Target))
				if err != nil {
					t.Fatal(err)
				}
				if !strings.Contains(office.Text(notes.Root(), ans, "t"), "notes "+want) {
					t.Fatal("original speaker notes lost")
				}
			}
		}
		if notesCount != 1 {
			t.Fatal("adding an object changed notes relationships", notesCount)
		}
	}
	// Every copied internal relationship must still resolve.
	for _, name := range p.Names() {
		if filepath.Ext(name) != ".rels" {
			continue
		}
		source := opc.SourceOf(name)
		rels, err := p.Rels(source)
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range rels {
			if r.TargetMode != "External" && !p.Has(opc.Resolve(source, r.Target)) {
				t.Fatal("dangling", name, r)
			}
		}
	}
}

func TestDeleteSlideUpdatesCustomShowAndSections(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	raw, in, out := filepath.Join(dir, "raw.pptx"), filepath.Join(dir, "in.pptx"), filepath.Join(dir, "out.pptx")
	if err := Create(ctx, Spec{Slides: []Slide{{}, {}}}, dir, raw); err != nil {
		t.Fatal(err)
	}
	p, _ := opc.Open(raw)
	main, _ := presentationPart(p)
	d, _ := p.XML(main)
	ids := office.Children(office.Child(d.Root(), pns, "sldIdLst"), pns, "sldId")
	show := d.Root().CreateElement("p:custShowLst").CreateElement("p:custShow")
	show.CreateAttr("name", "Review")
	show.CreateAttr("id", "1")
	list := show.CreateElement("p:sldLst")
	for _, id := range ids {
		list.CreateElement("p:sld").CreateAttr("r:id", id.SelectAttrValue("r:id", ""))
	}
	ext := d.Root().CreateElement("p:extLst").CreateElement("p:ext")
	ext.CreateAttr("uri", "{521415D9-36F7-43E2-AB2F-B90AF26B5E84}")
	sections := ext.CreateElement("p14:sectionLst")
	sections.CreateAttr("xmlns:p14", "http://schemas.microsoft.com/office/powerpoint/2010/main")
	section := sections.CreateElement("p14:section")
	section.CreateAttr("name", "Review")
	section.CreateAttr("id", "{12345678-1234-1234-1234-123456789ABC}")
	sl := section.CreateElement("p14:sldIdLst")
	for _, id := range ids {
		sl.CreateElement("p14:sldId").CreateAttr("id", id.SelectAttrValue("id", ""))
	}
	if err := p.SetXML(main, d); err != nil {
		t.Fatal(err)
	}
	if err := p.SaveNew(in); err != nil {
		t.Fatal(err)
	}
	if err := DeckEditFile(ctx, in, out, dir, DeckEditSpec{Edits: []DeckEdit{{Operation: "delete-slide", Slide: 1}}}); err != nil {
		t.Fatal(err)
	}
	result, _ := opc.Open(out)
	got, _ := result.XML(main)
	if refs := got.FindElements("//p:custShow/p:sldLst/p:sld"); len(refs) != 1 || refs[0].SelectAttrValue("r:id", "") != ids[1].SelectAttrValue("r:id", "") {
		t.Fatal("custom show still references deleted slide")
	}
	if refs := got.FindElements("//p14:section/p14:sldIdLst/p14:sldId"); len(refs) != 1 || refs[0].SelectAttrValue("id", "") != ids[1].SelectAttrValue("id", "") {
		t.Fatal("section still references deleted slide")
	}
}

func TestDuplicateSlidePreservesNavigationTargets(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	raw, in, out := filepath.Join(dir, "raw.pptx"), filepath.Join(dir, "in.pptx"), filepath.Join(dir, "out.pptx")
	slide := Slide{Notes: "Keep notes", Elements: []Element{{Type: "text", Text: "Navigation", X: 1, Y: 1, W: 5, H: 1}}}
	if err := Create(ctx, Spec{Slides: []Slide{slide, slide}}, dir, raw); err != nil {
		t.Fatal(err)
	}
	p, err := opc.Open(raw)
	if err != nil {
		t.Fatal(err)
	}
	slides, err := office.PresentationSlides(p)
	if err != nil {
		t.Fatal(err)
	}
	d, err := p.XML(slides[0])
	if err != nil {
		t.Fatal(err)
	}
	props := office.All(d.Root(), ans, "rPr")[0]
	for i, target := range slides {
		relative := opc.Relative(slides[0], target)
		rid, err := p.AddRel(slides[0], rns+"/slide", relative, false)
		if err != nil {
			t.Fatal(err)
		}
		tag := "a:hlinkClick"
		if i == 1 {
			tag = "a:hlinkMouseOver"
		}
		h := props.CreateElement(tag)
		h.CreateAttr("r:id", rid)
		h.CreateAttr("action", "ppaction://hlinksldjump")
	}
	if err := p.SetXML(slides[0], d); err != nil {
		t.Fatal(err)
	}
	if err := p.SaveNew(in); err != nil {
		t.Fatal(err)
	}
	if err := DeckEditFile(ctx, in, out, dir, DeckEditSpec{Edits: []DeckEdit{{Operation: "duplicate-slide", Slide: 1, Position: 3}}}); err != nil {
		t.Fatal(err)
	}
	got, err := opc.Open(out)
	if err != nil {
		t.Fatal(err)
	}
	listed, err := office.PresentationSlides(got)
	if err != nil || len(listed) != 3 {
		t.Fatal(listed, err)
	}
	dup := listed[2]
	doc, err := got.XML(dup)
	if err != nil {
		t.Fatal(err)
	}
	rels, err := got.Rels(dup)
	if err != nil {
		t.Fatal(err)
	}
	targets := map[string]string{}
	for _, r := range rels {
		targets[r.ID] = opc.Resolve(dup, r.Target)
	}
	for tag, want := range map[string]string{"hlinkClick": dup, "hlinkMouseOver": slides[1]} {
		h := office.All(doc.Root(), ans, tag)[0]
		if target := targets[h.SelectAttrValue("r:id", "")]; target != want {
			t.Fatalf("%s points to %s; want %s", tag, target, want)
		}
	}
	// Only the requested slide is duplicated, not the linked destination.
	count := 0
	for _, name := range got.Names() {
		if got.ContentType(name) == "application/vnd.openxmlformats-officedocument.presentationml.slide+xml" {
			count++
		}
	}
	if count != 3 {
		t.Fatalf("created %d slide parts, want 3", count)
	}
	// Notes must be copied, with their slide back-link pointing at the copy.
	for _, r := range rels {
		if r.Type == rns+"/notesSlide" {
			note := opc.Resolve(dup, r.Target)
			nr, err := got.Rels(note)
			if err != nil {
				t.Fatal(err)
			}
			for _, back := range nr {
				if back.Type == rns+"/slide" && opc.Resolve(note, back.Target) != dup {
					t.Fatal("notes still reference source slide")
				}
			}
		}
	}
	if string(got.Get(slides[0])) != string(p.Get(slides[0])) {
		t.Fatal("original slide changed")
	}
}

func deleteSlideFixture(t *testing.T, slides []Slide, prepare func(*opc.Package)) (string, string) {
	t.Helper()
	dir := t.TempDir()
	im := image.NewRGBA(image.Rect(0, 0, 4, 4))
	var buf bytes.Buffer
	if err := png.Encode(&buf, im); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "pic.png"), buf.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	in := filepath.Join(dir, "in.pptx")
	if err := Create(context.Background(), Spec{Slides: slides}, dir, in); err != nil {
		t.Fatal(err)
	}
	if prepare != nil {
		p, err := opc.Open(in)
		if err != nil {
			t.Fatal(err)
		}
		prepare(p)
		if err = os.Remove(in); err != nil {
			t.Fatal(err)
		}
		if err = p.SaveNew(in); err != nil {
			t.Fatal(err)
		}
	}
	return dir, in
}

func assertPackageClean(t *testing.T, p *opc.Package) {
	t.Helper()
	types, err := p.XML("[Content_Types].xml")
	if err != nil {
		t.Fatal(err)
	}
	for _, o := range types.Root().SelectElements("Override") {
		if name := strings.TrimPrefix(o.SelectAttrValue("PartName", ""), "/"); !p.Has(name) {
			t.Fatal("override for missing part", name)
		}
	}
	for _, name := range p.Names() {
		if filepath.Ext(name) != ".rels" {
			continue
		}
		source := opc.SourceOf(name)
		if source != "" && !p.Has(source) {
			t.Fatal("rels for missing part", name)
		}
		rels, err := p.Rels(source)
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range rels {
			if r.TargetMode != "External" && !p.Has(opc.Resolve(source, r.Target)) {
				t.Fatal("dangling", name, r)
			}
		}
	}
	main, _ := presentationPart(p)
	d, _ := p.XML(main)
	rels, _ := p.Rels(main)
	slideRels := 0
	for _, r := range rels {
		if r.Type == rns+"/slide" {
			slideRels++
		}
	}
	if n := len(office.All(d.Root(), pns, "sldId")); slideRels != n {
		t.Fatal("presentation slide relationships", slideRels, "slides", n)
	}
}

func deleteSlide(t *testing.T, dir, in string, slide int) *opc.Package {
	t.Helper()
	out := filepath.Join(dir, "out.pptx")
	if err := DeckEditFile(context.Background(), in, out, dir, DeckEditSpec{Edits: []DeckEdit{{Operation: "delete-slide", Slide: slide}}}); err != nil {
		t.Fatal(err)
	}
	p, err := opc.Open(out)
	if err != nil {
		t.Fatal(err)
	}
	assertPackageClean(t, p)
	return p
}

func TestDeleteSlideRemovesNotes(t *testing.T) {
	dir, in := deleteSlideFixture(t, []Slide{{Notes: "keep me"}, {Notes: "SECRET-NOTE layoffs list"}}, nil)
	p := deleteSlide(t, dir, in, 2)
	for _, name := range p.Names() {
		if bytes.Contains(p.Get(name), []byte("SECRET-NOTE")) {
			t.Fatal("deleted notes remain in", name)
		}
	}
	for _, part := range []string{"ppt/slides/slide2.xml", "ppt/notesSlides/notesSlide2.xml"} {
		if p.Has(part) || p.Has(opc.RelsName(part)) {
			t.Fatal("deleted slide part remains", part)
		}
	}
	if !bytes.Contains(p.Get("ppt/notesSlides/notesSlide1.xml"), []byte("keep me")) {
		t.Fatal("remaining notes lost")
	}
}

func TestDeleteSlideRemovesExclusiveMediaAndCharts(t *testing.T) {
	chart := &Chart{Type: "column", Categories: []string{"A", "B"}, Series: []Series{{Name: "S", Values: []float64{1, 2}}}}
	dir, in := deleteSlideFixture(t, []Slide{{}, {Elements: []Element{
		{Type: "image", Path: "pic.png", X: 1, Y: 1, W: 1, H: 1},
		{Type: "chart", X: 3, Y: 1, W: 3, H: 2, Chart: chart},
	}}}, nil)
	before, _ := opc.Open(in)
	var owned []string
	for _, n := range before.Names() {
		if strings.HasPrefix(n, "ppt/media/") || strings.HasPrefix(n, "ppt/charts/") || strings.HasPrefix(n, "ppt/embeddings/") {
			owned = append(owned, n)
		}
	}
	if len(owned) < 3 {
		t.Fatal("fixture lacks media, chart and workbook", owned)
	}
	p := deleteSlide(t, dir, in, 2)
	for _, n := range owned {
		if p.Has(n) {
			t.Fatal("exclusive part retained", n)
		}
	}
	for _, n := range []string{"ppt/slideMasters/slideMaster1.xml", "ppt/theme/theme1.xml", "ppt/notesMasters/notesMaster1.xml"} {
		if before.Has(n) && !p.Has(n) {
			t.Fatal("shared part removed", n)
		}
	}
}

func TestDeleteSlideKeepsSharedImage(t *testing.T) {
	pic := Element{Type: "image", Path: "pic.png", X: 1, Y: 1, W: 1, H: 1}
	var media string
	dir, in := deleteSlideFixture(t, []Slide{{Elements: []Element{pic}}, {Elements: []Element{pic}}}, func(p *opc.Package) {
		slides, _ := office.PresentationSlides(p)
		first, _ := p.Rels(slides[0])
		for _, r := range first {
			if r.Type == rns+"/image" {
				media = opc.Resolve(slides[0], r.Target)
			}
		}
		target := opc.Relative(slides[1], media)
		doc, _ := p.XML(opc.RelsName(slides[1]))
		for _, r := range doc.Root().ChildElements() {
			if r.SelectAttrValue("Type", "") == rns+"/image" {
				r.CreateAttr("Target", target)
			}
		}
		if err := p.SetXML(opc.RelsName(slides[1]), doc); err != nil {
			t.Fatal(err)
		}
	})
	if media == "" {
		t.Fatal("no image")
	}
	p := deleteSlide(t, dir, in, 1)
	if !p.Has(media) {
		t.Fatal("shared image removed", media)
	}
}

func TestDeleteSlideDropsHyperlinksToIt(t *testing.T) {
	text := Element{Type: "text", Text: "Jump", X: 1, Y: 1, W: 3, H: 1}
	var linking string
	dir, in := deleteSlideFixture(t, []Slide{{Elements: []Element{text}}, {}}, func(p *opc.Package) {
		slides, _ := office.PresentationSlides(p)
		linking = slides[0]
		target := opc.Relative(slides[0], slides[1])
		id, err := p.AddRel(slides[0], rns+"/slide", target, false)
		if err != nil {
			t.Fatal(err)
		}
		d, _ := p.XML(slides[0])
		pr := office.All(d.Root(), pns, "cNvPr")[1]
		link := pr.CreateElement("a:hlinkClick")
		link.CreateAttr("r:id", id)
		link.CreateAttr("action", "ppaction://hlinksldjump")
		if err = p.SetXML(slides[0], d); err != nil {
			t.Fatal(err)
		}
	})
	p := deleteSlide(t, dir, in, 2)
	rels, _ := p.Rels(linking)
	for _, r := range rels {
		if r.Type == rns+"/slide" {
			t.Fatal("dangling slide link", r)
		}
	}
	d, _ := p.XML(linking)
	if len(office.All(d.Root(), ans, "hlinkClick")) != 0 {
		t.Fatal("hyperlink to deleted slide remains")
	}
	if !strings.Contains(office.Text(d.Root(), ans, "t"), "Jump") {
		t.Fatal("linking shape lost")
	}
}
func TestDeleteSlideKeepsDetachedLayoutDependencies(t *testing.T) {
	const layout = "ppt/slideLayouts/private.xml"
	const media = "ppt/media/layout-image.png"
	dir, in := deleteSlideFixture(t, []Slide{{}, {}}, func(p *opc.Package) {
		p.Set(layout, []byte(`<p:sldLayout xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main"><p:cSld><p:spTree/></p:cSld></p:sldLayout>`))
		if err := p.EnsureContentType(layout, "application/vnd.openxmlformats-officedocument.presentationml.slideLayout+xml"); err != nil {
			t.Fatal(err)
		}
		var imageData bytes.Buffer
		if err := png.Encode(&imageData, image.NewRGBA(image.Rect(0, 0, 4, 4))); err != nil {
			t.Fatal(err)
		}
		p.Set(media, imageData.Bytes())
		if err := p.EnsureContentType(media, "image/png"); err != nil {
			t.Fatal(err)
		}
		if _, err := p.AddRel(layout, rns+"/image", "../media/layout-image.png", false); err != nil {
			t.Fatal(err)
		}
		slides, err := office.PresentationSlides(p)
		if err != nil {
			t.Fatal(err)
		}
		if err := removeRels(p, slides[1], func(r *etree.Element) bool { return r.SelectAttrValue("Type", "") == rns+"/slideLayout" }); err != nil {
			t.Fatal(err)
		}
		if _, err := p.AddRel(slides[1], rns+"/slideLayout", "../slideLayouts/private.xml", false); err != nil {
			t.Fatal(err)
		}
	})
	p := deleteSlide(t, dir, in, 2)
	if !p.Has(layout) || !p.Has(media) {
		t.Fatal("retained layout or its image removed")
	}
}

func TestDeleteSlideRemovesExclusiveComments(t *testing.T) {
	const comments = "ppt/comments/comment1.xml"
	dir, in := deleteSlideFixture(t, []Slide{{}, {}}, func(p *opc.Package) {
		p.Set(comments, []byte(`<p:cmLst xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main"><p:cm authorId="0" idx="1"><p:pos x="0" y="0"/><p:text>PRIVATE-COMMENT</p:text></p:cm></p:cmLst>`))
		if err := p.EnsureContentType(comments, "application/vnd.openxmlformats-officedocument.presentationml.comments+xml"); err != nil {
			t.Fatal(err)
		}
		slides, err := office.PresentationSlides(p)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := p.AddRel(slides[1], rns+"/comments", "../comments/comment1.xml", false); err != nil {
			t.Fatal(err)
		}
	})
	p := deleteSlide(t, dir, in, 2)
	if p.Has(comments) {
		t.Fatal("deleted slide comments remain")
	}
}
