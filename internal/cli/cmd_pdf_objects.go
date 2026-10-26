package cli

import (
	"context"
	"github.com/openvibely/openvibely-paperworks/internal/pdf"
	"os"
	"path/filepath"
)

func init() {
	register(&Command{Name: "pdf-objects", ValueOpts: []string{}, Summary: "List top-level PDF artwork/text objects with page, index, type and bounding box.", Usage: "paperworks pdf-objects INPUT.pdf", Run: func(ctx context.Context, a Args) (any, error) {
		in, err := input(a, 1)
		if err != nil {
			return nil, err
		}
		data, err := os.ReadFile(in[0])
		if err != nil {
			return nil, err
		}
		return pdf.Objects(ctx, data)
	}})
	register(&Command{Name: "pdf-object-edit", ValueOpts: []string{"spec", "output"}, Summary: "Remove selected PDF objects and overlay vector artwork from another PDF page into a new file.", Usage: "paperworks pdf-object-edit INPUT.pdf --spec EDITS.json --output NEW.pdf", Run: func(ctx context.Context, a Args) (any, error) {
		in, out, err := oneInOut(a)
		if err != nil {
			return nil, err
		}
		var s pdf.ObjectEditSpec
		if err = a.JSON("spec", &s); err != nil {
			return nil, err
		}
		err = pdf.EditObjects(ctx, in, out, filepath.Dir(a.Path("spec")), s)
		return map[string]any{"output": out, "visual_review_required": true}, err
	}})
}
