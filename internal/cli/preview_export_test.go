package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/openvibely/openvibely-paperworks/internal/convert"
	"github.com/openvibely/openvibely-paperworks/internal/pdf"
	"github.com/xuri/excelize/v2"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

func readPreviewPNG(t *testing.T, path string) image.Image {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	im, err := png.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	return im
}

func TestPreviewPNGReuse(t *testing.T) {
	for _, cols := range []int{0, 1, 3, 10} {
		counts := []int{1, 11}
		if cols == 0 {
			counts = []int{1, 3}
		}
		for _, count := range counts {
			t.Run(fmt.Sprintf("columns%d/pages%d", cols, count), func(t *testing.T) {
				root := t.TempDir()
				stage := filepath.Join(root, "stage")
				rendered := filepath.Join(stage, "rendered")
				if err := os.MkdirAll(rendered, 0700); err != nil {
					t.Fatal(err)
				}
				var pages []string
				var original [][]byte
				var infos []os.FileInfo
				colors := make([]color.RGBA, count)
				for i := 0; i < count; i++ {
					colors[i] = color.RGBA{R: uint8(40 + i*13), G: uint8(70 + i*9), B: uint8(100 + i*7), A: 255}
					w, h := 40, 60 // portrait
					if i%2 == 1 {
						w, h = 80, 40 // landscape
					}
					p := filepath.Join(rendered, fmt.Sprintf("renderer-page-%04d.png", count-i))
					im := image.NewRGBA(image.Rect(0, 0, w, h))
					for y := 0; y < h; y++ {
						for x := 0; x < w; x++ {
							im.SetRGBA(x, y, colors[i])
						}
					}
					if err := writePreview(p, im, "png"); err != nil {
						t.Fatal(err)
					}
					if err := os.Chmod(p, 0644); err != nil {
						t.Fatal(err)
					}
					data, err := os.ReadFile(p)
					if err != nil {
						t.Fatal(err)
					}
					info, err := os.Stat(p)
					if err != nil {
						t.Fatal(err)
					}
					original = append(original, data)
					infos = append(infos, info)
					pages = append(pages, p)
				}
				out := filepath.Join(root, "out")
				result, err := exportRenderedPreviews(context.Background(), stage, out, pages, "png", cols)
				if err != nil {
					t.Fatal(err)
				}
				resultMap := result.(map[string]any)
				got := resultMap["pages"].([]string)
				if len(got) != count {
					t.Fatalf("pages result has %d entries, want %d", len(got), count)
				}
				for i, p := range got {
					wantName := fmt.Sprintf("page-%04d.png", i+1)
					if filepath.Base(p) != wantName {
						t.Fatalf("page result %d: got %q, want %q", i, filepath.Base(p), wantName)
					}
					data, err := os.ReadFile(p)
					if err != nil || !bytes.Equal(data, original[i]) {
						t.Fatal("PNG bytes changed", err)
					}
					info, err := os.Stat(p)
					if err != nil {
						t.Fatal(err)
					}
					if !os.SameFile(info, infos[i]) || info.Mode().Perm() != 0600 {
						t.Fatal("inode or permissions changed", info)
					}
					if _, err := os.Stat(pages[i]); !os.IsNotExist(err) {
						t.Fatal("rendered copy retained", err)
					}
				}

				sheetPath := filepath.Join(out, "contact-sheet.png")
				if cols == 0 {
					if resultMap["contact_sheet"] != "" {
						t.Fatalf("disabled contact sheet result: %v", resultMap["contact_sheet"])
					}
					if _, err := os.Stat(sheetPath); !os.IsNotExist(err) {
						t.Fatal("contact sheet created with columns zero", err)
					}
					return
				}
				if resultMap["contact_sheet"] != sheetPath {
					t.Fatalf("contact sheet result: got %v, want %s", resultMap["contact_sheet"], sheetPath)
				}
				info, err := os.Stat(sheetPath)
				if err != nil || info.Mode().Perm() != 0600 {
					t.Fatal("contact sheet permissions", err)
				}
				im := readPreviewPNG(t, sheetPath)
				rows := (count + cols - 1) / cols
				wantBounds := image.Rect(0, 0, cols*previewCellWidth, rows*previewCellHeight)
				if im.Bounds() != wantBounds {
					t.Fatalf("contact sheet dimensions: got %v, want %v", im.Bounds(), wantBounds)
				}
				for i := 0; i < count; i++ {
					cellX, cellY := i%cols*previewCellWidth, i/cols*previewCellHeight
					wantName := filepath.Base(got[i])
					wantBand := expectedPreviewLabelBand(wantName)
					for y := 0; y < previewLabelBandHeight; y++ {
						for x := 0; x < previewCellWidth; x++ {
							gotPixel := color.RGBAModel.Convert(im.At(cellX+x, cellY+y))
							wantPixel := color.RGBAModel.Convert(wantBand.At(x, y))
							if gotPixel != wantPixel {
								t.Fatalf("label for cell %d (%q) differs at (%d,%d): got %v, want %v", i, wantName, x, y, gotPixel, wantPixel)
							}
						}
					}

					// Thumbnail content is fitted below the label band in a 300x376
					// box, preserving both portrait and landscape page pixels.
					want := colors[i]
					center := image.Pt(cellX+previewCellWidth/2, cellY+previewLabelBandHeight+previewThumbnailTopGap+previewThumbnailHeight/2)
					if gotPixel := color.RGBAModel.Convert(im.At(center.X, center.Y)); gotPixel != want {
						t.Fatalf("thumbnail center for page %d: got %v, want %v", i+1, gotPixel, want)
					}
					if gotPixel := color.RGBAModel.Convert(im.At(cellX+previewCellWidth/2, cellY+previewLabelBandHeight+previewThumbnailTopGap-1)); gotPixel != (color.RGBA{255, 255, 255, 255}) {
						t.Fatalf("label-to-thumbnail gap for page %d: got %v, want white", i+1, gotPixel)
					}
					if i%2 == 0 { // 40x60 portrait: 250x376 at x=35, y=34.
						assertPreviewPixel(t, im, cellX+35, cellY+34, want)
						assertPreviewPixel(t, im, cellX+34, cellY+34, color.RGBA{255, 255, 255, 255})
					} else { // 80x40 landscape: 300x150 at x=10, y=147.
						assertPreviewPixel(t, im, cellX+10, cellY+147, want)
						assertPreviewPixel(t, im, cellX+10, cellY+146, color.RGBA{255, 255, 255, 255})
					}
				}
			})
		}
	}
}

