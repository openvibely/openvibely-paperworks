package wordedit

import (
	"encoding/xml"
	"github.com/beevik/etree"
	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"os"
	"strconv"
	"strings"
	"testing"
)

// UNRUN source corrections: retain original fixtures and inspect exact nodes.
func TestAccessibilityAndMetadataAreScoped(t *testing.T) {
	referenceWordCaller(t)
	out := mustEdit(t, base(t), map[string]any{"operation": "accessibility", "header_tables": []any{0.0}})
	body := office.Child(referenceXML(t, out, "word/document.xml"), nsW, "body")
	tables := office.Children(body, nsW, "tbl")
	if len(tables) == 0 {
		t.Fatal("missing first table")
	}
	rows := office.Children(tables[0], nsW, "tr")
	if len(rows) == 0 || office.Child(office.Child(rows[0], nsW, "trPr"), nsW, "tblHeader") == nil {
		t.Fatal("first table's first row lacks header property")
	}
	creator := office.Child(referenceXML(t, out, "docProps/core.xml"), nsDC, "creator")
	if creator == nil {
		t.Fatal("missing author")
	}
	referenceEqual(t, creator.Text(), "Private author")
	out = mustEdit(t, out, map[string]any{"operation": "metadata", "clear": []any{"author"}})
	creator = office.Child(referenceXML(t, out, "docProps/core.xml"), nsDC, "creator")
	// reference DOCX library returns an empty string when the property is absent.
	author := ""
	if creator != nil {
		author = creator.Text()
	}
	referenceEqual(t, author, "")
	referenceEqual(t, paras(t, out)[0], "The old price is $10.")
}

func TestGoogleTitleRemovesTitleBordersOnly(t *testing.T) {
	referenceWordCaller(t)
	src := referenceRewrite(t, base(t), func(p *opc.Package) {
		referenceSetXML(t, p, "word/document.xml", func(root *etree.Element) {
			body := office.Child(root, nsW, "body")
			first := office.Children(body, nsW, "p")[0]
			props := etree.NewElement("w:pPr")
			props.CreateElement("w:pBdr")
			first.InsertChildAt(0, props)
			title := etree.NewElement("w:p")
			titleProps := title.CreateElement("w:pPr")
			titleProps.CreateElement("w:pStyle").CreateAttr("w:val", "Title")
			titleProps.CreateElement("w:pBdr")
			title.CreateElement("w:r").CreateElement("w:t").SetText("Report")
			body.InsertChildAt(office.Child(body, nsW, "sectPr").Index(), title)
		})
		referenceSetXML(t, p, "word/styles.xml", func(root *etree.Element) {
			for _, e := range office.Children(root, nsW, "style") {
				if office.AttrV(e, nsW, "styleId") == "Title" {
					props := office.Child(e, nsW, "pPr")
					if props == nil {
						props = e.CreateElement("w:pPr")
					}
					props.CreateElement("w:pBdr")
					return
				}
			}
			t.Fatal("fixture lacks Title style")
		})
	})
	out := mustEdit(t, src, map[string]any{"operation": "google-title"})
	body := office.Child(referenceXML(t, out, "word/document.xml"), nsW, "body")
	paragraphs := office.Children(body, nsW, "p")
	if len(paragraphs) < 2 {
		t.Fatal("missing body/title paragraphs")
	}
	foundTitle := false
	for _, style := range office.Children(referenceXML(t, out, "word/styles.xml"), nsW, "style") {
		if office.AttrV(style, nsW, "styleId") == "Title" {
			foundTitle = true
			if office.Child(office.Child(style, nsW, "pPr"), nsW, "pBdr") != nil {
				t.Fatal("Title style border remains")
			}
			break
		}
	}
	if !foundTitle {
		t.Fatal("missing Title style")
	}
	if office.Child(office.Child(paragraphs[len(paragraphs)-1], nsW, "pPr"), nsW, "pBdr") != nil {
		t.Fatal("title paragraph border remains")
	}
	if office.Child(office.Child(paragraphs[0], nsW, "pPr"), nsW, "pBdr") == nil {
		t.Fatal("original body border removed")
	}
}

