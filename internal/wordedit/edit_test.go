package wordedit

import (
	"bytes"
	"fmt"

	"github.com/beevik/etree"
	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const rels = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rIdS" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/><Relationship Id="rIdT" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/settings" Target="settings.xml"/><Relationship Id="rIdN" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/numbering" Target="numbering.xml"/>%s</Relationships>`

func bold(text string) string {
	return `<w:r><w:rPr><w:b/></w:rPr><w:t xml:space="preserve">` + text + `</w:t></w:r>`
}
func run(text string) string { return `<w:r><w:t xml:space="preserve">` + text + `</w:t></w:r>` }

func TestTrackedMovesAcceptRejectAndRemoveMarkers(t *testing.T) {
	referenceWordCaller(t)
	body := `<w:p>`
	for _, tag := range []string{"moveFrom", "moveTo"} {
		text := map[string]string{"moveFrom": "old location", "moveTo": "new location"}[tag]
		body += fmt.Sprintf(`<w:%sRangeStart w:id="3"/><w:%s>%s</w:%s><w:%sRangeEnd w:id="3"/>`, tag, tag, run(text), tag, tag)
	}
	src := mk(t, "moves", body+`</w:p>`, nil)
	src = referenceRewrite(t, src, func(p *opc.Package) {
		referenceSetXML(t, p, "word/settings.xml", func(root *etree.Element) { root.CreateElement("w:trackRevisions") })
	})
	before, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	for _, op := range []string{"accept", "reject"} {
		want := map[string]string{"accept": "new location", "reject": "old location"}[op]
		out := mustEdit(t, src, map[string]any{"operation": op})
		if got := paras(t, out)[0]; got != want {
			t.Fatalf("%s: %q", op, got)
		}
		office.Iter(referenceXML(t, out, "word/document.xml"), func(e *etree.Element) bool {
			if strings.Contains("{"+e.NamespaceURI()+"}"+e.Tag, "move") {
				t.Fatal("move marker remains", e.Tag)
			}
			return true
		})
		if office.Child(referenceXML(t, out, "word/settings.xml"), nsW, "trackRevisions") != nil {
			t.Fatal("tracking remains")
		}
		after, err := os.ReadFile(src)
		if err != nil || !bytes.Equal(before, after) {
			t.Fatal("source changed", err)
		}
	}
}

func TestAcceptRejectCanPreserveTrackingSetting(t *testing.T) {
	referenceWordCaller(t)
	src := mustEdit(t, base(t), map[string]any{"operation": "redline", "old": "$10", "new": "$20", "author": "Reviewer"})
	tracked := referenceRewrite(t, src, func(p *opc.Package) {
		referenceSetXML(t, p, "word/settings.xml", func(root *etree.Element) { root.CreateElement("w:trackRevisions") })
	})
	for _, mode := range []string{"accept", "reject"} {
		for _, keep := range []bool{true, false} {
			out := mustEdit(t, tracked, map[string]any{"operation": mode, "keep_tracking_on": keep})
			want := map[string]string{"accept": "$20", "reject": "$10"}[mode]
			contains(t, paras(t, out)[0], want)
			if (office.Child(referenceXML(t, out, "word/settings.xml"), nsW, "trackRevisions") != nil) != keep {
				t.Fatalf("%s keep=%v tracking setting wrong", mode, keep)
			}
		}
	}
}

// Supplemental validation is separate from the original caller sequence.
func TestTrackingSettingRejectsNonBoolean(t *testing.T) {
	wantErr(t, base(t), map[string]any{"operation": "accept", "keep_tracking_on": "yes"}, "boolean")
}

func between(s, a, b string) string {
	i := strings.Index(s, a)
	j := strings.LastIndex(s, b)
	return s[i+len(a) : j]
}

func TestReplaceExistingInsertionKeepsRevisionChoicesAndFormat(t *testing.T) {
	referenceWordCaller(t)
	src := mk(t, "ins", `<w:p>`+run("Before ")+`<w:ins w:id="7" w:author="Original">`+bold("proposed")+`</w:ins>`+run(" after.")+`</w:p>`, nil)
	edited := mustEdit(t, src, map[string]any{"operation": "redline-insertion", "id": "7", "new": "revised", "author": "Reviewer"})
	doc := part(t, edited, "word/document.xml")
	contains(t, doc, `w:id="8"`)
	contains(t, doc, `w:id="9"`)
	root := referenceXML(t, edited, "word/document.xml")
	var ids []string
	for _, e := range root.FindElements(".//*") {
		if e.NamespaceURI() == nsW && (e.Tag == "ins" || e.Tag == "del") {
			ids = append(ids, office.AttrV(e, nsW, "id"))
			referenceEqual(t, office.AttrV(e, nsW, "author"), "Reviewer")
			referenceEqual(t, len(office.All(e, nsW, "b")), 1)
		}
	}
	referenceEqual(t, ids, []string{"8", "9"})
	// Both the deletion and replacement insertion retain the original bold run.
	if strings.Count(doc, `<w:b/>`) != 2 {
		t.Fatalf("revision replacement lost run formatting: %s", doc)
	}
	if strings.Count(doc, `w:author="Reviewer"`) != 2 {
		t.Fatal("expected two Reviewer revisions")
	}
	for _, mode := range []string{"accept", "reject"} {
		want := map[string]string{"accept": "Before revised after.", "reject": "Before proposed after."}[mode]
		if got := paras(t, mustEdit(t, edited, map[string]any{"operation": mode}))[0]; got != want {
			t.Fatalf("%s: %q", mode, got)
		}
	}
	wantErr(t, src, map[string]any{"operation": "redline-insertion", "id": "99", "new": "bad", "author": "Reviewer"}, "insertion id")
}

func TestCommentAppendResolutionAndReopen(t *testing.T) {
	referenceWordCaller(t)
	src := mustEdit(t, base(t), map[string]any{"operation": "comment", "text": "The old price is $10.", "comment": "First", "author": "Reviewer"})
	src = mustEdit(t, src, map[string]any{"operation": "comment-edit", "id": 0.0, "append": "Second", "resolved": true})
	src = mustEdit(t, src, map[string]any{"operation": "comment-edit", "id": 0.0, "append": "Third"})
	// UNRUN source correction: Reference reads comments.get(0), not report text
	// or the final paragraph across every comment in the package.
	comments := office.Children(referenceXML(t, src, "word/comments.xml"), nsW, "comment")
	var selected *etree.Element
	for _, comment := range comments {
		if office.AttrV(comment, nsW, "id") == "0" {
			selected = comment
			break
		}
	}
	if selected == nil {
		t.Fatal("missing comment ID 0")
	}
	referenceEqual(t, referenceCommentText(selected), "First\nSecond\nThird")
	referenceEqual(t, report(t, src)["comment_details"].([]any)[0].(map[string]any)["resolved"], true)
	// etree root[0] selects the first direct element, regardless of its tag.
	entries := referenceXML(t, src, "word/commentsExtended.xml").ChildElements()
	if len(entries) == 0 {
		t.Fatal("missing first comment extension entry")
	}
	ext := entries[0]
	referenceEqual(t, office.AttrV(ext, nsW15, "done"), "1")
	paragraphs := office.Children(selected, nsW, "p")
	if len(paragraphs) == 0 {
		t.Fatal("comment ID 0 has no paragraphs")
	}
	extID, extPresent := office.Attr(ext, nsW15, "paraId")
	paragraphID, paragraphPresent := office.Attr(paragraphs[len(paragraphs)-1], "http://schemas.microsoft.com/office/word/2010/wordml", "paraId")
	// Reference .get distinguishes absent (None) from an explicitly empty value.
	referenceEqual(t, extPresent, paragraphPresent)
	referenceEqual(t, extID, paragraphID)
	reopened := mustEdit(t, src, map[string]any{"operation": "comment-edit", "id": 0.0, "resolved": false})
	entries = referenceXML(t, reopened, "word/commentsExtended.xml").ChildElements()
	if len(entries) == 0 {
		t.Fatal("missing first reopened comment extension entry")
	}
	referenceEqual(t, office.AttrV(entries[0], nsW15, "done"), "0")
	if report(t, reopened)["comment_details"].([]any)[0].(map[string]any)["resolved"] != false {
		t.Fatal("not reopened")
	}
}

func TestTrackedHyperlinkReplacementPreservesTargetAndFormat(t *testing.T) {
	referenceWordCaller(t)
	for _, internal := range []bool{false, true} {
		fixture, attr := "hyperlink-external", `r:id="rId9"`
		if internal {
			fixture, attr = "hyperlink-internal", `w:anchor="summary"`
		}
		src := referenceOriginalWordFixture(t, fixture)
		revised := mustEdit(t, src, map[string]any{"operation": "redline", "old": "old wording", "new": "new wording", "author": "Reviewer"})
		doc := part(t, revised, "word/document.xml")
		link := between(doc, "<w:hyperlink", "</w:hyperlink>")
		contains(t, link, "<w:delText")
		contains(t, link, "new wording")
		lr := office.All(referenceXML(t, revised, "word/document.xml"), nsW, "hyperlink")[0]
		var deleted strings.Builder
		for _, e := range office.All(lr, nsW, "delText") {
			deleted.WriteString(e.Text())
		}
		referenceEqual(t, deleted.String(), "old wording")
		referenceEqual(t, referenceTexts(lr), "new wording")
		referenceEqual(t, len(office.All(lr, nsW, "ins")), 1)
		for _, mode := range []string{"accept", "reject"} {
			want := map[string]string{"accept": "new wording", "reject": "old wording"}[mode]
			out := mustEdit(t, revised, map[string]any{"operation": mode})
			if got := paras(t, out)[0]; got != "See "+want+" today." {
				t.Fatalf("%s: %q", mode, got)
			}
			doc := part(t, out, "word/document.xml")
			contains(t, doc, attr)
			contains(t, between(doc, "<w:hyperlink", "</w:hyperlink>"), "<w:b/>")
			paragraph := referenceStoryParagraphs(t, out, "word/document.xml")[0]
			link := office.Children(paragraph, nsW, "hyperlink")[0]
			if internal {
				referenceEqual(t, office.AttrV(link, nsW, "anchor"), "summary")
			} else {
				id := office.AttrV(link, office.NSR, "id")
				var address string
				for _, rel := range referenceXML(t, out, "word/_rels/document.xml.rels").ChildElements() {
					if rel.SelectAttrValue("Id", "") == id {
						address = rel.SelectAttrValue("Target", "")
					}
				}
				referenceEqual(t, address, "https://example.org")
			}
			for _, r := range office.Children(link, nsW, "r") {
				b := office.Child(office.Child(r, nsW, "rPr"), nsW, "b")
				if b == nil {
					t.Fatal("hyperlink run lacks bold")
				}
				switch office.AttrV(b, nsW, "val") {
				case "", "1", "true", "on":
				default:
					t.Fatal("hyperlink run bold disabled")
				}
			}
		}
	}
}

func TestInlineControlRevisionsKeepControlAndSurroundingText(t *testing.T) {
	referenceWordCaller(t)
	for _, tag := range []string{"ins", "del", "moveTo", "moveFrom"} {
		for _, linked := range []bool{false, true} {
			for _, mode := range []string{"accept", "reject"} {
				textTag := "w:t"
				if tag == "del" || tag == "moveFrom" {
					textTag = "w:delText"
				}
				rev := fmt.Sprintf(`<w:%s w:id="1"><w:r><w:rPr><w:b/></w:rPr><%s xml:space="preserve">42</%s></w:r></w:%s>`, tag, textTag, textTag, tag)
				if linked {
					rev = `<w:hyperlink w:anchor="summary">` + rev + `</w:hyperlink>`
				}
				src := mk(t, "ctl", `<w:p>`+run("Value: ")+`<w:sdt><w:sdtPr><w:tag w:val="amount"/></w:sdtPr><w:sdtContent>`+rev+`</w:sdtContent></w:sdt>`+run(" total.")+`</w:p>`, nil)
				out := mustEdit(t, src, map[string]any{"operation": mode})
				doc := part(t, out, "word/document.xml")
				keep := (tag == "ins" || tag == "moveTo") == (mode == "accept")
				want := "Value:  total."
				if keep {
					want = "Value: 42 total."
				}
				root := referenceXML(t, out, "word/document.xml")
				controls := office.All(root, nsW, "sdt")
				referenceEqual(t, len(controls), 1)
				referenceEqual(t, between(doc, "<w:sdtPr>", "</w:sdtPr>"), between(part(t, src, "word/document.xml"), "<w:sdtPr>", "</w:sdtPr>"))
				referenceEqual(t, referenceTexts(root), want)
				referenceEqual(t, len(office.All(root, nsW, tag)), 0)
				if keep && len(office.All(controls[0], nsW, "b")) == 0 {
					t.Fatal("control lost bold property")
				}
				if linked {
					referenceEqual(t, office.AttrV(office.All(controls[0], nsW, "hyperlink")[0], nsW, "anchor"), "summary")
				}
				// Supplemental fixed tag/property checks do not replace the original equality.
				contains(t, doc, `<w:tag w:val="amount"/>`)
			}
		}
	}
}

func TestHyperlinkFormatRevisionRejectedWithoutOutput(t *testing.T) {
	referenceWordCaller(t)
	src := mk(t, "fmt", `<w:p><w:hyperlink w:anchor="summary"><w:ins><w:r><w:rPr><w:rPrChange/></w:rPr><w:t xml:space="preserve">Website</w:t></w:r></w:ins></w:hyperlink></w:p>`, nil)
	for _, op := range []string{"accept", "reject"} {
		wantErr(t, src, map[string]any{"operation": op}, "formatting/table")
	}
}

func TestRegexRedactionPreservesRunLengths(t *testing.T) {
	referenceWordCaller(t)
	src := referenceOriginalWordFixture(t, "regex-private")
	before, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	out := mustEdit(t, src, map[string]any{"operation": "redact", "patterns": []any{`AB-\d{4}`}, "replacement": "X", "preserve_length": true})
	if got := paras(t, out)[0]; got != "ID XXXXXXX end" {
		t.Fatalf("%q", got)
	}
	paragraph := office.Children(office.Child(referenceXML(t, out, "word/document.xml"), nsW, "body"), nsW, "p")[0]
	runs := office.Children(paragraph, nsW, "r")
	lengths := []int{}
	for _, r := range runs {
		lengths = append(lengths, len([]rune(office.Text(r, nsW, "t"))))
	}
	referenceEqual(t, lengths, []int{6, 8})
	if office.Child(office.Child(runs[0], nsW, "rPr"), nsW, "b") == nil {
		t.Fatal("bold formatting lost")
	}
	if got := referenceStoryTexts(t, out, "word/header1.xml", false)[0]; got != "XXXXXXX" {
		t.Fatalf("header %q", got)
	}
	after, err := os.ReadFile(src)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("redaction changed source", err)
	}
	wantErr(t, src, map[string]any{"operation": "redact", "patterns": []any{"a*"}}, ".")
	// Supplemental checks retain exact split text and italic formatting.
	referenceEqual(t, office.Text(runs[0], nsW, "t"), "ID XXX")
	referenceEqual(t, office.Text(runs[1], nsW, "t"), "XXXX end")
	if office.Child(office.Child(runs[1], nsW, "rPr"), nsW, "i") == nil {
		t.Fatal("italic formatting lost")
	}
	after, err = os.ReadFile(src)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("rejection changed source", err)
	}
}

