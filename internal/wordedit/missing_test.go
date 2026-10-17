package wordedit

// Direct ports of previously missing test_word_edit.py cases. Fixtures and
// outputs are constructed under t.TempDir; no generated documents are retained.
import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/beevik/etree"
	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
)

func referenceXML(t *testing.T, path, name string) *etree.Element {
	t.Helper()
	d := etree.NewDocument()
	if err := d.ReadFromString(part(t, path, name)); err != nil {
		t.Fatal(err)
	}
	return d.Root()
}
func referenceTexts(root *etree.Element) string {
	var b strings.Builder
	for _, e := range office.All(root, nsW, "t") {
		b.WriteString(e.Text())
	}
	return b.String()
}
func referenceEqual(t *testing.T, got, want any) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v; want %#v", got, want)
	}
}
func referenceRewrite(t *testing.T, src string, change func(*opc.Package)) string {
	t.Helper()
	p, e := opc.Open(src)
	if e != nil {
		t.Fatal(e)
	}
	change(p)
	raw, e := p.Bytes()
	if e != nil {
		t.Fatal(e)
	}
	out := filepath.Join(t.TempDir(), "input.docx")
	if e = os.WriteFile(out, raw, 0600); e != nil {
		t.Fatal(e)
	}
	return out
}
func referenceSetXML(t *testing.T, p *opc.Package, name string, change func(*etree.Element)) {
	t.Helper()
	d, e := p.XML(name)
	if e != nil {
		t.Fatal(e)
	}
	change(d.Root())
	b, e := d.WriteToBytes()
	if e != nil {
		t.Fatal(e)
	}
	p.Set(name, b)
}