func TestExistingOutputIsNotOverwritten(t *testing.T) {
	referenceWordCaller(t)
	src := base(t)
	before, e := os.ReadFile(src)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		after, e := os.ReadFile(src)
		if e != nil {
			t.Fatal(e)
		}
		referenceEqual(t, after, before)
	})
	if _, err := Edit(src, src, map[string]any{"operation": "metadata", "clear": []any{"author"}}, ""); err == nil {
		t.Fatal("expected refusal")
	}
}

func TestCommentUpdateAndRemoval(t *testing.T) {
	referenceWordCaller(t)
	out := mustEdit(t, base(t), map[string]any{"operation": "comment", "text": "The old price is $10.", "comment": "Old review", "author": "Reviewer"})
	// Reference reads the allocated ID from the first comment before editing it.
	comments := office.Children(referenceXML(t, out, "word/comments.xml"), nsW, "comment")
	if len(comments) == 0 {
		t.Fatal("missing first comment")
	}
	ident, err := strconv.Atoi(office.AttrV(comments[0], nsW, "id"))
	if err != nil {
		t.Fatal(err)
	}
	out = mustEdit(t, out, map[string]any{"operation": "comment-edit", "id": float64(ident), "text": "Corrected review"})
	comments = office.Children(referenceXML(t, out, "word/comments.xml"), nsW, "comment")
	if len(comments) == 0 {
		t.Fatal("missing updated comment")
	}
	referenceEqual(t, len(office.Children(comments[0], nsW, "p")), 1)
	for _, tag := range []string{"tab", "br", "cr"} {
		referenceEqual(t, len(office.All(comments[0], nsW, tag)), 0)
	}
	referenceEqual(t, referenceCommentText(comments[0]), "Corrected review")
	out = mustEdit(t, out, map[string]any{"operation": "comments-remove"})
	pkg, err := opc.Open(out)
	if err != nil {
		t.Fatal(err)
	}
	if pkg.Has("word/comments.xml") {
		t.Fatal("comments part remains")
	}
	if paras(t, out)[0] != "The old price is $10." {
		t.Fatal("text changed")
	}
	referenceEqual(t, len(office.All(referenceXML(t, out, "word/document.xml"), nsW, "commentRangeStart")), 0)
	// The reference removes markers, not the empty reference run.
	first := office.Children(office.Child(referenceXML(t, out, "word/document.xml"), nsW, "body"), nsW, "p")[0]
	runs := office.Children(first, nsW, "r")
	referenceEqual(t, len(runs), 3)
	referenceEqual(t, len(runs[2].ChildElements()), 0)
	absent(t, part(t, out, "word/document.xml"), "commentRange")
}

func TestBookmarksAndReferences(t *testing.T) {
	referenceWordCaller(t)
	out := mustEdit(t, base(t), map[string]any{"operation": "bookmark", "paragraph": 0.0, "name": "summary"})
	out2 := mustEdit(t, out, map[string]any{"operation": "reference", "name": "summary", "text": "Back to summary"})
	ps := paras(t, out2)
	if ps[len(ps)-1] != "Back to summary" {
		t.Fatalf("%q", ps)
	}
	body := office.Child(referenceXML(t, out2, "word/document.xml"), nsW, "body")
	paragraphs := office.Children(body, nsW, "p")
	if len(paragraphs) == 0 {
		t.Fatal("missing body paragraphs")
	}
	links := office.Children(paragraphs[len(paragraphs)-1], nsW, "hyperlink")
	if len(links) == 0 {
		t.Fatal("last paragraph lacks hyperlink")
	}
	referenceEqual(t, office.AttrV(links[0], nsW, "anchor"), "summary")
	wantErr(t, out2, map[string]any{"operation": "bookmark", "paragraph": 0.0, "name": "summary"}, ".")
	wantErr(t, out2, map[string]any{"operation": "reference", "name": "missing"}, ".")
}

