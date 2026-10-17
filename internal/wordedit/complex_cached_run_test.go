package wordedit

import "testing"

// Pinned reference DOCX library Run.text setter: empty text has no w:t, whitespace
// gets xml:space only when needed, and each CR/LF is a separate break.
func TestReferenceComplexCachedRun(t *testing.T) {
	for _, tc := range []struct{ text, body string }{
		{"", ""}, {"0", `<w:t>0</w:t>`},
		{" x ", `<w:t xml:space="preserve"> x </w:t>`},
		{"A\tB\nC\r\nD", `<w:t>A</w:t><w:tab/><w:t>B</w:t><w:br/><w:t>C</w:t><w:br/><w:br/><w:t>D</w:t>`},
		{"<&", `<w:t>&lt;&amp;</w:t>`},
	} {
		want := `<w:r><w:rPr><w:b/></w:rPr>` + tc.body + `</w:r>`
		if got := referenceComplexCachedRun(tc.text); got != want {
			t.Fatalf("%q: %s != %s", tc.text, got, want)
		}
	}
}
