package office

import (
	"archive/zip"
	"bytes"
	"fmt"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

type referenceCheckPart struct{ name, data string }

func referenceCheckPackage(t *testing.T, parts []referenceCheckPart) string {
	t.Helper()
	return referenceCheckPackageNamed(t, "source.zip", parts)
}
func referenceCheckPackageNamed(t *testing.T, name string, parts []referenceCheckPart) string {
	t.Helper()
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	for _, part := range parts {
		w, e := z.CreateHeader(&zip.FileHeader{Name: part.name, Method: zip.Store})
		if e != nil {
			t.Fatal(e)
		}
		if _, e = w.Write([]byte(part.data)); e != nil {
			t.Fatal(e)
		}
	}
	if e := z.Close(); e != nil {
		t.Fatal(e)
	}
	p := filepath.Join(t.TempDir(), name)
	if e := os.WriteFile(p, b.Bytes(), 0600); e != nil {
		t.Fatal(e)
	}
	return p
}
func TestReferenceNotesMasterRepair(t *testing.T) {
	src := referenceNotesFixture(t)
	check := func(path string, want bool) {
		t.Helper()
		r, e := PackageReport(path)
		if e != nil {
			t.Fatal(e)
		}
		if strings.Contains(fmt.Sprint(r["issues"]), "notes-master") != want {
			t.Fatal(r["issues"])
		}
	}
	check(src, true)
	out := filepath.Join(t.TempDir(), "out.pptx")
	if _, e := RepairNotes(src, out); e != nil {
		t.Fatal(e)
	}
	check(out, false)
	a, e := opc.Open(src)
	if e != nil {
		t.Fatal(e)
	}
	b, e := opc.Open(out)
	if e != nil {
		t.Fatal(e)
	}
	if !strings.Contains(referenceFirstSlideNotesText(t, out), "Preserve these speaker notes") {
		t.Error("notes lost")
	}
	var an, bn []string
	for _, p := range a.Parts {
		an = append(an, p.Name)
		if p.Name != "ppt/presentation.xml" && !bytes.Equal(p.Data, b.Get(p.Name)) {
			t.Error("modified", p.Name)
		}
	}
	for _, p := range b.Parts {
		bn = append(bn, p.Name)
	}
	if !reflect.DeepEqual(an, bn) {
		t.Error("archive order/membership changed", an, bn)
	}
	again := filepath.Join(t.TempDir(), "again.pptx")
	if _, e := RepairNotes(out, again); e != nil {
		t.Fatal(e)
	}
	c, e := opc.Open(again)
	if e != nil {
		t.Fatal(e)
	}
	for _, p := range b.Parts {
		if !bytes.Equal(p.Data, c.Get(p.Name)) {
			t.Error("non-idempotent", p.Name)
		}
	}
	if _, e := RepairNotes(src, src); e == nil {
		t.Fatal("in-place repair accepted")
	}
}
func TestReferenceHeadingLinkAndNoteAudit(t *testing.T) {
	r, unchanged := referenceInspectionRead(t, "audit.docx", []referenceCheckPart{{"word/document.xml", `<w:document xmlns:w="` + NSW + `"><w:body><w:p><w:pPr><w:pStyle w:val="Heading3"/></w:pPr><w:r><w:t>Section</w:t></w:r><w:hyperlink><w:r><w:t>click here</w:t></w:r></w:hyperlink><w:r><w:footnoteReference w:id="5"/></w:r></w:p></w:body></w:document>`}})
	if r["headings"].([]any)[0].(map[string]any)["level"] != float64(3) {
		t.Fatal(r["headings"])
	}
	for _, message := range []string{"heading level jumps", "descriptive label", "missing footnote 5"} {
		found := false
		for _, issue := range r["issues"].([]any) {
			if strings.Contains(issue.(string), message) {
				found = true
				break
			}
		}
		if !found {
			t.Fatal("missing", message, r["issues"])
		}
	}
	unchanged()
}

func TestReferencePreservationApprovedParts(t *testing.T) {
	a := referenceCheckPackageNamed(t, "before.docx", []referenceCheckPart{{"word/document.xml", `<w:document xmlns:w="` + NSW + `"/>`}, {"opaque.bin", "unchanged"}})
	b := referenceCheckPackageNamed(t, "after.docx", []referenceCheckPart{{"word/document.xml", `<w:document xmlns:w="` + NSW + `"><w:body/></w:document>`}, {"opaque.bin", "changed"}})
	if _, e := VerifyPreservation(a, b, []string{"word/document.xml"}); e == nil || !regexp.MustCompile(`opaque.bin`).MatchString(e.Error()) {
		t.Fatal(e)
	}
	r, e := VerifyPreservation(a, b, []string{"word/document.xml", "opaque.bin"})
	if e != nil {
		t.Fatal(e)
	}
	if !reflect.DeepEqual(r["changed_parts"], []string{"opaque.bin", "word/document.xml"}) {
		t.Fatal(r)
	}
}
func TestReferenceStoriesNotesSectionsAndStyles(t *testing.T) {
	r, unchanged := referenceInspectionRead(t, "stories.docx", []referenceCheckPart{
		{"word/document.xml", `<w:document xmlns:w="` + NSW + `" xmlns:r="` + NSR + `"><w:body><w:p><w:r><w:rPr><w:b/><w:rFonts w:ascii="Arial"/></w:rPr><w:t>Looks like heading</w:t></w:r><w:del><w:r><w:delText>Removed</w:delText></w:r></w:del><w:fldSimple w:instr="PAGE"><w:r><w:t>2</w:t></w:r></w:fldSimple><w:r><w:footnoteReference w:id="1"/></w:r></w:p><w:tbl><w:tr><w:tc><w:p><w:r><w:t>Cell</w:t></w:r></w:p><w:tbl><w:tr><w:tc><w:p><w:r><w:t>Nested</w:t></w:r></w:p></w:tc></w:tr></w:tbl></w:tc></w:tr></w:tbl><w:p><w:r><w:pict><w:txbxContent><w:p><w:r><w:t>Textbox</w:t></w:r></w:p></w:txbxContent></w:pict></w:r></w:p><w:p><w:pPr><w:sectPr><w:headerReference w:type="default" r:id="h"/></w:sectPr></w:pPr></w:p><w:sectPr/></w:body></w:document>`},
		{"word/_rels/document.xml.rels", `<Relationships><Relationship Id="h" Target="header1.xml"/></Relationships>`},
		{"word/header1.xml", `<w:hdr xmlns:w="` + NSW + `"><w:p><w:r><w:t>Header</w:t></w:r></w:p></w:hdr>`},
		{"word/footnotes.xml", `<w:footnotes xmlns:w="` + NSW + `"><w:footnote w:id="1"><w:p><w:r><w:t>Note</w:t></w:r></w:p></w:footnote><w:footnote w:id="1"/></w:footnotes>`}})
	var entries []any
	for _, story := range r["stories"].([]any) {
		part := story.(map[string]any)
		if part["part"] == "word/document.xml" {
			entries = part["content"].([]any)
			break
		}
	}
	if entries == nil {
		t.Fatal("missing main document story")
	}
	for _, text := range []string{"Cell", "Nested", "Textbox", "Removed", "PAGE", "2"} {
		n := 0
		for _, e := range entries {
			if e.(map[string]any)["text"] == text {
				n++
			}
		}
		if n != 1 {
			t.Fatal(text, n)
		}
	}
	found := false
	for _, e := range r["issues"].([]any) {
		found = found || e == "Duplicate footnote ID 1"
	}
	if !found {
		t.Fatal(r["issues"])
	}
	found = false
	for _, e := range r["style_warnings"].([]any) {
		found = found || e.(map[string]any)["reason"] == "possible heading without outline level"
	}
	if !found {
		t.Fatal(r["style_warnings"])
	}
	if r["font_usage"].(map[string]any)["Arial"] != float64(1) {
		t.Fatal(r["font_usage"])
	}
	links := r["section_links"].([]any)
	if links[0].(map[string]any)["links"].(map[string]any)["header:default"].(map[string]any)["inherited"] != false {
		t.Fatal(links[0])
	}
	want := map[string]any{"target": "header1.xml", "inherited": true}
	if !reflect.DeepEqual(links[1].(map[string]any)["links"].(map[string]any)["header:default"], want) {
		t.Fatal(links[1])
	}
	unchanged()
}

func TestReferenceMissingContentTypes(t *testing.T) {
	r, unchanged := referenceInspectionRead(t, "types.docx", []referenceCheckPart{{"word/document.xml", `<w:document xmlns:w="` + NSW + `"/>`}, {"[Content_Types].xml", `<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Override PartName="/missing.xml" ContentType="application/xml"/></Types>`}})
	for _, want := range []string{"Missing content type: word/document.xml", "Content type references missing part: missing.xml"} {
		found := false
		for _, v := range r["issues"].([]any) {
			found = found || v == want
		}
		if !found {
			t.Fatal("missing", want, r["issues"])
		}
	}
	// Supplemental source preservation follows all original assertions.
	unchanged()
}
