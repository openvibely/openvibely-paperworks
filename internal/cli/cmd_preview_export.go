package cli

import (
	"context"
	"fmt"
	"github.com/HugoSmits86/nativewebp"
	"github.com/openvibely/openvibely-paperworks/internal/convert"
	xdraw "golang.org/x/image/draw"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
)

func init() {
	register(&Command{Name: "preview-export", Summary: "Export document previews as PNG/JPEG/WebP and an optional contact sheet.", Usage: "paperworks preview-export INPUT --output NEW_DIR [--format png|jpeg|webp] [--columns 3] [--max-pixels 1600] [--sheet NAME --range A1:D20]", Run: func(ctx context.Context, a Args) (any, error) {
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
			return nil, err
		}
		out := a.Path("output")
		if _, err = os.Stat(out); !os.IsNotExist(err) {
			return nil, usagef("output must be a new directory")
		}
		stage, err := os.MkdirTemp(filepath.Dir(out), ".paperworks-preview-")
		if err != nil {
			return nil, err
		}
		defer os.RemoveAll(stage)
		pages, err := convert.RenderFile(ctx, in[0], filepath.Join(stage, "rendered"), a.Str("sheet"), a.Str("range"), opt)
		if err != nil {
			return nil, officeErr(err)
		}
		results := []string{}
		thumbs := []image.Image{}
		for i, p := range pages {
			if err = ctx.Err(); err != nil {
				return nil, err
			}
			f, err := os.Open(p)
			if err != nil {
				return nil, err
			}
			im, err := png.Decode(f)
			f.Close()
			if err != nil {
				return nil, err
			}
			name := fmt.Sprintf("page-%04d.%s", i+1, format)
			if err = writePreview(filepath.Join(stage, name), im, format); err != nil {
				return nil, err
			}
			results = append(results, filepath.Join(out, name))
			if cols > 0 {
				if len(pages) > 200 {
					return nil, usagef("contact sheet supports at most 200 pages; select a smaller document")
				}
				thumb := image.NewRGBA(image.Rect(0, 0, 320, 420))
				draw.Draw(thumb, thumb.Bounds(), &image.Uniform{color.White}, image.Point{}, draw.Src)
				bounds := im.Bounds()
				w, h := 300, 400
				if bounds.Dx()*h > bounds.Dy()*w {
					h = bounds.Dy() * w / bounds.Dx()
				} else {
					w = bounds.Dx() * h / bounds.Dy()
				}
				r := image.Rect((320-w)/2, (420-h)/2, (320+w)/2, (420+h)/2)
				xdraw.CatmullRom.Scale(thumb, r, im, bounds, draw.Over, nil)
				thumbs = append(thumbs, thumb)
			}
		}
		contact := ""
		if cols > 0 && len(thumbs) > 0 {
			rows := (len(thumbs) + cols - 1) / cols
			sheet := image.NewRGBA(image.Rect(0, 0, cols*320, rows*420))
			draw.Draw(sheet, sheet.Bounds(), &image.Uniform{color.White}, image.Point{}, draw.Src)
			for i, im := range thumbs {
				r := image.Rect(i%cols*320, i/cols*420, (i%cols+1)*320, (i/cols+1)*420)
				draw.Draw(sheet, r, im, image.Point{}, draw.Src)
			}
			if err = writePreview(filepath.Join(stage, "contact-sheet.png"), sheet, "png"); err != nil {
				return nil, err
			}
			contact = filepath.Join(out, "contact-sheet.png")
		}
		if err = os.RemoveAll(filepath.Join(stage, "rendered")); err != nil {
			return nil, err
		}
		if err = os.Rename(stage, out); err != nil {
			return nil, err
		}
		return map[string]any{"pages": results, "contact_sheet": contact, "visually_inspected": false}, nil
	}})
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
