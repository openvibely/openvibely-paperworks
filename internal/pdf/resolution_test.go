package pdf

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The original Reference method calls render_pages with an absent input; validation
// must precede file access. Presence flags preserve Go's omitted-zero defaults.
func TestReferenceRendererResolutionValidationAPI(t *testing.T) {
	dir := t.TempDir()
	for _, c := range []struct {
		name    string
		options RenderOptions
		message string
	}{
		{"zero-dpi", RenderOptions{DPISet: true}, "DPI must be between 36 and 600"},
		{"nan-dpi", RenderOptions{DPI: math.NaN()}, "DPI must be between 36 and 600"},
		{"small-max", RenderOptions{MaxPixels: 10}, "max_pixels must be between 100 and 10000"},
		{"zero-max", RenderOptions{MaxPixelsSet: true}, "max_pixels must be between 100 and 10000"},
	} {
		t.Run(c.name, func(t *testing.T) {
			files, err := RenderPages(context.Background(), filepath.Join(dir, "unused.pdf"), dir, c.options)
			if err == nil || err.Error() != c.message || len(files) != 0 {
				t.Fatalf("files=%v error=%v", files, err)
			}
		})
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 0 {
		t.Fatalf("validation wrote output: %v %v", entries, err)
	}
	defaults := RenderOptions{}
	if err := defaults.Validate(); err != nil || defaults.MaxPixels != 1600 || defaults.DPI != 0 {
		t.Fatalf("defaults: %+v %v", defaults, err)
	}
	_, err = RenderPages(context.Background(), filepath.Join(dir, "unused.pdf"), dir, RenderOptions{})
	if err == nil || !os.IsNotExist(err) || strings.Contains(err.Error(), "DPI") {
		t.Fatalf("omitted options must reach file access: %v", err)
	}
}
