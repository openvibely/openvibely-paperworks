package wordedit

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParagraphDeletePreservesFieldBoundaries(t *testing.T) {
	begin := `<w:r><w:fldChar w:fldCharType="begin"/></w:r>`
	separator := `<w:r><w:fldChar w:fldCharType="separate"/></w:r>`
	end := `<w:r><w:fldChar w:fldCharType="end"/></w:r>`
	paragraph := func(text string) string { return `<w:p>` + text + `</w:p>` }
	for _, tc := range []struct {
		name, body string
		index      int
		reject     bool
	}{
		{"opening", paragraph(begin+separator+run("First")) + paragraph(run("Second")+end), 0, true},
		{"closing", paragraph(begin+separator+run("First")) + paragraph(run("Second")+end), 1, true},
		// Equal numbers of starts and ends are not enough: they must pair
		// within the deleted paragraph, not with neighboring paragraphs.
		{"closing then opening", paragraph(begin+separator) + paragraph(end+begin+separator) + paragraph(end), 1, true},
		{"external separator", paragraph(begin) + paragraph(separator+run("Result")) + paragraph(end), 1, true},
		{"nested boundary", paragraph(begin+separator) + paragraph(begin+separator+run("Nested")+end+end), 1, true},
		{"complete field", paragraph(begin + separator + run("Field") + end), 0, false},
		{"complete nested fields", paragraph(begin + separator + begin + separator + run("Nested") + end + end), 0, false},
		{"simple field", paragraph(`<w:fldSimple w:instr="DATE">` + run("Date") + `</w:fldSimple>`), 0, false},
		{"ordinary paragraph", paragraph(run("Remove me")), 0, false},
		{"middle of result", paragraph(begin+separator+run("First")) + paragraph(run("Remove me")) + paragraph(run("Last")+end), 1, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := mk(t, "fields", tc.body+paragraph(run("Unrelated text")), nil)
			original, err := os.ReadFile(source)
			if err != nil {
				t.Fatal(err)
			}
			out := filepath.Join(t.TempDir(), "deleted.docx")
			_, err = Edit(source, out, map[string]any{"operation": "paragraph-delete", "paragraph": float64(tc.index)}, "")
			if tc.reject {
				if err == nil || !strings.Contains(err.Error(), "field") {
					t.Fatalf("expected field boundary rejection, got %v", err)
				}
				if _, err := os.Stat(out); !os.IsNotExist(err) {
					t.Fatal("rejected edit published output", err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			input := out
			if tc.reject {
				input = source
			}
			ps := paras(t, input)
			if !tc.reject && len(ps) != len(paras(t, source))-1 {
				t.Fatal("paragraph was not deleted")
			}
			// After either a safe deletion or rejection, ordinary following
			// text must remain editable rather than becoming part of a field.
			replaced := filepath.Join(t.TempDir(), "replaced.docx")
			_, err = Edit(input, replaced, map[string]any{"operation": "replace-text", "paragraph": float64(len(ps) - 1), "old": "Unrelated", "new": "Updated"}, "")
			if err != nil {
				t.Fatal("following text became uneditable", err)
			}
			got := paras(t, replaced)
			if got[len(got)-1] != "Updated text" {
				t.Fatal(got)
			}
			after, err := os.ReadFile(source)
			if err != nil || !bytes.Equal(original, after) {
				t.Fatal("source changed", err)
			}
		})
	}
}

func TestParagraphDeleteFieldBoundaryAbortsBatch(t *testing.T) {
	source := mk(t, "fields", `<w:p>`+run("Original")+`</w:p><w:p><w:r><w:fldChar w:fldCharType="begin"/></w:r></w:p><w:p><w:r><w:fldChar w:fldCharType="end"/></w:r></w:p>`, nil)
	original, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "out.docx")
	_, err = Edit(source, out, map[string]any{"operation": "batch", "edits": []any{
		map[string]any{"operation": "replace-text", "paragraph": float64(0), "old": "Original", "new": "Changed"},
		map[string]any{"operation": "paragraph-delete", "paragraph": float64(2)},
	}}, "")
	if err == nil || !strings.Contains(err.Error(), "field") {
		t.Fatalf("expected rejection, got %v", err)
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Fatal("failed batch published output", err)
	}
	after, err := os.ReadFile(source)
	if err != nil || !bytes.Equal(original, after) {
		t.Fatal("source changed", err)
	}
}
