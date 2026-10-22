package convert

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/pdf"
	"github.com/openvibely/openvibely-paperworks/internal/wordedit"
	pdfcpuapi "github.com/pdfcpu/pdfcpu/pkg/api"
)

func referenceRuntimeWordEdit(t *testing.T, src, dst string, spec map[string]any) string {
	t.Helper()
	// The public editor receives JSON numbers, as did the original script.
	raw, e := json.Marshal(spec)
	if e != nil {
		t.Fatal(e)
	}
	var decoded map[string]any
	if e = json.Unmarshal(raw, &decoded); e != nil {
		t.Fatal(e)
	}
	if _, e := wordedit.Edit(src, dst, decoded, filepath.Dir(src)); e != nil {
		t.Fatal(e)
	}
	return dst
}
func referenceRuntimeWordConvert(t *testing.T, src, dir string) ([]string, func()) {
	t.Helper()
	r, e := Convert(context.Background(), src, dir, Options{})
	if e != nil {
		t.Fatal(e)
	}
	path := r["output"].(string)
	text, e := referencePypdfPages(t, contents(t, path))
	if e != nil {
		t.Fatal(e)
	}
	return text, func() {
		t.Helper()
		pages, e := pdf.RenderPages(context.Background(), path, dir, pdf.RenderOptions{})
		if e != nil || len(pages) == 0 {
			t.Fatalf("render pages: %v %v", pages, e)
		}
	}
}
func referenceRuntimeWordRender(t *testing.T, src, dir string) []string {
	t.Helper()
	text, render := referenceRuntimeWordConvert(t, src, dir)
	render()
	return text
}

func TestReferenceStandaloneRuntimeWordRevisions(t *testing.T) {
	referenceRuntimeOffice(t)
	dir := t.TempDir()
	src := referenceRuntimeOriginalPackageFixture(t, "runtime-word-fixtures.zip", "word", ".docx")
	equation := referenceRuntimeWordEdit(t, src, filepath.Join(dir, "equation.docx"), map[string]any{"operation": "equation", "mathml": `<math><mi>E</mi><mo>=</mo><mi>m</mi><msup><mi>c</mi><mn>2</mn></msup></math>`})
	redline := referenceRuntimeWordEdit(t, equation, filepath.Join(dir, "redline.docx"), map[string]any{"operation": "redline", "old": "old price", "new": "new price", "author": "Fixture"})
	for _, c := range []struct{ action, want string }{{"accept", "The new price is $10."}, {"reject", "The old price is $10."}} {
		t.Run(c.action, func(t *testing.T) {
			saved := referenceRuntimeWordEdit(t, redline, filepath.Join(dir, c.action+".docx"), map[string]any{"operation": c.action})
			pages, render := referenceRuntimeWordConvert(t, saved, filepath.Join(dir, c.action))
			text := strings.Join(pages, "\n")
			if !strings.Contains(text, c.want) {
				t.Fatalf("missing %q in %q", c.want, text)
			}
			render()
		})
	}
}
func TestReferenceStandaloneRuntimeWordWorkflow(t *testing.T) {
	referenceRuntimeOffice(t)
	dir := t.TempDir()
	src := referenceRuntimeOriginalPackageFixture(t, "runtime-word-fixtures.zip", "workflow-source", ".docx")
	for i, op := range []map[string]any{
		{"operation": "redact", "patterns": []any{`AB-\d{4}`}, "replacement": "X", "preserve_length": true},
		{"operation": "note", "marker": "[[NOTE]]", "text": "Source checked against the account record."},
		{"operation": "control", "marker": "{{CLIENT}}", "tag": "client"},
		{"operation": "control-fill", "tag": "client", "text": "Example Company"},
		{"operation": "table-geometry", "table": 0, "widths_twips": []any{4000, 2200}},
		{"operation": "caption", "label": "Table", "text": "Revenue by service", "bookmark": "revenue", "after_table": 0},
		{"operation": "materialize"}, {"operation": "navigation"},
	} {
		src = referenceRuntimeWordEdit(t, src, filepath.Join(dir, fmt.Sprintf("step-%d.docx", i)), op)
	}
	pages, render := referenceRuntimeWordConvert(t, src, filepath.Join(dir, "render"))
	text := strings.Join(pages, "\n")
	for _, want := range []string{"Contents", "Example Company", "XXXXXXX", "Revenue by service", "Source checked"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in %q", want, text)
		}
	}
	if strings.Contains(text, "AB-1234") {
		t.Fatal("redacted account leaked", text)
	}
	render()
}
func TestReferenceStandaloneRuntimeTOCPages(t *testing.T) {
	referenceRuntimeOffice(t)
	dir := t.TempDir()
	src := referenceRuntimeOriginalPackageFixture(t, "runtime-word-fixtures.zip", "toc-source", ".docx")
	field := referenceRuntimeWordEdit(t, src, filepath.Join(dir, "field.docx"), map[string]any{"operation": "toc", "marker": "[[CONTENTS]]"})
	r, e := Convert(context.Background(), field, filepath.Join(dir, "refreshed"), Options{Refresh: true})
	if e != nil {
		t.Fatal(e)
	}
	pages, render := referenceRuntimeWordConvert(t, r["output"].(string), filepath.Join(dir, "render"))
	if len(pages) != 2 {
		t.Fatalf("page count %d: %q", len(pages), pages)
	}
	// Text extraction may insert whitespace between dot leaders and page numbers.
	for _, pattern := range []string{`(?m)^First section\.+[ \t]*1[ \t]*$`, `(?m)^Second section\.+[ \t]*2[ \t]*$`} {
		if !regexp.MustCompile(pattern).MatchString(pages[0]) {
			t.Errorf("missing %s in %q", pattern, pages[0])
		}
	}
	if strings.Contains(pages[0], "Update table of contents") || !strings.Contains(pages[1], "Second section") {
		t.Error("TOC unresolved or section moved", pages)
	}
	render()
}

