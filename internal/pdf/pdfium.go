package pdf

import (
	"context"
	"fmt"
	"image"
	"image/draw"
	"math"
	"os"
	"path/filepath"
	"sync"

	pdfium "github.com/klippa-app/go-pdfium"
	"github.com/klippa-app/go-pdfium/requests"
	"github.com/klippa-app/go-pdfium/webassembly"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/experimental"
)

var (
	poolOnce   sync.Once
	pool       pdfium.Pool
	poolErr    error
	engineGate = make(chan struct{}, 1)
)

// engine returns a PDFium instance running inside the embedded WebAssembly
// runtime. Documents are passed as bytes; only system fonts are exposed through
// a confined read-only filesystem with synthetic parent directories.
func engine(ctx context.Context) (pdfium.Pdfium, func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	select {
	case engineGate <- struct{}{}:
	case <-ctx.Done():
		return nil, nil, ctx.Err()
	}
	poolOnce.Do(func() {
		pool, poolErr = webassembly.Init(webassembly.Config{
			MinIdle: 1, MaxIdle: 1, MaxTotal: 1, ReuseWorkers: true,
			FSConfig:      systemFontFS(),
			RuntimeConfig: wazero.NewRuntimeConfig().WithCoreFeatures(api.CoreFeaturesV2 | experimental.CoreFeaturesExceptionHandling).WithMemoryLimitPages(8192).WithCloseOnContextDone(true),
		})
	})
	if poolErr != nil {
		<-engineGate
		return nil, nil, fmt.Errorf("start embedded PDFium: %w", poolErr)
	}
	inst, err := pool.GetInstanceWithContext(ctx)
	if err != nil {
		<-engineGate
		return nil, nil, err
	}
	// Borrowing with a context only cancels the pool wait. PDFium uses its own
	// worker context for calls, so cancellation must explicitly kill that worker.
	// Serialize Kill and Close: Kill intentionally bypasses the call mutex to
	// interrupt running WASM, but must never race with normal worker return.
	release := make(chan struct{})
	watched := make(chan bool, 1)
	go func() {
		select {
		case <-ctx.Done():
			_ = inst.Kill()
			watched <- true
		case <-release:
			watched <- false
		}
	}()
	return inst, func() {
		close(release)
		killed := <-watched
		if !killed {
			_ = inst.Close()
		}
		<-engineGate
	}, nil
}

type RenderOptions struct {
	DPISet       bool    // distinguish an explicitly supplied zero DPI from the omitted default
	MaxPixelsSet bool    // distinguish explicit zero from the omitted default
	Opaque       bool    // use the reference preview opaque bitmap; default retains alpha for appearance comparisons
	DPI          float64 // 0 means use MaxPixels
	MaxPixels    int     // longest edge in pixels when DPI is 0
	Pages        []int   // 1-based; empty means all
}

func (o *RenderOptions) Validate() error {
	if (o.DPISet || o.DPI != 0) && (o.DPI < 36 || o.DPI > 600 || math.IsNaN(o.DPI)) {
		return fmt.Errorf("DPI must be between 36 and 600")
	}
	if o.MaxPixels == 0 && !o.MaxPixelsSet {
		o.MaxPixels = 1600
	}
	if o.MaxPixels < 100 || o.MaxPixels > 10000 {
		return fmt.Errorf("max_pixels must be between 100 and 10000")
	}
	return nil
}

// RenderImages rasterizes pages with their form-field appearances. The
// callback receives a copy that remains valid after the call.
func RenderImages(ctx context.Context, data []byte, opt RenderOptions, each func(page int, img *image.RGBA) error) (int, error) {
	if err := opt.Validate(); err != nil {
		return 0, err
	}
	inst, done, err := engine(ctx)
	if err != nil {
		return 0, err
	}
	defer done()
	doc, err := inst.OpenDocument(&requests.OpenDocument{File: &data})
	if err != nil {
		return 0, fmt.Errorf("open PDF: %w", err)
	}
	defer inst.FPDF_CloseDocument(&requests.FPDF_CloseDocument{Document: doc.Document})
	count, err := inst.FPDF_GetPageCount(&requests.FPDF_GetPageCount{Document: doc.Document})
	if err != nil {
		return 0, err
	}
	pages := opt.Pages
	if len(pages) == 0 {
		for i := 1; i <= count.PageCount; i++ {
			pages = append(pages, i)
		}
	}
	for _, p := range pages {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		if p < 1 || p > count.PageCount {
			return 0, fmt.Errorf("page %d is outside 1-%d", p, count.PageCount)
		}
		page := requests.Page{ByIndex: &requests.PageByIndex{Document: doc.Document, Index: p - 1}}
		size, err := inst.GetPageSize(&requests.GetPageSize{Page: page})
		if err != nil {
			return 0, err
		}
		if err := renderDimensions(size.Width, size.Height, opt); err != nil {
			return 0, err
		}
		var rgba *image.RGBA
		if opt.Opaque {
			scale := float64(opt.MaxPixels) / math.Max(size.Width, size.Height)
			w := int(math.Max(1, math.Ceil(size.Width*scale)))
			h := int(math.Max(1, math.Ceil(size.Height*scale)))
			if opt.DPI != 0 {
				scale = math.Round(opt.DPI) / 72
				w, h = int(math.Ceil(size.Width*scale)), int(math.Ceil(size.Height*scale))
			}
			rgba, err = renderOpaque(inst, doc.Document, page, w, h)
			if err != nil {
				return 0, fmt.Errorf("render page %d: %w", p, err)
			}

		} else {
			if opt.DPI != 0 {
				r, err := inst.RenderPageInDPI(&requests.RenderPageInDPI{Page: page, DPI: int(math.Round(opt.DPI)), RenderForm: true, Document: &doc.Document})
				if err != nil {
					return 0, fmt.Errorf("render page %d: %w", p, err)
				}
				rgba = cloneRGBA(r.Result.RenderedImage)
				r.Cleanup()
			} else {
				scale := float64(opt.MaxPixels) / math.Max(size.Width, size.Height)
				w := int(math.Max(1, math.Round(size.Width*scale)))
				h := int(math.Max(1, math.Round(size.Height*scale)))
				r, err := inst.RenderPageInPixels(&requests.RenderPageInPixels{Page: page, Width: w, Height: h, RenderForm: true, Document: &doc.Document})
				if err != nil {
					return 0, fmt.Errorf("render page %d: %w", p, err)
				}
				rgba = cloneRGBA(r.Result.RenderedImage)
				r.Cleanup()
			}
		}

		if err := each(p, rgba); err != nil {
			return 0, err
		}
	}
	return len(pages), nil
}

