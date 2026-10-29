package cli

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"

	"github.com/HugoSmits86/nativewebp"
	"github.com/openvibely/openvibely-paperworks/internal/convert"
	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

// previewOutputExistsError retains the usage diagnostic and exit code while
// exposing the filesystem identity expected by callers.
type previewOutputExistsError struct{ UsageError }

func (e previewOutputExistsError) Unwrap() []error {
	return []error{e.UsageError, os.ErrExist}
}

const (
	previewCellWidth        = 320
	previewCellHeight       = 420
	previewLabelBandHeight  = 24
	previewThumbnailTopGap  = 10
	previewThumbnailWidth   = 300
	previewThumbnailHeight  = 376
	previewLabelLeftPadding = 8
	previewLabelBaseline    = 18
)

func init() {
	register(&Command{Name: "preview-export", ValueOpts: []string{"output", "format", "columns", "max-pixels", "dpi", "sheet", "range"}, Summary: "Export document previews as PNG/JPEG/WebP and an optional contact sheet.", Usage: "paperworks preview-export INPUT --output NEW_DIR [--format png|jpeg|webp] [--columns 3] [--dpi N | --max-pixels N] [--sheet NAME --range A1:D20]", Run: func(ctx context.Context, a Args) (any, error) {
		return runPreviewExport(ctx, a, convert.RenderFile)
	}})
}

func runPreviewExport(ctx context.Context, a Args, render previewFileRenderer) (any, error) {
	in, err := input(a, 1)
	if err != nil {
		return nil, err
	}
	if err = a.Require("output"); err != nil {
		return nil, err
	}
	format := a.Str("format")
	if format == "" {
		format = "png"
	}
	if format != "png" && format != "jpeg" && format != "webp" {
		return nil, usagef("format must be png, jpeg or webp")
	}
	cols, err := a.Int("columns", 0)
	if err != nil {
		return nil, err
	}
	if cols < 0 || cols > 10 {
		return nil, usagef("columns must be 0..10; zero disables contact sheet")
	}
	opt, err := renderOptions(a)
	if err != nil {
		return nil, UsageError{err.Error()}
	}
	out := a.Path("output")
	if _, err = os.Lstat(out); !os.IsNotExist(err) {
		usage := UsageError{"output must be a new directory"}
		if err == nil {
			return nil, previewOutputExistsError{usage}
		}
		return nil, usage
	}
	stage, cleanup, err := createPreviewStage(out, ".paperworks-preview-")
	if err != nil {
		return nil, err
	}
	defer cleanup()
	pages, err := render(ctx, in[0], filepath.Join(stage, "rendered"), a.Str("sheet"), a.Str("range"), opt)
	if err != nil {
		return nil, officeErr(err)
	}
	return exportRenderedPreviews(ctx, stage, out, pages, format, cols)
}

// exportRenderedPreviews consumes renderer output in its returned page order.
// The caller owns staging cleanup on both success and failure.
func exportRenderedPreviews(ctx context.Context, stage, out string, pages []string, format string, cols int) (any, error) {
	var err error
	if cols > 0 && len(pages) > 200 {
		return nil, usagef("contact sheet supports at most 200 pages; select a smaller document")
	}
	results := []string{}
	var sheet *image.RGBA
	for i, p := range pages {
		if err = ctx.Err(); err != nil {
			return nil, err
		}
		if cols > 0 && sheet == nil {
			rows := (len(pages) + cols - 1) / cols
			sheet = image.NewRGBA(image.Rect(0, 0, cols*previewCellWidth, rows*previewCellHeight))
			draw.Draw(sheet, sheet.Bounds(), &image.Uniform{color.White}, image.Point{}, draw.Src)
		}
		var im image.Image
		if format != "png" || cols > 0 {
			f, err := os.Open(p)
			if err != nil {
				return nil, err
			}
			im, err = png.Decode(f)
			closeErr := f.Close()
			if err != nil {
				return nil, err
			}
			if closeErr != nil {
				return nil, closeErr
			}
		}
		name := fmt.Sprintf("page-%04d.%s", i+1, format)
		dest := filepath.Join(stage, name)
		if format == "png" {
			// The renderer already encoded the final pixels. Move that inode
			// exclusively within staging, without a second encoded page set.
			if err = os.Chmod(p, 0600); err != nil {
				return nil, err
			}
			if err = renamePreviewExclusive(p, dest); err != nil {
				return nil, err
			}
		} else if err = writePreview(dest, im, format); err != nil {
			return nil, err
		}
		results = append(results, filepath.Join(out, name))
		if cols > 0 {
			cellX, cellY := i%cols*previewCellWidth, i/cols*previewCellHeight
			cellBounds := image.Rect(cellX, cellY, cellX+previewCellWidth, cellY+previewCellHeight)
			cell := sheet.SubImage(cellBounds).(*image.RGBA)
			bounds := im.Bounds()
			w, h := previewThumbnailWidth, previewThumbnailHeight
			if bounds.Dx()*h > bounds.Dy()*w {
				h = bounds.Dy() * w / bounds.Dx()
			} else {
				w = bounds.Dx() * h / bounds.Dy()
			}
			x := cellX + (previewCellWidth-w)/2
			y := cellY + previewLabelBandHeight + previewThumbnailTopGap + (previewThumbnailHeight-h)/2
			r := image.Rect(x, y, x+w, y+h)
			xdraw.CatmullRom.Scale(sheet, r, im, bounds, draw.Over, nil)
			drawPreviewPageLabel(cell, name)
		}
	}
	contact := ""
	if sheet != nil {
		if err = writePreview(filepath.Join(stage, "contact-sheet.png"), sheet, "png"); err != nil {
			return nil, err
		}
		contact = filepath.Join(out, "contact-sheet.png")
	}
	if err = os.RemoveAll(filepath.Join(stage, "rendered")); err != nil {
		return nil, err
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	if err = renamePreviewExclusive(stage, out); err != nil {
		return nil, err
	}
	return map[string]any{"pages": results, "contact_sheet": contact, "visually_inspected": false}, nil
}

func drawPreviewPageLabel(cell *image.RGBA, label string) {
	origin := cell.Bounds().Min
	band := image.Rect(origin.X, origin.Y, origin.X+previewCellWidth, origin.Y+previewLabelBandHeight)
	draw.Draw(cell, band, &image.Uniform{color.RGBA{R: 32, G: 40, B: 50, A: 255}}, image.Point{}, draw.Src)
	drawer := font.Drawer{
		Dst:  cell,
		Src:  image.NewUniform(color.White),
		Face: basicfont.Face7x13,
		Dot:  fixed.P(cell.Bounds().Min.X+previewLabelLeftPadding, cell.Bounds().Min.Y+previewLabelBaseline),
	}
	drawer.DrawString(label)
}

func writePreview(p string, im image.Image, format string) error {
	f, err := os.OpenFile(p, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	if format == "jpeg" {
		err = jpeg.Encode(f, im, &jpeg.Options{Quality: 92})
	} else if format == "webp" {
		err = nativewebp.Encode(f, im, nil)
	} else {
		err = png.Encode(f, im)
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	return closeErr
}
