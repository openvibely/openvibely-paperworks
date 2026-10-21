package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/openvibely/openvibely-paperworks/internal/pdf"
)

func TestWordSectionBreakPaginationThroughCLI(t *testing.T) {
	for _, tc := range []struct {
		kind       string
		secondPage int
	}{
		{"continuous", 1}, {"nextPage", 2}, {"", 2}, {"evenPage", 2}, {"oddPage", 3},
	} {
		t.Run(tc.kind, func(t *testing.T) {
			dir := t.TempDir()
			in, out := filepath.Join(dir, "in.docx"), filepath.Join(dir, "out.docx")
			spec := writeSpec(t, dir, "create.json", map[string]any{"page_numbers": true, "blocks": []any{
				map[string]any{"type": "paragraph", "text": "A"},
				map[string]any{"type": "paragraph", "text": "B"},
			}})
			runCLI(t, "create-docx", "--spec", spec, "--output", in)
			original, err := os.ReadFile(in)
			if err != nil {
				t.Fatal(err)
			}
			edit := map[string]any{"operation": "section-insert", "section": 0, "paragraph": 0}
			if tc.kind != "" {
				edit["type"] = tc.kind
			}
			spec = writeSpec(t, dir, "edit.json", edit)
			runCLI(t, "word-edit", in, "--spec", spec, "--output", out)
			result := runCLI(t, "convert", out, "--output", filepath.Join(dir, "pdf"))
			raw, err := os.ReadFile(result["output"].(string))
			if err != nil {
				t.Fatal(err)
			}
			pages, err := pdf.ExtractLayout(context.Background(), raw, pdf.LayoutOptions{})
			if err != nil {
				t.Fatal(err)
			}
			positions := map[string][]int{}
			for _, page := range pages {
				for _, c := range page.Characters {
					positions[c.Text] = append(positions[c.Text], page.Page)
				}
			}
			// LibreOffice can omit blank pages inserted for odd-page starts.
			// Verify logical page numbers from the rendered PAGE footer fields.
			for text, want := range map[string]int{"A": 1, "B": tc.secondPage} {
				got := positions[text]
				number := positions[strconv.Itoa(want)]
				if len(got) != 1 || len(number) != 1 || got[0] != number[0] {
					t.Errorf("%s appears on PDF pages %v; expected on the page numbered %d, found on %v", text, got, want, number)
				}
			}
			after, err := os.ReadFile(in)
			if err != nil || !bytes.Equal(original, after) {
				t.Fatal("input changed", err)
			}
		})
	}
}
