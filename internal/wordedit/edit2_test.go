package wordedit

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/beevik/etree"
	"github.com/openvibely/openvibely-paperworks/internal/office"
)

const headerRel = `<Relationship Id="rIdH" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/header" Target="header1.xml"/>`

func withHeader(hdr string) map[string]string {
	return map[string]string{
		"word/header1.xml":             `<w:hdr ` + wNS + `>` + hdr + `</w:hdr>`,
		"word/_rels/document.xml.rels": fmt.Sprintf(rels, headerRel),
	}
}

func heading(level int, text string) string {
	return fmt.Sprintf(`<w:p><w:pPr><w:pStyle w:val="Heading%d"/></w:pPr>%s</w:p>`, level, run(text))
}

func TestScopedHeadingNormalizationPreservesNumbering(t *testing.T) {
	referenceWordCaller(t)
	src := referenceOriginalWordFixture(t, "styles-sections")
	out := mustEdit(t, src, map[string]any{"operation": "normalize", "styles": []any{"Heading 1"}, "heading_spacing": []any{12.0, 6.0}})
	doc := part(t, out, "word/document.xml")
	contains(t, doc, `w:before="240"`)
	contains(t, doc, `w:after="120"`)
	contains(t, doc, `<w:i/>`)
	root := referenceXML(t, out, "word/document.xml")
	ps := office.Children(office.Child(root, nsW, "body"), nsW, "p")
	// Scope Reference's paragraph-format and run assertions exactly.
	spacing := office.Child(office.Child(ps[0], nsW, "pPr"), nsW, "spacing")
	if spacing == nil {
		t.Fatal("heading has no direct spacing")
	}
	referenceEqual(t, office.AttrV(spacing, nsW, "before"), "240")
	referenceEqual(t, office.AttrV(spacing, nsW, "after"), "120")
	styleID := office.AttrV(office.Child(office.Child(ps[1], nsW, "pPr"), nsW, "pStyle"), nsW, "val")
	var styleName string
	for _, style := range office.Children(referenceXML(t, out, "word/styles.xml"), nsW, "style") {
		if office.AttrV(style, nsW, "styleId") == styleID {
			styleName = office.AttrV(office.Child(style, nsW, "name"), nsW, "val")
		}
	}
	referenceEqual(t, styleName, "List Number")
	referenceRequireDirectRunProperty(t, ps[1], 0, "i")
	sections := office.All(root, nsW, "sectPr")
	referenceEqual(t, len(sections), 2)
	referenceEqual(t, len(office.All(sections[1], nsW, "headerReference")), 0)
	if len(office.Children(sections[0], nsW, "headerReference")) == 0 {
		t.Fatal("first section lacks linked header")
	}
	headerPart, _ := referenceFirstSectionStory(t, out, "header")
	referenceEqual(t, referenceStoryTexts(t, out, headerPart, false)[0], "Linked header")
	if part(t, src, "word/numbering.xml") != part(t, out, "word/numbering.xml") {
		t.Fatal("numbering changed")
	}
}

func TestTemplateSlotFillPreservesUnrelatedParts(t *testing.T) {
	referenceWordCaller(t)
	src := referenceOriginalWordFixture(t, "opaque-template")
	before, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	controlled := mustEdit(t, src, map[string]any{"operation": "control", "marker": "{{NAME}}", "tag": "customer"})
	controlledBefore, err := os.ReadFile(controlled)
	if err != nil {
		t.Fatal(err)
	}
	filled := mustEdit(t, controlled, map[string]any{"operation": "control-fill", "tag": "customer", "text": "Ada"})
	for _, out := range []string{controlled, filled} {
		preserved, err := office.VerifyPreservation(src, out, []string{"word/document.xml"})
		if err != nil || preserved["preservation_passed"] != true {
			t.Fatalf("preservation %v %v", preserved, err)
		}
		contains(t, part(t, out, "word/document.xml"), `w:tag w:val="customer"`)
	}
	after, err := os.ReadFile(src)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("source changed", err)
	}

	r, err := office.VerifyPreservation(controlled, filled, []string{"word/document.xml"})
	if err != nil || r["preservation_passed"] != true {
		t.Fatalf("preservation %v %v", r, err)
	}
	controlledAfter, err := os.ReadFile(controlled)
	if err != nil || !bytes.Equal(controlledBefore, controlledAfter) {
		t.Fatal("control-fill changed template", err)
	}
	if part(t, filled, "word/opaque.bin") != "opaque-template-data" {
		t.Fatal("opaque part changed")
	}
	// UNRUN: Reference joins every descendant w:t, including control content.
	contains(t, referenceTexts(referenceXML(t, filled, "word/document.xml")), "Customer Ada remains styled.")
}

