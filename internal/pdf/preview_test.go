package pdf

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"os"
	"testing"
)

// PDFForms.test_fallback_preview_displays_radio_checkbox_text_and_choice.
// The input is regenerated from the unchanged original PDFForms.form helper;
// provenance.json records its exact source hash. The retained page RGB oracles
// were captured before production changes and are reconciled against this source.
// PNG encoding equality is not asserted.
func TestReferenceFallbackPreviewExactPixels(t *testing.T) {
	src := "../../testdata/reference-mirror/form.pdf"
	before, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	paths, err := RenderPages(context.Background(), src, t.TempDir(), RenderOptions{DPI: 144})
	if err != nil || len(paths) != 2 {
		t.Fatal(paths, err)
	}
	load := func(path string) *image.RGBA {
		t.Helper()
		f, e := os.Open(path)
		if e != nil {
			t.Fatal(e)
		}
		defer f.Close()
		img, e := png.Decode(f)
		if e != nil {
			t.Fatal(e)
		}
		out := image.NewRGBA(img.Bounds())
		draw.Draw(out, out.Bounds(), img, img.Bounds().Min, draw.Src)
		return out
	}
	reference := referenceFormImages(t, before)
	if len(reference) != len(paths) {
		t.Fatal("reference page count changed")
	}
	for i, path := range paths {
		actual := load(path)
		expected := load(fmt.Sprintf("../../testdata/reference-mirror/form-page-%04d.png", i+1))
		if !sameImage(actual, expected) {
			t.Fatalf("page %d differs from reference pixels", i+1)
		}
		if !sameImage(reference[i], expected) {
			t.Fatalf("independent images helper differs from pinned reference pixels on page %d", i+1)
		}
		if i == 0 && !equalBools(radioMarks(actual), []bool{true, false, false}) {
			t.Fatal("radio state changed")
		}
	}
	after, err := os.ReadFile(src)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("source changed", err)
	}
}