func TestReferenceWordImportedCommentStateAndAlias(t *testing.T) {
	referenceWordCaller(t)
	src := mustEdit(t, base(t), map[string]any{"operation": "comment", "text": "The old price is $10.", "comment": "Review", "author": "Reviewer"})
	src = referenceRewrite(t, src, func(p *opc.Package) {
		referenceSetXML(t, p, "word/comments.xml", func(root *etree.Element) {
			var c *etree.Element
			for _, candidate := range office.Children(root, nsW, "comment") {
				if office.AttrV(candidate, nsW, "id") == "0" {
					c = candidate
					break
				}
			}
			if c == nil {
				t.Fatal("missing imported comment ID 0")
			}
			c.CreateAttr("w:done", "1")
			c.CreateAttr("w:initials", "RV")
		})
	})
	src = mustEdit(t, src, map[string]any{"operation": "control", "paragraph": 0.0, "tag": "client"})
	src = referenceRewrite(t, src, func(p *opc.Package) {
		referenceSetXML(t, p, "word/document.xml", func(root *etree.Element) {
			office.All(root, nsW, "sdtPr")[0].CreateElement("w:alias").CreateAttr("w:val", "Client name")
		})
	})
	before, _ := os.ReadFile(src)
	r := report(t, src)
	referenceEqual(t, r["controls"].([]any)[0].(map[string]any)["alias"], "Client name")
	c := r["comment_details"].([]any)[0].(map[string]any)
	referenceEqual(t, c["initials"], "RV")
	referenceEqual(t, c["resolved"], true)
	out := mustEdit(t, src, map[string]any{"operation": "comment-edit", "id": 0.0, "resolved": false})
	referenceEqual(t, report(t, out)["comment_details"].([]any)[0].(map[string]any)["resolved"], false)
	var reopened *etree.Element
	for _, c := range office.Children(referenceXML(t, out, "word/comments.xml"), nsW, "comment") {
		if office.AttrV(c, nsW, "id") == "0" {
			reopened = c
			break
		}
	}
	if reopened == nil {
		t.Fatal("missing reopened comment ID 0")
	}
	referenceEqual(t, office.AttrV(reopened, nsW, "done"), "0")
	after, _ := os.ReadFile(src)
	referenceEqual(t, after, before)
}
func TestReferenceWordMaterializeOnlySelectedFieldTypes(t *testing.T) {
	referenceWordCaller(t)
	// add_bookmark allocates ID 1. add_field uses word.node, which sets
	// xml:space even without edge whitespace; Document.add_paragraph does not.
	src := mk(t, "fields", `<w:p><w:bookmarkStart w:id="1" w:name="target"/><w:r><w:t>Target</w:t></w:r><w:bookmarkEnd w:id="1"/></w:p><w:p><w:fldSimple w:instr="SEQ Figure" w:dirty="true"><w:r><w:t xml:space="preserve">99</w:t></w:r></w:fldSimple><w:fldSimple w:instr="REF target" w:dirty="true"><w:r><w:t xml:space="preserve">Old target</w:t></w:r></w:fldSimple></w:p>`, nil)
	for _, tc := range []struct {
		only   string
		values []string
	}{{"SEQ", []string{"1", "Old target"}}, {"REF", []string{"99", "Target"}}} {
		t.Run(tc.only, func(t *testing.T) {
			out := mustEdit(t, src, map[string]any{"operation": "materialize", "only": []any{tc.only}})
			var got []string
			for _, f := range office.All(referenceXML(t, out, "word/document.xml"), nsW, "fldSimple") {
				got = append(got, referenceTexts(f))
			}
			referenceEqual(t, got, tc.values)
		})
	}
	wantErr(t, src, map[string]any{"operation": "materialize", "only": []any{"PAGE"}}, "only must select")
}
func TestReferenceWordCommentInspectionWhitespace(t *testing.T) {
	referenceWordCaller(t)
	// UNRUN: build Document.add_comment's input independently of Edit.
	src := mk(t, "comment", `<w:p><w:commentRangeStart w:id="0"/><w:r><w:t>Review these totals.</w:t></w:r><w:commentRangeEnd w:id="0"/><w:r><w:rPr><w:rStyle w:val="CommentReference"/></w:rPr><w:commentReference w:id="0"/></w:r></w:p>`, nil)
	src = referenceRewrite(t, src, func(p *opc.Package) {
		p.Set("word/comments.xml", []byte(`<w:comments
  xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main"
  xmlns:pic="http://schemas.openxmlformats.org/drawingml/2006/picture"
  xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"
  xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"
  xmlns:wp="http://schemas.openxmlformats.org/drawingml/2006/wordprocessingDrawing"
  xmlns:mc="http://schemas.openxmlformats.org/markup-compatibility/2006"
  xmlns:a14="http://schemas.microsoft.com/office/drawing/2010/main"
  xmlns:w14="http://schemas.microsoft.com/office/word/2010/wordml"
  xmlns:wpc="http://schemas.microsoft.com/office/word/2010/wordprocessingCanvas"
><w:comment w:id="0" w:author="Reviewer" w:initials="" w:date="`+time.Now().UTC().Format(time.RFC3339)+`"><w:p><w:pPr><w:pStyle w:val="CommentText"/></w:pPr><w:r><w:rPr><w:rStyle w:val="CommentReference"/></w:rPr><w:annotationRef/></w:r><w:r><w:t>Revenue: 10</w:t></w:r></w:p><w:p><w:pPr><w:pStyle w:val="CommentText"/></w:pPr><w:r><w:t>Costs: 20</w:t></w:r></w:p><w:p><w:pPr><w:pStyle w:val="CommentText"/></w:pPr><w:r><w:t>Details</w:t></w:r><w:r><w:tab/><w:t>A</w:t><w:br/><w:t>B</w:t><w:cr/><w:t>C</w:t></w:r></w:p></w:comment></w:comments>`))
		if _, err := p.AddRel("word/document.xml", relBase+"comments", "comments.xml", false); err != nil {
			t.Fatal(err)
		}
		if err := p.EnsureContentType("word/comments.xml", ctBase+"comments+xml"); err != nil {
			t.Fatal(err)
		}
	})
	before, _ := os.ReadFile(src)
	c := report(t, src)["comment_details"].([]any)[0].(map[string]any)
	referenceEqual(t, c["text"], "Revenue: 10\nCosts: 20\nDetails\tA\nB\nC")
	referenceEqual(t, c["anchored_text"].([]any)[0].(map[string]any)["text"], "Review these totals.")
	after, _ := os.ReadFile(src)
	referenceEqual(t, after, before)
}
func TestReferenceWordNotesPartsAndReferences(t *testing.T) {
	referenceWordCaller(t)
	src := referenceOriginalWordFixture(t, "base")
	for i, kind := range []string{"footnote", "footnote", "endnote"} {
		src = mustEdit(t, src, map[string]any{"operation": "note", "kind": kind, "paragraph": 0.0, "text": fmt.Sprint("Source ", i)})
	}
	root := referenceXML(t, src, "word/footnotes.xml")
	var ids []string
	for _, e := range root.ChildElements() {
		ids = append(ids, office.AttrV(e, nsW, "id"))
	}
	referenceEqual(t, ids, []string{"-1", "0", "1", "2"})
	contains(t, part(t, src, "word/footnotes.xml"), "Source 1")
	contains(t, part(t, src, "word/endnotes.xml"), "Source 2")
	referenceEqual(t, len(office.All(referenceXML(t, src, "word/document.xml"), nsW, "footnoteReference")), 2)
}
func TestReferenceWordPrivacyPreservesText(t *testing.T) {
	referenceWordCaller(t)
	src := mustEdit(t, base(t), map[string]any{"operation": "comment", "text": "The old price is $10.", "comment": "Keep review text", "author": "Secret reviewer"})
	out := mustEdit(t, src, map[string]any{"operation": "privacy"})
	// reference DOCX library core_properties.author reads the first direct creator and
	// returns empty text when that property is absent (not an indexing error).
	author := ""
	if creator := office.Child(referenceXML(t, out, "docProps/core.xml"), nsDC, "creator"); creator != nil {
		for _, token := range creator.Child {
			text, ok := token.(*etree.CharData)
			if !ok {
				break
			}
			author += text.Data
		}
	}
	referenceEqual(t, author, "")
	referenceEqual(t, referenceFirstCommentText(t, out), "Keep review text")
	absent(t, part(t, out, "word/comments.xml"), "Secret reviewer")
	referenceEqual(t, paras(t, out)[0], "The old price is $10.")
}

