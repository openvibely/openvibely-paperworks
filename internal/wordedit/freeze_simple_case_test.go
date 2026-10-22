package wordedit

import (
	"strings"
	"testing"

	"github.com/openvibely/openvibely-paperworks/internal/office"
)

func boldSimpleField(instr, cached string) string {
	return `<w:fldSimple w:instr="` + instr + `"><w:r><w:rPr><w:b/></w:rPr><w:t xml:space="preserve">` + cached + `</w:t></w:r></w:fldSimple>`
}

func TestFreezeSimpleReferenceIsCaseInsensitive(t *testing.T) {
	for _, instr := range []string{`ref results \h`, `Ref results \h`, `pageref results \h`, `PageRef results \h`, ` REF results \h `} {
		t.Run(instr, func(t *testing.T) {
			src := mk(t, "simple-ref", `<w:p>`+run("See ")+boldSimpleField(instr, "12")+`</w:p>`, nil)
			out := mustEdit(t, src, map[string]any{"operation": "freeze-references"})
			if got := paras(t, out)[0]; got != "See 12" {
				t.Fatalf("%q", got)
			}
			doc := part(t, out, "word/document.xml")
			absent(t, doc, "w:fldSimple")
			absent(t, strings.ToLower(doc), "ref results")
			paragraph := office.Children(office.Child(referenceXML(t, out, "word/document.xml"), nsW, "body"), nsW, "p")[0]
			referenceRequireDirectRunProperty(t, paragraph, 1, "b")
		})
	}
}

func TestFreezeSimpleLowercaseReferenceWithoutCacheFails(t *testing.T) {
	src := mk(t, "simple-ref-empty", `<w:p>`+run("See ")+`<w:fldSimple w:instr="ref results \h"/></w:p>`, nil)
	wantErr(t, src, map[string]any{"operation": "freeze-references"}, "no cached result")
}

func TestFreezeSimpleNonReferenceFieldsUntouched(t *testing.T) {
	for _, instr := range []string{"REFERENCE x", "refx y", "SEQ Figure", "PAGE"} {
		t.Run(instr, func(t *testing.T) {
			src := mk(t, "simple-nonref", `<w:p>`+run("See ")+simpleField(instr, "7")+`</w:p>`, nil)
			out := mustEdit(t, src, map[string]any{"operation": "freeze-references"})
			doc := part(t, out, "word/document.xml")
			contains(t, doc, `w:fldSimple w:instr="`+instr+`"`)
		})
	}
}

func TestFreezeLowercaseReferenceSimpleComplexParity(t *testing.T) {
	const instr = `ref results \h`
	simple := mustEdit(t, mk(t, "parity-simple", `<w:p>`+run("See ")+simpleField(instr, "12")+`</w:p>`, nil), map[string]any{"operation": "freeze-references"})
	complexOut := mustEdit(t, mk(t, "parity-complex", `<w:p>`+run("See ")+complexField(" "+instr+" ", "12")+`</w:p>`, nil), map[string]any{"operation": "freeze-references"})
	got, want := paras(t, simple), paras(t, complexOut)
	referenceEqual(t, got, want)
	referenceEqual(t, got[0], "See 12")
}