func TestReferenceStandaloneRuntimeTableGeometry(t *testing.T) {
	referenceRuntimeOffice(t)
	dir := t.TempDir()
	t.Run("typed-exchange", func(t *testing.T) {
		src := referenceRuntimeOriginalWorkbookFixture(t, "runtime-table-fixtures.zip", "table-source")
		dst := filepath.Join(dir, "exchanged.docx")
		if _, e := TableExchange(src, dst, "Source", "A1:B3", 0); e != nil {
			t.Fatal(e)
		}
		referenceRuntimeWordRender(t, dst, filepath.Join(dir, "exchange-render"))
	})
	t.Run("merged-widths", func(t *testing.T) {
		merged := referenceRuntimeOriginalPackageFixture(t, "runtime-table-fixtures.zip", "merged-source", ".docx")
		result := referenceRuntimeWordEdit(t, merged, filepath.Join(dir, "widths.docx"), map[string]any{"operation": "table-geometry", "table": 0, "widths_twips": []any{2000, 3500, 2500}})
		referenceRuntimeWordRender(t, result, filepath.Join(dir, "merged-render"))
	})
}

func TestReferenceStandaloneRuntimeSections(t *testing.T) {
	referenceRuntimeOffice(t)
	ctx := context.Background()
	dir := t.TempDir()
	source := referenceRuntimeOriginalPackageFixture(t, "runtime-word-fixtures.zip", "sections", ".docx")
	report, e := office.PackageReport(source)
	if e != nil {
		t.Fatal(e)
	}
	links := report["section_links"].([]any)
	if len(links) != 2 {
		t.Fatalf("section links %v", links)
	}
	if links[1].(map[string]any)["links"].(map[string]any)["header:default"].(map[string]any)["inherited"] != true {
		t.Error("header not inherited", links)
	}
	sections := report["sections"].([]any)
	key := "{" + office.NSW + "}w"
	a := sections[0].(map[string]any)["pgSz"].(map[string]string)
	b := sections[1].(map[string]any)["pgSz"].(map[string]string)
	if a[key] == b[key] {
		t.Error("same section widths", a, b)
	}
	r, e := Convert(ctx, source, filepath.Join(dir, "render"), Options{})
	if e != nil {
		t.Fatal(e)
	}
	path := r["output"].(string)
	data := contents(t, path)
	loaded, e := pdfcpuapi.ReadContext(ctx, bytes.NewReader(data), nil)
	if e != nil {
		t.Fatal(e)
	}
	dims := referenceRuntimePDFDimensions(t, data)
	if len(dims) != 2 {
		t.Fatalf("PDF pages %v", dims)
	}
	if dims[0].Width >= dims[0].Height {
		t.Error("first page not portrait", dims[0])
	}
	if dims[1].Width <= dims[1].Height {
		t.Error("second page not landscape", dims[1])
	}
	pages, e := referencePypdfPages(t, data)
	if e != nil {
		t.Fatal(e)
	}
	for i, page := range pages {
		if !strings.Contains(page, "Shared report header") {
			t.Error("missing inherited header", i, page)
		}
		if !strings.Contains(page, fmt.Sprint(i+1)) {
			t.Error("missing page field", i, page)
		}
	}
	if !strings.Contains(pages[0], "Source: Fixture reference after link") {
		t.Error("inline link text changed", pages[0])
	}
	if e := loaded.EnsurePageCount(); e != nil {
		t.Fatal(e)
	}
	page, _, _, e := loaded.PageDict(ctx, 1, false)
	if e != nil {
		t.Fatal(e)
	}
	annotations, e := loaded.DereferenceArray(page["Annots"])
	if e != nil || len(annotations) == 0 {
		t.Error("missing link annotations", annotations, e)
	}
	rendered, e := pdf.RenderPages(ctx, path, filepath.Join(dir, "render"), pdf.RenderOptions{})
	if e != nil || len(rendered) == 0 {
		t.Error("render", rendered, e)
	}
}