func expectedPreviewLabelBand(label string) image.Image {
	band := image.NewRGBA(image.Rect(0, 0, previewCellWidth, previewLabelBandHeight))
	background := color.RGBA{R: 32, G: 40, B: 50, A: 255}
	for y := 0; y < previewLabelBandHeight; y++ {
		for x := 0; x < previewCellWidth; x++ {
			band.SetRGBA(x, y, background)
		}
	}
	drawer := font.Drawer{
		Dst:  band,
		Src:  image.NewUniform(color.White),
		Face: basicfont.Face7x13,
		Dot:  fixed.P(previewLabelLeftPadding, previewLabelBaseline),
	}
	drawer.DrawString(label)
	return band
}

func assertPreviewPixel(t *testing.T, im image.Image, x, y int, want color.RGBA) {
	t.Helper()
	if got := color.RGBAModel.Convert(im.At(x, y)); got != want {
		t.Fatalf("contact pixel (%d,%d): got %v, want %v", x, y, got, want)
	}
}

func TestPreviewPNGNoDecode(t *testing.T) {
	// A deliberately opaque payload proves the no-contact path never decodes.
	root := t.TempDir()
	stage := filepath.Join(root, "stage")
	os.Mkdir(stage, 0700)
	p := filepath.Join(stage, "rendered.png")
	os.WriteFile(p, []byte("not an encoded image"), 0644)
	if _, err := exportRenderedPreviews(context.Background(), stage, filepath.Join(root, "out"), []string{p}, "png", 0); err != nil {
		t.Fatal(err)
	}
}

