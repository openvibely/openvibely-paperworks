package cli

import (
	"context"
	"github.com/openvibely/openvibely-paperworks/internal/pdf"
	"os"
)

func init() {
	register(&Command{Name: "pdf-layout", ValueOpts: []string{"spec"}, Summary: "Extract PDF characters with positions, selected regions, and explicitly bounded tables (not OCR).", Usage: "paperworks pdf-layout INPUT.pdf [--spec SELECTION.json]", Run: func(ctx context.Context, a Args) (any, error) {
		in, err := input(a, 1)
		if err != nil {
			return nil, err
		}
		var s pdf.LayoutOptions
		if a.Str("spec") != "" {
			if err = a.JSON("spec", &s); err != nil {
				return nil, err
			}
		}
		data, err := os.ReadFile(in[0])
		if err != nil {
			return nil, err
		}
		return pdf.ExtractLayout(ctx, data, s)
	}})
}
