package cli

import (
	"context"
	"github.com/openvibely/openvibely-paperworks/internal/pptx"
	"path/filepath"
)

func init() {
	register(&Command{Name: "create-pptx", Summary: "Create native slides, text, tables, charts, images and speaker notes from JSON.", Usage: "paperworks create-pptx --spec DECK.json --output NEW.pptx", Run: func(ctx context.Context, a Args) (any, error) {
		if len(a.Pos) != 0 {
			return nil, usagef("create-pptx takes no positional arguments")
		}
		out := a.Path("output")
		if err := newFile(out, ""); err != nil {
			return nil, err
		}
		var s pptx.Spec
		if err := a.JSON("spec", &s); err != nil {
			return nil, err
		}
		if err := pptx.Create(ctx, s, filepath.Dir(a.Path("spec")), out); err != nil {
			return nil, err
		}
		return map[string]any{"output": out, "slides": len(s.Slides), "visual_review_required": true, "native_application_verified": false}, nil
	}})
}

func init() {
	register(&Command{Name: "slide-edit", Summary: "Edit PPTX text, formatting, geometry, images, fills and notes while preserving unrelated parts.", Usage: "paperworks slide-edit INPUT.pptx --spec EDITS.json --output NEW.pptx", Run: func(ctx context.Context, a Args) (any, error) {
		in, out, err := oneInOut(a)
		if err != nil {
			return nil, err
		}
		if ext(in) != ".pptx" {
			return nil, usagef("slide-edit requires PPTX")
		}
		var s pptx.EditSpec
		if err = a.JSON("spec", &s); err != nil {
			return nil, err
		}
		for i := range s.Edits {
			if s.Edits[i].Path != "" && !filepath.IsAbs(s.Edits[i].Path) {
				s.Edits[i].Path = filepath.Join(filepath.Dir(a.Path("spec")), s.Edits[i].Path)
			}
		}
		parts, err := pptx.EditFile(ctx, in, out, s)
		if err != nil {
			return nil, err
		}
		return map[string]any{"output": out, "changed_parts": parts, "visual_review_required": true}, nil
	}})
}
