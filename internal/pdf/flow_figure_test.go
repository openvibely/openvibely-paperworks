package pdf

import (
	"context"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

func TestFlowFigureCaptionPaginationAndPixels(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	asset := filepath.Join(dir, "figure.png")
	im := image.NewRGBA(image.Rect(0, 0, 240, 120))
	for y := 0; y < 120; y++ {
		for x := 0; x < 240; x++ {
			im.SetRGBA(x, y, color.RGBA{R: 220, G: 30, B: 40, A: 255})
		}
	}
	f, err := os.Create(asset)
	if err != nil {
		t.Fatal(err)
	}
	if err = png.Encode(f, im); err != nil {
		t.Fatal(err)
	}
	if err = f.Close(); err != nil {
		t.Fatal(err)
	}
	spec := CreateSpec{Paper: "A4", Flow: &FlowSpec{Blocks: []FlowBlock{
		{Kind: "paragraph", Text: "Before figure."},
		{Kind: "spacer", Height: 520},
		{Kind: "figure", Src: "figure.png", Width: 240, Height: 160, Caption: "Figure 1. Revenue projection shown in red; this caption wraps without splitting from its image."},
		{Kind: "paragraph", Text: "After figure."},
	}}}
	pages, err := flowPages(ctx, spec.Paper, spec.Flow)
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) != 2 || len(pages[0].Images) != 0 || len(pages[1].Images) != 1 {
		t.Fatalf("wrong figure pagination: %+v", pages)
	}
	out := filepath.Join(dir, "figure.pdf")
	if err = Create(ctx, spec, dir, out); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	text, err := PageText(ctx, raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(text) != 2 || strings.Contains(text[0], "Figure 1") || !strings.Contains(text[1], "Figure 1") || !strings.Contains(text[1], "After figure.") {
		t.Fatal(text)
	}
	// All caption words survive wrapping and extraction on the image's page.
	for _, word := range strings.Fields(spec.Flow.Blocks[2].Caption) {
		if !strings.Contains(text[1], word) {
			t.Fatalf("missing caption word %s", word)
		}
	}
	coloredPages := 0
	_, err = RenderImages(ctx, raw, RenderOptions{DPI: 72}, func(page int, img *image.RGBA) error {
		bounds := image.Rectangle{}
		pixels := 0
		for y := 0; y < img.Bounds().Dy(); y++ {
			for x := 0; x < img.Bounds().Dx(); x++ {
				c := img.RGBAAt(x, y)
				if c.R > 180 && c.G < 70 && c.B < 80 {
					pixels++
					bounds = bounds.Union(image.Rect(x, y, x+1, y+1))
				}
			}
		}
		if pixels > 0 {
			coloredPages++
			if bounds.Dx() < 238 || bounds.Dx() > 242 || bounds.Dy() < 118 || bounds.Dy() > 122 {
				t.Errorf("image proportions/size changed: %v", bounds)
			}
			if bounds.Min.Y < 90 || bounds.Max.Y > 214 {
				t.Errorf("figure not at next-page top: %v", bounds)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if coloredPages != 1 {
		t.Fatalf("figure appears on %d pages", coloredPages)
	}
}

func TestFlowHeadingKeptWithFollowingFigure(t *testing.T) {
	const (
		margin    = 72.0
		fontSize  = 12
		leading   = 16.8
		remaining = 160.0
		figureSrc = "figure.png"
	)
	paper := "A4"
	usable := types.PaperSize[paper].Height - 2*margin

	cases := []struct {
		name        string
		height      float64
		caption     string
		wrappedOnly bool
	}{
		{name: "one-line caption", height: 140, caption: "Figure caption."},
		{name: "wrapped caption", height: 100, caption: "A deliberately long caption that wraps across multiple short lines beside the figure.", wrappedOnly: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			captionLines, err := wrapFlow(context.Background(), tc.caption, "Helvetica", fontSize, 180)
			if err != nil {
				t.Fatal(err)
			}
			if tc.wrappedOnly {
				if len(captionLines) < 2 {
					t.Fatalf("caption did not wrap: %q", tc.caption)
				}
				oneLinePairHeight := float64(fontSize+6)*1.4 + leading/2 + tc.height + leading + leading/2
				if oneLinePairHeight > remaining {
					t.Fatalf("test setup does not fit a one-line caption: %.1f > %.1f", oneLinePairHeight, remaining)
				}
			} else if len(captionLines) != 1 {
				t.Fatalf("caption should occupy one line, got %d", len(captionLines))
			}

			flow := &FlowSpec{Margin: margin, FontSize: fontSize, Leading: leading, Blocks: []FlowBlock{
				{Kind: "spacer", Height: usable - remaining},
				{Kind: "heading", Text: "Keep me"},
				{Kind: "figure", Src: figureSrc, Width: 180, Height: tc.height, Caption: tc.caption},
			}}
			pages, err := flowPages(context.Background(), paper, flow)
			if err != nil {
				t.Fatal(err)
			}
			if len(pages) != 2 || len(pages[0].Text) != 0 || len(pages[0].Images) != 0 || len(pages[1].Images) != 1 {
				t.Fatalf("heading and figure were not kept together: %+v", pages)
			}
			if len(pages[1].Text) != 1+len(captionLines) || pages[1].Text[0].Value != "Keep me" {
				t.Fatalf("heading/caption text on next page is incomplete: %+v", pages[1].Text)
			}
			for i, line := range captionLines {
				if got := pages[1].Text[i+1].Value; got != line {
					t.Fatalf("caption line %d = %q, want %q", i+1, got, line)
				}
			}
		})
	}
}

func TestFlowHeadingFigureAlreadyFitsWithoutPageBreak(t *testing.T) {
	const (
		margin  = 72.0
		font    = 12
		leading = 16.8
	)
	paper := "A4"
	usable := types.PaperSize[paper].Height - 2*margin
	caption := "A deliberately long caption that wraps across multiple short lines beside the figure."
	captionLines, err := wrapFlow(context.Background(), caption, "Helvetica", font, 180)
	if err != nil {
		t.Fatal(err)
	}
	if len(captionLines) < 2 {
		t.Fatalf("caption did not wrap: %q", caption)
	}

	for _, tc := range []struct {
		name   string
		spacer float64
	}{
		{name: "at page start"},
		{name: "pair fits in remaining space", spacer: usable - 220},
	} {
		t.Run(tc.name, func(t *testing.T) {
			blocks := []FlowBlock{}
			if tc.spacer > 0 {
				blocks = append(blocks, FlowBlock{Kind: "spacer", Height: tc.spacer})
			}
			blocks = append(blocks,
				FlowBlock{Kind: "heading", Text: "Keep me"},
				FlowBlock{Kind: "figure", Src: "figure.png", Width: 180, Height: 100, Caption: caption},
			)
			pages, err := flowPages(context.Background(), paper, &FlowSpec{Margin: margin, FontSize: font, Leading: leading, Blocks: blocks})
			if err != nil {
				t.Fatal(err)
			}
			if len(pages) != 1 || len(pages[0].Images) != 1 || len(pages[0].Text) != len(captionLines)+1 || pages[0].Text[0].Value != "Keep me" {
				t.Fatalf("fitting heading/figure pair caused a page break: %+v", pages)
			}
		})
	}
}

func TestFlowHeadingFigureTooTallDoesNotAddBlankPages(t *testing.T) {
	const margin = 72.0
	paper := "A4"
	usable := types.PaperSize[paper].Height - 2*margin
	pages, err := flowPages(context.Background(), paper, &FlowSpec{Margin: margin, Blocks: []FlowBlock{
		{Kind: "heading", Text: "Keep me"},
		{Kind: "figure", Src: "figure.png", Width: 180, Height: usable - 10},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) != 2 || len(pages[0].Text) != 1 || pages[0].Text[0].Value != "Keep me" || len(pages[1].Images) != 1 {
		t.Fatalf("oversized heading/figure pair created unexpected pagination: %+v", pages)
	}
}

func TestFlowFigureRejectionIsAtomic(t *testing.T) {
	for _, b := range []FlowBlock{
		{Kind: "figure", Src: "missing.png", Width: 200, Height: 100},
		{Kind: "figure", Src: "https://example.com/x.png", Width: 200, Height: 100},
		{Kind: "figure", Src: "x.png", Width: 800, Height: 100},
		{Kind: "figure", Src: "x.png", Width: 200, Height: 800},
		{Kind: "figure", Src: "x.png", Width: 200, Height: 680, Caption: "This caption pushes the figure beyond the available page height."},
		{Kind: "figure", Src: "x.png", Width: 0, Height: 100},
	} {
		out := filepath.Join(t.TempDir(), "bad.pdf")
		if err := Create(context.Background(), CreateSpec{Flow: &FlowSpec{Blocks: []FlowBlock{b}}}, ".", out); err == nil {
			t.Fatal("accepted invalid figure", b)
		}
		if _, err := os.Stat(out); !os.IsNotExist(err) {
			t.Fatal("invalid figure created output")
		}
	}
}
