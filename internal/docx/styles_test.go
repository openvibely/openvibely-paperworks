package docx

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/beevik/etree"
	"github.com/openvibely/openvibely-paperworks/internal/docxtemplate"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstalledStylesOverlay(t *testing.T) {
	base := "../../testdata/document-examples/document_word/examples"
	raw, err := os.ReadFile(filepath.Join(base, "report.json"))
	if err != nil {
		t.Fatal(err)
	}
	var spec Spec
	if err = json.Unmarshal(raw, &spec); err != nil {
		t.Fatal(err)
	}
	raw, err = Build(spec, base)
	if err != nil {
		t.Fatal(err)
	}
	p, err := opc.Read(raw)
	if err != nil {
		t.Fatal(err)
	}
	// Unchanged pinned reference DOCX library Word example: full template with Arial/black
	// overrides for Normal, Title and Heading 1, plus Normal size 11.
	if got := fmt.Sprintf("%x", sha256.Sum256(p.Get("word/styles.xml"))); got != "47a2ba004dc01598a5b3b80bda5507d8ab598e6169782cefe68c7e45e8655a62" {
		t.Fatalf("complete reference styles hash: %s", got)
	}
}

func TestSparseStylesOverlay(t *testing.T) {
	pkg, err := docxtemplate.Open()
	if err != nil {
		t.Fatal(err)
	}
	template := pkg.Get("word/styles.xml")
	for _, prefix := range []string{"w", "x"} {
		overlay := []byte(`<` + prefix + `:styles xmlns:` + prefix + `="` + NSW + `"><` + prefix + `:style ` + prefix + `:type="paragraph" ` + prefix + `:styleId="Normal"><` + prefix + `:name ` + prefix + `:val="Changed"/></` + prefix + `:style><` + prefix + `:style ` + prefix + `:type="paragraph" ` + prefix + `:styleId="New"><` + prefix + `:name ` + prefix + `:val="New"/></` + prefix + `:style></` + prefix + `:styles>`)
		got, err := overlayStyles(template, overlay)
		if err != nil {
			t.Fatal(err)
		}
		a, _ := opc.ParseXML(template)
		b, _ := opc.ParseXML(got)
		if len(b.Root().ChildElements()) != len(a.Root().ChildElements())+1 {
			t.Fatal("template definitions lost")
		}
		// Only Normal may differ among original elements. Preserve all other XML.
		for i, e := range a.Root().ChildElements() {
			if e.SelectAttrValue("w:styleId", "") == "Normal" {
				continue
			}
			ad, bd := etree.NewDocument(), etree.NewDocument()
			ad.SetRoot(e.Copy())
			bd.SetRoot(b.Root().ChildElements()[i].Copy())
			ar, _ := ad.WriteToBytes()
			br, _ := bd.WriteToBytes()
			if !bytes.Equal(ar, br) {
				t.Fatalf("unmentioned definition changed at %d", i)
			}
		}
		if !strings.Contains(string(got), `val="Changed"`) {
			t.Fatal("override absent")
		}
	}
	for _, body := range []string{`<w:style/>`, `<w:style w:styleId="A"/><w:style w:styleId="A"/>`, `<w:bad/>`} {
		if _, err := overlayStyles(template, []byte(`<w:styles xmlns:w="`+NSW+`">`+body+`</w:styles>`)); err == nil {
			t.Fatal("invalid overlay accepted", body)
		}
	}
	if _, err := Build(Spec{StylesFile: "a", StylesOverlayFile: "b"}, ""); err == nil {
		t.Fatal("ambiguous resources accepted")
	}
}
