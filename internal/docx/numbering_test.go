package docx

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"testing"

	"github.com/openvibely/openvibely-paperworks/internal/opc"
)

func TestNumberingPreservedWithoutLists(t *testing.T) {
	for _, blocks := range [][]Block{nil, {{Type: "paragraph", Text: "Plain content"}}, {{Type: "table", Rows: [][]string{{"A", "B"}}}}} {
		raw, err := Build(Spec{Blocks: blocks}, "")
		if err != nil {
			t.Fatal(err)
		}
		p, err := opc.Read(raw)
		if err != nil {
			t.Fatal(err)
		}
		// Unchanged pinned reference DOCX library blank and installed Word example agree.
		if got := fmt.Sprintf("%x", sha256.Sum256(p.Get("word/numbering.xml"))); got != "70976f19cbcd896e51890859fe6ecb3467a5a7ad0c040160fedc5a1993cb09ce" {
			t.Fatalf("reference numbering hash: %s", got)
		}
	}
}

func TestAuthoredListNumberingRetained(t *testing.T) {
	for _, ordered := range []bool{false, true} {
		raw, err := Build(Spec{Blocks: []Block{{Type: "list", Ordered: ordered, Items: []string{"First", "Second"}}}}, "")
		if err != nil {
			t.Fatal(err)
		}
		p, err := opc.Read(raw)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(p.Get("word/numbering.xml"), []byte(xmlHead+numberingXML)) {
			t.Fatal("authored list definitions changed")
		}
		id := 2
		if ordered {
			id = 1
		}
		if bytes.Count(p.Get("word/document.xml"), []byte(fmt.Sprintf(`<w:numId w:val="%d"/>`, id))) != 2 {
			t.Fatal("list paragraphs do not reference their numbering")
		}
	}
}

func TestOrderedListBlocksRestartNumbering(t *testing.T) {
	for _, separator := range []Block{
		{Type: "paragraph", Text: "Between lists"},
		{Type: "heading", Level: 2, Text: "Between lists"},
	} {
		t.Run(separator.Type, func(t *testing.T) {
			blocks := []Block{
				{Type: "list", Ordered: true, Items: []string{"First item", "Second item"}},
				separator,
				{Type: "list", Ordered: true, Items: []string{"Third item", "Fourth item"}},
			}
			raw, err := Build(Spec{Blocks: blocks}, "")
			if err != nil {
				t.Fatal(err)
			}
			pkg, err := opc.Read(raw)
			if err != nil {
				t.Fatal(err)
			}

			doc, err := opc.ParseXML(pkg.Get("word/document.xml"))
			if err != nil {
				t.Fatal(err)
			}
			var listRefs []struct {
				text string
				id   string
			}
			for _, paragraph := range doc.Root().FindElement("w:body").ChildElements() {
				pPr := paragraph.FindElement("w:pPr")
				if pPr == nil {
					continue
				}
				numPr := pPr.FindElement("w:numPr")
				if numPr == nil {
					continue
				}
				numID := numPr.FindElement("w:numId").SelectAttrValue("w:val", "")
				text := paragraph.FindElement("w:r/w:t").Text()
				listRefs = append(listRefs, struct {
					text string
					id   string
				}{text: text, id: numID})
			}
			if len(listRefs) != 4 {
				t.Fatalf("got %d numbered paragraphs, want 4", len(listRefs))
			}
			if listRefs[0].id != listRefs[1].id || listRefs[2].id != listRefs[3].id {
				t.Fatalf("paragraphs within a list use different numbering instances: %+v", listRefs)
			}
			if listRefs[0].id == listRefs[2].id {
				t.Fatalf("separate lists share numbering instance %s", listRefs[0].id)
			}
			wantTexts := []string{"First item", "Second item", "Third item", "Fourth item"}
			for i, want := range wantTexts {
				if listRefs[i].text != want {
					t.Fatalf("numbered paragraph %d has text %q, want %q", i, listRefs[i].text, want)
				}
			}

			numbering, err := opc.ParseXML(pkg.Get("word/numbering.xml"))
			if err != nil {
				t.Fatal(err)
			}
			var abstractID string
			for _, num := range numbering.Root().ChildElements() {
				if num.Tag == "num" && num.SelectAttrValue("w:numId", "") == listRefs[2].id {
					abstractID = num.FindElement("w:abstractNumId").SelectAttrValue("w:val", "")
					if num.FindElement("w:lvlOverride") != nil {
						t.Fatal("second list unexpectedly overrides its numbering level")
					}
				}
			}
			if abstractID == "" {
				t.Fatalf("numbering definition missing for second list numId %s", listRefs[2].id)
			}
			for _, abstract := range numbering.Root().ChildElements() {
				if abstract.Tag != "abstractNum" || abstract.SelectAttrValue("w:abstractNumId", "") != abstractID {
					continue
				}
				level := abstract.FindElement("w:lvl")
				if level == nil || level.FindElement("w:start").SelectAttrValue("w:val", "") != "1" ||
					level.FindElement("w:numFmt").SelectAttrValue("w:val", "") != "decimal" ||
					level.FindElement("w:lvlText").SelectAttrValue("w:val", "") != "%1." {
					t.Fatal("second list does not use decimal numbering starting at 1")
				}
				return
			}
			t.Fatalf("abstract numbering definition %s missing", abstractID)
		})
	}
}