func TestTableGeometryObeysSectionAndIndent(t *testing.T) {
	referenceWordCaller(t)
	src := referenceOriginalWordFixture(t, "page-table")
	for _, c := range []struct {
		w      []any
		indent float64
	}{{[]any{6000.0, 6000.0}, 0}, {[]any{4000.0, 4000.0}, 2000}, {[]any{1000.0, 1000.0}, -1}} {
		wantErr(t, src, map[string]any{"operation": "table-geometry", "table": 0.0, "widths_twips": c.w, "indent_twips": c.indent}, ".")
	}
	out := mustEdit(t, src, map[string]any{"operation": "table-geometry", "table": 0.0, "widths_twips": []any{4000.0, 4000.0}, "indent_twips": 200.0})
	body := office.Child(referenceXML(t, out, "word/document.xml"), nsW, "body")
	table := office.Children(body, nsW, "tbl")[0]
	var widths []string
	for _, col := range office.Children(office.Child(table, nsW, "tblGrid"), nsW, "gridCol") {
		widths = append(widths, office.AttrV(col, nsW, "w"))
	}
	referenceEqual(t, widths, []string{"4000", "4000"})
}

func TestInsertionIDDiagnosticExactAndNoOutput(t *testing.T) {
	for _, count := range []int{0, 2} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			body := "<w:p>" + strings.Repeat(`<w:ins w:id="7" w:author="Original">`+run("proposed")+`</w:ins>`, count) + "</w:p>"
			src := mk(t, "invalid-insertion", body, nil)
			before, err := os.ReadFile(src)
			if err != nil {
				t.Fatal(err)
			}
			out := filepath.Join(t.TempDir(), "output.docx")
			_, err = Edit(src, out, map[string]any{"operation": "redline-insertion", "id": "7", "new": "replacement", "author": "Reviewer"}, "")
			if err == nil || err.Error() != "Insertion ID must identify exactly one revision" {
				t.Fatalf("diagnostic: %v", err)
			}
			if _, err := os.Stat(out); !os.IsNotExist(err) {
				t.Fatalf("unexpected output: %v", err)
			}
			after, err := os.ReadFile(src)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatal("source changed", err)
			}
		})
	}
}