func TestPreviewExportFailures(t *testing.T) {
	for _, kind := range []string{"cancelled", "missing", "decode", "collision", "existing", "empty-cancelled", "too-many"} {
		t.Run(kind, func(t *testing.T) {
			root := t.TempDir()
			stage := filepath.Join(root, "stage")
			os.Mkdir(stage, 0700)
			out := filepath.Join(root, "out")
			p := filepath.Join(stage, "rendered.png")
			os.WriteFile(p, []byte("invalid"), 0644)
			pages := []string{p}
			cols := 0
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			switch kind {
			case "cancelled":
				cancel()
			case "missing":
				pages = append(pages, filepath.Join(stage, "missing.png"))
			case "decode":
				cols = 3
			case "collision":
				os.WriteFile(filepath.Join(stage, "page-0001.png"), []byte("keep"), 0600)
			case "existing":
				os.Mkdir(out, 0700)
			case "empty-cancelled":
				pages = nil
				cancel()
			case "too-many":
				pages = make([]string, 201)
				cols = 3
			}
			if _, err := exportRenderedPreviews(ctx, stage, out, pages, "png", cols); err == nil {
				t.Fatal("expected error")
			}
			if kind == "existing" {
				entries, err := os.ReadDir(out)
				if err != nil || len(entries) != 0 {
					t.Fatal("existing output modified")
				}
			} else if _, err := os.Stat(out); !os.IsNotExist(err) {
				t.Fatal("partial output published", err)
			}
		})
	}
	// Command-level renderer failures must also remove private staging.
	root := t.TempDir()
	in := filepath.Join(root, "broken.pdf")
	os.WriteFile(in, []byte("invalid"), 0600)
	var stdout, stderr bytes.Buffer
	if Main([]string{"preview-export", in, "--output", filepath.Join(root, "out")}, &stdout, &stderr) == 0 {
		t.Fatal("expected render failure")
	}
	matches, _ := filepath.Glob(filepath.Join(root, ".paperworks-preview-*"))
	if len(matches) != 0 {
		t.Fatal("stage leaked", matches)
	}
}

func TestPreviewExportPNGRendererPixels(t *testing.T) {
	root := t.TempDir()
	in := filepath.Join(root, "input.pdf")
	var pages []any
	for i := 0; i < 3; i++ {
		w, h := 200, 300
		if i == 1 {
			w, h = h, w
		}
		pages = append(pages, map[string]any{"width": w, "height": h, "elements": []any{map[string]any{"type": "text", "x": 20, "y": 40, "text": fmt.Sprintf("Page %d Café", i+1)}}})
	}
	spec := writeSpec(t, root, "pdf.json", map[string]any{"canvas": map[string]any{"pages": pages}})
	runCLI(t, "create-pdf", "--spec", spec, "--output", in)
	for _, size := range []int{1600, 3200} {
		for _, selection := range [][]int{{1}, {1, 2, 3}, {3, 1}} {
			for _, cols := range []int{0, 3} {
				t.Run(fmt.Sprintf("%d/%v/%d", size, selection, cols), func(t *testing.T) {
					stage := t.TempDir()
					paths, err := convert.RenderFile(context.Background(), in, filepath.Join(stage, "rendered"), "", "", pdf.RenderOptions{MaxPixels: size, Pages: selection})
					if err != nil {
						t.Fatal(err)
					}
					original := make([][]byte, len(paths))
					for i, p := range paths {
						original[i], err = os.ReadFile(p)
						if err != nil {
							t.Fatal(err)
						}
					}
					out := filepath.Join(t.TempDir(), "out")
					r, err := exportRenderedPreviews(context.Background(), stage, out, paths, "png", cols)
					if err != nil {
						t.Fatal(err)
					}
					for i, p := range r.(map[string]any)["pages"].([]string) {
						data, err := os.ReadFile(p)
						if err != nil || !bytes.Equal(data, original[i]) {
							t.Fatal("renderer pixels/dimensions changed", err)
						}
					}
				})
			}
		}
	}
	for _, format := range []string{"", "png", "webp"} {
		args := []string{"preview-export", in, "--output", filepath.Join(root, "cli-"+format)}
		if format != "" {
			args = append(args, "--format", format)
		}
		r := runCLI(t, args...)
		if len(r["pages"].([]any)) != 3 {
			t.Fatal(r)
		}
	}
}