func TestCaptionAndTocAreFields(t *testing.T) {
	referenceWordCaller(t)
	out := mustEdit(t, referenceOriginalWordFixture(t, "base"), map[string]any{"operation": "caption", "label": "Figure", "text": "Revenue", "bookmark": "revenue"})
	out = mustEdit(t, out, map[string]any{"operation": "toc", "levels": "1-3"})
	root := referenceXML(t, out, "word/document.xml")
	var fields, codes, chars []string
	for _, e := range office.All(root, nsW, "fldSimple") {
		fields = append(fields, office.AttrV(e, nsW, "instr"))
	}
	referenceEqual(t, fields, []string{`SEQ Figure \* ARABIC`})
	for _, e := range office.All(root, nsW, "instrText") {
		codes = append(codes, strings.TrimSpace(e.Text()))
	}
	referenceEqual(t, codes, []string{`TOC \o "1-3" \h \z \u`})
	for _, e := range office.All(root, nsW, "fldChar") {
		chars = append(chars, office.AttrV(e, nsW, "fldCharType"))
	}
	referenceEqual(t, chars, []string{"begin", "separate", "end"})
	settings := referenceXML(t, out, "word/settings.xml")
	referenceEqual(t, office.AttrV(office.Child(settings, nsW, "updateFields"), nsW, "val"), "true")
	if len(office.All(root, nsW, "bookmarkStart")) == 0 {
		t.Fatal("missing bookmark")
	}
}

// Batch25: original fixture and ordered control assertions compared with pinned Reference.
func TestContentControlFillRetainsPropertiesAndStyle(t *testing.T) {
	referenceWordCaller(t)
	out := mustEdit(t, referenceOriginalWordFixture(t, "base"), map[string]any{"operation": "control", "paragraph": 0.0, "tag": "customer"})
	out = mustEdit(t, out, map[string]any{"operation": "control-fill", "tag": "customer", "text": "Ada"})
	control := office.All(referenceXML(t, out, "word/document.xml"), nsW, "sdt")[0]
	var textNodes []string
	for _, node := range office.All(control, nsW, "t") {
		textNodes = append(textNodes, node.Text())
	}
	referenceEqual(t, textNodes, []string{"Ada"})
	referenceEqual(t, len(office.All(control, nsW, "b")), 1)
	referenceEqual(t, office.AttrV(office.All(control, nsW, "tag")[0], nsW, "val"), "customer")
	wantErr(t, out, map[string]any{"operation": "control-fill", "tag": "unknown", "text": "Bad"}, ".")
}

// UNRUN: exact empty-control content and direct paragraph scope.
func TestEmptyControlsFillInlineAndBlock(t *testing.T) {
	referenceWordCaller(t)
	sdt := `<w:sdt><w:sdtPr><w:text/><w:tag w:val="client"/></w:sdtPr><w:sdtContent></w:sdtContent></w:sdt>`
	for _, inline := range []bool{false, true} {
		body := `<w:p>` + run("Client: ") + `</w:p>` + sdt
		if inline {
			body = `<w:p>` + run("Client: ") + sdt + `</w:p>`
		}
		out := mustEdit(t, mk(t, "empty", body, nil), map[string]any{"operation": "control-fill", "tag": "client", "text": "Ada"})
		root := referenceXML(t, out, "word/document.xml")
		content := office.All(root, nsW, "sdtContent")[0]
		referenceEqual(t, referenceTexts(content), "Ada")
		children := content.ChildElements()
		if len(children) == 0 {
			t.Fatal("empty filled control")
		}
		want := "p"
		if inline {
			want = "r"
		}
		referenceEqual(t, children[0].NamespaceURI(), nsW)
		referenceEqual(t, children[0].Tag, want)
		// reference DOCX library Paragraph.text omits inline content controls. This
		// fixture has one ordinary text run; inspect that direct run only.
		paragraph := office.Children(office.Child(root, nsW, "body"), nsW, "p")[0]
		var ordinary strings.Builder
		for _, r := range office.Children(paragraph, nsW, "r") {
			ordinary.WriteString(referenceTexts(r))
		}
		referenceEqual(t, ordinary.String(), "Client: ")
	}
}

// Batch25: original fixture and ordered control assertions compared with pinned Reference.
func TestRefillMultilineAndTabbedControlKeepsStyle(t *testing.T) {
	referenceWordCaller(t)
	src := mustEdit(t, base(t), map[string]any{"operation": "control", "paragraph": 0.0, "tag": "client"})
	first := mustEdit(t, src, map[string]any{"operation": "control-fill", "tag": "client", "text": "Ada\nLovelace\tClient"})
	second := mustEdit(t, first, map[string]any{"operation": "control-fill", "tag": "client", "text": "Grace\nHopper\tClient"})
	contentNode := office.All(referenceXML(t, second, "word/document.xml"), nsW, "sdtContent")[0]
	if got := referenceParagraphText(contentNode.ChildElements()[0]); got != "Grace\nHopper\tClient" {
		t.Fatalf("%q", got)
	}
	children := contentNode.ChildElements()
	if len(children) == 0 {
		t.Fatal("missing refill paragraph")
	}
	referenceRequireDirectRunProperty(t, children[0], 0, "b")
	referenceEqual(t, len(office.All(contentNode, nsW, "br")), 1)
	referenceEqual(t, len(office.All(contentNode, nsW, "tab")), 1)
}

