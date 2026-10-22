package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"testing"

	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
)

func TestLineSpacingThroughCLI(t *testing.T) {
	dir := t.TempDir()
	in, out := filepath.Join(dir, "in.pptx"), filepath.Join(dir, "out.pptx")
	spec := writeSpec(t, dir, "create.json", map[string]any{"slides": []any{map[string]any{"elements": []any{map[string]any{"type": "text", "w": 4, "h": 2, "paragraphs": []any{map[string]any{"text": "Keep this text", "format": map[string]any{"line_spacing": 0.92, "after": 12}}}}}}}})
	runCLI(t, "create-pptx", "--spec", spec, "--output", in)
	check := func(path, want string) {
		t.Helper()
		p, err := opc.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		x, err := p.XML("ppt/slides/slide1.xml")
		if err != nil {
			t.Fatal(err)
		}
		pr := x.FindElement("//a:pPr")
		if pr == nil {
			t.Fatal("no paragraph properties")
		}
		line := pr.FindElement("a:lnSpc/a:spcPct")
		if line == nil || line.SelectAttrValue("val", "") != want {
			t.Fatal("incorrect line spacing", line)
		}
		if len(pr.FindElements("a:lnSpc")) != 1 {
			t.Fatal("duplicate line spacing")
		}
		after := pr.FindElement("a:spcAft/a:spcPts")
		if after == nil || after.SelectAttrValue("val", "") != "1200" {
			t.Fatal("paragraph spacing lost")
		}
		if office.Text(x.Root(), office.NSA, "t") != "Keep this text" {
			t.Fatal("text changed")
		}
	}
	check(in, "92000")
	edits := writeSpec(t, dir, "edit.json", map[string]any{"edits": []any{map[string]any{"operation": "paragraph-format", "slide": 1, "shape_id": "2", "paragraph_style": map[string]any{"line_spacing": 1.2}}}})
	runCLI(t, "slide-edit", in, "--spec", edits, "--output", out)
	check(out, "120000")
	check(in, "92000")
	for _, v := range []float64{0, -1, 0.000001, 1001} {
		bad := writeSpec(t, dir, "bad.json", map[string]any{"edits": []any{map[string]any{"operation": "paragraph-format", "slide": 1, "shape_id": "2", "paragraph_style": map[string]any{"line_spacing": v}}}})
		badOut := filepath.Join(dir, "bad.pptx")
		var stdout, stderr bytes.Buffer
		if Main([]string{"slide-edit", in, "--spec", bad, "--output", badOut}, &stdout, &stderr) == 0 {
			t.Fatalf("accepted %v", v)
		}
		if _, err := os.Stat(badOut); !os.IsNotExist(err) {
			t.Fatal("invalid spacing published output")
		}
	}
	for _, cmd := range []string{"create-pptx", "slide-edit"} {
		schema := runCLI(t, "schema", cmd)
		found := false
		for _, definition := range schema["$defs"].(map[string]any) {
			props, _ := definition.(map[string]any)["properties"].(map[string]any)
			if props["line_spacing"] != nil {
				found = true
			}
		}
		if !found {
			t.Fatal("line_spacing missing from schema", cmd)
		}
	}
}