func TestPreviewExportXLSXSelection(t *testing.T) {
	root := t.TempDir()
	in := filepath.Join(root, "input.xlsx")
	f := excelize.NewFile()
	f.SetCellValue("Sheet1", "A1", "selected")
	f.SetCellValue("Sheet1", "Z100", "outside")
	f.NewSheet("Other")
	f.SetCellValue("Other", "A1", "other sheet")
	if err := f.SaveAs(in); err != nil {
		t.Fatal(err)
	}
	f.Close()
	for _, cellRange := range []string{"", "A1:B4"} {
		for _, cols := range []string{"0", "3"} {
			args := []string{"--sheet", "Sheet1"}
			if cellRange != "" {
				args = append(args, "--range", cellRange)
			}
			ref := runCLI(t, append([]string{"render", in, "--output", filepath.Join(t.TempDir(), "reference")}, args...)...)
			r := runCLI(t, append([]string{"preview-export", in, "--output", filepath.Join(t.TempDir(), "export"), "--columns", cols}, args...)...)
			want, got := ref["pages"].([]any), r["pages"].([]any)
			if len(want) != len(got) {
				t.Fatal("page count")
			}
			for i := range want {
				a, _ := os.ReadFile(want[i].(string))
				b, _ := os.ReadFile(got[i].(string))
				if !bytes.Equal(a, b) {
					t.Fatal("selection pixels changed")
				}
			}
		}
	}
}

// Cancel on the final check, after a page has already moved into staging.
type previewCancelBeforePublish struct {
	context.Context
	calls int
}

func (c *previewCancelBeforePublish) Err() error {
	c.calls++
	if c.calls > 1 {
		return context.Canceled
	}
	return nil
}

func TestPreviewExportCancellationBeforePublication(t *testing.T) {
	root := t.TempDir()
	stage := filepath.Join(root, "stage")
	if err := os.Mkdir(stage, 0700); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(stage, "source.png")
	if err := os.WriteFile(p, []byte("opaque renderer output"), 0644); err != nil {
		t.Fatal(err)
	}
	ctx := &previewCancelBeforePublish{Context: context.Background()}
	out := filepath.Join(root, "out")
	if _, err := exportRenderedPreviews(ctx, stage, out, []string{p}, "png", 0); err != context.Canceled {
		t.Fatal(err)
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Fatal("cancelled export was published")
	}
	if _, err := os.Stat(filepath.Join(stage, "page-0001.png")); err != nil {
		t.Fatal("test did not reach publication", err)
	}
}

func TestPreviewExportCommandCancellationCleanup(t *testing.T) {
	root := t.TempDir()
	out := filepath.Join(root, "out")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := commands["preview-export"].Run(ctx, Args{Pos: []string{"../../testdata/reference-mirror/form.pdf"}, Flags: map[string]string{"output": out}})
	if err == nil {
		t.Fatal("expected cancellation")
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 0 {
		t.Fatal("cancelled command left staging or output", entries, err)
	}
}

func TestPreviewExportRejectsExistingOutput(t *testing.T) {
	for _, symlink := range []bool{false, true} {
		t.Run(fmt.Sprint(symlink), func(t *testing.T) {
			root := t.TempDir()
			out := filepath.Join(root, "out")
			if symlink {
				if err := os.Symlink(filepath.Join(root, "missing"), out); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := os.Mkdir(out, 0700); err != nil {
					t.Fatal(err)
				}
			}
			_, err := commands["preview-export"].Run(context.Background(), Args{Pos: []string{"../../testdata/reference-mirror/form.pdf"}, Flags: map[string]string{"output": out}})
			var usage UsageError
			if !errors.As(err, &usage) || !errors.Is(err, os.ErrExist) {
				t.Fatal("expected existing-output rejection with usage and filesystem identity", err)
			}
			if _, err := os.Lstat(out); err != nil {
				t.Fatal("existing path changed", err)
			}
			stages, err := filepath.Glob(filepath.Join(root, ".paperworks-preview-*"))
			if err != nil || len(stages) > 0 {
				t.Fatal("stage leaked", stages, err)
			}
		})
	}
}
