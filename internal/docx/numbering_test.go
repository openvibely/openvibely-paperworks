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
