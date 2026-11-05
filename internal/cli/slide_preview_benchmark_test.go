package cli

import (
	"context"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/openvibely/openvibely-paperworks/internal/convert"
	"github.com/openvibely/openvibely-paperworks/internal/lo"
	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"github.com/openvibely/openvibely-paperworks/internal/pdf"
	"github.com/openvibely/openvibely-paperworks/internal/pptx"
)

// BenchmarkSlidePreview100 compares the former full-deck layout/render path
// with the selected path on a generated 100-slide deck. The fake rasterizer
// creates one small PNG per page so the benchmark is reproducible without
// LibreOffice; its page count is reported as pages-rendered/op.
func BenchmarkSlidePreview100(b *testing.B) {
	inputPath := makeSlidePreviewBenchmarkDeck(b)

	cases := []struct {
		name      string
		selection string
		selected  map[int]bool
		layouts   int
		pages     int
	}{
		{name: "all", layouts: 100, pages: 100},
		{name: "one-of-100", selection: "37", selected: map[int]bool{37: true}, layouts: 1, pages: 1},
		{name: "discontiguous-3-of-100", selection: "2,37,99", selected: map[int]bool{2: true, 37: true, 99: true}, layouts: 3, pages: 3},
	}
	for _, tc := range cases {
		for _, mode := range []string{"baseline", "selected"} {
			b.Run(mode+"/"+tc.name, func(b *testing.B) {
				out := filepath.Join(b.TempDir(), "preview")
				var durations []time.Duration
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					start := time.Now()
					var err error
					if mode == "baseline" {
						err = benchmarkFullSlidePreview(inputPath, out, tc.selected)
					} else {
						args := Args{Pos: []string{inputPath}, Flags: map[string]string{"output": out}}
						if tc.selection != "" {
							args.Flags["slides"] = tc.selection
						}
						render := previewFileRenderer(func(_ context.Context, _, renderDir, _, _ string, _ pdf.RenderOptions) ([]string, error) {
							return benchmarkPages(renderDir, tc.pages)
						})
						_, err = runSlidePreview(context.Background(), args, render)
					}
					durations = append(durations, time.Since(start))
					if err != nil {
						b.Fatal(err)
					}
				}
				b.StopTimer()
				sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
				median := durations[len(durations)/2]
				layouts, pages := tc.layouts, tc.pages
				if mode == "baseline" {
					layouts, pages = 100, 100
				}
				b.ReportMetric(float64(layouts), "layouts/op")
				b.ReportMetric(float64(pages), "pages-rendered/op")
				b.ReportMetric(float64(median.Nanoseconds()), "median-ns/op")
			})
		}
	}
}

// BenchmarkSlidePreview100LibreOffice compares real PDF export and page
// rasterization. Run with -benchtime=3x for a median over three full renders.
func BenchmarkSlidePreview100LibreOffice(b *testing.B) {
	if _, err := lo.Find(); err != nil {
		b.Skipf("LibreOffice unavailable: %v", err)
	}
	inputPath := makeSlidePreviewBenchmarkDeck(b)
	cases := []struct {
		name      string
		selection string
		selected  map[int]bool
		layouts   int
		pages     int
	}{
		{name: "all", layouts: 100, pages: 100},
		{name: "one-of-100", selection: "37", selected: map[int]bool{37: true}, layouts: 1, pages: 1},
	}
	for _, tc := range cases {
		for _, mode := range []string{"baseline", "selected"} {
			b.Run(mode+"/"+tc.name, func(b *testing.B) {
				out := filepath.Join(b.TempDir(), "preview")
				durations := make([]time.Duration, 0, b.N)
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					start := time.Now()
					var err error
					if mode == "baseline" {
						err = benchmarkFullSlidePreviewWithRenderer(inputPath, out, tc.selected, convert.RenderFile)
					} else {
						args := Args{Pos: []string{inputPath}, Flags: map[string]string{"output": out, "max-pixels": "1600"}}
						if tc.selection != "" {
							args.Flags["slides"] = tc.selection
						}
						_, err = runSlidePreview(context.Background(), args, convert.RenderFile)
					}
					durations = append(durations, time.Since(start))
					if err != nil {
						b.Fatal(err)
					}
				}
				b.StopTimer()
				sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
				median := durations[len(durations)/2]
				layouts, pages := tc.layouts, tc.pages
				if mode == "baseline" {
					layouts, pages = 100, 100
				}
				b.ReportMetric(float64(layouts), "layouts/op")
				b.ReportMetric(float64(pages), "pages-rendered/op")
				b.ReportMetric(float64(median.Nanoseconds()), "median-ns/op")
			})
		}
	}
}