func TestUpdateFieldsThroughCLI(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "in.docx")
	spec := writeSpec(t, dir, "create.json", map[string]any{"blocks": []any{map[string]any{"type": "paragraph", "text": "Keep original content"}}})
	runCLI(t, "create-docx", "--spec", spec, "--output", in)
	request := writeSpec(t, dir, "request.json", map[string]any{"operation": "update-fields"})
	// Cover inserting the flag, repeating it, enabling a false flag, and missing settings.
	for i := 0; i < 4; i++ {
		out := filepath.Join(t.TempDir(), "out.docx")
		before, err := opc.Open(in)
		if err != nil {
			t.Fatal(err)
		}
		if i == 2 {
			x, err := before.XML("word/settings.xml")
			if err != nil {
				t.Fatal(err)
			}
			office.Child(x.Root(), office.NSW, "updateFields").CreateAttr("w:val", "false")
			if err := before.SetXML("word/settings.xml", x); err != nil {
				t.Fatal(err)
			}
		}
		if i == 3 {
			before.Remove("word/settings.xml")
			rels, err := before.XML("word/_rels/document.xml.rels")
			if err != nil {
				t.Fatal(err)
			}
			for _, r := range rels.Root().ChildElements() {
				if r.SelectAttrValue("Type", "") == "http://schemas.openxmlformats.org/officeDocument/2006/relationships/settings" {
					rels.Root().RemoveChild(r)
				}
			}
			if err := before.SetXML("word/_rels/document.xml.rels", rels); err != nil {
				t.Fatal(err)
			}
			ct, err := before.XML("[Content_Types].xml")
			if err != nil {
				t.Fatal(err)
			}
			for _, r := range ct.Root().ChildElements() {
				if r.SelectAttrValue("PartName", "") == "/word/settings.xml" {
					ct.Root().RemoveChild(r)
				}
			}
			if err := before.SetXML("[Content_Types].xml", ct); err != nil {
				t.Fatal(err)
			}
		}
		if i >= 2 {
			in = filepath.Join(t.TempDir(), "modified.docx")
			if err := before.SaveNew(in); err != nil {
				t.Fatal(err)
			}
		}
		original, err := os.ReadFile(in)
		if err != nil {
			t.Fatal(err)
		}
		runCLI(t, "word-edit", in, "--spec", request, "--output", out)
		unchanged, err := os.ReadFile(in)
		if err != nil || !bytes.Equal(original, unchanged) {
			t.Fatal("source modified")
		}

		after, err := opc.Open(out)
		if err != nil {
			t.Fatal(err)
		}
		settings, err := after.XML("word/settings.xml")
		if err != nil {
			t.Fatal(err)
		}
		flags := office.Children(settings.Root(), office.NSW, "updateFields")
		if len(flags) != 1 || office.AttrV(flags[0], office.NSW, "val") != "true" {
			t.Fatal("refresh flag missing/duplicated")
		}
		if compat := office.Child(settings.Root(), office.NSW, "compat"); compat != nil && flags[0].Index() > compat.Index() {
			t.Fatal("updateFields in wrong schema order")
		}
		rels, err := after.Rels("word/document.xml")
		if err != nil {
			t.Fatal(err)
		}
		linked := false
		for _, rel := range rels {
			if rel.Type == "http://schemas.openxmlformats.org/officeDocument/2006/relationships/settings" && opc.Resolve("word/document.xml", rel.Target) == "word/settings.xml" {
				linked = true
			}
		}
		if !linked || after.ContentType("word/settings.xml") != "application/vnd.openxmlformats-officedocument.wordprocessingml.settings+xml" {
			t.Fatal("settings part not registered")
		}
		for _, part := range before.Parts {
			allowed := part.Name == "word/settings.xml" || (!before.Has("word/settings.xml") && (part.Name == "[Content_Types].xml" || part.Name == "word/_rels/document.xml.rels"))
			if !allowed && (!after.Has(part.Name) || !bytes.Equal(part.Data, after.Get(part.Name))) {
				t.Fatal("unrelated part changed", part.Name)
			}
		}
		in = out
	}
	schema := runCLI(t, "schema", "word-edit")
	operations := schema["properties"].(map[string]any)["operation"].(map[string]any)["enum"].([]any)
	found := false
	for _, op := range operations {
		if op == "update-fields" {
			found = true
		}
	}
	if !found {
		t.Fatal("operation not discoverable")
	}
}

func TestCreateLayoutUsesFinalParagraphSpacing(t *testing.T) {
	dir := t.TempDir()
	layout := map[string]any{"x": 1, "y": 1, "width": 3, "height": 6, "root": map[string]any{
		"mode": "column", "gap": 0.2, "children": []any{
			map[string]any{"shape_id": "2", "fit_text": true},
			map[string]any{"shape_id": "3", "fit_text": true},
		},
	}}
	geometry := func(path string) [][2]int64 {
		t.Helper()
		p, err := opc.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		x, err := p.XML("ppt/slides/slide1.xml")
		if err != nil {
			t.Fatal(err)
		}
		var result [][2]int64
		for _, shape := range x.FindElements("//p:sp") {
			transform := shape.FindElement("p:spPr/a:xfrm")
			if transform == nil {
				t.Fatal("missing shape geometry")
			}
			y, err := strconv.ParseInt(transform.FindElement("a:off").SelectAttrValue("y", ""), 10, 64)
			if err != nil {
				t.Fatal(err)
			}
			height, err := strconv.ParseInt(transform.FindElement("a:ext").SelectAttrValue("cy", ""), 10, 64)
			if err != nil {
				t.Fatal(err)
			}
			result = append(result, [2]int64{y, height})
		}
		return result
	}
	var previous [][2]int64
	for i, spacing := range []float64{1, 2} {
		spec := writeSpec(t, dir, "create-layout.json", map[string]any{"font": "Go", "slides": []any{map[string]any{
			"layout": layout, "elements": []any{
				map[string]any{"type": "text", "font_size": 24, "w": 3, "h": 1, "paragraphs": []any{map[string]any{
					"text":   "A long title with many words that needs several lines in a narrow column.",
					"format": map[string]any{"line_spacing": spacing, "before": 6, "after": 12},
				}}},
				map[string]any{"type": "text", "font_size": 12, "w": 3, "h": 1, "text": "Following paragraph"},
			},
		}}})
		in := filepath.Join(dir, strconv.Itoa(i)+".pptx")
		runCLI(t, "create-pptx", "--spec", spec, "--output", in)
		got := geometry(in)
		if len(got) != 2 {
			t.Fatal("expected two text boxes", got)
		}
		if got[1][0] < got[0][0]+got[0][1]+182879 {
			t.Fatal("following text overlaps or lacks gap", got)
		}
		edits := writeSpec(t, dir, "layout.json", map[string]any{"edits": []any{map[string]any{"operation": "layout", "slide": 1, "layout": layout}}})
		out := filepath.Join(dir, strconv.Itoa(i)+"-reapplied.pptx")
		runCLI(t, "slide-edit", in, "--spec", edits, "--output", out)
		if again := geometry(out); !reflect.DeepEqual(got, again) {
			t.Fatalf("creation used stale paragraph formatting: created %v; reapplied %v", got, again)
		}
		if i > 0 && (got[0][1] <= previous[0][1] || got[1][0] <= previous[1][0]) {
			t.Fatal("expanded line spacing failed to enlarge text and move the next shape", previous, got)
		}
		previous = got
	}
}
