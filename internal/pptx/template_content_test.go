package pptx

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"os"
	"path/filepath"
	"testing"
)

func TestTemplateContentReference(t *testing.T) {
	base := "../../testdata/document-examples/document_presentations/examples"
	raw, err := os.ReadFile(filepath.Join(base, "revenue-slide.json"))
	if err != nil {
		t.Fatal(err)
	}
	var s Spec
	if err = json.Unmarshal(raw, &s); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "out.pptx")
	if err = Create(context.Background(), s, base, out); err != nil {
		t.Fatal(err)
	}
	p, err := opc.Open(out)
	if err != nil {
		t.Fatal(err)
	}
	var want map[string]string
	raw, err = os.ReadFile("testdata/content-hashes/hashes.json")
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(raw, &want); err != nil {
		t.Fatal(err)
	}
	for name, h := range want {
		if got := fmt.Sprintf("%x", sha256.Sum256(p.Get(name))); got != h {
			t.Errorf("%s: %s want %s", name, got, h)
		}
	}
}

func TestTemplateContentAuthoredValues(t *testing.T) {
	base := "../../testdata/document-examples/document_presentations/examples"
	raw, err := os.ReadFile(filepath.Join(base, "revenue-slide.json"))
	if err != nil {
		t.Fatal(err)
	}
	var s Spec
	if err = json.Unmarshal(raw, &s); err != nil {
		t.Fatal(err)
	}
	s.Title = "Title & <subject>"
	s.Slides[0].Notes = "First & second\n\nLast <line>"
	s.Slides[0].Background = "123456"
	s.Slides[0].Elements[0].Name = "Named title"
	out := filepath.Join(t.TempDir(), "values.pptx")
	if err = Create(context.Background(), s, base, out); err != nil {
		t.Fatal(err)
	}
	p, err := opc.Open(out)
	if err != nil {
		t.Fatal(err)
	}
	d, err := p.XML("docProps/core.xml")
	if err != nil {
		t.Fatal(err)
	}
	if d.Root().SelectElement("dc:title").Text() != s.Title {
		t.Fatal("title not preserved")
	}
	d, err = p.XML("ppt/notesSlides/notesSlide1.xml")
	if err != nil {
		t.Fatal(err)
	}
	ps := d.Root().FindElements(".//p:txBody/a:p")
	if len(ps) != 3 || ps[0].FindElement("a:r/a:t").Text() != "First & second" || len(ps[1].ChildElements()) != 0 || ps[2].FindElement("a:r/a:t").Text() != "Last <line>" {
		t.Fatal("notes paragraph content changed")
	}
	d, err = p.XML("ppt/slides/slide1.xml")
	if err != nil {
		t.Fatal(err)
	}
	if d.Root().FindElement(".//p:bg//a:srgbClr").SelectAttrValue("val", "") != "123456" {
		t.Fatal("background lost")
	}
	if d.Root().FindElement(".//p:sp/p:nvSpPr/p:cNvPr").SelectAttrValue("name", "") != "Named title" {
		t.Fatal("explicit name lost")
	}
}
