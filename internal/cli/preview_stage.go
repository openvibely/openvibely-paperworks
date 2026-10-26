package cli

import (
	"context"
	"os"
	"path/filepath"

	"github.com/openvibely/openvibely-paperworks/internal/pdf"
)

type previewFileRenderer func(context.Context, string, string, string, string, pdf.RenderOptions) ([]string, error)

// createPreviewStage creates a private sibling directory and returns its cleanup
// function. Callers defer cleanup immediately so every later error discards the
// unpublished stage.
func createPreviewStage(out, prefix string) (string, func(), error) {
	parent := filepath.Dir(out)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return "", nil, err
	}
	stage, err := os.MkdirTemp(parent, prefix)
	if err != nil {
		return "", nil, err
	}
	return stage, func() { _ = os.RemoveAll(stage) }, nil
}