func TestReferenceStandaloneRuntimeAdvancedWord(t *testing.T) {
	referenceRuntimeOffice(t)
	dir := t.TempDir()
	src := referenceRuntimeOriginalPackageFixture(t, "runtime-word-fixtures.zip", "word", ".docx")
	equation := referenceRuntimeWordEdit(t, src, filepath.Join(dir, "equation.docx"), map[string]any{"operation": "equation", "mathml": `<math><mi>E</mi><mo>=</mo><mi>m</mi><msup><mi>c</mi><mn>2</mn></msup></math>`})
	redline := referenceRuntimeWordEdit(t, equation, filepath.Join(dir, "redline.docx"), map[string]any{"operation": "redline", "old": "old price", "new": "new price", "author": "Fixture"})
	accepted := referenceRuntimeWordEdit(t, redline, filepath.Join(dir, "accepted.docx"), map[string]any{"operation": "accept"})
	rejected := referenceRuntimeWordEdit(t, redline, filepath.Join(dir, "rejected.docx"), map[string]any{"operation": "reject"})
	source := accepted
	for i, op := range []map[string]any{{"operation": "note", "paragraph": 1, "text": "Verified source note"}, {"operation": "caption", "label": "Figure", "text": "Revenue results"}, {"operation": "materialize"}} {
		source = referenceRuntimeWordEdit(t, source, filepath.Join(dir, fmt.Sprintf("advanced-%d.docx", i)), op)
	}
	beforeDir := filepath.Join(dir, "advanced-render")
	beforePages, renderBefore := referenceRuntimeWordConvert(t, source, beforeDir)
	text := strings.Join(beforePages, "\n")
	if !strings.Contains(text, "Verified source note") || !strings.Contains(text, "Revenue results") {
		t.Fatal("missing note/caption", text)
	}
	renderBefore()
	watermarked := referenceRuntimeWordEdit(t, source, filepath.Join(dir, "watermark.docx"), map[string]any{"operation": "watermark", "text": "DRAFT"})
	afterDir := filepath.Join(dir, "watermark-render")
	text = strings.Join(referenceRuntimeWordRender(t, watermarked, afterDir), "\n")
	if !strings.Contains(text, "The new price is $10.") {
		t.Error("body changed", text)
	}
	readPNG := func(path string) image.Image {
		t.Helper()
		f, e := os.Open(path)
		if e != nil {
			t.Fatal(e)
		}
		defer f.Close()
		im, e := png.Decode(f)
		if e != nil {
			t.Fatal(e)
		}
		return im
	}
	before, after := readPNG(filepath.Join(beforeDir, "page-0001.png")), readPNG(filepath.Join(afterDir, "page-0001.png"))
	if before.Bounds() != after.Bounds() {
		t.Fatal("page bounds changed", before.Bounds(), after.Bounds())
	}
	rgbDiff := func(x, y int) bool {
		return referenceRuntimeRGB(before.At(x, y)) != referenceRuntimeRGB(after.At(x, y))
	}
	changed := false
	for y := before.Bounds().Min.Y; y < before.Bounds().Max.Y; y++ {
		for x := before.Bounds().Min.X; x < before.Bounds().Max.X; x++ {
			changed = changed || rgbDiff(x, y)
		}
	}
	if !changed {
		t.Error("watermark has no visible pixels")
	}
	// Compare exactly the Reference crop. Image.At outside the bounds gives black,
	// equivalent to Pillow crop padding when either page is narrower than 1200.
	for y := 0; y < 400; y++ {
		for x := 0; x < 1200; x++ {
			if rgbDiff(x, y) {
				t.Fatalf("watermark changed protected top crop at %d,%d", x, y)
			}
		}
	}
	report, e := Compare(context.Background(), accepted, rejected, filepath.Join(dir, "diff"), pdf.RenderOptions{})
	if e != nil {
		t.Fatal(e)
	}
	pages := report["pages"].([]PageChange)
	if len(pages) == 0 || pages[0].Status != "changed" {
		t.Error("revision diff", pages)
	}
}

// Pillow convert("RGB") drops alpha without compositing. PNG decoding yields
// NRGBA for alpha-bearing inputs; retain even hidden RGB at alpha zero.
func referenceRuntimeRGB(c color.Color) [3]uint8 {
	v := color.NRGBAModel.Convert(c).(color.NRGBA)
	return [3]uint8{v.R, v.G, v.B}
}
func TestReferenceRuntimeRGBDropsAlpha(t *testing.T) {
	for _, alpha := range []uint8{0, 1, 127, 255} {
		if got := referenceRuntimeRGB(color.NRGBA{R: 12, G: 128, B: 250, A: alpha}); got != [3]uint8{12, 128, 250} {
			t.Fatal(alpha, got)
		}
	}
}
