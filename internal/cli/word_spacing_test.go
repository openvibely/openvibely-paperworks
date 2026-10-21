package cli

import (
	"bytes"
	"context"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/beevik/etree"
	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"github.com/openvibely/openvibely-paperworks/internal/pdf"
)

func TestWordLineSpacingUsesTwipsThroughCLI(t *testing.T) {
	dir := t.TempDir()
	var blocks, edits []any
	rules := []string{"", "auto", "atLeast", "exact"}
	for i := range rules {
		size := 18
		if rules[i] == "atLeast" {
			size = 30 // Minimum spacing would expand beyond the requested 24pt.
		}
		blocks = append(blocks, map[string]any{"type": "paragraph", "runs": []any{map[string]any{"text": string(rune('A' + i)), "size": size}}})
		edits = append(edits, map[string]any{"operation": "paragraph-format", "paragraph": i, "line": 480})
	}
	blocks = append(blocks, map[string]any{"type": "paragraph", "text": "Keep this paragraph"})
	spec := writeSpec(t, dir, "create.json", map[string]any{"font": "Arial", "font_size": 18, "blocks": blocks})
	base := filepath.Join(dir, "base.docx")
	runCLI(t, "create-docx", "--spec", spec, "--output", base)
	pkg, err := opc.Open(base)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := pkg.XML("word/document.xml")
	if err != nil {
		t.Fatal(err)
	}
	paras := doc.FindElements("//w:body/w:p")
	for i, rule := range rules {
		p := paras[i]
		pr := office.Child(p, office.NSW, "pPr")
		if pr == nil {
			pr = p.CreateElement("w:pPr")
			p.RemoveChild(pr)
			p.InsertChildAt(0, pr)
		}
		if rule != "" {
			spacing := pr.CreateElement("w:spacing")
			spacing.CreateAttr("w:line", "720")
			spacing.CreateAttr("w:lineRule", rule)
		}
		run := office.Child(p, office.NSW, "r")
		for j := 0; j < 2; j++ {
			run.CreateElement("w:br")
			run.CreateElement("w:t").SetText(string(rune('A' + i)))
		}
	}
	if err := pkg.SetXML("word/document.xml", doc); err != nil {
		t.Fatal(err)
	}
	in, out := filepath.Join(dir, "input.docx"), filepath.Join(dir, "out.docx")
	if err := pkg.SaveNew(in); err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile(in)
	if err != nil {
		t.Fatal(err)
	}
	spec = writeSpec(t, dir, "edit.json", map[string]any{"operation": "batch", "edits": edits})
	runCLI(t, "word-edit", in, "--spec", spec, "--output", out)
	result := runCLI(t, "convert", out, "--output", filepath.Join(dir, "converted"))
	data, err := os.ReadFile(result["output"].(string))
	if err != nil {
		t.Fatal(err)
	}
	pages, err := pdf.ExtractLayout(context.Background(), data, pdf.LayoutOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for i, rule := range rules {
		var tops []float64
		for _, page := range pages {
			for _, c := range page.Characters {
				if c.Text == string(rune('A'+i)) {
					tops = append(tops, c.Top)
				}
			}
		}
		if len(tops) != 3 {
			t.Fatalf("rule %q: expected three lines, got %v", rule, tops)
		}
		for j := 1; j < len(tops); j++ {
			if gap := tops[j-1] - tops[j]; math.Abs(gap-24) > 0.5 {
				t.Errorf("rule %q: line spacing is %.2f points, want 24 (480 twips)", rule, gap)
			}
		}
	}
	after, err := os.ReadFile(in)
	if err != nil || !bytes.Equal(original, after) {
		t.Fatal("input changed", err)
	}
	edited, err := opc.Open(out)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range pkg.Names() {
		if name != "word/document.xml" && !bytes.Equal(pkg.Get(name), edited.Get(name)) {
			t.Fatal("unrelated part changed", name)
		}
	}
	updated, err := edited.XML("word/document.xml")
	if err != nil {
		t.Fatal(err)
	}
	serialize := func(p *etree.Element) string {
		d := etree.NewDocument()
		d.SetRoot(p.Copy())
		s, err := d.WriteToString()
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	if serialize(updated.FindElements("//w:body/w:p")[4]) != serialize(paras[4]) {
		t.Fatal("untouched paragraph changed")
	}
}
