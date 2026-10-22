package convert

import (
	"context"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"github.com/openvibely/openvibely-paperworks/internal/pdf"
)

const compareTextWordNS = `http://schemas.openxmlformats.org/wordprocessingml/2006/main`

func compareTextDocument(body string) string {
	return `<w:document xmlns:w="` + compareTextWordNS + `"><w:body>` + body + `</w:body></w:document>`
}

func compareTextPart(root, body string) string {
	return `<w:` + root + ` xmlns:w="` + compareTextWordNS + `">` + body + `</w:` + root + `>`
}

func writeCompareTextDOCX(t *testing.T, parts map[string]string) string {
	t.Helper()
	base := referenceRuntimeOriginalPackageFixture(t, "runtime-word-fixtures.zip", "before", ".docx")
	p, err := opc.Open(base)
	if err != nil {
		t.Fatal(err)
	}
	for name, raw := range parts {
		doc, err := opc.ParseXML([]byte(raw))
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		if p.Has(name) {
			if err := p.SetXML(name, doc); err != nil {
				t.Fatalf("set %s: %v", name, err)
			}
		} else {
			p.Set(name, []byte(raw))
		}
	}
	data, err := p.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "text.docx")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func compareTextDOCXPair(t *testing.T, beforeParts, afterParts map[string]string) (map[string]any, string, string, string) {
	t.Helper()
	dir := t.TempDir()
	before := writeCompareTextDOCX(t, beforeParts)
	after := writeCompareTextDOCX(t, afterParts)

	img := image.NewRGBA(image.Rect(0, 0, 12, 12))
	draw.Draw(img, img.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	pngPath := filepath.Join(dir, "identical.png")
	f, err := os.Create(pngPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, img); err != nil {
		f.Close()
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	ctx := withDeps(dependencies{
		convert: func(_ context.Context, _, out string, _ Options) (map[string]any, error) {
			return map[string]any{"output": filepath.Join(out, "mock.pdf")}, nil
		},
		render: func(context.Context, string, string, pdf.RenderOptions) ([]string, error) {
			return []string{pngPath}, nil
		},
	})
	result, err := Compare(ctx, before, after, filepath.Join(dir, "diff"), pdf.RenderOptions{})
	if err != nil {
		t.Fatal(err)
	}
	beforeText, err := os.ReadFile(filepath.Join(dir, "diff", "text-before.txt"))
	if err != nil {
		t.Fatal(err)
	}
	afterText, err := os.ReadFile(filepath.Join(dir, "diff", "text-after.txt"))
	if err != nil {
		t.Fatal(err)
	}
	diff, err := os.ReadFile(result["text_diff"].(string))
	if err != nil {
		t.Fatal(err)
	}
	return result, string(beforeText), string(afterText), string(diff)
}

func TestCompareDOCXInlineSeparators(t *testing.T) {
	plain := compareTextDocument(`<w:p><w:r><w:t>leftright</w:t></w:r></w:p>`)
	cases := []struct {
		name      string
		separator string
		text      string
	}{
		{"tab", `<w:tab/>`, "left\tright"},
		{"break-no-type", `<w:br/>`, "left\nright"},
		{"break-text-wrapping", `<w:br w:type="textWrapping"/>`, "left\nright"},
		{"carriage-return", `<w:cr/>`, "left\nright"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			separated := compareTextDocument(`<w:p><w:r><w:t>left</w:t></w:r><w:r>` + tc.separator + `</w:r><w:r><w:t>right</w:t></w:r></w:p>`)
			for _, reverse := range []bool{false, true} {
				t.Run(map[bool]string{false: "added", true: "removed"}[reverse], func(t *testing.T) {
					beforeXML, afterXML := plain, separated
					if reverse {
						beforeXML, afterXML = separated, plain
					}
					result, before, after, diff := compareTextDOCXPair(t,
						map[string]string{"word/document.xml": beforeXML},
						map[string]string{"word/document.xml": afterXML},
					)
					if result["text_changed"] != true {
						t.Fatalf("text_changed = %v; diff %q", result["text_changed"], diff)
					}
					plainText, separatedText := "leftright\n", tc.text+"\n"
					oldText, newText := plainText, separatedText
					separatedDiff, removedSeparatorDiff := "+left\tright\n", "-left\tright\n"
					if strings.Contains(tc.text, "\n") {
						separatedDiff, removedSeparatorDiff = "+left\n+right\n", "-left\n-right\n"
					}
					oldDiff, newDiff := "-leftright\n", separatedDiff
					if reverse {
						oldText, newText = separatedText, plainText
						oldDiff, newDiff = removedSeparatorDiff, "+leftright\n"
					}
					if diff == "" || !strings.Contains(diff, oldDiff) || !strings.Contains(diff, newDiff) {
						t.Fatalf("unreadable or empty separator diff: %q", diff)
					}
					if !strings.Contains(before, oldText) {
						t.Fatalf("before artifact lost %q: %q", oldText, before)
					}
					if !strings.Contains(after, newText) {
						t.Fatalf("after artifact lost %q: %q", newText, after)
					}
					pages := result["pages"].([]PageChange)
					if len(pages) != 1 || pages[0].Status != "same" {
						t.Fatalf("pixel-identical pages changed: %v", pages)
					}
				})
			}
		})
	}
}

func TestDOCXTextSeparatorsRetainOrderAndBoundaries(t *testing.T) {
	t.Run("repeated-leading-trailing", func(t *testing.T) {
		xml := compareTextDocument(`<w:p><w:r><w:tab/><w:tab/><w:t>A</w:t><w:br/><w:br/><w:t>B</w:t><w:cr/><w:tab/></w:r></w:p>`)
		path := writeCompareTextDOCX(t, map[string]string{"word/document.xml": xml})
		lines, err := docxLines(path)
		if err != nil {
			t.Fatal(err)
		}
		got := strings.Join(lines, "")
		if !strings.Contains(got, "\t\tA\n\nB\n\t\n") {
			t.Fatalf("separators were normalized or reordered: %q", got)
		}
	})

	t.Run("table-cell-paragraphs", func(t *testing.T) {
		xml := compareTextDocument(`<w:tbl><w:tr><w:tc><w:p><w:r><w:t>cell</w:t><w:tab/><w:t>one</w:t></w:r></w:p><w:p><w:r><w:t>second</w:t></w:r></w:p></w:tc></w:tr></w:tbl>`)
		path := writeCompareTextDOCX(t, map[string]string{"word/document.xml": xml})
		lines, err := docxLines(path)
		if err != nil {
			t.Fatal(err)
		}
		got := strings.Join(lines, "")
		if !strings.Contains(got, "cell\tone\nsecond\n") {
			t.Fatalf("table paragraphs lost order/boundaries: %q", got)
		}
	})

	t.Run("header-footer-parts", func(t *testing.T) {
		beforeParts := map[string]string{
			"word/document.xml": compareTextDocument(`<w:p><w:r><w:t>body</w:t></w:r></w:p>`),
			"word/header1.xml":  compareTextPart("hdr", `<w:p><w:r><w:t>head</w:t></w:r></w:p>`),
			"word/footer1.xml":  compareTextPart("ftr", `<w:p><w:r><w:t>foot</w:t></w:r></w:p>`),
		}
		afterParts := map[string]string{
			"word/document.xml": compareTextDocument(`<w:p><w:r><w:t>body</w:t></w:r></w:p>`),
			"word/header1.xml":  compareTextPart("hdr", `<w:p><w:r><w:t>head</w:t><w:tab/><w:t>line</w:t></w:r></w:p>`),
			"word/footer1.xml":  compareTextPart("ftr", `<w:p><w:r><w:t>foot</w:t></w:r></w:p>`),
		}
		result, before, after, diff := compareTextDOCXPair(t, beforeParts, afterParts)
		if result["text_changed"] != true || diff == "" {
			t.Fatalf("header separator was not detected: changed=%v diff=%q", result["text_changed"], diff)
		}
		for _, text := range []string{"[word/document.xml]", "[word/header1.xml]", "[word/footer1.xml]", "foot\n"} {
			if !strings.Contains(before, text) || !strings.Contains(after, text) {
				t.Errorf("part/text boundary %q missing from artifacts", text)
			}
		}
		if !strings.Contains(before, "head\n") {
			t.Errorf("header text missing from before artifact: %q", before)
		}
		if !strings.Contains(after, "head\tline\n") {
			t.Errorf("header tab missing from after artifact: %q", after)
		}
	})

	t.Run("page-column-markers", func(t *testing.T) {
		xml := compareTextDocument(`<w:p><w:r><w:t>A</w:t><w:br w:type="page"/><w:t>B</w:t><w:br w:type="column"/><w:t>C</w:t></w:r></w:p>`)
		path := writeCompareTextDOCX(t, map[string]string{"word/document.xml": xml})
		lines, err := docxLines(path)
		if err != nil {
			t.Fatal(err)
		}
		got := strings.Join(lines, "")
		if !strings.Contains(got, "A[page break]B[column break]C\n") {
			t.Fatalf("page/column breaks were not distinct: %q", got)
		}
	})
}

func TestCompareIdenticalDOCXWithSeparatorsUnchanged(t *testing.T) {
	xml := compareTextDocument(`<w:p><w:r><w:t>same</w:t><w:tab/><w:t>text</w:t><w:br/><w:t>next line</w:t><w:cr/></w:r></w:p>`)
	parts := map[string]string{"word/document.xml": xml}
	result, before, after, diff := compareTextDOCXPair(t, parts, parts)
	if result["text_changed"] != false || diff != "" {
		t.Fatalf("identical separated text changed: changed=%v diff=%q", result["text_changed"], diff)
	}
	if before != after {
		t.Fatalf("identical artifacts differ:\nbefore %q\nafter  %q", before, after)
	}
	pages := result["pages"].([]PageChange)
	if len(pages) != 1 || pages[0].Status != "same" {
		t.Fatalf("identical rendered pages changed: %v", pages)
	}
}