// UNRUN fixture correction: model the first section's default header creation
// without replacing styles/settings and other existing document relationships.
func referenceReferenceHeader(t *testing.T, src, text string) string {
	t.Helper()
	return referenceRewrite(t, src, func(pkg *opc.Package) {
		pkg.Set("word/header1.xml", []byte(`<w:hdr `+wNS+`><w:p><w:pPr><w:pStyle w:val="Header"/></w:pPr>`+run(text)+`</w:p></w:hdr>`))
		id, err := pkg.AddRel("word/document.xml", relBase+"header", "header1.xml", false)
		if err != nil {
			t.Fatal(err)
		}
		if err := pkg.EnsureContentType("word/header1.xml", ctBase+"header+xml"); err != nil {
			t.Fatal(err)
		}
		referenceSetXML(t, pkg, "word/document.xml", func(root *etree.Element) {
			section := office.Child(office.Child(root, nsW, "body"), nsW, "sectPr")
			if section == nil {
				t.Fatal("reference fixture missing section")
			}
			ref := etree.NewElement("w:headerReference")
			ref.CreateAttr("w:type", "default")
			ref.CreateAttr("r:id", id)
			section.InsertChildAt(0, ref)
			// Pinned default.docx: 8.5-inch page with 1.25-inch side margins.
			margins := office.Child(section, nsW, "pgMar")
			if margins == nil {
				t.Fatal("reference fixture missing margins")
			}
			margins.CreateAttr("w:left", "1800")
			margins.CreateAttr("w:right", "1800")
			if office.Child(section, nsW, "cols") == nil {
				section.CreateElement("w:cols").CreateAttr("w:space", "720")
			}
			if office.Child(section, nsW, "docGrid") == nil {
				section.CreateElement("w:docGrid").CreateAttr("w:linePitch", "360")
			}
		})
	})
}

