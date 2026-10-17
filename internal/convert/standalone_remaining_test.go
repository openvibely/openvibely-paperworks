package convert

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"github.com/openvibely/openvibely-paperworks/internal/pdf"
	"github.com/xuri/excelize/v2"
)

// test_document_runtime.py:598-600. Unknown source cells must remain unknown;
// neither the row formula nor the total may invent the obscured amount.
func TestReferenceStandaloneRuntimeReconstruction(t *testing.T) {
	referenceRuntimeOffice(t)
	dir := t.TempDir()
	must := func(e error) {
		t.Helper()
		if e != nil {
			t.Fatal(e)
		}
	}
	src := referenceRuntimeOriginalWorkbookFixture(t, "runtime-workflow-fixtures.zip", "reconstructed")
	r, e := Convert(context.Background(), src, filepath.Join(dir, "calculated"), Options{Recalculate: true})
	must(e)
	out := r["output"].(string)
	saved := referenceRuntimeRead(t, out)
	t.Run("visible-and-unknown", func(t *testing.T) {
		id, e := saved.GetCellValue("Reconstruction", "A2", excelize.Options{RawCellValue: true})
		if e != nil || id != "001" {
			t.Errorf("identifier %q: %v", id, e)
		}
		idType, e := saved.GetCellType("Reconstruction", "A2")
		if e != nil || (idType != excelize.CellTypeInlineString && idType != excelize.CellTypeSharedString) {
			t.Errorf("identifier must remain text, got %v: %v", idType, e)
		}
		if got := referenceRuntimeValue(t, saved, "Reconstruction", "D2"); got != 30 {
			t.Errorf("visible amount %v", got)
		}
		value, e := saved.GetCellValue("Reconstruction", "C3")
		if e != nil || value != "" {
			t.Errorf("unknown price %q: %v", value, e)
		}
		kind, e := saved.GetCellType("Reconstruction", "C3")
		if e != nil || kind != excelize.CellTypeUnset {
			t.Errorf("unknown cell type %v: %v", kind, e)
		}
	})
	t.Run("unavailable", func(t *testing.T) {
		for _, cell := range []string{"D3", "D4"} {
			v, e := saved.GetCellValue("Reconstruction", cell)
			if e != nil || v != "Unavailable" {
				t.Errorf("%s=%q: %v", cell, v, e)
			}
		}
	})
	t.Run("render", func(t *testing.T) {
		pages, e := RenderFile(context.Background(), out, filepath.Join(dir, "render"), "", "", pdf.RenderOptions{})
		if e != nil || len(pages) == 0 {
			t.Fatalf("render %v: %v", pages, e)
		}
	})
}

func TestReferenceStandaloneRuntimeTwoSlides(t *testing.T) {
	referenceRuntimeOffice(t)
	dir := t.TempDir()
	ctx := context.Background()
	src := referenceRuntimeOriginalPackageFixture(t, "runtime-workflow-fixtures.zip", "runtime-two-slides", ".pptx")
	r, e := Convert(ctx, src, filepath.Join(dir, "slides"), Options{})
	if e != nil {
		t.Fatal(e)
	}
	path := r["output"].(string)
	t.Run("pdf-pages", func(t *testing.T) {
		if count := len(referenceRuntimePDFDimensions(t, contents(t, path))); count != 2 {
			t.Errorf("pages %d", count)
		}
	})
	t.Run("images", func(t *testing.T) {
		images, e := pdf.RenderPages(ctx, path, filepath.Join(dir, "slides"), pdf.RenderOptions{})
		if e != nil || len(images) != 2 {
			t.Errorf("images %v: %v", images, e)
		}
	})
}

func TestReferenceStandaloneRuntimeAdjacentCaptions(t *testing.T) {
	referenceRuntimeOffice(t)
	dir := t.TempDir()
	src := referenceRuntimeOriginalPackageFixture(t, "runtime-word-fixtures.zip", "caption-source", ".docx")
	for _, c := range []struct {
		label string
		index int
	}{{"two", 3}, {"one", 2}} {
		src = referenceRuntimeWordEdit(t, src, filepath.Join(dir, "caption-"+c.label+".docx"), map[string]any{"operation": "caption", "label": "Figure", "text": strings.ToUpper(c.label[:1]) + c.label[1:], "after_paragraph": c.index, "bookmark": c.label})
	}
	for i, pair := range [][2]string{{"[[ONE]]", "one"}, {"[[TWO]]", "two"}, {"[[HEAD1]]", "one"}, {"[[HEAD2]]", "two"}} {
		src = referenceRuntimeWordEdit(t, src, filepath.Join(dir, fmt.Sprintf("reference-%d.docx", i)), map[string]any{"operation": "reference", "name": pair[1], "field": true, "marker": pair[0]})
	}
	src = referenceRuntimeWordEdit(t, src, filepath.Join(dir, "final.docx"), map[string]any{"operation": "materialize"})
	r, e := Convert(context.Background(), src, filepath.Join(dir, "converted"), Options{})
	if e != nil {
		t.Fatal(e)
	}
	path := r["output"].(string)
	t.Run("text", func(t *testing.T) {
		text := strings.Join(referenceRuntimePDFTexts(t, contents(t, path)), "\n")
		for _, want := range []string{"Figures 1 and 2 show the supplied examples.", "Reference figures 1 and 2", "Figure 1: One", "Figure 2: Two"} {
			if !strings.Contains(text, want) {
				t.Errorf("missing %q in %q", want, text)
			}
		}
	})
	t.Run("caption-styles", func(t *testing.T) {
		p, e := opc.Open(src)
		if e != nil {
			t.Fatal(e)
		}
		d, e := p.XML("word/document.xml")
		if e != nil {
			t.Fatal(e)
		}
		count := 0
		for _, p := range office.All(d.Root(), office.NSW, "p") {
			pr := office.Child(p, office.NSW, "pPr")
			if pr != nil {
				style := office.Child(pr, office.NSW, "pStyle")
				if style != nil {
					value, _ := office.Attr(style, office.NSW, "val")
					if value == "Caption" {
						count++
					}
				}
			}
		}
		if count != 2 {
			t.Errorf("caption count %d", count)
		}
	})
	t.Run("render", func(t *testing.T) {
		pages, e := pdf.RenderPages(context.Background(), path, filepath.Join(dir, "converted"), pdf.RenderOptions{})
		if e != nil || len(pages) == 0 {
			t.Errorf("render %v: %v", pages, e)
		}
	})
}