// Batch25: original fixture and ordered control assertions compared with pinned Reference.
func TestWrapRepeatedPlaceholdersAndFillAll(t *testing.T) {
	referenceWordCaller(t)
	src := referenceOriginalWordFixture(t, "wrap-placeholders")
	wrapped := mustEdit(t, src, map[string]any{"operation": "control", "marker": "{{client}}", "tag": "client"})
	pkg, err := opc.Open(wrapped)
	if err != nil {
		t.Fatal(err)
	}
	var controls []*etree.Element
	for _, root := range referenceElementParts(t, pkg) {
		controls = append(controls, office.All(root, nsW, "sdt")...)
	}
	referenceEqual(t, len(controls), 5)
	ids := map[string]bool{}
	for _, control := range controls {
		id := office.Child(office.Child(control, nsW, "sdtPr"), nsW, "id")
		if id == nil {
			t.Fatal("missing direct control ID")
		}
		ids[office.AttrV(id, nsW, "val")] = true
	}
	referenceEqual(t, len(ids), 5)
	// Regression: IDs follow relationship traversal (body, header, footer),
	// not sorted ZIP names (which would visit footer before header).
	for _, story := range []struct{ kind, id string }{{"header", "4"}, {"footer", "5"}} {
		_, root := referenceFirstSectionStory(t, wrapped, story.kind)
		control := office.All(root, nsW, "sdt")[0]
		referenceEqual(t, office.AttrV(office.Child(office.Child(control, nsW, "sdtPr"), nsW, "id"), nsW, "val"), story.id)
	}
	var runs []string
	ctl := office.All(referenceXML(t, wrapped, "word/document.xml"), nsW, "sdtContent")[0]
	for _, r := range ctl.ChildElements() {
		runs = append(runs, referenceTexts(r))
	}
	referenceEqual(t, runs, []string{"{{cli", "ent}}"})
	children := ctl.ChildElements()
	if len(children) != 2 {
		t.Fatalf("placeholder children: got %d; want 2", len(children))
	}
	for i, property := range []string{"b", "i"} {
		rPr := office.Child(children[i], nsW, "rPr")
		if rPr == nil || office.Child(rPr, nsW, property) == nil {
			t.Fatalf("placeholder child %d missing direct %s property", i, property)
		}
	}
	out := mustEdit(t, wrapped, map[string]any{"operation": "control-fill", "tag": "client", "text": "Ada"})
	bodyNode := office.Child(referenceXML(t, out, "word/document.xml"), nsW, "body")
	referenceEqual(t, referenceTexts(office.Children(bodyNode, nsW, "p")[0]), "Dear Ada / Ada!")
	referenceEqual(t, referenceTexts(office.Children(bodyNode, nsW, "tbl")[0]), "Table Ada")
	referenceEqual(t, referenceTexts(referenceXML(t, out, "word/header1.xml")), "Header Ada")
	referenceEqual(t, referenceTexts(referenceXML(t, out, "word/footer1.xml")), "Footer Ada")
}