func TestReferenceWordCaptionReferencesInBodyAndHeader(t *testing.T) {
	referenceWordCaller(t)
	src := referenceOriginalWordFixture(t, "refs")
	src = mustEdit(t, src, map[string]any{"operation": "caption", "label": "Table", "text": "Evidence", "after_table": 0.0, "bookmark": "evidence"})
	for _, marker := range []string{"[[BODY]]", "[[HEADER]]"} {
		src = mustEdit(t, src, map[string]any{"operation": "reference", "name": "evidence", "field": true, "marker": marker})
	}
	out := mustEdit(t, src, map[string]any{"operation": "materialize"})
	referenceEqual(t, referenceStoryTexts(t, out, "word/document.xml", true)[0], "Table 1 explains it.")
	_, header := referenceFirstSectionStory(t, out, "header")
	referenceEqual(t, referenceTexts(header), "See table 1 today.")
	n := 0
	styles := referenceXML(t, out, "word/styles.xml")
	for _, p := range referenceStoryParagraphs(t, out, "word/document.xml") {
		caption, err := referenceParagraphIsCaption(p, styles)
		if err != nil {
			t.Fatal(err)
		}
		if caption {
			n++
		}
	}
	referenceEqual(t, n, 1)
}
func TestReferenceWordMultipleReferencesPreserveExistingLinks(t *testing.T) {
	referenceWordCaller(t)
	src := referenceOriginalWordFixture(t, "multiple-references")
	src = mustEdit(t, src, map[string]any{"operation": "bookmark", "paragraph": 1.0, "name": "target"})
	src = mustEdit(t, src, map[string]any{"operation": "caption", "label": "Figure", "text": "Example", "bookmark": "figure"})
	for _, marker := range []string{"[[ONE]]", "[[TWO]]", "[[H1]]", "[[H2]]"} {
		src = mustEdit(t, src, map[string]any{"operation": "reference", "name": "figure", "field": true, "marker": marker})
	}
	out := mustEdit(t, src, map[string]any{"operation": "materialize"})
	referenceEqual(t, referenceStoryTexts(t, out, "word/document.xml", true)[0], "See 1 and 1. Existing link")
	_, header := referenceFirstSectionStory(t, out, "header")
	referenceEqual(t, referenceTexts(header), "1 / 1")
	p := referenceStoryParagraphs(t, out, "word/document.xml")[0]
	link := office.Child(p, nsW, "hyperlink")
	if link == nil {
		t.Fatal("missing direct paragraph hyperlink")
	}
	referenceEqual(t, office.AttrV(link, nsW, "anchor"), "target")
	referenceEqual(t, len(office.All(p, nsW, "fldSimple")), 2)
}
func TestReferenceWordMarkerCannotJoinAcrossField(t *testing.T) {
	referenceWordCaller(t)
	src := referenceOriginalWordFixture(t, "field-barrier")
	wantErr(t, src, map[string]any{"operation": "control", "marker": "[[MARK]]", "tag": "bad"}, "ordinary text")
}
func TestReferenceWordSequenceMaterialization(t *testing.T) {
	referenceWordCaller(t)
	src := mustEdit(t, base(t), map[string]any{"operation": "caption", "label": "Figure", "text": "One", "bookmark": "one"})
	src = mustEdit(t, src, map[string]any{"operation": "caption", "label": "Figure", "text": "Two"})
	out := mustEdit(t, src, map[string]any{"operation": "materialize"})
	var got []string
	for _, e := range office.All(referenceXML(t, out, "word/document.xml"), nsW, "fldSimple") {
		got = append(got, referenceTexts(e))
	}
	referenceEqual(t, got, []string{"1", "2"})
}
func TestReferenceWordRedactionAcrossStoriesAndMetadata(t *testing.T) {
	referenceWordCaller(t)
	src := referenceOriginalWordFixture(t, "redaction-stories")
	out := mustEdit(t, src, map[string]any{"operation": "redact", "values": []any{"old price"}})
	referenceEqual(t, paras(t, out)[0], "The [REDACTED] is $10.")
	p := referenceStoryParagraphs(t, out, "word/document.xml")[0]
	referenceRequireDirectRunProperty(t, p, 0, "b")
	_, header := referenceFirstSectionStory(t, out, "header")
	referenceEqual(t, referenceParagraphText(office.Children(header, nsW, "p")[0]), "[REDACTED]")
	referenceEqual(t, office.All(referenceXML(t, out, "docProps/core.xml"), nsDC, "subject")[0].Text(), "[REDACTED]")
	referenceEqual(t, referenceFirstCommentText(t, out), "[REDACTED]")
	pkg, e := opc.Open(out)
	if e != nil {
		t.Fatal(e)
	}
	for _, p := range pkg.Parts {
		if bytes.Contains(p.Data, []byte("old price")) {
			t.Errorf("secret retained in %s", p.Name)
		}
	}
}
func TestReferenceWordScopedStyleNormalization(t *testing.T) {
	referenceWordCaller(t)
	out := mustEdit(t, referenceOriginalWordFixture(t, "base"), map[string]any{"operation": "normalize", "styles": []any{"Normal"}})
	p := referenceStoryParagraphs(t, out, "word/document.xml")[0]
	runs := office.Children(p, nsW, "r")
	if len(runs) < 2 {
		t.Fatalf("missing original direct runs: got %d", len(runs))
	}
	// Run.bold/italic is None only when the direct rPr property is absent.
	// A present property with w:val="0" is False, not None.
	for i, property := range []string{"b", "i"} {
		if office.Child(office.Child(runs[i], nsW, "rPr"), nsW, property) != nil {
			t.Errorf("run %d retains direct %s property", i, property)
		}
	}
	referenceEqual(t, paras(t, out)[0], "The old price is $10.")
}
func TestReferenceWordMultipleWatermarksUniqueAndIndependent(t *testing.T) {
	referenceWordCaller(t)
	src := mustEdit(t, base(t), map[string]any{"operation": "watermark", "text": "DRAFT"})
	src = mustEdit(t, src, map[string]any{"operation": "watermark", "text": "CONFIDENTIAL"})
	p, e := opc.Open(src)
	if e != nil {
		t.Fatal(e)
	}
	type optionalID struct {
		value   string
		present bool
	}
	idOf := func(el *etree.Element) optionalID {
		value, present := office.Attr(el, "", "id")
		return optionalID{value, present}
	}
	ids := []optionalID{}
	for _, root := range referenceElementParts(t, p) {
		office.Iter(root, func(el *etree.Element) bool {
			if el.NamespaceURI() == nsV && (el.Tag == "shape" || el.Tag == "shapetype") {
				ids = append(ids, idOf(el))
			}
			return true
		})
		types := map[optionalID]bool{}
		for _, el := range office.All(root, nsV, "shapetype") {
			types[idOf(el)] = true
		}
		for _, el := range office.All(root, nsV, "shape") {
			value, present := office.Attr(el, "", "type")
			if !present {
				t.Fatal("shape type absent")
			}
			if !types[optionalID{strings.TrimLeft(value, "#"), true}] {
				t.Fatal("unresolved shape type")
			}
		}
	}
	unique := map[optionalID]bool{}
	for _, id := range ids {
		unique[id] = true
	}
	referenceEqual(t, len(ids), len(unique))
	out := mustEdit(t, src, map[string]any{"operation": "watermark-remove", "contains": "DRAFT"})
	_, root := referenceFirstSectionStory(t, out, "header")
	var text []string
	for _, e := range office.All(root, nsV, "textpath") {
		if s := e.SelectAttrValue("string", ""); s != "" {
			text = append(text, s)
		}
	}
	referenceEqual(t, text, []string{"CONFIDENTIAL"})
	referenceEqual(t, len(office.All(root, nsV, "shapetype")), 1)
}
func TestReferenceWordWatermarkRemovalPreservesHeader(t *testing.T) {
	referenceWordCaller(t)
	src := referenceOriginalWordFixture(t, "watermark-header")
	src = mustEdit(t, src, map[string]any{"operation": "watermark", "text": "DRAFT"})
	_, markedHeader := referenceFirstSectionStory(t, src, "header")
	if len(office.All(markedHeader, nsV, "textpath")) == 0 {
		t.Fatal("missing watermark")
	}
	out := mustEdit(t, src, map[string]any{"operation": "watermark-remove", "contains": "DRAFT"})
	_, cleanHeader := referenceFirstSectionStory(t, out, "header")
	referenceEqual(t, referenceParagraphText(office.Children(cleanHeader, nsW, "p")[0]), "Company header")
	referenceEqual(t, len(office.All(cleanHeader, nsV, "textpath")), 0)
}