func TestStaticNavigationResolvesAllLinks(t *testing.T) {
	referenceWordCaller(t)
	src := referenceOriginalWordFixture(t, "navigation-headings")
	out := mustEdit(t, src, map[string]any{"operation": "navigation"})
	root := referenceXML(t, out, "word/document.xml")
	if got := paras(t, out)[0]; got != "Contents Bottom" {
		t.Fatalf("first paragraph %q", got)
	}
	// UNRUN: count hyperlink elements, including those lacking an anchor.
	// Reference's Element.get returns None for missing attributes; retain that
	// distinction from an explicitly empty attribute in the bookmark set.
	type optionalAttribute struct {
		value   string
		present bool
	}
	links := office.All(root, nsW, "hyperlink")
	referenceEqual(t, len(links), 6)
	marks := map[optionalAttribute]bool{}
	for _, bookmark := range office.All(root, nsW, "bookmarkStart") {
		value, present := office.Attr(bookmark, nsW, "name")
		marks[optionalAttribute{value, present}] = true
	}
	for _, link := range links {
		value, present := office.Attr(link, nsW, "anchor")
		if !marks[optionalAttribute{value, present}] {
			t.Fatalf("dangling link anchor %q (present %v)", value, present)
		}
	}
	n := 0
	for _, p := range paras(t, out) {
		if p == "Back to contents" {
			n++
		}
	}
	if n != 2 {
		t.Fatalf("back links %d", n)
	}
}

func attrValues(doc, attr string) []string {
	var out []string
	for _, chunk := range strings.Split(doc, attr+`="`)[1:] {
		out = append(out, chunk[:strings.Index(chunk, `"`)])
	}
	return out
}

func TestMarkersInHeaderAndBodyPreserveAdjacentRuns(t *testing.T) {
	referenceWordCaller(t)
	src := referenceOriginalWordFixture(t, "markers-header")
	src = mustEdit(t, src, map[string]any{"operation": "note", "marker": "[[NOTE]]", "text": "Footnote"})
	src = mustEdit(t, src, map[string]any{"operation": "control", "marker": "{{CUSTOMER}}", "tag": "customer"})
	wantErr(t, src, map[string]any{"operation": "control", "paragraph": 0.0, "tag": "customer"}, "already exists")
	out := mustEdit(t, src, map[string]any{"operation": "control-fill", "tag": "customer", "text": "Ada"})
	if got := paras(t, out)[0]; got != "Before  after" {
		t.Fatalf("%q", got)
	}
	doc := part(t, out, "word/document.xml")
	contains(t, doc, "<w:b/>")
	contains(t, doc, "<w:i/>")
	contains(t, doc, "w:footnoteReference")
	paragraph := office.Children(office.Child(referenceXML(t, out, "word/document.xml"), nsW, "body"), nsW, "p")[0]
	referenceRequireDirectRunProperty(t, paragraph, 0, "b")
	referenceRequireDirectRunProperty(t, paragraph, -1, "i")
	if got := referenceTexts(referenceXML(t, out, "word/header1.xml")); got != "Dear Ada, welcome" {
		t.Fatalf("header %q", got)
	}
	if len(office.All(paragraph, nsW, "footnoteReference")) == 0 {
		t.Fatal("first paragraph has no footnote reference")
	}
}

func TestCaptionNumberReferenceAtMarkerAndFreeze(t *testing.T) {
	referenceWordCaller(t)
	src := referenceOriginalWordFixture(t, "caption-marker")
	src = mustEdit(t, src, map[string]any{"operation": "caption", "label": "Table", "text": "Results", "bookmark": "results", "after_table": 0.0})
	// UNRUN: Reference checks the first table's immediate element sibling.
	body := office.Child(referenceXML(t, src, "word/document.xml"), nsW, "body")
	children := body.ChildElements()
	found := false
	for i, child := range children {
		if child.Tag != "tbl" || child.NamespaceURI() != nsW {
			continue
		}
		if i+1 >= len(children) || children[i+1].Tag != "p" || children[i+1].NamespaceURI() != nsW {
			t.Fatal("first table is not immediately followed by a paragraph")
		}
		found = true
		break
	}
	if !found {
		t.Fatal("missing first table")
	}
	src = mustEdit(t, src, map[string]any{"operation": "reference", "name": "results", "field": true, "marker": "[[REF]]"})
	src = mustEdit(t, src, map[string]any{"operation": "materialize"})
	out := mustEdit(t, src, map[string]any{"operation": "freeze-references"})
	if got := paras(t, out)[0]; got != "See 1 for results." {
		t.Fatalf("%q", got)
	}
	referenceEqual(t, len(office.All(referenceXML(t, out, "word/document.xml"), nsW, "fldSimple")), 1)
}