func benchmarkFullSlidePreview(inputPath, out string, selected map[int]bool) error {
	render := previewFileRenderer(func(_ context.Context, _, renderDir, _, _ string, _ pdf.RenderOptions) ([]string, error) {
		pkg, err := opc.Open(inputPath)
		if err != nil {
			return nil, err
		}
		slides, err := office.PresentationSlides(pkg)
		if err != nil {
			return nil, err
		}
		return benchmarkPages(renderDir, len(slides))
	})
	return benchmarkFullSlidePreviewWithRenderer(inputPath, out, selected, render)
}

func benchmarkFullSlidePreviewWithRenderer(inputPath, out string, selected map[int]bool, render previewFileRenderer) error {
	pkg, err := opc.Open(inputPath)
	if err != nil {
		return err
	}
	reports, err := office.PresentationLayouts(pkg)
	if err != nil {
		return err
	}
	stage, cleanup, err := createPreviewStage(out, ".paperworks-slide-preview-benchmark-")
	if err != nil {
		return err
	}
	defer cleanup()
	snapshot := filepath.Join(stage, "source.pptx")
	if err := pkg.SaveNew(snapshot); err != nil {
		return err
	}
	renderDir := filepath.Join(stage, "rendered")
	pages, err := render(context.Background(), snapshot, renderDir, "", "", pdf.RenderOptions{MaxPixels: 1600, MaxPixelsSet: true})
	if err != nil {
		return err
	}
	_, err = publishSelectedSlidePreview(context.Background(), stage, out, pages, reports, selected)
	return err
}

func makeSlidePreviewBenchmarkDeck(b *testing.B) string {
	b.Helper()
	root := b.TempDir()
	inputPath := filepath.Join(root, "deck-100.pptx")
	slides := make([]pptx.Slide, 100)
	for i := range slides {
		slides[i].Elements = []pptx.Element{
			{Type: "text", Text: fmt.Sprintf("Quarterly results, slide %d", i+1), X: 0.5, Y: 0.5, W: 6, H: 0.8, FontSize: 24},
			{Type: "text", Text: "Revenue, margin, and operating expense", X: 0.5, Y: 1.5, W: 6, H: 0.6, FontSize: 16},
			{Type: "table", X: 0.5, Y: 2.5, W: 5, H: 1.5, Rows: [][]string{{"Metric", "Value"}, {"Revenue", "100"}}},
		}
	}
	if err := pptx.Create(context.Background(), pptx.Spec{Slides: slides}, root, inputPath); err != nil {
		b.Fatal(err)
	}
	return inputPath
}

func benchmarkPages(renderDir string, count int) ([]string, error) {
	if err := os.MkdirAll(renderDir, 0755); err != nil {
		return nil, err
	}
	pages := make([]string, 0, count)
	for i := 0; i < count; i++ {
		page := filepath.Join(renderDir, fmt.Sprintf("page-%03d.png", i+1))
		if err := writePreview(page, image.NewRGBA(image.Rect(0, 0, 8, 6)), "png"); err != nil {
			return nil, err
		}
		pages = append(pages, page)
	}
	return pages, nil
}