func TestReferenceWordHyperlinkRevisionsAllStories(t *testing.T) {
	referenceWordCaller(t)
	for _, tag := range []string{"ins", "del", "moveTo", "moveFrom"} {
		for _, op := range []string{"accept", "reject"} {
			t.Run(tag+"/"+op, func(t *testing.T) {
				src := referenceOriginalWordFixture(t, "link-stories-"+tag+"-"+op)
				out := mustEdit(t, src, map[string]any{"operation": op})
				keep := (tag == "ins" || tag == "moveTo") == (op == "accept")
				want := "Read Website  today."
				if keep {
					want = "Read Website updated today."
				}
				pkg, e := opc.Open(out)
				if e != nil {
					t.Fatal(e)
				}
				// UNRUN: select the same four paragraphs as Reference, not every
				// descendant paragraph (which could silently omit an empty story).
				body := office.Child(referenceXML(t, out, "word/document.xml"), nsW, "body")
				tables := office.Children(body, nsW, "tbl")
				if len(tables) == 0 {
					t.Fatal("missing first body table")
				}
				rows := office.Children(tables[0], nsW, "tr")
				if len(rows) == 0 {
					t.Fatal("missing first table row")
				}
				cells := office.Children(rows[0], nsW, "tc")
				if len(cells) == 0 {
					t.Fatal("missing first table cell")
				}
				headerName, header := referenceFirstSectionStory(t, out, "header")
				footerName, footer := referenceFirstSectionStory(t, out, "footer")
				stories := []struct {
					name       string
					paragraphs []*etree.Element
				}{
					{"word/document.xml", office.Children(body, nsW, "p")},
					{"word/document.xml", office.Children(cells[0], nsW, "p")},
					{headerName, office.Children(header, nsW, "p")},
					{footerName, office.Children(footer, nsW, "p")},
				}
				for index, story := range stories {
					if len(story.paragraphs) == 0 {
						t.Fatalf("missing first paragraph for story %d", index)
					}
					p := story.paragraphs[0]
					referenceEqual(t, referenceParagraphText(p), want)
					links := office.Children(p, nsW, "hyperlink")
					referenceEqual(t, len(links), 1)
					l := links[0]
					if index%2 == 1 {
						referenceEqual(t, office.AttrV(l, nsW, "anchor"), "summary")
					} else {
						rs, err := pkg.Rels(story.name)
						if err != nil {
							t.Fatal(err)
						}
						target := ""
						for _, r := range rs {
							if r.ID == office.AttrV(l, "http://schemas.openxmlformats.org/officeDocument/2006/relationships", "id") {
								target = r.Target
							}
						}
						referenceEqual(t, target, "https://example.org")
					}
					if keep {
						referenceRequireDirectRunProperty(t, l, -1, "b")
					}
					referenceEqual(t, len(office.All(p, nsW, tag)), 0)
					referenceEqual(t, len(office.All(p, nsW, "delText")), 0)
				}
			})
		}
	}
}