// UNRUN: original field order, key-presence checks, and independent fixture.
func TestInspectCombinesSplitFieldInstructions(t *testing.T) {
	referenceWordCaller(t)
	field := referenceSplitField
	body := `<w:p>` + field([]string{" RE", "F summary "}, true) + `<w:fldSimple w:instr=" PAGE " w:dirty="true"><w:r><w:t xml:space="preserve">1</w:t></w:r></w:fldSimple>` + field([]string{" SE", "Q Figure "}, true) + `</w:p><w:p>` + field([]string{" RE", "F unfinished"}, false) + `</w:p>`
	src := referenceReferenceHeader(t, mk(t, "split", body, nil), "")
	src = referenceRewrite(t, src, func(p *opc.Package) {
		p.Set("word/header1.xml", []byte(`<w:hdr `+wNS+`><w:p><w:pPr><w:pStyle w:val="Header"/></w:pPr>`+field([]string{" NUM", "PAGES "}, true)+`</w:p></w:hdr>`))
	})
	original, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	var bodyFields, hdr []string
	var bodyReports []map[string]any
	incomplete := 0
	for _, f := range report(t, src)["fields"].([]any) {
		m := f.(map[string]any)
		if m["part"] == "word/document.xml" {
			bodyReports = append(bodyReports, m)
			bodyFields = append(bodyFields, m["instruction"].(string))
		}
		if strings.Contains(m["part"].(string), "header") {
			hdr = append(hdr, m["instruction"].(string))
		}
		if m["incomplete"] == true {
			incomplete++
		}
	}
	referenceEqual(t, bodyFields, []string{"REF summary", "PAGE", "SEQ Figure", "REF unfinished"})
	referenceEqual(t, bodyReports[len(bodyReports)-1]["incomplete"], true)
	for _, m := range bodyReports[:len(bodyReports)-1] {
		_, present := m["incomplete"]
		referenceEqual(t, present, false)
	}
	referenceEqual(t, hdr, []string{"NUMPAGES"})
	after, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	referenceEqual(t, after, original)
	// Retained supplemental check, after the five original assertions.
	referenceEqual(t, incomplete, 1)
}

// UNRUN: exact text-node assertions; original fixture fidelity remains open.
// UNRUN: clone the operation-created block control and preserve the base table.
func TestRepeatedControlTagFillsAllStoriesAndBarriers(t *testing.T) {
	referenceWordCaller(t)
	template := mustEdit(t, base(t), map[string]any{"operation": "control", "paragraph": 0.0, "tag": "customer"})
	// Start from the original saved package graph, but insert controls copied
	// from the Go operation above; fixture data cannot replace its result.
	shell := referenceOriginalWordFixture(t, "repeat-stories")
	source := referenceRewrite(t, shell, func(pkg *opc.Package) {
		produced := referenceXML(t, template, "word/document.xml").Copy()
		body := office.Child(produced, nsW, "body")
		section := office.Child(body, nsW, "sectPr")
		originalSection := office.Child(office.Child(referenceXML(t, shell, "word/document.xml"), nsW, "body"), nsW, "sectPr")
		for i, ref := range originalSection.ChildElements() {
			if ref.Tag == "headerReference" || ref.Tag == "footerReference" {
				section.InsertChildAt(i, ref.Copy())
			}
		}
		control := office.All(produced, nsW, "sdt")[0].Copy()
		control.CreateAttr("xmlns:w", nsW)
		content := office.Child(control, nsW, "sdtContent")
		block := content.ChildElements()[0]
		content.RemoveChild(block)
		for _, child := range block.ChildElements() {
			content.AddChild(child.Copy())
		}
		insert := func(paragraph *etree.Element) {
			for _, child := range paragraph.ChildElements() {
				if child.Tag != "pPr" || child.NamespaceURI() != nsW {
					paragraph.RemoveChild(child)
				}
			}
			before := paragraph.CreateElement("w:r").CreateElement("w:t")
			before.CreateAttr("xml:space", "preserve")
			before.SetText("Before ")
			paragraph.AddChild(control.Copy())
			after := paragraph.CreateElement("w:r").CreateElement("w:t")
			after.CreateAttr("xml:space", "preserve")
			after.SetText(" after")
		}
		table := office.Children(body, nsW, "tbl")[0]
		cell := office.Children(office.Children(table, nsW, "tr")[0], nsW, "tc")[0]
		insert(office.Children(cell, nsW, "p")[0])
		doc := etree.NewDocument()
		doc.SetRoot(produced)
		if err := pkg.SetXML("word/document.xml", doc); err != nil {
			t.Fatal(err)
		}
		for _, kind := range []string{"header", "footer"} {
			name, _ := referenceFirstSectionStory(t, shell, kind)
			referenceSetXML(t, pkg, name, func(root *etree.Element) { insert(office.Children(root, nsW, "p")[0]) })
		}
	})
	original, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	out := mustEdit(t, source, map[string]any{"operation": "control-fill", "tag": "customer", "text": "Ada"})
	var controls []*etree.Element
	pkg, err := opc.Open(out)
	if err != nil {
		t.Fatal(err)
	}
	// UNRUN: enumerate reachable element-bearing parts, as Reference does.
	for _, root := range referenceElementParts(t, pkg) {
		for _, c := range office.All(root, nsW, "sdt") {
			controls = append(controls, c)
		}
	}
	referenceEqual(t, len(controls), 4)
	for _, c := range controls {
		var values []string
		for _, node := range office.All(c, nsW, "t") {
			values = append(values, node.Text())
		}
		referenceEqual(t, values, []string{"Ada"})
		if len(office.All(c, nsW, "b")) == 0 {
			t.Fatal("control lost bold property")
		}
		props := office.Child(c, nsW, "sdtPr")
		tag := office.Child(props, nsW, "tag")
		if tag == nil {
			t.Fatal("control missing direct tag")
		}
		referenceEqual(t, office.AttrV(tag, nsW, "val"), "customer")
	}
	for _, kind := range []string{"header", "footer"} {
		_, story := referenceFirstSectionStory(t, out, kind)
		paragraph := office.Children(story, nsW, "p")[0]
		referenceEqual(t, referenceTexts(paragraph), "Before Ada after")
	}
	body := office.Child(referenceXML(t, out, "word/document.xml"), nsW, "body")
	table := office.Children(body, nsW, "tbl")[0]
	cell := office.Children(office.Children(table, nsW, "tr")[0], nsW, "tc")[0]
	referenceEqual(t, referenceTexts(office.Children(cell, nsW, "p")[0]), "Before Ada after")
	after, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	referenceEqual(t, after, original)
	headerName, _ := referenceFirstSectionStory(t, source, "header")
	for _, barrier := range []string{"lock", "dataBinding", "richText"} {
		bad := referenceRewrite(t, source, func(pkg *opc.Package) {
			referenceSetXML(t, pkg, headerName, func(root *etree.Element) {
				props := office.All(root, nsW, "sdtPr")[0]
				if barrier == "richText" {
					props.RemoveChild(office.Child(props, nsW, "text"))
				}
				props.CreateElement("w:" + barrier)
			})
		})
		wantErr(t, bad, map[string]any{"operation": "control-fill", "tag": "customer", "text": "Ada"}, ".")
	}
}

