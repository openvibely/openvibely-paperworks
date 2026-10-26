package cli

import (
	"context"
	"github.com/openvibely/openvibely-paperworks/internal/pdf"
	"os"
)

func init() {
	register(&Command{Name: "pdf-fonts", ValueOpts: []string{}, Summary: "List font names, embedding status and pages used by PDF text objects, including nested forms.", Usage: "paperworks pdf-fonts INPUT.pdf", Run: func(ctx context.Context, a Args) (any, error) {
		in, err := input(a, 1)
		if err != nil {
			return nil, err
		}
		data, err := os.ReadFile(in[0])
		if err != nil {
			return nil, err
		}
		return pdf.ListFonts(ctx, data)
	}})
}
