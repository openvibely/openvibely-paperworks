package convert

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openvibely/openvibely-paperworks/internal/pdf"
)

// Original runtime lines 874-886: retain the middle slide's hidden flag in the
// input, render all three in order, and preserve the complete source package.
func TestReferenceStandaloneRuntimeHiddenSlides(t *testing.T) {
	referenceRuntimeOffice(t)
	ctx := context.Background()
	labels := []string{"VISIBLE_FIRST", "HIDDEN_MIDDLE", "VISIBLE_LAST"}
	src := referenceRuntimeOriginalPackageFixture(t, "library-report-fixtures.zip", "runtime-hidden-slides", ".pptx")
	before := contents(t, src)
	out := filepath.Join(t.TempDir(), "render")
	images, e := RenderFile(ctx, src, out, "", "", pdf.RenderOptions{})
	if e != nil {
		t.Fatal(e)
	}
	t.Run("images", func(t *testing.T) {
		if len(images) != 3 {
			t.Errorf("images %d, want 3", len(images))
		}
	})
	pages := referenceRuntimePDFTexts(t, contents(t, filepath.Join(out, "runtime-hidden-slides.pdf")))
	t.Run("pages", func(t *testing.T) {
		if len(pages) != 3 {
			t.Errorf("pages %d, want 3", len(pages))
		}
	})
	for i, label := range labels {
		t.Run(label, func(t *testing.T) {
			if i >= len(pages) || !strings.Contains(pages[i], label) {
				t.Errorf("page %d missing %s: %v", i, label, pages)
			}
		})
	}
	t.Run("source", func(t *testing.T) {
		if !bytes.Equal(before, contents(t, src)) {
			t.Error("source changed")
		}
	})
}

// Original runtime lines 889-897: all four labels including the rotated Y title
// must survive real rendering, with the source bytes unchanged.
func TestReferenceStandaloneRuntimeHistogramAxes(t *testing.T) {
	referenceRuntimeOffice(t)
	ctx := context.Background()
	src := filepath.Join("..", "..", "testdata", "fixtures", "reference-histogram-axes.pptx")
	before := contents(t, src)
	out := filepath.Join(t.TempDir(), "render")
	images, e := RenderFile(ctx, src, out, "", "", pdf.RenderOptions{})
	if e != nil {
		t.Fatal(e)
	}
	t.Run("images", func(t *testing.T) {
		if len(images) != 1 {
			t.Errorf("images %d, want 1", len(images))
		}
	})
	text := referenceRuntimePDFBottomToTopText(t, contents(t, filepath.Join(out, "reference-histogram-axes.pdf")), 0)
	for _, label := range []string{"Frequency", "Observations", "10.0", "2.0"} {
		t.Run(label, func(t *testing.T) {
			if !strings.Contains(text, label) {
				t.Errorf("missing %s: %s", label, text)
			}
		})
	}
	t.Run("source", func(t *testing.T) {
		if !bytes.Equal(before, contents(t, src)) {
			t.Error("source changed")
		}
	})
}
