package wordedit

import (
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"os"
	"path/filepath"
	"testing"
)

func TestHeaderFooterTablesPreserveBodyAndSource(t *testing.T) {
	src := mk(t, "header-table", `<w:p>`+run("Original body")+`</w:p>`, nil)
	original, _ := os.ReadFile(src)
	for _, kind := range []string{"header", "footer"} {
		t.Run(kind, func(t *testing.T) {
			out := filepath.Join(t.TempDir(), "out.docx")
			_, err := Edit(src, out, map[string]any{"operation": "header-footer", "section": 0.0, "kind": kind, "rows": [][]string{{"Left", "Middle\nNext", "Right"}}, "widths_twips": []int{2000, 2000, 2000}}, "")
			if err != nil {
				t.Fatal(err)
			}
			p, _ := opc.Open(out)
			d, err := p.XML("word/" + kind + "Edit1.xml")
			if err != nil {
				t.Fatal(err)
			}
			cells := all(d.Root(), "tc")
			if len(cells) != 3 || len(all(d.Root(), "br")) != 1 {
				t.Fatal("missing native table/break")
			}
			if all(cells[0], "t")[0].Text() != "Left" {
				t.Fatal("missing cell text")
			}
			if got := paras(t, out); len(got) != 1 || got[0] != "Original body" {
				t.Fatal(got)
			}
			rels, _ := p.Rels("word/document.xml")
			found := false
			for _, r := range rels {
				if r.Target == kind+"Edit1.xml" {
					found = true
				}
			}
			if !found {
				t.Fatal("missing relationship")
			}
		})
	}
	after, _ := os.ReadFile(src)
	if string(original) != string(after) {
		t.Fatal("source changed")
	}
	for _, extra := range []map[string]any{{"rows": [][]string{{"A", "B"}, {"C"}}, "widths_twips": []int{1000, 1000}}, {"rows": [][]string{{"A"}}, "widths_twips": []int{-1}}, {"rows": [][]string{{"A"}}, "widths_twips": []int{1000}, "text": "conflict"}} {
		extra["operation"] = "header-footer"
		extra["section"] = 0.0
		extra["kind"] = "footer"
		out := filepath.Join(t.TempDir(), "bad.docx")
		if _, err := Edit(src, out, extra, ""); err == nil {
			t.Fatal("accepted invalid table")
		}
		if _, err := os.Stat(out); !os.IsNotExist(err) {
			t.Fatal("published failed table")
		}
	}
}
