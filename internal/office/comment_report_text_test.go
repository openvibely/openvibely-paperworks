package office

import (
	"bytes"
	"github.com/beevik/etree"
	"path/filepath"
	"testing"
)

// Pinned document_native.presentation_comments uses paragraph itertext(),
// not the slide text-frame soft-break model. Oracle: Batch42 comment-text.
func TestCommentReportXMLText(t *testing.T) {
	for _, tc := range []struct{ body, want string }{
		{"<a:r><a:t>Before</a:t></a:r><a:br/><a:r><a:t>after</a:t></a:r>", "Beforeafter"},
		{"lead<a:r><a:t>run</a:t>tail</a:r>end", "leadruntailend"},
		{"<a:r><a:t>A</a:t></a:r><!--ignored-->B<?probe ignored?>C<a:fld><a:t>D</a:t></a:fld>", "ABCD"},
		{"<a:r><a:t>one</a:t></a:r></a:p><a:p><a:r><a:t>two</a:t></a:r>", "one\ntwo"},
	} {
		t.Run(tc.want, func(t *testing.T) {
			p := fixture(t, "reference-comments.pptx")
			const part = "ppt/comments/modernComment.xml"
			d, err := p.XML(part)
			if err != nil {
				t.Fatal(err)
			}
			cm := Child(d.Root(), nsModern, "cm")
			body := etree.NewDocument()
			if err := body.ReadFromString(`<m:txBody xmlns:m="` + nsModern + `" xmlns:a="` + NSA + `"><a:bodyPr/><a:lstStyle/><a:p>` + tc.body + `</a:p></m:txBody>`); err != nil {
				t.Fatal(err)
			}
			cm.RemoveChild(Child(cm, nsModern, "txBody"))
			cm.AddChild(body.Root().Copy())
			if err := p.SetXML(part, d); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(t.TempDir(), "comment.pptx")
			if err := p.SaveNew(path); err != nil {
				t.Fatal(err)
			}
			p = previewOpen(t, path)
			before := append([]byte(nil), p.Get(part)...)
			got, err := SlideComments(p, SlideCommentSpec{})
			if err != nil || len(got) == 0 {
				t.Fatal(got, err)
			}
			if got[0].Text != tc.want {
				t.Errorf("text=%q want %q", got[0].Text, tc.want)
			}
			if !bytes.Equal(before, p.Get(part)) {
				t.Fatal("listing mutated comment XML")
			}
		})
	}
}
