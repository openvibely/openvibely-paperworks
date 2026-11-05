package docx

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/beevik/etree"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
)

func TestEquationBlocksWriteNativeOMMLInOrder(t *testing.T) {
	raw, err := Build(Spec{Blocks: []Block{
		{Type: "paragraph", Text: "Before equation"},
		{Type: "equation", MathML: `<math><mi>x</mi><mo>=</mo><mfrac><mi>a</mi><mi>b</mi></mfrac><mo>+</mo><msup><mi>c</mi><mn>2</mn></msup></math>`},
		{Type: "paragraph", Text: "After equation"},
	}}, "")
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
	children := doc.Root().FindElement("w:body").ChildElements()
	if len(children) != 4 || children[0].Tag != "p" || children[1].Tag != "p" || children[2].Tag != "p" || children[3].Tag != "sectPr" {
		t.Fatalf("unexpected document body block order: %v", elementTags(children))
	}
	if got := children[0].FindElement("w:r/w:t").Text(); got != "Before equation" {
		t.Fatalf("first paragraph text = %q", got)
	}
	om := children[1].FindElement("m:oMath")
	if om == nil {
		t.Fatal("equation block has no native m:oMath element")
	}
	if got := om.NamespaceURI(); got != "http://schemas.openxmlformats.org/officeDocument/2006/math" {
		t.Fatalf("equation namespace = %q", got)
	}
	if om.FindElement("m:f") == nil || om.FindElement("m:sSup") == nil {
		t.Fatal("fraction or superscript was not converted to native OMML")
	}
	if got := children[2].FindElement("w:r/w:t").Text(); got != "After equation" {
		t.Fatalf("last paragraph text = %q", got)
	}
}

func TestEquationBlocksCanLeadAndTrailTextBlocks(t *testing.T) {
	raw, err := Build(Spec{Blocks: []Block{
		{Type: "equation", MathML: `<math><mi>a</mi></math>`},
		{Type: "paragraph", Text: "Between first and second equations"},
		{Type: "equation", MathML: `<math><mi>b</mi></math>`},
		{Type: "paragraph", Text: "Between second and last equations"},
		{Type: "equation", MathML: `<math><mi>c</mi></math>`},
	}}, "")
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
	children := doc.Root().FindElement("w:body").ChildElements()
	if len(children) != 6 || children[5].Tag != "sectPr" {
		t.Fatalf("unexpected block count/order: %v", elementTags(children))
	}
	for _, index := range []int{0, 2, 4} {
		if children[index].FindElement("m:oMath") == nil {
			t.Fatalf("equation at block position %d is missing OMML", index)
		}
	}
	for _, index := range []int{1, 3} {
		if children[index].FindElement("m:oMath") != nil {
			t.Fatalf("text block at position %d moved or became an equation", index)
		}
	}
}

func TestInvalidEquationLeavesNoOutput(t *testing.T) {
	for _, tc := range []struct {
		name string
		math string
		want string
	}{
		{name: "malformed XML", math: `<math><mi>x</math>`, want: "invalid MathML"},
		{name: "unsupported element", math: `<math><mstyle><mi>x</mi></mstyle></math>`, want: "unsupported MathML element/arity: mstyle"},
		{name: "empty expression", math: `<math/>`, want: "MathML expression is empty"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			output := filepath.Join(t.TempDir(), "invalid.docx")
			err := Create(Spec{Blocks: []Block{
				{Type: "paragraph", Text: "This must not be written"},
				{Type: "equation", MathML: tc.math},
			}}, "", output)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Create error = %v, want message containing %q", err, tc.want)
			}
			if _, statErr := os.Stat(output); !os.IsNotExist(statErr) {
				t.Fatalf("invalid equation left an output file (stat error: %v)", statErr)
			}
		})
	}
}

func elementTags(elements []*etree.Element) []string {
	tags := make([]string, 0, len(elements))
	for _, element := range elements {
		tags = append(tags, element.Tag)
	}
	return tags
}
