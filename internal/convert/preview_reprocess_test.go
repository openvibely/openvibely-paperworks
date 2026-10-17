package convert

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/openvibely/openvibely-paperworks/internal/office"
)

// Saved histogram previews retain their native chart parts. Discovery still
// requests preview processing, but a second pass need not create a new file.
func TestConvertSavedHistogramPreview(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "saved.pptx")
	if changed, err := office.OfficePreviewCopy("../../testdata/fixtures/reference-categorical-histogram.pptx", source); err != nil || !changed {
		t.Fatal(changed, err)
	}
	before := contents(t, source)
	if needed, err := office.NeedsOfficePreview(source); err != nil || !needed {
		t.Fatal(needed, err)
	}
	unused := filepath.Join(dir, "unused.pptx")
	if changed, err := office.OfficePreviewCopy(source, unused); err != nil || changed {
		t.Fatal(changed, err)
	}
	if _, err := os.Stat(unused); !os.IsNotExist(err) {
		t.Fatalf("unchanged preview wrote a file: %v", err)
	}
	called := false
	ctx := withDeps(dependencies{export: func(_ context.Context, input, output, target string) (string, error) {
		called = true
		if input != source {
			return "", fmt.Errorf("export input %q, want existing source %q", input, source)
		}
		if !bytes.Equal(contents(t, input), before) {
			t.Fatal("source changed before export")
		}
		if target != "pdf" {
			t.Fatal(target)
		}
		path := filepath.Join(output, "saved.pdf")
		return path, os.WriteFile(path, []byte("export-routing sentinel"), 0600)
	}})
	if _, err := Convert(ctx, source, filepath.Join(dir, "out"), Options{}); err != nil {
		t.Fatal(err)
	}
	if !called || !bytes.Equal(contents(t, source), before) {
		t.Fatal("missing export or source changed")
	}
}