func TestReferenceWordPrivacyNoteStories(t *testing.T) {
	referenceWordCaller(t)
	src := referenceOriginalWordFixture(t, "privacy-stories")
	original, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	out := mustEdit(t, src, map[string]any{"operation": "privacy"})
	// Inclusive C14N with comments, matching etree.tostring(method="c14n").
	for _, name := range []string{"word/document.xml", "word/header1.xml", "word/footer1.xml", "word/review-footnotes.xml", "word/review-endnotes.xml"} {
		before := referenceCanonicalParse(t, part(t, src, name))
		var strip func(*etree.Element)
		strip = func(e *etree.Element) {
			for i := len(e.Attr) - 1; i >= 0; i-- {
				a := e.Attr[i]
				if a.NamespaceURI() == nsW && (strings.HasPrefix(a.Key, "rsid") || a.Key == "author" || a.Key == "date" || a.Key == "initials") {
					e.Attr = append(e.Attr[:i], e.Attr[i+1:]...)
				}
			}
			for _, c := range e.ChildElements() {
				strip(c)
			}
		}
		strip(before)
		t.Run(name, func(t *testing.T) {
			referenceEqual(t, referenceCanonicalRoot(t, referenceCanonicalParse(t, part(t, out, name))), referenceCanonicalRoot(t, before))
		})
	}
	referenceEqual(t, part(t, out, "customXml/fixture.xml"), `<metadata xmlns="urn:fixture">Keep exactly</metadata>`)
	referenceEqual(t, part(t, out, "word/_rels/document.xml.rels"), part(t, src, "word/_rels/document.xml.rels"))
	after, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	referenceEqual(t, after, original)
}

