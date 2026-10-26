package cli

import (
	"context"

	"github.com/openvibely/openvibely-paperworks/internal/office"
)

func init() {
	register(&Command{Name: "slide-comments", ValueOpts: []string{"requirements", "output"}, Summary: "List or edit native modern PowerPoint comments, text anchors, replies, resolution and reactions.", Usage: "paperworks slide-comments INPUT.pptx [--requirements COMMENT.json --output NEW.pptx]", Run: func(ctx context.Context, a Args) (any, error) {
		file, r, err := listOrEdit(ctx, a, "slide-comments", "PPTX", office.SlideCommentSpec{},
			func(s office.SlideCommentSpec) string { return s.Action }, office.SlideComments)
		if err != nil {
			return nil, err
		}
		return map[string]any{"file": file, "comments": r, "native_powerpoint_ui_verified": false}, nil
	}})
}
