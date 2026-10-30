package pdf

import (
	"context"
	"errors"
	"fmt"
	"image"
	"image/draw"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
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
	poolOnce.Do(func() { pool, poolErr = initPool() })
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

func runtimeConfig() wazero.RuntimeConfig {
	return wazero.NewRuntimeConfig().WithCoreFeatures(api.CoreFeaturesV2 | experimental.CoreFeaturesExceptionHandling).WithMemoryLimitPages(8192).WithCloseOnContextDone(true)
}

func initPool() (pdfium.Pool, error) {
	cfg := webassembly.Config{
		MinIdle: 1, MaxIdle: 1, MaxTotal: 1, ReuseWorkers: true,
		FSConfig: systemFontFS(),
	}
	if cache, versionDir := compilationCache(); cache != nil {
		cached := cfg
		cached.RuntimeConfig = runtimeConfig().WithCompilationCache(cache)
		p, err := webassembly.Init(cached)
		if err == nil {
			return p, nil
		}
		// wazero reports corrupt entries and failed writes as compile errors and
		// never evicts them, so drop this version's entries for the next run.
		_ = cache.Close(context.Background())
		_ = os.RemoveAll(versionDir)
	}
	cfg.RuntimeConfig = runtimeConfig()
	return webassembly.Init(cfg)
}

// compilationCache returns a persistent cache of compiled PDFium native code
// and the wazero version directory holding its entries, or nil when no
// private, writable cache directory is available.
// PAPERWORKS_WASM_CACHE overrides the directory; "off" disables caching.
func compilationCache() (wazero.CompilationCache, string) {
	dir, ok := compilationCacheDir()
	if !ok {
		return nil, ""
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, ""
	}
	// The cache holds executable code, so every directory that could swap or
	// plant entries must be controlled by this user (or root).
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil || !trustedCachePath(resolved) {
		return nil, ""
	}
	versionDir := filepath.Join(resolved, wazeroCacheSubdir())
	if err := os.Mkdir(versionDir, 0o700); err != nil && !errors.Is(err, fs.ErrExist) {
		return nil, ""
	}
	if !privateDir(versionDir) || !writableDir(versionDir) {
		return nil, ""
	}
	cache, err := wazero.NewCompilationCacheWithDir(resolved)
	if err != nil {
		return nil, ""
	}
	return cache, versionDir
}

// wazeroCacheSubdir mirrors the version-specific directory name chosen by
// wazero.NewCompilationCacheWithDir.
func wazeroCacheSubdir() string {
	version := ""
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, dep := range info.Deps {
			if strings.Contains(dep.Path, "github.com/tetratelabs/wazero") {
				version = dep.Version
			}
		}
		if version == "" || version == "(devel)" {
			version = info.Main.Version
		}
	}
	if version == "" || version == "(devel)" {
		version = "dev"
	}
	return "wazero-" + version + "-" + runtime.GOARCH + "-" + runtime.GOOS
}

// trustedCachePath reports whether dir is private to this user and no
// ancestor lets another user replace it.
func trustedCachePath(dir string) bool {
	if !privateDir(dir) {
		return false
	}
	for p := filepath.Dir(dir); ; p = filepath.Dir(p) {
		st, err := os.Stat(p)
		if err != nil || !st.IsDir() {
			return false
		}
		owner, ok := fileOwner(st)
		if !ok || (owner != cacheUID() && owner != 0) {
			return false
		}
		if st.Mode().Perm()&0o022 != 0 && st.Mode()&fs.ModeSticky == 0 {
			return false
		}
		if parent := filepath.Dir(p); parent == p {
			return true
		}
	}
}

func privateDir(dir string) bool {
	st, err := os.Lstat(dir)
	if err != nil || !st.IsDir() || st.Mode().Perm()&0o022 != 0 {
		return false
	}
	owner, ok := fileOwner(st)
	return ok && owner == cacheUID()
}

func writableDir(dir string) bool {
	f, err := os.CreateTemp(dir, ".probe-*")
	if err != nil {
		return false
	}
	name := f.Name()
	_ = f.Close()
	return os.Remove(name) == nil
}

func compilationCacheDir() (string, bool) {
	switch v := os.Getenv("PAPERWORKS_WASM_CACHE"); v {
	case "off", "0", "false":
		return "", false
	case "":
	default:
		if !filepath.IsAbs(v) {
			return "", false
		}
		return v, true
	}
	base := os.Getenv("XDG_CACHE_HOME")
	if base == "" {
		var err error
		if base, err = os.UserCacheDir(); err != nil {
			return "", false
		}
	}
	if !filepath.IsAbs(base) {
		return "", false
	}
	return filepath.Join(base, "paperworks", "wazero"), true
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
		f, err := os.CreateTemp(outDir, ".paperworks-page-*")
		if err != nil {
			return err
		}
		defer os.Remove(f.Name())
		if err := encodePagePNG(f, img); err != nil {
			f.Close()
			return err
		}
		if err := f.Chmod(0o644); err != nil {
			f.Close()
			return err
		}
		if err := f.Close(); err != nil {
			return err
		}
		if err := os.Rename(f.Name(), name); err != nil {
			return err
		}
		files = append(files, name)
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("renderer produced no page images")
	}
	if err := removeStalePageImages(outDir, files); err != nil {
		return nil, err
	}
	return files, nil
}

// removeStalePageImages removes only numbered PNGs owned by RenderPages, and is
// called after the full render succeeds so a failed render cannot prune previews.
func removeStalePageImages(outDir string, current []string) error {
	keep := make(map[string]struct{}, len(current))
	for _, path := range current {
		keep[filepath.Base(path)] = struct{}{}
	}
	entries, err := os.ReadDir(outDir)
	if err != nil {
		return fmt.Errorf("list rendered page images: %w", err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !isPageImageName(name) {
			continue
		}
		if _, ok := keep[name]; ok {
			continue
		}
		if err := os.Remove(filepath.Join(outDir, name)); err != nil {
			return fmt.Errorf("remove stale page image %q: %w", name, err)
		}
	}
	return nil
}

func isPageImageName(name string) bool {
	const prefix, suffix = "page-", ".png"
	if !strings.HasPrefix(name, prefix) || !strings.HasSuffix(name, suffix) {
		return false
	}
	number := strings.TrimSuffix(strings.TrimPrefix(name, prefix), suffix)
	if len(number) < 4 {
		return false
	}
	for _, digit := range number {
		if digit < '0' || digit > '9' {
			return false
		}
	}
	page, err := strconv.Atoi(number)
	return err == nil && page > 0
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