// UNRUN: source-corrected List Number fixture, native PNG dimensions and inline scope.
func TestReferenceWordMergeNumberingAndImages(t *testing.T) {
	referenceWordCaller(t)
	other := referenceOriginalWordFixture(t, "merge-append")
	raw, e := os.ReadFile(other)
	if e != nil {
		t.Fatal(e)
	}
	out := mustEdit(t, base(t), map[string]any{"operation": "merge", "files": []any{other}})
	referenceEqual(t, paras(t, out)[0], "The old price is $10.")
	found := false
	for _, p := range paras(t, out) {
		found = found || p == "Appendix"
	}
	referenceEqual(t, found, true)
	// reference DOCX library InlineShapes uses //w:p/w:r/w:drawing/wp:inline,
	// not every descendant wp:inline (for example inside a hyperlink).
	inlineCount := 0
	for _, paragraph := range office.All(referenceXML(t, out, "word/document.xml"), nsW, "p") {
		for _, run := range office.Children(paragraph, nsW, "r") {
			for _, drawing := range office.Children(run, nsW, "drawing") {
				inlineCount += len(office.Children(drawing, "http://schemas.openxmlformats.org/drawingml/2006/wordprocessingDrawing", "inline"))
			}
		}
	}
	referenceEqual(t, inlineCount, 1)
	p, e := opc.Open(out)
	if e != nil {
		t.Fatal(e)
	}
	found = false
	for _, part := range p.Parts {
		found = found || strings.HasPrefix(part.Name, "word/media/")
	}
	referenceEqual(t, found, true)
	referenceEqual(t, p.Has("word/numbering.xml"), true)
	after, e := os.ReadFile(other)
	if e != nil {
		t.Fatal(e)
	}
	referenceEqual(t, after, raw)
}
