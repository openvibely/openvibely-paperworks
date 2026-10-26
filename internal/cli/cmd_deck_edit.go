package cli

import (
	"context"
	"github.com/openvibely/openvibely-paperworks/internal/pptx"
	"path/filepath"
)

func init() {
	register(&Command{Name: "deck-edit", ValueOpts: []string{"spec", "output"}, Summary: "Add, duplicate, reorder or remove slides, or add native objects to an existing deck.", Usage: "paperworks deck-edit INPUT.pptx --spec EDITS.json --output NEW.pptx", Run: func(ctx context.Context, a Args) (any, error) {
		in, out, err := oneInOut(a)
		if err != nil {
			return nil, err
		}
		var s pptx.DeckEditSpec
		if err = a.JSON("spec", &s); err != nil {
			return nil, err
		}
		err = pptx.DeckEditFile(ctx, in, out, filepath.Dir(a.Path("spec")), s)
		return map[string]any{"output": out, "visual_review_required": true}, err
	}})
}