func TestEditingRestrictionNotEncryption(t *testing.T) {
	referenceWordCaller(t)
	src := base(t)
	originalDocument := part(t, src, "word/document.xml")
	out := mustEdit(t, src, map[string]any{"operation": "protection", "mode": "comments"})
	settings := part(t, out, "word/settings.xml")
	// UNRUN source correction: Reference reads the direct settings child.
	protection := office.Child(referenceXML(t, out, "word/settings.xml"), nsW, "documentProtection")
	if protection == nil {
		t.Fatal("missing direct documentProtection")
	}
	referenceEqual(t, protection.SelectAttrValue("w:edit", ""), "comments")
	settingsChildren := referenceXML(t, out, "word/settings.xml").ChildElements()
	referenceEqual(t, settingsChildren[len(settingsChildren)-1].Tag, "documentProtection")
	contains(t, settings, `w:enforcement="1"`)
	if strings.Count(settings, "<w:documentProtection") != 1 {
		t.Fatal("expected exactly one restriction")
	}
	if part(t, out, "word/document.xml") != originalDocument {
		t.Fatal("restriction changed document content")
	}
	out = mustEdit(t, out, map[string]any{"operation": "protection", "mode": "none"})
	if office.Child(referenceXML(t, out, "word/settings.xml"), nsW, "documentProtection") != nil {
		t.Fatal("direct documentProtection remains after removal")
	}
	absent(t, part(t, out, "word/settings.xml"), "documentProtection")
	if part(t, out, "word/document.xml") != originalDocument || part(t, src, "word/document.xml") != originalDocument {
		t.Fatal("restriction removal changed document content or source")
	}
}

func referenceSplitField(code []string, complete bool) string {
	s := `<w:r><w:fldChar w:fldCharType="begin"/></w:r>`
	for _, c := range code {
		s += `<w:r><w:instrText xml:space="preserve">` + referenceSplitFieldEscape(c) + `</w:instrText></w:r>`
	}
	if complete {
		s += `<w:r><w:fldChar w:fldCharType="separate"/></w:r><w:r><w:t>Cached result</w:t></w:r><w:r><w:fldChar w:fldCharType="end"/></w:r>`
	}
	return s
}

func referenceSplitFieldEscape(s string) string {
	var b strings.Builder
	if err := xml.EscapeText(&b, []byte(s)); err != nil {
		panic(err)
	}
	return b.String()
}