func cloneRGBA(src image.Image) *image.RGBA {
	b := src.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	// PDFium bitmaps are white-filled already; composite over white anyway.
	draw.Draw(dst, dst.Bounds(), image.White, image.Point{}, draw.Src)
	draw.Draw(dst, dst.Bounds(), src, b.Min, draw.Over)
	return dst
}

// RenderPages writes page-NNNN.png files into an existing directory.
func RenderPages(ctx context.Context, pdfPath, outDir string, opt RenderOptions) ([]string, error) {
	opt.Opaque = true
	if err := opt.Validate(); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(pdfPath)
	if err != nil {
		return nil, err
	}
	var files []string
	_, err = RenderImages(ctx, data, opt, func(page int, img *image.RGBA) error {
		name := filepath.Join(outDir, fmt.Sprintf("page-%04d.png", page))
		f, err := os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err != nil {
			return err
		}
		if err := encodePagePNG(f, img); err != nil {
			f.Close()
			return err
		}
		files = append(files, name)
		return f.Close()
	})
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("renderer produced no page images")
	}
	return files, nil
}

// PageText returns the plain text of every page in order.
func PageText(ctx context.Context, data []byte) ([]string, error) {
	inst, done, err := engine(ctx)
	if err != nil {
		return nil, err
	}
	defer done()
	doc, err := inst.OpenDocument(&requests.OpenDocument{File: &data})
	if err != nil {
		return nil, fmt.Errorf("open PDF: %w", err)
	}
	defer inst.FPDF_CloseDocument(&requests.FPDF_CloseDocument{Document: doc.Document})
	count, err := inst.FPDF_GetPageCount(&requests.FPDF_GetPageCount{Document: doc.Document})
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, count.PageCount)
	for i := 0; i < count.PageCount; i++ {
		t, err := inst.GetPageText(&requests.GetPageText{Page: requests.Page{ByIndex: &requests.PageByIndex{Document: doc.Document, Index: i}}})
		if err != nil {
			return nil, err
		}
		out = append(out, t.Text)
	}
	return out, nil
}

// PageFonts lists font names used by text on each page.
func PageFonts(ctx context.Context, data []byte) ([]map[string]bool, error) {
	inst, done, err := engine(ctx)
	if err != nil {
		return nil, err
	}
	defer done()
	doc, err := inst.OpenDocument(&requests.OpenDocument{File: &data})
	if err != nil {
		return nil, fmt.Errorf("open PDF: %w", err)
	}
	defer inst.FPDF_CloseDocument(&requests.FPDF_CloseDocument{Document: doc.Document})
	count, err := inst.FPDF_GetPageCount(&requests.FPDF_GetPageCount{Document: doc.Document})
	if err != nil {
		return nil, err
	}
	var out []map[string]bool
	for i := 0; i < count.PageCount; i++ {
		s, err := inst.GetPageTextStructured(&requests.GetPageTextStructured{
			Page: requests.Page{ByIndex: &requests.PageByIndex{Document: doc.Document, Index: i}},
			Mode: requests.GetPageTextStructuredModeRects, CollectFontInformation: true,
		})
		if err != nil {
			return nil, err
		}
		m := map[string]bool{}
		for _, r := range s.Rects {
			if r.FontInformation != nil && r.FontInformation.Name != "" {
				m[r.FontInformation.Name] = true
			}
		}
		out = append(out, m)
	}
	return out, nil
}

// Bound both PDFium's bitmap and our Go copy before either allocates.
func renderDimensions(width, height float64, opt RenderOptions) error {
	if width <= 0 || height <= 0 || math.IsNaN(width) || math.IsNaN(height) || math.IsInf(width, 0) || math.IsInf(height, 0) {
		return fmt.Errorf("invalid PDF page dimensions")
	}
	scale := float64(opt.MaxPixels) / math.Max(width, height)
	if opt.DPI != 0 {
		scale = math.Round(opt.DPI) / 72
	}
	w, h := math.Ceil(width*scale), math.Ceil(height*scale)
	if w > 32768 || h > 32768 || w*h > 40_000_000 {
		return fmt.Errorf("render exceeds 40 million pixels or 32768 pixels per edge; reduce resolution")
	}
	return nil
}
