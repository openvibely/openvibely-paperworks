package wordedit

import (
	"bytes"
	"github.com/beevik/etree"
	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const footerRel = `<Relationship Id="rIdF" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/footer" Target="footer1.xml"/>`

func simpleField(instr, cached string) string {
	return `<w:fldSimple w:instr="` + instr + `">` + run(cached) + `</w:fldSimple>`
}

func TestMaterializeMixedFieldsKeepsFormat(t *testing.T) {
	referenceWordCaller(t)
	src := referenceOriginalWordFixture(t, "mixed-fields")
	out := mustEdit(t, src, map[string]any{"operation": "materialize"})
	referenceEqual(t, referenceStoryTexts(t, out, "word/document.xml", true), []string{"See 2", "Figure 1", "Figure 2 retained", "Again 2"})
	if h := referenceStoryTexts(t, out, "word/header1.xml", true)[0]; h != "2" {
		t.Fatalf("header %q", h)
	}
	if f := referenceStoryTexts(t, out, "word/footer1.xml", true)[0]; f != "9" {
		t.Fatalf("footer %q", f)
	}
	// UNRUN correction: Reference checks formatting on specific direct runs.
	bodyNode := office.Child(referenceXML(t, out, "word/document.xml"), nsW, "body")
	paragraphs := office.Children(bodyNode, nsW, "p")
	if len(paragraphs) != 4 {
		t.Fatalf("paragraph count: %d", len(paragraphs))
	}
	runs := office.Children(paragraphs[2], nsW, "r")
	foundResult := false
	for _, r := range runs {
		if referenceTexts(r) == "2" {
			pr := office.Child(r, nsW, "rPr")
			b := office.Child(pr, nsW, "b")
			if b == nil || (office.AttrV(b, nsW, "val") != "" && office.AttrV(b, nsW, "val") != "1" && office.AttrV(b, nsW, "val") != "true" && office.AttrV(b, nsW, "val") != "on") {
				t.Fatal("first result run is not explicitly bold")
			}
			foundResult = true
			break
		}
	}
	if !foundResult || len(runs) == 0 {
		t.Fatal("missing result run")
	}
	i := office.Child(office.Child(runs[len(runs)-1], nsW, "rPr"), nsW, "i")
	if i == nil || (office.AttrV(i, nsW, "val") != "" && office.AttrV(i, nsW, "val") != "1" && office.AttrV(i, nsW, "val") != "true" && office.AttrV(i, nsW, "val") != "on") {
		t.Fatal("last run is not explicitly italic")
	}
	for _, name := range []string{"word/document.xml", "word/header1.xml"} {
		var oldCode, newCode []string
		for _, e := range office.All(referenceXML(t, src, name), nsW, "instrText") {
			oldCode = append(oldCode, e.Text())
		}
		for _, e := range office.All(referenceXML(t, out, name), nsW, "instrText") {
			newCode = append(newCode, e.Text())
		}
		referenceEqual(t, newCode, oldCode)
		referenceEqual(t, len(office.All(referenceXML(t, out, name), nsW, "fldChar")), len(office.All(referenceXML(t, src, name), nsW, "fldChar")))
	}
}

func TestMaterializeInvalidComplexFieldDoesNotPublish(t *testing.T) {
	referenceWordCaller(t)
	cases := map[string]string{
		"unterminated":   strings.Replace(complexField(" SEQ Figure ", "0"), `<w:fldChar w:fldCharType="end"/>`, "", 1),
		"missing-target": complexField(" REF absent ", "0"),
		"switch":         complexField(" SEQ Figure \\r 9 ", "0"),
		"nested":         strings.Replace(complexField(" SEQ Figure ", "0"), `<w:r><w:fldChar w:fldCharType="separate"/></w:r>`, `<w:r><w:fldChar w:fldCharType="separate"/></w:r><w:fldSimple w:instr="SEQ Other"/>`, 1),
	}
	for _, name := range []string{"unterminated", "missing-target", "switch", "nested"} {
		field := cases[name]
		t.Run(name, func(t *testing.T) {
			wantErr(t, mk(t, name, `<w:p>`+field+`</w:p>`, nil), map[string]any{"operation": "materialize"}, ".")
		})
	}
}

func TestMaterializeEmptyResultAndCrossParagraphBookmark(t *testing.T) {
	referenceWordCaller(t)
	src := referenceOriginalWordFixture(t, "empty-cross-field")
	out := mustEdit(t, src, map[string]any{"operation": "materialize"})
	ps := paras(t, out)
	if ps[len(ps)-1] != "See First second!" || ps[0] != "First " {
		t.Fatalf("%q", ps)
	}
}

func TestCommentSubstringMatchesTrackedTextAndTables(t *testing.T) {
	referenceWordCaller(t)
	src := referenceOriginalWordFixture(t, "comment-pattern")
	out := mustEdit(t, src, map[string]any{"operation": "comment", "contains": "price", "ignore_case": true, "all_matches": true, "comment": "Check", "author": "Reviewer"})
	root := referenceXML(t, out, "word/document.xml")
	referenceEqual(t, len(office.Children(referenceXML(t, out, "word/comments.xml"), nsW, "comment")), 2)
	for _, comment := range office.Children(referenceXML(t, out, "word/comments.xml"), nsW, "comment") {
		referenceEqual(t, office.AttrV(comment, nsW, "initials"), "")
	}
	referenceEqual(t, len(office.All(root, nsW, "commentRangeStart")), 2)
	referenceEqual(t, office.All(root, nsW, "delText")[0].Text(), "Old PRICE")
}

func TestNavigationIncludesOutlineHeadingsAndCaptions(t *testing.T) {
	referenceWordCaller(t)
	src := referenceOriginalWordFixture(t, "outline-navigation")
	src = mustEdit(t, src, map[string]any{"operation": "caption", "label": "Table", "text": "Totals"})
	src = mustEdit(t, src, map[string]any{"operation": "materialize"})
	out := mustEdit(t, src, map[string]any{"operation": "navigation"})
	// UNRUN correction: Reference tests membership/count of whole hyperlink labels.
	var labels []string
	for _, link := range office.All(referenceXML(t, out, "word/document.xml"), nsW, "hyperlink") {
		labels = append(labels, referenceTexts(link))
	}
	for _, want := range []string{"Custom heading", "Figure 1: Results", "Table 1: Totals"} {
		found := false
		for _, label := range labels {
			if label == want {
				found = true
			}
		}
		if !found {
			t.Fatalf("missing exact label %q in %q", want, labels)
		}
	}
	back := 0
	for _, label := range labels {
		if label == "Back to contents" {
			back++
		}
	}
	referenceEqual(t, back, 3)
}

func TestDotxStylePackPreservesDocumentPart(t *testing.T) {
	referenceWordCaller(t)
	src := referenceOriginalWordFixture(t, "base")
	template := referenceOriginalWordFixture(t, "style-template")
	raw, err := os.ReadFile(template)
	if err != nil {
		t.Fatal(err)
	}
	dotx := filepath.Join(t.TempDir(), "template.dotx")
	if err := os.WriteFile(dotx, raw, 0600); err != nil {
		t.Fatal(err)
	}
	beforeSource, _ := os.ReadFile(src)
	beforeTemplate, _ := os.ReadFile(dotx)
	out := mustEdit(t, src, map[string]any{"operation": "style-pack", "template": dotx})
	// UNRUN correction: resolve Normal specifically, not any size in styles.xml.
	var normalSize string
	for _, style := range office.Children(referenceXML(t, out, "word/styles.xml"), nsW, "style") {
		if office.AttrV(style, nsW, "styleId") == "Normal" {
			size := office.Child(office.Child(style, nsW, "rPr"), nsW, "sz")
			normalSize = office.AttrV(size, nsW, "val")
			break
		}
	}
	referenceEqual(t, normalSize, "30")
	for _, name := range []string{"word/document.xml", "docProps/core.xml"} {
		if part(t, out, name) != part(t, src, name) {
			t.Fatalf("%s changed", name)
		}
	}
	afterSource, _ := os.ReadFile(src)
	afterTemplate, _ := os.ReadFile(dotx)
	if !bytes.Equal(beforeSource, afterSource) || !bytes.Equal(beforeTemplate, afterTemplate) {
		t.Fatal("style import modified source or template")
	}
	contains(t, part(t, out, "[Content_Types].xml"), "wordprocessingml.document.main+xml")
	if strings.Contains(part(t, out, "[Content_Types].xml"), "wordprocessingml.template.main+xml") {
		t.Fatal("output document acquired template content type")
	}

}

func TestRedlineCrossRunAcceptRejectPreserveFormat(t *testing.T) {
	referenceWordCaller(t)
	revised := mustEdit(t, referenceOriginalWordFixture(t, "base"), map[string]any{"operation": "redline", "old": "old price", "new": "new price", "author": "Reviewer"})
	root := referenceXML(t, revised, "word/document.xml")
	if len(office.All(root, nsW, "ins")) != 1 || len(office.All(root, nsW, "del")) != 1 {
		t.Fatal("expected one insertion and one deletion")
	}
	acc := mustEdit(t, revised, map[string]any{"operation": "accept"})
	rej := mustEdit(t, revised, map[string]any{"operation": "reject"})
	if paras(t, acc)[0] != "The new price is $10." || paras(t, rej)[0] != "The old price is $10." {
		t.Fatalf("%q / %q", paras(t, acc)[0], paras(t, rej)[0])
	}
	a := part(t, acc, "word/document.xml")
	contains(t, a, "<w:b/>")
	contains(t, a, "<w:i/>")
	// Reference uses direct runs and on/off values; batch25 verifies this caller.
	runs := office.Children(office.All(referenceXML(t, acc, "word/document.xml"), nsW, "p")[0], nsW, "r")
	if len(runs) == 0 {
		t.Fatal("accepted paragraph has no direct runs")
	}
	for _, check := range []struct {
		index int
		tag   string
	}{{0, "b"}, {len(runs) - 1, "i"}} {
		property := office.Child(office.Child(runs[check.index], nsW, "rPr"), nsW, check.tag)
		if property == nil {
			t.Fatalf("run %d lacks %s", check.index, check.tag)
		}
		value := office.AttrV(property, nsW, "val")
		if value != "" && value != "1" && value != "true" && value != "on" {
			t.Fatalf("run %d %s is not enabled: %q", check.index, check.tag, value)
		}
	}
	absent(t, a, "<w:del ")
	referenceEqual(t, len(office.All(referenceXML(t, acc, "word/document.xml"), nsW, "del")), 0)
}

func TestAmbiguousEditFailsWithoutFile(t *testing.T) {
	referenceWordCaller(t)
	wantErr(t, base(t), map[string]any{"operation": "redline", "old": "not there", "new": "x", "author": "Reviewer"}, ".")
}

func TestNontextRevisionNotSilentlyAccepted(t *testing.T) {
	referenceWordCaller(t)
	src := referenceRewrite(t, base(t), func(pkg *opc.Package) {
		referenceSetXML(t, pkg, "word/document.xml", func(root *etree.Element) {
			paragraph := office.Child(office.Child(root, nsW, "body"), nsW, "p")
			props := office.Child(paragraph, nsW, "pPr")
			if props == nil {
				props = etree.NewElement("w:pPr")
				paragraph.InsertChildAt(0, props)
			}
			props.CreateElement("w:pPrChange")
		})
	})
	wantErr(t, src, map[string]any{"operation": "accept"}, ".")
}

func TestCommentsAreRealComments(t *testing.T) {
	referenceWordCaller(t)
	out := mustEdit(t, base(t), map[string]any{"operation": "comment", "text": "The old price is $10.", "comment": "Check the price", "author": "Reviewer"})
	// UNRUN correction: compare the first comment's complete text, not a substring.
	comments := office.Children(referenceXML(t, out, "word/comments.xml"), nsW, "comment")
	if len(comments) == 0 {
		t.Fatal("missing comment")
	}
	referenceEqual(t, len(office.Children(comments[0], nsW, "p")), 1)
	for _, tag := range []string{"tab", "br", "cr"} {
		referenceEqual(t, len(office.All(comments[0], nsW, tag)), 0)
	}
	referenceEqual(t, referenceCommentText(comments[0]), "Check the price")
	doc := referenceXML(t, out, "word/document.xml")
	referenceEqual(t, len(office.All(doc, nsW, "commentRangeStart")), 1)
	referenceEqual(t, len(office.All(doc, nsW, "commentReference")), 1)
}

func TestNativeEquationAndUnsupportedMath(t *testing.T) {
	referenceWordCaller(t)
	out := mustEdit(t, base(t), map[string]any{"operation": "equation", "mathml": "<math><mfrac><msup><mi>x</mi><mn>2</mn></msup><mi>y</mi></mfrac></math>"})
	doc := referenceXML(t, out, "word/document.xml")
	referenceEqual(t, len(office.All(doc, nsM, "f")), 1)
	referenceEqual(t, len(office.All(doc, nsM, "sSup")), 1)
	referenceEqual(t, len(office.All(doc, nsW, "drawing")), 0)
	wantErr(t, base(t), map[string]any{"operation": "equation", "mathml": "<math><unknown/></math>"}, ".")
}

func TestEditableMatrixAndRoots(t *testing.T) {
	referenceWordCaller(t)
	out := mustEdit(t, base(t), map[string]any{"operation": "equation", "mathml": `<math><mfenced open="[" close="]"><mtable><mtr><mtd><mn>1</mn></mtd><mtd><mn>2</mn></mtd></mtr><mtr><mtd><mn>3</mn></mtd><mtd><mroot><mi>x</mi><mn>3</mn></mroot></mtd></mtr></mtable></mfenced><mo>+</mo><msqrt><mi>x</mi><mo>+</mo><mn>1</mn></msqrt></math>`})
	doc := referenceXML(t, out, "word/document.xml")
	referenceEqual(t, len(office.All(doc, nsM, "mr")), 2)
	referenceEqual(t, len(office.All(doc, nsM, "rad")), 2)
}
