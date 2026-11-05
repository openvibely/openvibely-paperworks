package pptx

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
)

func TestAuthoredTextAndAttributesEscape(t *testing.T) {
	text := "A & B < C > \"quoted\" 'apostrophe' café"
	shapeName := "Name & < C > \"shape\" 'café'"
	description := "Description & < C > \"shape\" 'café'"
	font := "Font & < C > \"family\" 'café'"
	target := "https://example.test/?q=A&B<tea>\"café\""

	spec := Spec{
		Title: text,
		Font:  font,
		Slides: []Slide{{Elements: []Element{{
			Type:        "text",
			Name:        shapeName,
			Description: description,
			Text:        text,
			X:           1,
			Y:           1,
			W:           4,
			H:           2,
		}}}},
	}
	output := filepath.Join(t.TempDir(), "escaped.pptx")
	if err := Create(context.Background(), spec, ".", output); err != nil {
		t.Fatal(err)
	}
	pkg, err := opc.Open(output)
	if err != nil {
		t.Fatal(err)
	}

	slide, err := pkg.XML("ppt/slides/slide1.xml")
	if err != nil {
		t.Fatal(err)
	}
	textNodes := office.All(slide.Root(), ans, "t")
	if len(textNodes) != 1 || textNodes[0].Text() != text {
		t.Fatalf("PPTX text nodes = %v", textNodes)
	}
	properties := office.All(slide.Root(), pns, "cNvPr")
	if len(properties) < 2 {
		t.Fatalf("shape metadata properties missing: %d", len(properties))
	}
	var foundMetadata bool
	for _, property := range properties {
		if property.SelectAttrValue("name", "") == shapeName && property.SelectAttrValue("descr", "") == description {
			foundMetadata = true
		}
	}
	if !foundMetadata {
		t.Fatal("shape name/description attributes were not preserved")
	}

	core, err := pkg.XML("docProps/core.xml")
	if err != nil {
		t.Fatal(err)
	}
	title := core.Root().FindElement("dc:title")
	if title == nil || title.Text() != text {
		t.Fatalf("metadata title = %v", title)
	}
	themePart, err := pkg.XML("ppt/theme/theme1.xml")
	if err != nil {
		t.Fatal(err)
	}
	var foundFont bool
	for _, latin := range office.All(themePart.Root(), ans, "latin") {
		if latin.SelectAttrValue("typeface", "") == font {
			foundFont = true
		}
	}
	if !foundFont {
		t.Fatal("theme font attribute was not preserved")
	}

	rels, err := opc.ParseXML([]byte(relationships(rel("rId99", "custom", target))))
	if err != nil {
		t.Fatalf("generated relationship XML does not parse: %v", err)
	}
	if got := rels.Root().FindElement("Relationship").SelectAttrValue("Target", ""); got != target {
		t.Fatalf("relationship target = %q, want %q", got, target)
	}

	for _, part := range pkg.Parts {
		if strings.HasSuffix(part.Name, ".xml") || strings.HasSuffix(part.Name, ".rels") {
			if _, err := opc.ParseXML(part.Data); err != nil {
				t.Errorf("generated XML part %s does not parse: %v", part.Name, err)
			}
		}
	}
}
