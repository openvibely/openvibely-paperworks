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

	"github.com/openvibely/openvibely-paperworks/internal/pdf"
)

// test_document_libraries.py:89-99 deliberately supplies identical rendered
// pixels for different document text. This exercises the real Compare path,
// replacing only conversion/rendering dependencies as the Reference mock does.
func TestReferenceStandaloneLibrariesCompareText(t *testing.T) {
	dir := t.TempDir()
	before := referenceRuntimeOriginalPackageFixture(t, "runtime-word-fixtures.zip", "before", ".docx")
	after := referenceRuntimeOriginalPackageFixture(t, "runtime-word-fixtures.zip", "after", ".docx")
	im := image.NewRGBA(image.Rect(0, 0, 20, 20))
	draw.Draw(im, im.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	identical := filepath.Join(dir, "identical.png")
	f, e := os.Create(identical)
	if e != nil {
		t.Fatal(e)
	}
	if e = png.Encode(f, im); e != nil {
		f.Close()
		t.Fatal(e)
	}
	if e = f.Close(); e != nil {
		t.Fatal(e)
	}
	calls := 0
	ctx := withDeps(dependencies{convert: func(_ context.Context, src, out string, _ Options) (map[string]any, error) {
		if src != before && src != after {
			t.Errorf("unexpected input %s", src)
		}
		return map[string]any{"output": filepath.Join(out, "mock.pdf")}, nil
	}, render: func(context.Context, string, string, pdf.RenderOptions) ([]string, error) {
		calls++
		return []string{identical}, nil
	}})
	result, e := Compare(ctx, before, after, filepath.Join(dir, "diff"), pdf.RenderOptions{DPI: 144})
	if e != nil {
		t.Fatal(e)
	}
	if calls != 2 {
		t.Fatalf("render calls %d, want 2", calls)
	}
	t.Run("same-pixels", func(t *testing.T) {
		pages := result["pages"].([]PageChange)
		if len(pages) == 0 || pages[0].Status != "same" {
			t.Errorf("pages: %v", pages)
		}
	})
	t.Run("text-changed", func(t *testing.T) {
		if result["text_changed"] != true {
			t.Errorf("text_changed: %v", result["text_changed"])
		}
	})
	t.Run("diff-content", func(t *testing.T) {
		b, e := os.ReadFile(result["text_diff"].(string))
		if e != nil {
			t.Fatal(e)
		}
		if !strings.Contains(string(b), "-Before") || !strings.Contains(string(b), "+After") {
			t.Errorf("diff: %s", b)
		}
	})
}

// test_document_runtime.py:507-510. Preserve HOME and use genuinely unusable
// inherited XDG paths while running the real renderer with the original DPI.
func TestReferenceStandaloneRuntimeIsolatedConfig(t *testing.T) {
	referenceRuntimeOffice(t)
	dir := t.TempDir()
	src := referenceRuntimeOriginalPackageFixture(t, "runtime-word-fixtures.zip", "word", ".docx")
	home := os.Getenv("HOME")
	t.Setenv("XDG_CONFIG_HOME", "/dev/null/unusable-config")
	t.Setenv("XDG_CACHE_HOME", "/dev/null/unusable-cache")
	pages, e := RenderFile(context.Background(), src, filepath.Join(dir, "isolated-config-render"), "", "", pdf.RenderOptions{DPI: 72})
	if e != nil {
		t.Fatal(e)
	}
	if len(pages) == 0 {
		t.Error("no rendered pages")
	}
	if os.Getenv("HOME") != home {
		t.Error("renderer changed HOME")
	}
}
