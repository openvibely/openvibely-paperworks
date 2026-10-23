package wordedit

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/openvibely/openvibely-paperworks/internal/opc"
)

func TestReplaceTextPreservesRenderedPageBreak(t *testing.T) {
	for _, tc := range []struct {
		name, runs, old, replacement string
		want                         []string
	}{
		{"before text", `<w:r><w:rPr><w:b/></w:rPr><w:lastRenderedPageBreak/><w:t>IBM Paver</w:t></w:r>`, "Paver", "Finn", []string{"b,lastRenderedPageBreak,t:IBM Finn"}},
		{"after text", `<w:r><w:rPr><w:b/></w:rPr><w:t>Paver</w:t><w:lastRenderedPageBreak/></w:r>`, "Paver", "Finn", []string{"b,t:Finn,lastRenderedPageBreak"}},
		{"across runs", bold("Pa") + `<w:r><w:lastRenderedPageBreak/><w:t>ver stays</w:t></w:r>`, "Paver", "Finn", []string{"b,t:Finn", "lastRenderedPageBreak,t: stays"}},
		{"newline", `<w:r><w:rPr><w:b/></w:rPr><w:lastRenderedPageBreak/><w:t>Paver</w:t></w:r>`, "Paver", "Fin\nn", []string{"b,lastRenderedPageBreak,t:Fin,br,t:n"}},
		{"tab with suffix marker", `<w:r><w:rPr><w:b/></w:rPr><w:t>IBM Paver end</w:t><w:lastRenderedPageBreak/></w:r>`, "Paver", "Fin\tn", []string{"b,t:IBM Fin,tab,t:n end,lastRenderedPageBreak"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := mk(t, "page-break", `<w:p>`+tc.runs+`</w:p><w:p>`+run("Unrelated Paver")+`</w:p>`, nil)
			out := filepath.Join(t.TempDir(), "out.docx")
			if _, err := Edit(src, out, map[string]any{"operation": "replace-text", "paragraph": float64(0), "old": tc.old, "new": tc.replacement}, ""); err != nil {
				t.Fatal(err)
			}
			ps := storyParas(t, out, "word/document.xml")
			referenceEqual(t, runShape(ps[0]), tc.want)
			referenceEqual(t, runShape(ps[1]), []string{"t:Unrelated Paver"})
			a, err := opc.Open(src)
			if err != nil {
				t.Fatal(err)
			}
			b, err := opc.Open(out)
			if err != nil {
				t.Fatal(err)
			}
			for _, n := range a.Names() {
				if n != "word/document.xml" && string(a.Get(n)) != string(b.Get(n)) {
					t.Fatal("unrelated part changed", n)
				}
			}
		})
	}
}

func TestRenderedPageBreakDoesNotMakeProtectedRunsEditable(t *testing.T) {
	marked := `<w:r><w:lastRenderedPageBreak/><w:t>Paver</w:t></w:r>`
	for _, body := range []string{
		`<w:p><w:hyperlink w:anchor="target">` + marked + `</w:hyperlink></w:p>`,
		`<w:p><w:ins w:id="1" w:author="Reviewer">` + marked + `</w:ins></w:p>`,
		`<w:p><w:fldSimple w:instr="DATE">` + marked + `</w:fldSimple></w:p>`,
		`<w:p><w:r><w:lastRenderedPageBreak/><w:br w:type="page"/><w:t>Paver</w:t></w:r></w:p>`,
	} {
		src := mk(t, "protected", body, nil)
		if _, err := Edit(src, filepath.Join(t.TempDir(), "out.docx"), map[string]any{"operation": "replace-text", "paragraph": float64(0), "old": "Paver", "new": "Finn"}, ""); err == nil || !strings.Contains(err.Error(), "requested occurrence not found") {
			t.Fatal("expected protected-content rejection", err, body)
		}
	}
}
