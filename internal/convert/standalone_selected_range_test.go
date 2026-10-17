package convert

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openvibely/openvibely-paperworks/internal/pdf"
)

// Original runtime 758-782: captured openpyxl fixture and independent text reader.
// General path/dimension acquisition equivalence remains a separate review gate.
func TestReferenceStandaloneRuntimeSelectedRange(t *testing.T) {
	referenceRuntimeOffice(t)
	ctx := context.Background()
	dir := t.TempDir()
	must := func(e error) {
		t.Helper()
		if e != nil {
			t.Fatal(e)
		}
	}
	src := referenceRuntimeOriginalWorkbookFixture(t, "runtime-workflow-fixtures.zip", "range-preview")
	before := contents(t, src)
	out := filepath.Join(dir, "render")
	images, e := RenderFile(ctx, src, out, "Report", "B2:C4", pdf.RenderOptions{})
	must(e)
	t.Run("images", func(t *testing.T) {
		if len(images) != 1 {
			t.Errorf("images %d, want 1", len(images))
		}
	})
	data := contents(t, filepath.Join(out, "range-preview.pdf"))
	pages := referenceRuntimePDFTexts(t, data)
	if len(pages) == 0 {
		t.Fatal("no rendered pages")
	}
	text := pages[0]
	for _, value := range []string{"Revenue", "14", "Conversion", "12.5%", "Merged footer"} {
		t.Run("text-"+value, func(t *testing.T) {
			if !strings.Contains(text, value) {
				t.Errorf("missing %q in %q", value, text)
			}
		})
	}
	t.Run("outside", func(t *testing.T) {
		if strings.Contains(text, "OUTSIDE") {
			t.Error(text)
		}
	})
	dims := referenceRuntimePDFDimensions(t, data)
	t.Run("dimensions", func(t *testing.T) {
		if len(dims) == 0 || !(dims[0].Width > 220 && dims[0].Width < 280 && dims[0].Height > 85 && dims[0].Height < 105) {
			t.Errorf("dimensions %v", dims)
		}
	})
	t.Run("conditional-fill", func(t *testing.T) {
		marks := referencePDFPaths(t, data)
		for _, mark := range marks {
			color := referencePDFPathColor(mark, false)
			if mark.rect && len(color) == 3 && color[1] > color[0] && color[1] > color[2] {
				return
			}
		}
		t.Errorf("no green-dominant rectangle fill: %+v", marks)
	})
	t.Run("source", func(t *testing.T) {
		if !bytes.Equal(before, contents(t, src)) {
			t.Error("source changed")
		}
	})
}

// Original runtime 865-870, including the subset rather than equality check.
func TestReferenceStandaloneRuntimeEmptyWorkbook(t *testing.T) {
	referenceRuntimeOffice(t)
	ctx := context.Background()
	dir := t.TempDir()
	src := referenceRuntimeOriginalWorkbookFixture(t, "runtime-workflow-fixtures.zip", "empty")
	before := contents(t, src)
	out := filepath.Join(dir, "render")
	images, e := RenderFile(ctx, src, out, "", "", pdf.RenderOptions{})
	if e != nil {
		t.Fatal(e)
	}
	t.Run("images", func(t *testing.T) {
		if len(images) != 1 {
			t.Errorf("images %d, want 1", len(images))
		}
	})
	pages := referenceRuntimePDFTexts(t, contents(t, filepath.Join(out, "empty.pdf")))
	t.Run("pages", func(t *testing.T) {
		if len(pages) != 1 {
			t.Errorf("pages %d, want 1", len(pages))
		}
	})
	t.Run("words", func(t *testing.T) {
		if len(pages) == 0 {
			t.Fatal("no page")
		}
		for _, word := range strings.Fields(pages[0]) {
			if word != "A" && word != "1" {
				t.Errorf("unexpected word %q", word)
			}
		}
	})
	t.Run("source", func(t *testing.T) {
		if !bytes.Equal(before, contents(t, src)) {
			t.Error("source changed")
		}
	})
}