func TestTableGeometryHandlesMergedCells(t *testing.T) {
	referenceWordCaller(t)
	src := referenceOriginalWordFixture(t, "merged-table")
	before, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	out := mustEdit(t, src, map[string]any{"operation": "table-geometry", "table": 0.0, "widths_twips": []any{1000.0, 2000.0, 3000.0}})
	after, err := os.ReadFile(src)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("source changed", err)
	}
	doc := part(t, out, "word/document.xml")
	// UNRUN: preserve direct-node scope and ordered list equality.
	body := office.Child(referenceXML(t, out, "word/document.xml"), nsW, "body")
	tables := office.Children(body, nsW, "tbl")
	if len(tables) == 0 {
		t.Fatal("missing first table")
	}
	table := tables[0]
	width := office.Child(office.Child(table, nsW, "tblPr"), nsW, "tblW")
	if width == nil {
		t.Fatal("missing table width")
	}
	referenceEqual(t, office.AttrV(width, nsW, "w"), "6000")
	var columns []string
	for _, column := range office.Children(office.Child(table, nsW, "tblGrid"), nsW, "gridCol") {
		columns = append(columns, office.AttrV(column, nsW, "w"))
	}
	referenceEqual(t, columns, []string{"1000", "2000", "3000"})
	rows := office.Children(table, nsW, "tr")
	if len(rows) == 0 {
		t.Fatal("missing first row")
	}
	var cells []string
	for _, cell := range office.Children(rows[0], nsW, "tc") {
		width := office.Child(office.Child(cell, nsW, "tcPr"), nsW, "tcW")
		if width == nil {
			t.Fatal("missing first-row cell width")
		}
		cells = append(cells, office.AttrV(width, nsW, "w"))
	}
	referenceEqual(t, cells, []string{"3000", "3000"})
	contains(t, doc, `<w:gridSpan w:val="2"/>`)
	contains(t, doc, "Merged")
	last := doc[strings.LastIndex(doc, "<w:tr>"):]
	for _, width := range []string{"1000", "2000", "3000"} {
		contains(t, last, `w:w="`+width+`"`)
	}
	row := between(doc, "<w:tr>", "</w:tr><w:tr>")
	if strings.Count(row, `w:w="3000"`) != 2 || strings.Count(row, "<w:tcW") != 2 {
		t.Fatalf("merged widths: %s", row)
	}
}

func complexField(instr, cached string) string {
	chars := []rune(instr)
	cut := min(3, len(chars))
	return `<w:r><w:fldChar w:fldCharType="begin"/></w:r><w:r><w:instrText xml:space="preserve">` + referenceSplitFieldEscape(string(chars[:cut])) + `</w:instrText></w:r><w:r><w:instrText xml:space="preserve">` + referenceSplitFieldEscape(string(chars[cut:])) + `</w:instrText></w:r><w:r><w:fldChar w:fldCharType="separate"/></w:r>` + referenceComplexCachedRun(cached) + `<w:r><w:fldChar w:fldCharType="end"/></w:r>`
}

// Match reference DOCX library Run.text assignment followed by Run.bold = True.
func referenceComplexCachedRun(text string) string {
	var out strings.Builder
	out.WriteString(`<w:r><w:rPr><w:b/></w:rPr>`)
	var chunk strings.Builder
	flush := func() {
		value := chunk.String()
		if value == "" {
			return
		}
		out.WriteString(`<w:t`)
		if strings.TrimSpace(value) != value {
			out.WriteString(` xml:space="preserve"`)
		}
		out.WriteString(`>` + referenceSplitFieldEscape(value) + `</w:t>`)
		chunk.Reset()
	}
	for _, c := range text {
		switch c {
		case '\t':
			flush()
			out.WriteString(`<w:tab/>`)
		case '\n', '\r':
			flush()
			out.WriteString(`<w:br/>`)
		default:
			chunk.WriteRune(c)
		}
	}
	flush()
	out.WriteString(`</w:r>`)
	return out.String()
}

func TestFreezeComplexCachedReferencePreservesStyle(t *testing.T) {
	referenceWordCaller(t)
	src := referenceOriginalWordFixture(t, "cached-field")
	before, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	out := mustEdit(t, src, map[string]any{"operation": "freeze-references"})
	if got := paras(t, out)[0]; got != "See 12 for details." {
		t.Fatalf("%q", got)
	}
	doc := part(t, out, "word/document.xml")
	paragraph := office.Children(office.Child(referenceXML(t, out, "word/document.xml"), nsW, "body"), nsW, "p")[0]
	referenceRequireDirectRunProperty(t, paragraph, 1, "b")
	for _, marker := range []string{"w:fldChar", "w:instrText", "PAGEREF"} {
		absent(t, doc, marker)
	}
	after, err := os.ReadFile(src)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("source changed", err)
	}
}

// UNRUN source port of reference DOCX library's direct Run.bold/italic truth assertion.
// A negative index selects from the end, matching Reference's runs[-1].
func referenceRequireDirectRunProperty(t *testing.T, paragraph *etree.Element, index int, property string) {
	t.Helper()
	runs := office.Children(paragraph, nsW, "r")
	if index < 0 {
		index += len(runs)
	}
	if index < 0 || index >= len(runs) {
		t.Fatalf("missing direct run %d (count %d)", index, len(runs))
	}
	prop := office.Child(office.Child(runs[index], nsW, "rPr"), nsW, property)
	if prop == nil {
		t.Fatalf("run %d lacks direct %s", index, property)
	}
	switch office.AttrV(prop, nsW, "val") {
	case "", "1", "true", "on":
	default:
		t.Fatalf("run %d has disabled/invalid %s: %q", index, property, office.AttrV(prop, nsW, "val"))
	}
}
