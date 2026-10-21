package wordedit

import (
	"path/filepath"
	"testing"

	"github.com/openvibely/openvibely-paperworks/internal/opc"
)

func TestInsertSectionPreservesExistingStarts(t *testing.T) {
	// Copy settings from section 0 while splitting section 1. Its existing
	// continuous start must remain, and section 2's odd-page start must not change.
	src := mk(t, "sections", `<w:p><w:pPr><w:sectPr><w:type w:val="nextPage"/><w:pgSz w:w="10000" w:h="14000"/></w:sectPr></w:pPr>`+run("First")+`</w:p><w:p>`+run("Split here")+`</w:p><w:p><w:pPr><w:sectPr><w:type w:val="continuous"/></w:sectPr></w:pPr>`+run("Remainder")+`</w:p><w:p>`+run("Last")+`</w:p>`, nil)
	pkg, err := opc.Open(src)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := pkg.XML("word/document.xml")
	if err != nil {
		t.Fatal(err)
	}
	body := child(doc.Root(), "body")
	valProp(child(body, "sectPr"), "type", "oddPage")
	if err = pkg.SetXML("word/document.xml", doc); err != nil {
		t.Fatal(err)
	}
	fixture := filepath.Join(t.TempDir(), "input.docx")
	if err = pkg.SaveNew(fixture); err != nil {
		t.Fatal(err)
	}
	out := mustEdit(t, fixture, map[string]any{"operation": "section-insert", "section": 0.0, "paragraph": 1.0, "type": "evenPage"})
	result, err := opc.Open(out)
	if err != nil {
		t.Fatal(err)
	}
	updated, err := result.XML("word/document.xml")
	if err != nil {
		t.Fatal(err)
	}
	sections := all(child(updated.Root(), "body"), "sectPr")
	if len(sections) != 4 {
		t.Fatalf("got %d sections", len(sections))
	}
	for i, want := range []string{"nextPage", "continuous", "evenPage", "oddPage"} {
		if got := getW(child(sections[i], "type"), "val"); got != want {
			t.Errorf("section %d starts %q, want %q", i, got, want)
		}
	}
	if getW(child(sections[1], "pgSz"), "w") != "10000" {
		t.Fatal("did not copy requested section settings")
	}
}
