package pdf

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
	"strings"
	"testing"
)

func TestOpaqueBGRPaddingAndOwnership(t *testing.T) {
	// Three-byte rows are padded to four bytes; padding is not a pixel.
	src := []byte{1, 2, 3, 99, 4, 5, 6, 88}
	got, err := opaqueBGR(src, 4, 1, 2)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got.Pix, []byte{3, 2, 1, 255, 6, 5, 4, 255}) {
		t.Fatal(got.Pix)
	}
	src[0] = 40
	if got.Pix[2] != 1 {
		t.Fatal("bitmap aliases worker memory")
	}
	for _, c := range []struct{ s, w, h int }{{0, 1, 1}, {2, 1, 1}, {4, 2, 1}, {4, 1, 3}, {-1, 1, 1}, {4, 0, 1}} {
		if _, err := opaqueBGR(src, c.s, c.w, c.h); err == nil {
			t.Fatal(c)
		}
	}
}

func TestOpaquePreviewFormsAndCancellation(t *testing.T) {
	data := formPDF()
	ctx, cancel := context.WithCancel(context.Background())
	sentinel := errors.New("stop after first page")
	calls := 0
	_, err := RenderImages(ctx, data, RenderOptions{DPI: 144, Opaque: true}, func(page int, img *image.RGBA) error {
		calls++
		if page != 1 || !equalBools(radioMarks(img), []bool{true, false, false}) {
			t.Fatal("lost radio state")
		}
		return sentinel
	})
	if !errors.Is(err, sentinel) || calls != 1 {
		t.Fatal(err, calls)
	}
	cancel()
	calls = 0
	_, err = RenderImages(ctx, data, RenderOptions{DPI: 144, Opaque: true}, func(int, *image.RGBA) error { calls++; return nil })
	if !errors.Is(err, context.Canceled) || calls != 0 {
		t.Fatal(err, calls)
	}
	// Exercise the public preview path, then reuse the worker after early return.
	source := writeTemp(t, "source.pdf", data)
	out := t.TempDir()
	paths, err := RenderPages(context.Background(), source, out, RenderOptions{DPI: 144})
	if err != nil || len(paths) != 2 {
		t.Fatal(paths, err)
	}
	for _, p := range paths {
		if filepath.Dir(p) != out {
			t.Fatal(p)
		}
	}
	after, err := os.ReadFile(source)
	if err != nil || !bytes.Equal(data, after) {
		t.Fatal("source changed", err)
	}
}

func TestCroppedPreviewFractionalScale(t *testing.T) {
	objects := formObjs()
	for _, index := range []int{2, 3} {
		objects[index] = strings.Replace(objects[index], "/MediaBox [0 0 400 400]", "/MediaBox [0 0 400 400] /CropBox [17 23 369 353]", 1)
	}
	data := rawPDF(objects)
	for _, tc := range []struct {
		opt           RenderOptions
		width, height int
	}{
		{RenderOptions{Opaque: true, MaxPixels: 333}, 333, 313},
		{RenderOptions{Opaque: true, DPI: 72}, 352, 330},
		{RenderOptions{Opaque: true, DPI: 144}, 704, 660},
	} {
		count, err := RenderImages(context.Background(), data, tc.opt, func(page int, img *image.RGBA) error {
			if img.Bounds().Dx() != tc.width || img.Bounds().Dy() != tc.height {
				t.Fatalf("page %d: got %v, want %dx%d", page, img.Bounds(), tc.width, tc.height)
			}
			return nil
		})
		if err != nil || count != 2 {
			t.Fatal(count, err)
		}
	}
}

func TestPreviewStampVisibility(t *testing.T) {
	for _, tc := range []struct {
		name    string
		flags   int
		visible bool
	}{
		{"visible", 4, true}, {"hidden", 6, false}, {"noview", 36, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			objects := formObjs()
			annotation, appearance := len(objects)+1, len(objects)+2
			for _, index := range []int{2, 3} {
				objects[index] = strings.Replace(objects[index], "/Annots [", fmt.Sprintf("/Annots [%d 0 R ", annotation), 1)
			}
			objects = append(objects, fmt.Sprintf("<< /Type /Annot /Subtype /Stamp /Rect [250 20 350 60] /F %d /AP << /N %d 0 R >> >>", tc.flags, appearance), stream("/Type /XObject /Subtype /Form /BBox [0 0 100 40] /Resources << >>", "q 0.2 0.6 0.8 rg 0 0 100 40 re f Q"))
			data := rawPDF(objects)
			source := writeTemp(t, "stamp.pdf", data)
			for _, opt := range []RenderOptions{{DPI: 72}, {DPI: 144}, {MaxPixels: 333}} {
				paths, err := RenderPages(context.Background(), source, t.TempDir(), opt)
				if err != nil || len(paths) != 2 {
					t.Fatal(paths, err)
				}
				for _, path := range paths {
					encoded, err := os.ReadFile(path)
					if err != nil {
						t.Fatal(err)
					}
					img, err := png.Decode(bytes.NewReader(encoded))
					if err != nil {
						t.Fatal(err)
					}
					got := color.RGBAModel.Convert(img.At(img.Bounds().Dx()*3/4, img.Bounds().Dy()*9/10)).(color.RGBA)
					want := color.RGBA{255, 255, 255, 255}
					if tc.visible {
						want = color.RGBA{51, 153, 204, 255}
					}
					if got != want {
						t.Fatalf("%s stamp center: got %v want %v", path, got, want)
					}
				}
			}
			after, err := os.ReadFile(source)
			if err != nil || !bytes.Equal(data, after) {
				t.Fatal("source changed", err)
			}
		})
	}
}

func TestPreviewPageTransparency(t *testing.T) {
	for _, tc := range []struct {
		blend   string
		overlap color.RGBA
	}{
		{"Normal", color.RGBA{128, 0, 127, 255}},
		{"Multiply", color.RGBA{128, 0, 0, 255}},
		{"Screen", color.RGBA{255, 0, 127, 255}},
	} {
		t.Run(tc.blend, func(t *testing.T) {
			objects := formObjs()
			for _, index := range []int{2, 3} {
				objects[index] = strings.Replace(objects[index], "/Resources <<", fmt.Sprintf("/Resources << /ExtGState << /AuditAlpha << /Type /ExtGState /ca 0.5 /BM /%s >> >>", tc.blend), 1)
			}
			for _, index := range []int{16, 17} {
				objects[index] = stream("", "q 1 0 0 rg 240 20 80 60 re f /AuditAlpha gs 0 0 1 rg 280 20 80 60 re f Q")
			}
			data := rawPDF(objects)
			source := writeTemp(t, "transparent.pdf", data)
			for _, opt := range []RenderOptions{{DPI: 72}, {DPI: 144}, {MaxPixels: 333}} {
				paths, err := RenderPages(context.Background(), source, t.TempDir(), opt)
				if err != nil || len(paths) != 2 {
					t.Fatal(paths, err)
				}
				for _, path := range paths {
					encoded, err := os.ReadFile(path)
					if err != nil {
						t.Fatal(err)
					}
					img, err := png.Decode(bytes.NewReader(encoded))
					if err != nil {
						t.Fatal(err)
					}
					for _, sample := range []struct {
						x    int
						want color.RGBA
					}{
						{260, color.RGBA{255, 0, 0, 255}}, {300, tc.overlap}, {340, color.RGBA{128, 128, 255, 255}},
					} {
						got := color.RGBAModel.Convert(img.At(img.Bounds().Dx()*sample.x/400, img.Bounds().Dy()*360/400)).(color.RGBA)
						if got != sample.want {
							t.Fatalf("%s x=%d got %v want %v", path, sample.x, got, sample.want)
						}
					}
				}
			}
			after, err := os.ReadFile(source)
			if err != nil || !bytes.Equal(data, after) {
				t.Fatal("source changed", err)
			}
		})
	}
}

func TestPreviewClippingWindingAndRestore(t *testing.T) {
	for _, operator := range []string{"W", "W*"} {
		t.Run(operator, func(t *testing.T) {
			objects := formObjs()
			for _, index := range []int{16, 17} {
				objects[index] = stream("", fmt.Sprintf("q 240 20 100 60 re 270 35 40 30 re %s n 0 0 1 rg 230 10 120 80 re f Q q 0 1 0 rg 350 30 20 20 re f Q", operator))
			}
			data := rawPDF(objects)
			source := writeTemp(t, "clipping.pdf", data)
			hole := color.RGBA{0, 0, 255, 255}
			if operator == "W*" {
				hole = color.RGBA{255, 255, 255, 255}
			}
			for _, opt := range []RenderOptions{{DPI: 72}, {DPI: 144}, {MaxPixels: 333}} {
				paths, err := RenderPages(context.Background(), source, t.TempDir(), opt)
				if err != nil || len(paths) != 2 {
					t.Fatal(paths, err)
				}
				for _, path := range paths {
					encoded, err := os.ReadFile(path)
					if err != nil {
						t.Fatal(err)
					}
					img, err := png.Decode(bytes.NewReader(encoded))
					if err != nil {
						t.Fatal(err)
					}
					for _, sample := range []struct {
						x, y int
						want color.RGBA
					}{
						{250, 50, color.RGBA{0, 0, 255, 255}},
						{290, 50, hole},
						{235, 50, color.RGBA{255, 255, 255, 255}},
						{360, 40, color.RGBA{0, 255, 0, 255}},
					} {
						got := color.RGBAModel.Convert(img.At(img.Bounds().Dx()*sample.x/400, img.Bounds().Dy()*(400-sample.y)/400)).(color.RGBA)
						if got != sample.want {
							t.Fatalf("%s at %d,%d: got %v want %v", path, sample.x, sample.y, got, sample.want)
						}
					}
				}
			}
			after, err := os.ReadFile(source)
			if err != nil || !bytes.Equal(data, after) {
				t.Fatal("source changed", err)
			}
		})
	}
}

func TestPreviewLineCaps(t *testing.T) {
	for _, operator := range []string{"0", "1", "2"} {
		t.Run(operator, func(t *testing.T) {
			objects := formObjs()
			for _, index := range []int{16, 17} {
				objects[index] = stream("", fmt.Sprintf("q %s J 20 w 0 0 1 RG 260 50 m 330 50 l S Q", operator))
			}
			data := rawPDF(objects)
			source := writeTemp(t, "linecap.pdf", data)
			white := color.RGBA{255, 255, 255, 255}
			blue := color.RGBA{0, 0, 255, 255}
			extension, corner := blue, white
			if operator == "0" {
				extension = white
			}
			if operator == "2" {
				corner = blue
			}
			for _, opt := range []RenderOptions{{DPI: 72}, {DPI: 144}, {MaxPixels: 333}} {
				paths, err := RenderPages(context.Background(), source, t.TempDir(), opt)
				if err != nil || len(paths) != 2 {
					t.Fatal(paths, err)
				}
				for _, path := range paths {
					encoded, err := os.ReadFile(path)
					if err != nil {
						t.Fatal(err)
					}
					img, err := png.Decode(bytes.NewReader(encoded))
					if err != nil {
						t.Fatal(err)
					}
					for _, sample := range []struct {
						x, y int
						want color.RGBA
					}{
						{290, 50, blue},
						{254, 50, extension},
						{252, 58, corner},
						{245, 50, white},
					} {
						got := color.RGBAModel.Convert(img.At(img.Bounds().Dx()*sample.x/400, img.Bounds().Dy()*(400-sample.y)/400)).(color.RGBA)
						if got != sample.want {
							t.Fatalf("%s at %d,%d: got %v want %v", path, sample.x, sample.y, got, sample.want)
						}
					}
				}
			}
			after, err := os.ReadFile(source)
			if err != nil || !bytes.Equal(data, after) {
				t.Fatal("source changed", err)
			}
		})
	}
}

func TestPreviewDashPhase(t *testing.T) {
	for _, operator := range []string{"0", "10"} {
		t.Run(operator, func(t *testing.T) {
			objects := formObjs()
			for _, index := range []int{16, 17} {
				objects[index] = stream("", fmt.Sprintf("q [10 10] %s d 0 J 6 w 0 0 1 RG 260 50 m 340 50 l S Q q 6 w 0 0 1 RG 260 75 m 340 75 l S Q", operator))
			}
			data := rawPDF(objects)
			source := writeTemp(t, "dash.pdf", data)
			white := color.RGBA{255, 255, 255, 255}
			blue := color.RGBA{0, 0, 255, 255}
			first, second := blue, white
			if operator == "10" {
				first, second = white, blue
			}
			for _, opt := range []RenderOptions{{DPI: 72}, {DPI: 144}, {MaxPixels: 333}} {
				paths, err := RenderPages(context.Background(), source, t.TempDir(), opt)
				if err != nil || len(paths) != 2 {
					t.Fatal(paths, err)
				}
				for _, path := range paths {
					encoded, err := os.ReadFile(path)
					if err != nil {
						t.Fatal(err)
					}
					img, err := png.Decode(bytes.NewReader(encoded))
					if err != nil {
						t.Fatal(err)
					}
					for _, sample := range []struct {
						x, y int
						want color.RGBA
					}{
						{265, 50, first},
						{275, 50, second},
						{275, 75, blue},
						{245, 50, white},
					} {
						got := color.RGBAModel.Convert(img.At(img.Bounds().Dx()*sample.x/400, img.Bounds().Dy()*(400-sample.y)/400)).(color.RGBA)
						if got != sample.want {
							t.Fatalf("%s at %d,%d: got %v want %v", path, sample.x, sample.y, got, sample.want)
						}
					}
				}
			}
			after, err := os.ReadFile(source)
			if err != nil || !bytes.Equal(data, after) {
				t.Fatal("source changed", err)
			}
		})
	}
}

func TestPreviewAffineTransforms(t *testing.T) {
	for _, operator := range []string{"1 .5 0 1 260 40", "0 1 -1 0 300 40"} {
		t.Run(operator, func(t *testing.T) {
			objects := formObjs()
			for _, index := range []int{16, 17} {
				objects[index] = stream("", fmt.Sprintf("q %s cm 0 0 1 rg 0 0 40 20 re f Q q 0 0 1 rg 325 45 10 10 re f Q", operator))
			}
			data := rawPDF(objects)
			source := writeTemp(t, "transform.pdf", data)
			white := color.RGBA{255, 255, 255, 255}
			blue := color.RGBA{0, 0, 255, 255}
			edge := blue
			if operator == "0 1 -1 0 300 40" {
				edge = white
			}
			for _, opt := range []RenderOptions{{DPI: 72}, {DPI: 144}, {MaxPixels: 333}} {
				paths, err := RenderPages(context.Background(), source, t.TempDir(), opt)
				if err != nil || len(paths) != 2 {
					t.Fatal(paths, err)
				}
				for _, path := range paths {
					encoded, err := os.ReadFile(path)
					if err != nil {
						t.Fatal(err)
					}
					img, err := png.Decode(bytes.NewReader(encoded))
					if err != nil {
						t.Fatal(err)
					}
					for _, sample := range []struct {
						x, y int
						want color.RGBA
					}{
						{290, 60, blue},
						{270, 50, edge},
						{330, 50, blue},
						{310, 60, white},
					} {
						got := color.RGBAModel.Convert(img.At(img.Bounds().Dx()*sample.x/400, img.Bounds().Dy()*(400-sample.y)/400)).(color.RGBA)
						if got != sample.want {
							t.Fatalf("%s at %d,%d: got %v want %v", path, sample.x, sample.y, got, sample.want)
						}
					}
				}
			}
			after, err := os.ReadFile(source)
			if err != nil || !bytes.Equal(data, after) {
				t.Fatal("source changed", err)
			}
		})
	}
}

// Retained same-input differential fixtures exercise shading resources and endpoint extension.
func TestPreviewAxialShading(t *testing.T) {
	for _, rule := range []string{"bounded", "extended"} {
		t.Run(rule, func(t *testing.T) {
			data, err := os.ReadFile("../../testdata/regression/pdf-shading-" + rule + "-baseline/source.pdf")
			if err != nil {
				t.Fatal(err)
			}
			source := writeTemp(t, "shading.pdf", data)
			for _, opt := range []RenderOptions{{DPI: 72}, {DPI: 144}, {MaxPixels: 333}} {
				paths, err := RenderPages(context.Background(), source, t.TempDir(), opt)
				if err != nil || len(paths) != 2 {
					t.Fatal(paths, err)
				}
				left, right := color.RGBA{255, 255, 255, 255}, color.RGBA{255, 255, 255, 255}
				if rule == "extended" {
					left, right = color.RGBA{255, 0, 0, 255}, color.RGBA{1, 0, 254, 255}
				}
				middle := color.RGBA{128, 0, 127, 255}
				if opt.MaxPixels == 333 {
					middle = color.RGBA{130, 0, 125, 255}
				}
				for _, path := range paths {
					encoded, err := os.ReadFile(path)
					if err != nil {
						t.Fatal(err)
					}
					img, err := png.Decode(bytes.NewReader(encoded))
					if err != nil {
						t.Fatal(err)
					}
					for _, sample := range []struct {
						x, y int
						want color.RGBA
					}{
						{250, 50, left}, {290, 50, middle}, {330, 50, right}, {290, 20, color.RGBA{255, 255, 255, 255}},
					} {
						got := color.RGBAModel.Convert(img.At(img.Bounds().Dx()*sample.x/400, img.Bounds().Dy()*(400-sample.y)/400)).(color.RGBA)
						if got != sample.want {
							t.Fatalf("%s at %d,%d: got %v want %v", path, sample.x, sample.y, got, sample.want)
						}
					}
				}
			}
			after, err := os.ReadFile(source)
			if err != nil || !bytes.Equal(data, after) {
				t.Fatal("source changed", err)
			}
		})
	}
}

func TestPreviewRadialShading(t *testing.T) {
	for _, rule := range []string{"bounded"} {
		t.Run(rule, func(t *testing.T) {
			data, err := os.ReadFile("../../testdata/regression/pdf-radial-" + rule + "-baseline/source.pdf")
			if err != nil {
				t.Fatal(err)
			}
			source := writeTemp(t, "shading.pdf", data)
			for _, opt := range []RenderOptions{{DPI: 72}, {DPI: 144}, {MaxPixels: 333}} {
				paths, err := RenderPages(context.Background(), source, t.TempDir(), opt)
				if err != nil || len(paths) != 2 {
					t.Fatal(paths, err)
				}
				left, right := color.RGBA{255, 255, 255, 255}, color.RGBA{255, 255, 255, 255}
				if rule == "extended" {
					left, right = color.RGBA{255, 0, 0, 255}, color.RGBA{1, 0, 254, 255}
				}
				middle := color.RGBA{128, 0, 127, 255}
				if opt.MaxPixels == 333 {
					middle = color.RGBA{129, 0, 126, 255}
				}
				for _, path := range paths {
					encoded, err := os.ReadFile(path)
					if err != nil {
						t.Fatal(err)
					}
					img, err := png.Decode(bytes.NewReader(encoded))
					if err != nil {
						t.Fatal(err)
					}
					for _, sample := range []struct {
						x, y int
						want color.RGBA
					}{
						{290, 50, left}, {310, 50, middle}, {325, 50, right}, {290, 20, color.RGBA{255, 255, 255, 255}},
					} {
						got := color.RGBAModel.Convert(img.At(img.Bounds().Dx()*sample.x/400, img.Bounds().Dy()*(400-sample.y)/400)).(color.RGBA)
						if got != sample.want {
							t.Fatalf("%s at %d,%d: got %v want %v", path, sample.x, sample.y, got, sample.want)
						}
					}
				}
			}
			after, err := os.ReadFile(source)
			if err != nil || !bytes.Equal(data, after) {
				t.Fatal("source changed", err)
			}
		})
	}
}

func TestPreviewStencilDecode(t *testing.T) {
	for _, rule := range []string{"normal", "inverted"} {
		t.Run(rule, func(t *testing.T) {
			data, err := os.ReadFile("../../testdata/regression/pdf-stencil-" + rule + "-baseline/source.pdf")
			if err != nil {
				t.Fatal(err)
			}
			source := writeTemp(t, "shading.pdf", data)
			for _, opt := range []RenderOptions{{DPI: 72}, {DPI: 144}, {MaxPixels: 333}} {
				paths, err := RenderPages(context.Background(), source, t.TempDir(), opt)
				if err != nil || len(paths) != 2 {
					t.Fatal(paths, err)
				}
				zero, one := color.RGBA{255, 0, 0, 255}, color.RGBA{0, 0, 255, 255}
				if rule == "inverted" {
					zero, one = one, zero
				}
				for _, path := range paths {
					encoded, err := os.ReadFile(path)
					if err != nil {
						t.Fatal(err)
					}
					img, err := png.Decode(bytes.NewReader(encoded))
					if err != nil {
						t.Fatal(err)
					}
					for _, sample := range []struct {
						x, y int
						want color.RGBA
					}{
						{245, 60, zero}, {255, 60, one}, {245, 40, one}, {255, 40, zero}, {230, 50, color.RGBA{255, 255, 255, 255}},
					} {
						got := color.RGBAModel.Convert(img.At(img.Bounds().Dx()*sample.x/400, img.Bounds().Dy()*(400-sample.y)/400)).(color.RGBA)
						if got != sample.want {
							t.Fatalf("%s at %d,%d: got %v want %v", path, sample.x, sample.y, got, sample.want)
						}
					}
				}
			}
			after, err := os.ReadFile(source)
			if err != nil || !bytes.Equal(data, after) {
				t.Fatal("source changed", err)
			}
		})
	}
}

func TestPreviewImageSoftMask(t *testing.T) {
	for _, rule := range []string{"normal"} {
		t.Run(rule, func(t *testing.T) {
			data, err := os.ReadFile("../../testdata/regression/pdf-softmask-baseline/source.pdf")
			if err != nil {
				t.Fatal(err)
			}
			source := writeTemp(t, "shading.pdf", data)
			for _, opt := range []RenderOptions{{DPI: 72}, {DPI: 144}, {MaxPixels: 333}} {
				paths, err := RenderPages(context.Background(), source, t.TempDir(), opt)
				if err != nil || len(paths) != 2 {
					t.Fatal(paths, err)
				}
				for _, path := range paths {
					encoded, err := os.ReadFile(path)
					if err != nil {
						t.Fatal(err)
					}
					img, err := png.Decode(bytes.NewReader(encoded))
					if err != nil {
						t.Fatal(err)
					}
					for _, sample := range []struct {
						x, y int
						want color.RGBA
					}{
						{255, 50, color.RGBA{0, 0, 255, 255}}, {285, 50, color.RGBA{128, 0, 127, 255}}, {315, 50, color.RGBA{255, 0, 0, 255}}, {230, 50, color.RGBA{255, 255, 255, 255}},
					} {
						got := color.RGBAModel.Convert(img.At(img.Bounds().Dx()*sample.x/400, img.Bounds().Dy()*(400-sample.y)/400)).(color.RGBA)
						if got != sample.want {
							t.Fatalf("%s at %d,%d: got %v want %v", path, sample.x, sample.y, got, sample.want)
						}
					}
				}
			}
			after, err := os.ReadFile(source)
			if err != nil || !bytes.Equal(data, after) {
				t.Fatal("source changed", err)
			}
		})
	}
}

func TestPreviewImageColorKeyMask(t *testing.T) {
	for _, rule := range []string{"normal"} {
		t.Run(rule, func(t *testing.T) {
			data, err := os.ReadFile("../../testdata/regression/pdf-colorkey-baseline/source.pdf")
			if err != nil {
				t.Fatal(err)
			}
			source := writeTemp(t, "shading.pdf", data)
			for _, opt := range []RenderOptions{{DPI: 72}, {DPI: 144}, {MaxPixels: 333}} {
				paths, err := RenderPages(context.Background(), source, t.TempDir(), opt)
				if err != nil || len(paths) != 2 {
					t.Fatal(paths, err)
				}
				for _, path := range paths {
					encoded, err := os.ReadFile(path)
					if err != nil {
						t.Fatal(err)
					}
					img, err := png.Decode(bytes.NewReader(encoded))
					if err != nil {
						t.Fatal(err)
					}
					for _, sample := range []struct {
						x, y int
						want color.RGBA
					}{
						{215, 50, color.RGBA{0, 0, 255, 255}},
						{245, 50, color.RGBA{0, 0, 255, 255}},
						{275, 50, color.RGBA{99, 20, 30, 255}},
						{305, 50, color.RGBA{200, 41, 50, 255}},
						{335, 50, color.RGBA{200, 40, 51, 255}},
						{190, 50, color.RGBA{255, 255, 255, 255}},
					} {
						got := color.RGBAModel.Convert(img.At(img.Bounds().Dx()*sample.x/400, img.Bounds().Dy()*(400-sample.y)/400)).(color.RGBA)
						if got != sample.want {
							t.Fatalf("%s at %d,%d: got %v want %v", path, sample.x, sample.y, got, sample.want)
						}
					}
				}
			}
			after, err := os.ReadFile(source)
			if err != nil || !bytes.Equal(data, after) {
				t.Fatal("source changed", err)
			}
		})
	}
}

func TestPreviewIndexedImagePalette(t *testing.T) {
	for _, rule := range []string{"normal"} {
		t.Run(rule, func(t *testing.T) {
			data, err := os.ReadFile("../../testdata/regression/pdf-indexed-baseline/source.pdf")
			if err != nil {
				t.Fatal(err)
			}
			source := writeTemp(t, "shading.pdf", data)
			for _, opt := range []RenderOptions{{DPI: 72}, {DPI: 144}, {MaxPixels: 333}} {
				paths, err := RenderPages(context.Background(), source, t.TempDir(), opt)
				if err != nil || len(paths) != 2 {
					t.Fatal(paths, err)
				}
				for _, path := range paths {
					encoded, err := os.ReadFile(path)
					if err != nil {
						t.Fatal(err)
					}
					img, err := png.Decode(bytes.NewReader(encoded))
					if err != nil {
						t.Fatal(err)
					}
					for _, sample := range []struct {
						x, y int
						want color.RGBA
					}{
						{218, 50, color.RGBA{255, 0, 0, 255}},
						{256, 50, color.RGBA{0, 255, 0, 255}},
						{293, 50, color.RGBA{0, 0, 255, 255}},
						{331, 50, color.RGBA{255, 255, 0, 255}},
						{190, 50, color.RGBA{255, 255, 255, 255}},
					} {
						got := color.RGBAModel.Convert(img.At(img.Bounds().Dx()*sample.x/400, img.Bounds().Dy()*(400-sample.y)/400)).(color.RGBA)
						if got != sample.want {
							t.Fatalf("%s at %d,%d: got %v want %v", path, sample.x, sample.y, got, sample.want)
						}
					}
				}
			}
			after, err := os.ReadFile(source)
			if err != nil || !bytes.Equal(data, after) {
				t.Fatal("source changed", err)
			}
		})
	}
}

func TestPreviewRGBImageComponentDecode(t *testing.T) {
	for _, rule := range []string{"normal"} {
		t.Run(rule, func(t *testing.T) {
			data, err := os.ReadFile("../../testdata/regression/pdf-rgb-decode-baseline/source.pdf")
			if err != nil {
				t.Fatal(err)
			}
			source := writeTemp(t, "shading.pdf", data)
			for _, opt := range []RenderOptions{{DPI: 72}, {DPI: 144}, {MaxPixels: 333}} {
				paths, err := RenderPages(context.Background(), source, t.TempDir(), opt)
				if err != nil || len(paths) != 2 {
					t.Fatal(paths, err)
				}
				for _, path := range paths {
					encoded, err := os.ReadFile(path)
					if err != nil {
						t.Fatal(err)
					}
					img, err := png.Decode(bytes.NewReader(encoded))
					if err != nil {
						t.Fatal(err)
					}
					for _, sample := range []struct {
						x, y int
						want color.RGBA
					}{
						{218, 50, color.RGBA{0, 0, 255, 255}},
						{256, 50, color.RGBA{255, 255, 255, 255}},
						{293, 50, color.RGBA{255, 0, 0, 255}},
						{331, 50, color.RGBA{0, 255, 255, 255}},
						{190, 50, color.RGBA{255, 255, 255, 255}},
					} {
						got := color.RGBAModel.Convert(img.At(img.Bounds().Dx()*sample.x/400, img.Bounds().Dy()*(400-sample.y)/400)).(color.RGBA)
						if got != sample.want {
							t.Fatalf("%s at %d,%d: got %v want %v", path, sample.x, sample.y, got, sample.want)
						}
					}
				}
			}
			after, err := os.ReadFile(source)
			if err != nil || !bytes.Equal(data, after) {
				t.Fatal("source changed", err)
			}
		})
	}
}

func TestPreviewASCIIHexInlineImage(t *testing.T) {
	for _, rule := range []string{"normal"} {
		t.Run(rule, func(t *testing.T) {
			data, err := os.ReadFile("../../testdata/regression/pdf-inline-baseline/source.pdf")
			if err != nil {
				t.Fatal(err)
			}
			source := writeTemp(t, "shading.pdf", data)
			for _, opt := range []RenderOptions{{DPI: 72}, {DPI: 144}, {MaxPixels: 333}} {
				paths, err := RenderPages(context.Background(), source, t.TempDir(), opt)
				if err != nil || len(paths) != 2 {
					t.Fatal(paths, err)
				}
				for _, path := range paths {
					encoded, err := os.ReadFile(path)
					if err != nil {
						t.Fatal(err)
					}
					img, err := png.Decode(bytes.NewReader(encoded))
					if err != nil {
						t.Fatal(err)
					}
					for _, sample := range []struct {
						x, y int
						want color.RGBA
					}{
						{218, 50, color.RGBA{255, 0, 0, 255}},
						{256, 50, color.RGBA{0, 255, 0, 255}},
						{293, 50, color.RGBA{0, 0, 255, 255}},
						{331, 50, color.RGBA{255, 255, 0, 255}},
						{190, 50, color.RGBA{255, 255, 255, 255}},
					} {
						got := color.RGBAModel.Convert(img.At(img.Bounds().Dx()*sample.x/400, img.Bounds().Dy()*(400-sample.y)/400)).(color.RGBA)
						if got != sample.want {
							t.Fatalf("%s at %d,%d: got %v want %v", path, sample.x, sample.y, got, sample.want)
						}
					}
				}
			}
			after, err := os.ReadFile(source)
			if err != nil || !bytes.Equal(data, after) {
				t.Fatal("source changed", err)
			}
		})
	}
}

func TestPreviewRunLengthImage(t *testing.T) {
	for _, rule := range []string{"normal"} {
		t.Run(rule, func(t *testing.T) {
			data, err := os.ReadFile("../../testdata/regression/pdf-runlength-baseline/source.pdf")
			if err != nil {
				t.Fatal(err)
			}
			source := writeTemp(t, "shading.pdf", data)
			for _, opt := range []RenderOptions{{DPI: 72}, {DPI: 144}, {MaxPixels: 333}} {
				paths, err := RenderPages(context.Background(), source, t.TempDir(), opt)
				if err != nil || len(paths) != 2 {
					t.Fatal(paths, err)
				}
				for _, path := range paths {
					encoded, err := os.ReadFile(path)
					if err != nil {
						t.Fatal(err)
					}
					img, err := png.Decode(bytes.NewReader(encoded))
					if err != nil {
						t.Fatal(err)
					}
					for _, sample := range []struct {
						x, y int
						want color.RGBA
					}{
						{218, 50, color.RGBA{255, 0, 0, 255}},
						{256, 50, color.RGBA{255, 255, 255, 255}},
						{293, 50, color.RGBA{0, 0, 255, 255}},
						{331, 50, color.RGBA{255, 255, 0, 255}},
						{190, 50, color.RGBA{255, 255, 255, 255}},
					} {
						got := color.RGBAModel.Convert(img.At(img.Bounds().Dx()*sample.x/400, img.Bounds().Dy()*(400-sample.y)/400)).(color.RGBA)
						if got != sample.want {
							t.Fatalf("%s at %d,%d: got %v want %v", path, sample.x, sample.y, got, sample.want)
						}
					}
				}
			}
			after, err := os.ReadFile(source)
			if err != nil || !bytes.Equal(data, after) {
				t.Fatal("source changed", err)
			}
		})
	}
}

func TestPreviewASCII85FlateImage(t *testing.T) {
	for _, rule := range []string{"normal"} {
		t.Run(rule, func(t *testing.T) {
			data, err := os.ReadFile("../../testdata/regression/pdf-filterchain-baseline/source.pdf")
			if err != nil {
				t.Fatal(err)
			}
			source := writeTemp(t, "shading.pdf", data)
			for _, opt := range []RenderOptions{{DPI: 72}, {DPI: 144}, {MaxPixels: 333}} {
				paths, err := RenderPages(context.Background(), source, t.TempDir(), opt)
				if err != nil || len(paths) != 2 {
					t.Fatal(paths, err)
				}
				for _, path := range paths {
					encoded, err := os.ReadFile(path)
					if err != nil {
						t.Fatal(err)
					}
					img, err := png.Decode(bytes.NewReader(encoded))
					if err != nil {
						t.Fatal(err)
					}
					for _, sample := range []struct {
						x, y int
						want color.RGBA
					}{
						{218, 50, color.RGBA{255, 0, 0, 255}},
						{256, 50, color.RGBA{255, 255, 255, 255}},
						{293, 50, color.RGBA{0, 0, 255, 255}},
						{331, 50, color.RGBA{255, 255, 0, 255}},
						{190, 50, color.RGBA{255, 255, 255, 255}},
					} {
						got := color.RGBAModel.Convert(img.At(img.Bounds().Dx()*sample.x/400, img.Bounds().Dy()*(400-sample.y)/400)).(color.RGBA)
						if got != sample.want {
							t.Fatalf("%s at %d,%d: got %v want %v", path, sample.x, sample.y, got, sample.want)
						}
					}
				}
			}
			after, err := os.ReadFile(source)
			if err != nil || !bytes.Equal(data, after) {
				t.Fatal("source changed", err)
			}
		})
	}
}

func TestPreviewTIFFPredictorImage(t *testing.T) {
	for _, rule := range []string{"normal"} {
		t.Run(rule, func(t *testing.T) {
			data, err := os.ReadFile("../../testdata/regression/pdf-predictor-baseline/source.pdf")
			if err != nil {
				t.Fatal(err)
			}
			source := writeTemp(t, "shading.pdf", data)
			for _, opt := range []RenderOptions{{DPI: 72}, {DPI: 144}, {MaxPixels: 333}} {
				paths, err := RenderPages(context.Background(), source, t.TempDir(), opt)
				if err != nil || len(paths) != 2 {
					t.Fatal(paths, err)
				}
				for _, path := range paths {
					encoded, err := os.ReadFile(path)
					if err != nil {
						t.Fatal(err)
					}
					img, err := png.Decode(bytes.NewReader(encoded))
					if err != nil {
						t.Fatal(err)
					}
					for _, sample := range []struct {
						x, y int
						want color.RGBA
					}{
						{218, 50, color.RGBA{255, 0, 0, 255}},
						{256, 50, color.RGBA{255, 255, 255, 255}},
						{293, 50, color.RGBA{0, 0, 255, 255}},
						{331, 50, color.RGBA{255, 255, 0, 255}},
						{190, 50, color.RGBA{255, 255, 255, 255}},
					} {
						got := color.RGBAModel.Convert(img.At(img.Bounds().Dx()*sample.x/400, img.Bounds().Dy()*(400-sample.y)/400)).(color.RGBA)
						if got != sample.want {
							t.Fatalf("%s at %d,%d: got %v want %v", path, sample.x, sample.y, got, sample.want)
						}
					}
				}
			}
			after, err := os.ReadFile(source)
			if err != nil || !bytes.Equal(data, after) {
				t.Fatal("source changed", err)
			}
		})
	}
}

func TestPreviewPNGRowPredictorImage(t *testing.T) {
	for _, rule := range []string{"normal"} {
		t.Run(rule, func(t *testing.T) {
			data, err := os.ReadFile("../../testdata/regression/pdf-png-predictor-baseline/source.pdf")
			if err != nil {
				t.Fatal(err)
			}
			source := writeTemp(t, "shading.pdf", data)
			for _, opt := range []RenderOptions{{DPI: 72}, {DPI: 144}, {MaxPixels: 333}} {
				paths, err := RenderPages(context.Background(), source, t.TempDir(), opt)
				if err != nil || len(paths) != 2 {
					t.Fatal(paths, err)
				}
				for _, path := range paths {
					encoded, err := os.ReadFile(path)
					if err != nil {
						t.Fatal(err)
					}
					img, err := png.Decode(bytes.NewReader(encoded))
					if err != nil {
						t.Fatal(err)
					}
					palette := []color.RGBA{{255, 0, 0, 255}, {255, 255, 255, 255}, {0, 0, 255, 255}, {255, 255, 0, 255}}
					for row := 0; row < 5; row++ {
						for col, x := range []int{218, 256, 293, 331} {
							y := 66 - row*8
							want := palette[(row+col)%4]
							got := color.RGBAModel.Convert(img.At(img.Bounds().Dx()*x/400, img.Bounds().Dy()*(400-y)/400)).(color.RGBA)
							if got != want {
								t.Fatalf("%s row %d col %d: got %v want %v", path, row, col, got, want)
							}
						}
					}
				}
			}
			after, err := os.ReadFile(source)
			if err != nil || !bytes.Equal(data, after) {
				t.Fatal("source changed", err)
			}
		})
	}
}

func TestPreviewGray16Image(t *testing.T) {
	for _, rule := range []string{"normal"} {
		t.Run(rule, func(t *testing.T) {
			data, err := os.ReadFile("../../testdata/regression/pdf-gray16-baseline/source.pdf")
			if err != nil {
				t.Fatal(err)
			}
			source := writeTemp(t, "shading.pdf", data)
			for _, opt := range []RenderOptions{{DPI: 72}, {DPI: 144}, {MaxPixels: 333}} {
				paths, err := RenderPages(context.Background(), source, t.TempDir(), opt)
				if err != nil || len(paths) != 2 {
					t.Fatal(paths, err)
				}
				for _, path := range paths {
					encoded, err := os.ReadFile(path)
					if err != nil {
						t.Fatal(err)
					}
					img, err := png.Decode(bytes.NewReader(encoded))
					if err != nil {
						t.Fatal(err)
					}
					for col, x := range []int{218, 256, 293, 331} {
						value := uint8(col * 85)
						want := color.RGBA{value, value, value, 255}
						got := color.RGBAModel.Convert(img.At(img.Bounds().Dx()*x/400, img.Bounds().Dy()*350/400)).(color.RGBA)
						if got != want {
							t.Fatalf("%s col %d: got %v want %v", path, col, got, want)
						}
					}
				}
			}
			after, err := os.ReadFile(source)
			if err != nil || !bytes.Equal(data, after) {
				t.Fatal("source changed", err)
			}
		})
	}
}

func TestPreviewCMYKImage(t *testing.T) {
	for _, rule := range []string{"normal"} {
		t.Run(rule, func(t *testing.T) {
			data, err := os.ReadFile("../../testdata/regression/pdf-cmyk-baseline/source.pdf")
			if err != nil {
				t.Fatal(err)
			}
			source := writeTemp(t, "shading.pdf", data)
			for _, opt := range []RenderOptions{{DPI: 72}, {DPI: 144}, {MaxPixels: 333}} {
				paths, err := RenderPages(context.Background(), source, t.TempDir(), opt)
				if err != nil || len(paths) != 2 {
					t.Fatal(paths, err)
				}
				for _, path := range paths {
					encoded, err := os.ReadFile(path)
					if err != nil {
						t.Fatal(err)
					}
					img, err := png.Decode(bytes.NewReader(encoded))
					if err != nil {
						t.Fatal(err)
					}
					for col, x := range []int{218, 256, 293, 331} {
						want := []color.RGBA{{0, 174, 239, 255}, {237, 2, 140, 255}, {255, 241, 1, 255}, {35, 31, 32, 255}}[col]
						got := color.RGBAModel.Convert(img.At(img.Bounds().Dx()*x/400, img.Bounds().Dy()*350/400)).(color.RGBA)
						if got != want {
							t.Fatalf("%s col %d: got %v want %v", path, col, got, want)
						}
					}
				}
			}
			after, err := os.ReadFile(source)
			if err != nil || !bytes.Equal(data, after) {
				t.Fatal("source changed", err)
			}
		})
	}
}

func TestPreviewSeparationImage(t *testing.T) {
	for _, rule := range []string{"normal"} {
		t.Run(rule, func(t *testing.T) {
			data, err := os.ReadFile("../../testdata/regression/pdf-separation-baseline/source.pdf")
			if err != nil {
				t.Fatal(err)
			}
			source := writeTemp(t, "shading.pdf", data)
			for _, opt := range []RenderOptions{{DPI: 72}, {DPI: 144}, {MaxPixels: 333}} {
				paths, err := RenderPages(context.Background(), source, t.TempDir(), opt)
				if err != nil || len(paths) != 2 {
					t.Fatal(paths, err)
				}
				for _, path := range paths {
					encoded, err := os.ReadFile(path)
					if err != nil {
						t.Fatal(err)
					}
					img, err := png.Decode(bytes.NewReader(encoded))
					if err != nil {
						t.Fatal(err)
					}
					for col, x := range []int{218, 256, 293, 331} {
						want := []color.RGBA{{255, 255, 255, 255}, {255, 170, 170, 255}, {255, 85, 85, 255}, {255, 0, 0, 255}}[col]
						got := color.RGBAModel.Convert(img.At(img.Bounds().Dx()*x/400, img.Bounds().Dy()*350/400)).(color.RGBA)
						if got != want {
							t.Fatalf("%s col %d: got %v want %v", path, col, got, want)
						}
					}
				}
			}
			after, err := os.ReadFile(source)
			if err != nil || !bytes.Equal(data, after) {
				t.Fatal("source changed", err)
			}
		})
	}
}

func TestPreviewDeviceNImage(t *testing.T) {
	for _, rule := range []string{"normal"} {
		t.Run(rule, func(t *testing.T) {
			data, err := os.ReadFile("../../testdata/regression/pdf-devicen-baseline/source.pdf")
			if err != nil {
				t.Fatal(err)
			}
			source := writeTemp(t, "shading.pdf", data)
			for _, opt := range []RenderOptions{{DPI: 72}, {DPI: 144}, {MaxPixels: 333}} {
				paths, err := RenderPages(context.Background(), source, t.TempDir(), opt)
				if err != nil || len(paths) != 2 {
					t.Fatal(paths, err)
				}
				for _, path := range paths {
					encoded, err := os.ReadFile(path)
					if err != nil {
						t.Fatal(err)
					}
					img, err := png.Decode(bytes.NewReader(encoded))
					if err != nil {
						t.Fatal(err)
					}
					for col, x := range []int{218, 256, 293, 331} {
						want := []color.RGBA{{255, 255, 0, 255}, {0, 255, 0, 255}, {255, 0, 0, 255}, {0, 0, 0, 255}}[col]
						got := color.RGBAModel.Convert(img.At(img.Bounds().Dx()*x/400, img.Bounds().Dy()*350/400)).(color.RGBA)
						if got != want {
							t.Fatalf("%s col %d: got %v want %v", path, col, got, want)
						}
					}
				}
			}
			after, err := os.ReadFile(source)
			if err != nil || !bytes.Equal(data, after) {
				t.Fatal("source changed", err)
			}
		})
	}
}

func TestPreviewSampledTintImage(t *testing.T) {
	for _, rule := range []string{"normal"} {
		t.Run(rule, func(t *testing.T) {
			data, err := os.ReadFile("../../testdata/regression/pdf-sampled-tint-baseline/source.pdf")
			if err != nil {
				t.Fatal(err)
			}
			source := writeTemp(t, "shading.pdf", data)
			for _, opt := range []RenderOptions{{DPI: 72}, {DPI: 144}, {MaxPixels: 333}} {
				paths, err := RenderPages(context.Background(), source, t.TempDir(), opt)
				if err != nil || len(paths) != 2 {
					t.Fatal(paths, err)
				}
				for _, path := range paths {
					encoded, err := os.ReadFile(path)
					if err != nil {
						t.Fatal(err)
					}
					img, err := png.Decode(bytes.NewReader(encoded))
					if err != nil {
						t.Fatal(err)
					}
					for col, x := range []int{218, 256, 293, 331} {
						want := []color.RGBA{{255, 0, 0, 255}, {0, 255, 0, 255}, {0, 0, 255, 255}, {255, 255, 255, 255}}[col]
						got := color.RGBAModel.Convert(img.At(img.Bounds().Dx()*x/400, img.Bounds().Dy()*350/400)).(color.RGBA)
						if got != want {
							t.Fatalf("%s col %d: got %v want %v", path, col, got, want)
						}
					}
				}
			}
			after, err := os.ReadFile(source)
			if err != nil || !bytes.Equal(data, after) {
				t.Fatal("source changed", err)
			}
		})
	}
}

func TestPreviewStitchedTintImage(t *testing.T) {
	for _, rule := range []string{"normal"} {
		t.Run(rule, func(t *testing.T) {
			data, err := os.ReadFile("../../testdata/regression/pdf-stitched-tint-baseline/source.pdf")
			if err != nil {
				t.Fatal(err)
			}
			source := writeTemp(t, "shading.pdf", data)
			for _, opt := range []RenderOptions{{DPI: 72}, {DPI: 144}, {MaxPixels: 333}} {
				paths, err := RenderPages(context.Background(), source, t.TempDir(), opt)
				if err != nil || len(paths) != 2 {
					t.Fatal(paths, err)
				}
				for _, path := range paths {
					encoded, err := os.ReadFile(path)
					if err != nil {
						t.Fatal(err)
					}
					img, err := png.Decode(bytes.NewReader(encoded))
					if err != nil {
						t.Fatal(err)
					}
					for col, x := range []int{218, 256, 293, 331} {
						want := []color.RGBA{{255, 0, 0, 255}, {85, 170, 0, 255}, {85, 85, 255, 255}, {255, 255, 255, 255}}[col]
						got := color.RGBAModel.Convert(img.At(img.Bounds().Dx()*x/400, img.Bounds().Dy()*350/400)).(color.RGBA)
						if got != want {
							t.Fatalf("%s col %d: got %v want %v", path, col, got, want)
						}
					}
				}
			}
			after, err := os.ReadFile(source)
			if err != nil || !bytes.Equal(data, after) {
				t.Fatal("source changed", err)
			}
		})
	}
}

// Reference equivalence only: these samples do not establish gamma application.
func TestPreviewCalGrayImage(t *testing.T) {
	for _, rule := range []string{"normal"} {
		t.Run(rule, func(t *testing.T) {
			data, err := os.ReadFile("../../testdata/regression/pdf-calgray-baseline/source.pdf")
			if err != nil {
				t.Fatal(err)
			}
			source := writeTemp(t, "shading.pdf", data)
			for _, opt := range []RenderOptions{{DPI: 72}, {DPI: 144}, {MaxPixels: 333}} {
				paths, err := RenderPages(context.Background(), source, t.TempDir(), opt)
				if err != nil || len(paths) != 2 {
					t.Fatal(paths, err)
				}
				for _, path := range paths {
					encoded, err := os.ReadFile(path)
					if err != nil {
						t.Fatal(err)
					}
					img, err := png.Decode(bytes.NewReader(encoded))
					if err != nil {
						t.Fatal(err)
					}
					for col, x := range []int{218, 256, 293, 331} {
						want := []color.RGBA{{0, 0, 0, 255}, {85, 85, 85, 255}, {170, 170, 170, 255}, {255, 255, 255, 255}}[col]
						got := color.RGBAModel.Convert(img.At(img.Bounds().Dx()*x/400, img.Bounds().Dy()*350/400)).(color.RGBA)
						if got != want {
							t.Fatalf("%s col %d: got %v want %v", path, col, got, want)
						}
					}
				}
			}
			after, err := os.ReadFile(source)
			if err != nil || !bytes.Equal(data, after) {
				t.Fatal("source changed", err)
			}
		})
	}
}

func TestPreviewCalRGBImage(t *testing.T) {
	for _, rule := range []string{"normal"} {
		t.Run(rule, func(t *testing.T) {
			data, err := os.ReadFile("../../testdata/regression/pdf-calrgb-baseline/source.pdf")
			if err != nil {
				t.Fatal(err)
			}
			source := writeTemp(t, "shading.pdf", data)
			for _, opt := range []RenderOptions{{DPI: 72}, {DPI: 144}, {MaxPixels: 333}} {
				paths, err := RenderPages(context.Background(), source, t.TempDir(), opt)
				if err != nil || len(paths) != 2 {
					t.Fatal(paths, err)
				}
				for _, path := range paths {
					encoded, err := os.ReadFile(path)
					if err != nil {
						t.Fatal(err)
					}
					img, err := png.Decode(bytes.NewReader(encoded))
					if err != nil {
						t.Fatal(err)
					}
					for col, x := range []int{218, 256, 293, 331} {
						want := []color.RGBA{{85, 0, 0, 255}, {0, 85, 0, 255}, {0, 0, 85, 255}, {85, 170, 128, 255}}[col]
						got := color.RGBAModel.Convert(img.At(img.Bounds().Dx()*x/400, img.Bounds().Dy()*350/400)).(color.RGBA)
						if got != want {
							t.Fatalf("%s col %d: got %v want %v", path, col, got, want)
						}
					}
				}
			}
			after, err := os.ReadFile(source)
			if err != nil || !bytes.Equal(data, after) {
				t.Fatal("source changed", err)
			}
		})
	}
}

func TestPreviewLabImage(t *testing.T) {
	for _, rule := range []string{"normal"} {
		t.Run(rule, func(t *testing.T) {
			data, err := os.ReadFile("../../testdata/regression/pdf-lab-baseline/source.pdf")
			if err != nil {
				t.Fatal(err)
			}
			source := writeTemp(t, "shading.pdf", data)
			for _, opt := range []RenderOptions{{DPI: 72}, {DPI: 144}, {MaxPixels: 333}} {
				paths, err := RenderPages(context.Background(), source, t.TempDir(), opt)
				if err != nil || len(paths) != 2 {
					t.Fatal(paths, err)
				}
				for _, path := range paths {
					encoded, err := os.ReadFile(path)
					if err != nil {
						t.Fatal(err)
					}
					img, err := png.Decode(bytes.NewReader(encoded))
					if err != nil {
						t.Fatal(err)
					}
					for col, x := range []int{218, 256, 293, 331} {
						want := []color.RGBA{{0, 0, 0, 255}, {120, 119, 119, 255}, {255, 255, 255, 255}, {167, 71, 230, 255}}[col]
						got := color.RGBAModel.Convert(img.At(img.Bounds().Dx()*x/400, img.Bounds().Dy()*350/400)).(color.RGBA)
						if got != want {
							t.Fatalf("%s col %d: got %v want %v", path, col, got, want)
						}
					}
				}
			}
			after, err := os.ReadFile(source)
			if err != nil || !bytes.Equal(data, after) {
				t.Fatal("source changed", err)
			}
		})
	}
}

func TestPreviewLZWImage(t *testing.T) {
	for _, rule := range []string{"normal"} {
		t.Run(rule, func(t *testing.T) {
			data, err := os.ReadFile("../../testdata/regression/pdf-lzw-baseline/source.pdf")
			if err != nil {
				t.Fatal(err)
			}
			source := writeTemp(t, "shading.pdf", data)
			for _, opt := range []RenderOptions{{DPI: 72}, {DPI: 144}, {MaxPixels: 333}} {
				paths, err := RenderPages(context.Background(), source, t.TempDir(), opt)
				if err != nil || len(paths) != 2 {
					t.Fatal(paths, err)
				}
				for _, path := range paths {
					encoded, err := os.ReadFile(path)
					if err != nil {
						t.Fatal(err)
					}
					img, err := png.Decode(bytes.NewReader(encoded))
					if err != nil {
						t.Fatal(err)
					}
					for col, x := range []int{218, 256, 293, 331} {
						want := []color.RGBA{{255, 0, 0, 255}, {255, 0, 0, 255}, {0, 255, 0, 255}, {0, 0, 255, 255}}[col]
						got := color.RGBAModel.Convert(img.At(img.Bounds().Dx()*x/400, img.Bounds().Dy()*350/400)).(color.RGBA)
						if got != want {
							t.Fatalf("%s col %d: got %v want %v", path, col, got, want)
						}
					}
				}
			}
			after, err := os.ReadFile(source)
			if err != nil || !bytes.Equal(data, after) {
				t.Fatal("source changed", err)
			}
		})
	}
}

func TestPreviewLZWSpecialImage(t *testing.T) {
	for _, rule := range []string{"normal"} {
		t.Run(rule, func(t *testing.T) {
			data, err := os.ReadFile("../../testdata/regression/pdf-lzw-special-baseline/source.pdf")
			if err != nil {
				t.Fatal(err)
			}
			source := writeTemp(t, "shading.pdf", data)
			for _, opt := range []RenderOptions{{DPI: 72}, {DPI: 144}, {MaxPixels: 333}} {
				paths, err := RenderPages(context.Background(), source, t.TempDir(), opt)
				if err != nil || len(paths) != 2 {
					t.Fatal(paths, err)
				}
				for _, path := range paths {
					encoded, err := os.ReadFile(path)
					if err != nil {
						t.Fatal(err)
					}
					img, err := png.Decode(bytes.NewReader(encoded))
					if err != nil {
						t.Fatal(err)
					}
					for col, x := range []int{218, 256, 293, 331} {
						want := []color.RGBA{{255, 255, 255, 255}, {0, 0, 0, 255}, {255, 0, 0, 255}, {0, 255, 0, 255}}[col]
						got := color.RGBAModel.Convert(img.At(img.Bounds().Dx()*x/400, img.Bounds().Dy()*350/400)).(color.RGBA)
						if got != want {
							t.Fatalf("%s col %d: got %v want %v", path, col, got, want)
						}
					}
				}
			}
			after, err := os.ReadFile(source)
			if err != nil || !bytes.Equal(data, after) {
				t.Fatal("source changed", err)
			}
		})
	}
}

func TestPreviewLZWResetImage(t *testing.T) {
	for _, rule := range []string{"normal"} {
		t.Run(rule, func(t *testing.T) {
			data, err := os.ReadFile("../../testdata/regression/pdf-lzw-reset-baseline/source.pdf")
			if err != nil {
				t.Fatal(err)
			}
			source := writeTemp(t, "shading.pdf", data)
			for _, opt := range []RenderOptions{{DPI: 72}, {DPI: 144}, {MaxPixels: 333}} {
				paths, err := RenderPages(context.Background(), source, t.TempDir(), opt)
				if err != nil || len(paths) != 2 {
					t.Fatal(paths, err)
				}
				for _, path := range paths {
					encoded, err := os.ReadFile(path)
					if err != nil {
						t.Fatal(err)
					}
					img, err := png.Decode(bytes.NewReader(encoded))
					if err != nil {
						t.Fatal(err)
					}
					for col, x := range []int{218, 256, 293, 331} {
						want := []color.RGBA{{255, 0, 255, 255}, {0, 0, 255, 255}, {0, 255, 0, 255}, {0, 0, 0, 255}}[col]
						got := color.RGBAModel.Convert(img.At(img.Bounds().Dx()*x/400, img.Bounds().Dy()*350/400)).(color.RGBA)
						if got != want {
							t.Fatalf("%s col %d: got %v want %v", path, col, got, want)
						}
					}
				}
			}
			after, err := os.ReadFile(source)
			if err != nil || !bytes.Equal(data, after) {
				t.Fatal("source changed", err)
			}
		})
	}
}

func TestPreviewRunLengthMaxImage(t *testing.T) {
	for _, rule := range []string{"normal"} {
		t.Run(rule, func(t *testing.T) {
			data, err := os.ReadFile("../../testdata/regression/pdf-runlength-max-baseline/source.pdf")
			if err != nil {
				t.Fatal(err)
			}
			source := writeTemp(t, "shading.pdf", data)
			for _, opt := range []RenderOptions{{DPI: 72}, {DPI: 144}, {MaxPixels: 333}} {
				paths, err := RenderPages(context.Background(), source, t.TempDir(), opt)
				if err != nil || len(paths) != 2 {
					t.Fatal(paths, err)
				}
				for _, path := range paths {
					encoded, err := os.ReadFile(path)
					if err != nil {
						t.Fatal(err)
					}
					img, err := png.Decode(bytes.NewReader(encoded))
					if err != nil {
						t.Fatal(err)
					}
					for col, x := range []int{218, 256, 293, 331} {
						want := []color.RGBA{{64, 64, 64, 255}, {64, 64, 64, 255}, {192, 192, 192, 255}, {192, 192, 192, 255}}[col]
						got := color.RGBAModel.Convert(img.At(img.Bounds().Dx()*x/400, img.Bounds().Dy()*350/400)).(color.RGBA)
						if got != want {
							t.Fatalf("%s col %d: got %v want %v", path, col, got, want)
						}
					}
				}
			}
			after, err := os.ReadFile(source)
			if err != nil || !bytes.Equal(data, after) {
				t.Fatal("source changed", err)
			}
		})
	}
}

func TestPreviewLZWWidthImage(t *testing.T) {
	for _, rule := range []string{"normal"} {
		t.Run(rule, func(t *testing.T) {
			data, err := os.ReadFile("../../testdata/regression/pdf-lzw-width-baseline/source.pdf")
			if err != nil {
				t.Fatal(err)
			}
			source := writeTemp(t, "shading.pdf", data)
			for _, opt := range []RenderOptions{{DPI: 72}, {DPI: 144}, {MaxPixels: 333}} {
				paths, err := RenderPages(context.Background(), source, t.TempDir(), opt)
				if err != nil || len(paths) != 2 {
					t.Fatal(paths, err)
				}
				for _, path := range paths {
					encoded, err := os.ReadFile(path)
					if err != nil {
						t.Fatal(err)
					}
					img, err := png.Decode(bytes.NewReader(encoded))
					if err != nil {
						t.Fatal(err)
					}
					for col, x := range []int{218, 256, 293, 331} {
						want := []color.RGBA{{32, 32, 32, 255}, {96, 96, 96, 255}, {160, 160, 160, 255}, {224, 224, 224, 255}}[col]
						got := color.RGBAModel.Convert(img.At(img.Bounds().Dx()*x/400, img.Bounds().Dy()*350/400)).(color.RGBA)
						if got != want {
							t.Fatalf("%s col %d: got %v want %v", path, col, got, want)
						}
					}
				}
			}
			after, err := os.ReadFile(source)
			if err != nil || !bytes.Equal(data, after) {
				t.Fatal("source changed", err)
			}
		})
	}
}

func TestPreviewLZWEarly0Image(t *testing.T) {
	for _, rule := range []string{"normal"} {
		t.Run(rule, func(t *testing.T) {
			data, err := os.ReadFile("../../testdata/regression/pdf-lzw-early0-baseline/source.pdf")
			if err != nil {
				t.Fatal(err)
			}
			source := writeTemp(t, "shading.pdf", data)
			for _, opt := range []RenderOptions{{DPI: 72}, {DPI: 144}, {MaxPixels: 333}} {
				paths, err := RenderPages(context.Background(), source, t.TempDir(), opt)
				if err != nil || len(paths) != 2 {
					t.Fatal(paths, err)
				}
				for _, path := range paths {
					encoded, err := os.ReadFile(path)
					if err != nil {
						t.Fatal(err)
					}
					img, err := png.Decode(bytes.NewReader(encoded))
					if err != nil {
						t.Fatal(err)
					}
					for col, x := range []int{218, 256, 293, 331} {
						want := []color.RGBA{{32, 32, 32, 255}, {96, 96, 96, 255}, {160, 160, 160, 255}, {224, 224, 224, 255}}[col]
						got := color.RGBAModel.Convert(img.At(img.Bounds().Dx()*x/400, img.Bounds().Dy()*350/400)).(color.RGBA)
						if got != want {
							t.Fatalf("%s col %d: got %v want %v", path, col, got, want)
						}
					}
				}
			}
			after, err := os.ReadFile(source)
			if err != nil || !bytes.Equal(data, after) {
				t.Fatal("source changed", err)
			}
		})
	}
}

func TestPreviewLZWWideImage(t *testing.T) {
	for _, rule := range []string{"normal"} {
		t.Run(rule, func(t *testing.T) {
			data, err := os.ReadFile("../../testdata/regression/pdf-lzw-wide-baseline/source.pdf")
			if err != nil {
				t.Fatal(err)
			}
			source := writeTemp(t, "shading.pdf", data)
			for _, opt := range []RenderOptions{{DPI: 72}, {DPI: 144}, {MaxPixels: 333}} {
				paths, err := RenderPages(context.Background(), source, t.TempDir(), opt)
				if err != nil || len(paths) != 2 {
					t.Fatal(paths, err)
				}
				for _, path := range paths {
					encoded, err := os.ReadFile(path)
					if err != nil {
						t.Fatal(err)
					}
					img, err := png.Decode(bytes.NewReader(encoded))
					if err != nil {
						t.Fatal(err)
					}
					for col, x := range []int{218, 256, 293, 331} {
						want := []color.RGBA{{32, 32, 32, 255}, {96, 96, 96, 255}, {160, 160, 160, 255}, {224, 224, 224, 255}}[col]
						got := color.RGBAModel.Convert(img.At(img.Bounds().Dx()*x/400, img.Bounds().Dy()*350/400)).(color.RGBA)
						if got != want {
							t.Fatalf("%s col %d: got %v want %v", path, col, got, want)
						}
					}
				}
			}
			after, err := os.ReadFile(source)
			if err != nil || !bytes.Equal(data, after) {
				t.Fatal("source changed", err)
			}
		})
	}
}

func TestPreviewLZWGrowthResetImage(t *testing.T) {
	for _, rule := range []string{"normal"} {
		t.Run(rule, func(t *testing.T) {
			data, err := os.ReadFile("../../testdata/regression/pdf-lzw-growth-reset-baseline/source.pdf")
			if err != nil {
				t.Fatal(err)
			}
			source := writeTemp(t, "shading.pdf", data)
			for _, opt := range []RenderOptions{{DPI: 72}, {DPI: 144}, {MaxPixels: 333}} {
				paths, err := RenderPages(context.Background(), source, t.TempDir(), opt)
				if err != nil || len(paths) != 2 {
					t.Fatal(paths, err)
				}
				for _, path := range paths {
					encoded, err := os.ReadFile(path)
					if err != nil {
						t.Fatal(err)
					}
					img, err := png.Decode(bytes.NewReader(encoded))
					if err != nil {
						t.Fatal(err)
					}
					for col, x := range []int{218, 256, 293, 331} {
						want := []color.RGBA{{32, 32, 32, 255}, {96, 96, 96, 255}, {160, 160, 160, 255}, {224, 224, 224, 255}}[col]
						got := color.RGBAModel.Convert(img.At(img.Bounds().Dx()*x/400, img.Bounds().Dy()*350/400)).(color.RGBA)
						if got != want {
							t.Fatalf("%s col %d: got %v want %v", path, col, got, want)
						}
					}
				}
			}
			after, err := os.ReadFile(source)
			if err != nil || !bytes.Equal(data, after) {
				t.Fatal("source changed", err)
			}
		})
	}
}

func TestPreviewLZWCrossRefImage(t *testing.T) {
	for _, rule := range []string{"normal"} {
		t.Run(rule, func(t *testing.T) {
			data, err := os.ReadFile("../../testdata/regression/pdf-lzw-crossref-baseline/source.pdf")
			if err != nil {
				t.Fatal(err)
			}
			source := writeTemp(t, "shading.pdf", data)
			for _, opt := range []RenderOptions{{DPI: 72}, {DPI: 144}, {MaxPixels: 333}} {
				paths, err := RenderPages(context.Background(), source, t.TempDir(), opt)
				if err != nil || len(paths) != 2 {
					t.Fatal(paths, err)
				}
				for _, path := range paths {
					encoded, err := os.ReadFile(path)
					if err != nil {
						t.Fatal(err)
					}
					img, err := png.Decode(bytes.NewReader(encoded))
					if err != nil {
						t.Fatal(err)
					}
					for col, x := range []int{218, 256, 293, 331} {
						want := []color.RGBA{{32, 32, 32, 255}, {96, 96, 96, 255}, {160, 160, 160, 255}, {224, 224, 224, 255}}[col]
						got := color.RGBAModel.Convert(img.At(img.Bounds().Dx()*x/400, img.Bounds().Dy()*350/400)).(color.RGBA)
						if got != want {
							t.Fatalf("%s col %d: got %v want %v", path, col, got, want)
						}
					}
				}
			}
			after, err := os.ReadFile(source)
			if err != nil || !bytes.Equal(data, after) {
				t.Fatal("source changed", err)
			}
		})
	}
}

func TestPreviewLZWBoundarySpecialImage(t *testing.T) {
	for _, rule := range []string{"normal"} {
		t.Run(rule, func(t *testing.T) {
			data, err := os.ReadFile("../../testdata/regression/pdf-lzw-boundary-special-baseline/source.pdf")
			if err != nil {
				t.Fatal(err)
			}
			source := writeTemp(t, "shading.pdf", data)
			for _, opt := range []RenderOptions{{DPI: 72}, {DPI: 144}, {MaxPixels: 333}} {
				paths, err := RenderPages(context.Background(), source, t.TempDir(), opt)
				if err != nil || len(paths) != 2 {
					t.Fatal(paths, err)
				}
				for _, path := range paths {
					encoded, err := os.ReadFile(path)
					if err != nil {
						t.Fatal(err)
					}
					img, err := png.Decode(bytes.NewReader(encoded))
					if err != nil {
						t.Fatal(err)
					}
					for col, x := range []int{218, 256, 293, 331} {
						want := []color.RGBA{{32, 32, 32, 255}, {96, 96, 96, 255}, {160, 160, 160, 255}, {224, 224, 224, 255}}[col]
						got := color.RGBAModel.Convert(img.At(img.Bounds().Dx()*x/400, img.Bounds().Dy()*350/400)).(color.RGBA)
						if got != want {
							t.Fatalf("%s col %d: got %v want %v", path, col, got, want)
						}
					}
				}
			}
			after, err := os.ReadFile(source)
			if err != nil || !bytes.Equal(data, after) {
				t.Fatal("source changed", err)
			}
		})
	}
}

func TestPreviewLZWLongSpecialImage(t *testing.T) {
	for _, rule := range []string{"normal"} {
		t.Run(rule, func(t *testing.T) {
			data, err := os.ReadFile("../../testdata/regression/pdf-lzw-long-special-baseline/source.pdf")
			if err != nil {
				t.Fatal(err)
			}
			source := writeTemp(t, "shading.pdf", data)
			for _, opt := range []RenderOptions{{DPI: 72}, {DPI: 144}, {MaxPixels: 333}} {
				paths, err := RenderPages(context.Background(), source, t.TempDir(), opt)
				if err != nil || len(paths) != 2 {
					t.Fatal(paths, err)
				}
				for _, path := range paths {
					encoded, err := os.ReadFile(path)
					if err != nil {
						t.Fatal(err)
					}
					img, err := png.Decode(bytes.NewReader(encoded))
					if err != nil {
						t.Fatal(err)
					}
					for col, x := range []int{218, 256, 293, 331} {
						want := []color.RGBA{{32, 32, 32, 255}, {32, 32, 32, 255}, {224, 224, 224, 255}, {224, 224, 224, 255}}[col]
						got := color.RGBAModel.Convert(img.At(img.Bounds().Dx()*x/400, img.Bounds().Dy()*350/400)).(color.RGBA)
						if got != want {
							t.Fatalf("%s col %d: got %v want %v", path, col, got, want)
						}
					}
				}
			}
			after, err := os.ReadFile(source)
			if err != nil || !bytes.Equal(data, after) {
				t.Fatal("source changed", err)
			}
		})
	}
}

func TestPreviewLZWMixedSpecialImage(t *testing.T) {
	for _, rule := range []string{"normal"} {
		t.Run(rule, func(t *testing.T) {
			data, err := os.ReadFile("../../testdata/regression/pdf-lzw-mixed-special-baseline/source.pdf")
			if err != nil {
				t.Fatal(err)
			}
			source := writeTemp(t, "shading.pdf", data)
			for _, opt := range []RenderOptions{{DPI: 72}, {DPI: 144}, {MaxPixels: 333}} {
				paths, err := RenderPages(context.Background(), source, t.TempDir(), opt)
				if err != nil || len(paths) != 2 {
					t.Fatal(paths, err)
				}
				for _, path := range paths {
					encoded, err := os.ReadFile(path)
					if err != nil {
						t.Fatal(err)
					}
					img, err := png.Decode(bytes.NewReader(encoded))
					if err != nil {
						t.Fatal(err)
					}
					for col, x := range []int{210, 232, 253, 275} {
						want := []color.RGBA{{32, 32, 32, 255}, {224, 224, 224, 255}, {32, 32, 32, 255}, {224, 224, 224, 255}}[col]
						got := color.RGBAModel.Convert(img.At(img.Bounds().Dx()*x/400, img.Bounds().Dy()*350/400)).(color.RGBA)
						if got != want {
							t.Fatalf("%s col %d: got %v want %v", path, col, got, want)
						}
					}
				}
			}
			after, err := os.ReadFile(source)
			if err != nil || !bytes.Equal(data, after) {
				t.Fatal("source changed", err)
			}
		})
	}
}

func TestPreviewLZWEarly0WideImage(t *testing.T) {
	for _, rule := range []string{"normal"} {
		t.Run(rule, func(t *testing.T) {
			data, err := os.ReadFile("../../testdata/regression/pdf-lzw-early0-wide-baseline/source.pdf")
			if err != nil {
				t.Fatal(err)
			}
			source := writeTemp(t, "shading.pdf", data)
			for _, opt := range []RenderOptions{{DPI: 72}, {DPI: 144}, {MaxPixels: 333}} {
				paths, err := RenderPages(context.Background(), source, t.TempDir(), opt)
				if err != nil || len(paths) != 2 {
					t.Fatal(paths, err)
				}
				for _, path := range paths {
					encoded, err := os.ReadFile(path)
					if err != nil {
						t.Fatal(err)
					}
					img, err := png.Decode(bytes.NewReader(encoded))
					if err != nil {
						t.Fatal(err)
					}
					for col, x := range []int{218, 256, 293, 331} {
						want := []color.RGBA{{32, 32, 32, 255}, {96, 96, 96, 255}, {160, 160, 160, 255}, {224, 224, 224, 255}}[col]
						got := color.RGBAModel.Convert(img.At(img.Bounds().Dx()*x/400, img.Bounds().Dy()*350/400)).(color.RGBA)
						if got != want {
							t.Fatalf("%s col %d: got %v want %v", path, col, got, want)
						}
					}
				}
			}
			after, err := os.ReadFile(source)
			if err != nil || !bytes.Equal(data, after) {
				t.Fatal("source changed", err)
			}
		})
	}
}

func TestPreviewLZWEarly0ResetImage(t *testing.T) {
	for _, rule := range []string{"normal"} {
		t.Run(rule, func(t *testing.T) {
			data, err := os.ReadFile("../../testdata/regression/pdf-lzw-early0-reset-baseline/source.pdf")
			if err != nil {
				t.Fatal(err)
			}
			source := writeTemp(t, "shading.pdf", data)
			for _, opt := range []RenderOptions{{DPI: 72}, {DPI: 144}, {MaxPixels: 333}} {
				paths, err := RenderPages(context.Background(), source, t.TempDir(), opt)
				if err != nil || len(paths) != 2 {
					t.Fatal(paths, err)
				}
				for _, path := range paths {
					encoded, err := os.ReadFile(path)
					if err != nil {
						t.Fatal(err)
					}
					img, err := png.Decode(bytes.NewReader(encoded))
					if err != nil {
						t.Fatal(err)
					}
					for col, x := range []int{218, 256, 293, 331} {
						want := []color.RGBA{{32, 32, 32, 255}, {96, 96, 96, 255}, {160, 160, 160, 255}, {224, 224, 224, 255}}[col]
						got := color.RGBAModel.Convert(img.At(img.Bounds().Dx()*x/400, img.Bounds().Dy()*350/400)).(color.RGBA)
						if got != want {
							t.Fatalf("%s col %d: got %v want %v", path, col, got, want)
						}
					}
				}
			}
			after, err := os.ReadFile(source)
			if err != nil || !bytes.Equal(data, after) {
				t.Fatal("source changed", err)
			}
		})
	}
}

func TestPreviewLZWRepeatedClearImage(t *testing.T) {
	for _, rule := range []string{"normal"} {
		t.Run(rule, func(t *testing.T) {
			data, err := os.ReadFile("../../testdata/regression/pdf-lzw-repeated-clear-baseline/source.pdf")
			if err != nil {
				t.Fatal(err)
			}
			source := writeTemp(t, "shading.pdf", data)
			for _, opt := range []RenderOptions{{DPI: 72}, {DPI: 144}, {MaxPixels: 333}} {
				paths, err := RenderPages(context.Background(), source, t.TempDir(), opt)
				if err != nil || len(paths) != 2 {
					t.Fatal(paths, err)
				}
				for _, path := range paths {
					encoded, err := os.ReadFile(path)
					if err != nil {
						t.Fatal(err)
					}
					img, err := png.Decode(bytes.NewReader(encoded))
					if err != nil {
						t.Fatal(err)
					}
					for col, x := range []int{218, 256, 293, 331} {
						want := []color.RGBA{{255, 0, 255, 255}, {0, 0, 255, 255}, {0, 255, 0, 255}, {0, 0, 0, 255}}[col]
						got := color.RGBAModel.Convert(img.At(img.Bounds().Dx()*x/400, img.Bounds().Dy()*350/400)).(color.RGBA)
						if got != want {
							t.Fatalf("%s col %d: got %v want %v", path, col, got, want)
						}
					}
				}
			}
			after, err := os.ReadFile(source)
			if err != nil || !bytes.Equal(data, after) {
				t.Fatal("source changed", err)
			}
		})
	}
}

func TestPreviewLZWTerminalClearImage(t *testing.T) {
	for _, rule := range []string{"normal"} {
		t.Run(rule, func(t *testing.T) {
			data, err := os.ReadFile("../../testdata/regression/pdf-lzw-terminal-clear-baseline/source.pdf")
			if err != nil {
				t.Fatal(err)
			}
			source := writeTemp(t, "shading.pdf", data)
			for _, opt := range []RenderOptions{{DPI: 72}, {DPI: 144}, {MaxPixels: 333}} {
				paths, err := RenderPages(context.Background(), source, t.TempDir(), opt)
				if err != nil || len(paths) != 2 {
					t.Fatal(paths, err)
				}
				for _, path := range paths {
					encoded, err := os.ReadFile(path)
					if err != nil {
						t.Fatal(err)
					}
					img, err := png.Decode(bytes.NewReader(encoded))
					if err != nil {
						t.Fatal(err)
					}
					for col, x := range []int{218, 256, 293, 331} {
						want := []color.RGBA{{255, 0, 255, 255}, {0, 0, 255, 255}, {0, 255, 0, 255}, {0, 0, 0, 255}}[col]
						got := color.RGBAModel.Convert(img.At(img.Bounds().Dx()*x/400, img.Bounds().Dy()*350/400)).(color.RGBA)
						if got != want {
							t.Fatalf("%s col %d: got %v want %v", path, col, got, want)
						}
					}
				}
			}
			after, err := os.ReadFile(source)
			if err != nil || !bytes.Equal(data, after) {
				t.Fatal("source changed", err)
			}
		})
	}
}

func TestPreviewLZWSaturationImage(t *testing.T) {
	for _, rule := range []string{"normal"} {
		t.Run(rule, func(t *testing.T) {
			data, err := os.ReadFile("../../testdata/regression/pdf-lzw-saturation-baseline/source.pdf")
			if err != nil {
				t.Fatal(err)
			}
			source := writeTemp(t, "shading.pdf", data)
			for _, opt := range []RenderOptions{{DPI: 72}, {DPI: 144}, {MaxPixels: 333}} {
				paths, err := RenderPages(context.Background(), source, t.TempDir(), opt)
				if err != nil || len(paths) != 2 {
					t.Fatal(paths, err)
				}
				for _, path := range paths {
					encoded, err := os.ReadFile(path)
					if err != nil {
						t.Fatal(err)
					}
					img, err := png.Decode(bytes.NewReader(encoded))
					if err != nil {
						t.Fatal(err)
					}
					for col, x := range []int{218, 256, 293, 331} {
						want := []color.RGBA{{32, 32, 32, 255}, {96, 96, 96, 255}, {160, 160, 160, 255}, {224, 224, 224, 255}}[col]
						got := color.RGBAModel.Convert(img.At(img.Bounds().Dx()*x/400, img.Bounds().Dy()*350/400)).(color.RGBA)
						if got != want {
							t.Fatalf("%s col %d: got %v want %v", path, col, got, want)
						}
					}
				}
			}
			after, err := os.ReadFile(source)
			if err != nil || !bytes.Equal(data, after) {
				t.Fatal("source changed", err)
			}
		})
	}
}

func TestPreviewLZWEarly0SaturationImage(t *testing.T) {
	for _, rule := range []string{"normal"} {
		t.Run(rule, func(t *testing.T) {
			data, err := os.ReadFile("../../testdata/regression/pdf-lzw-early0-saturation-baseline/source.pdf")
			if err != nil {
				t.Fatal(err)
			}
			source := writeTemp(t, "shading.pdf", data)
			for _, opt := range []RenderOptions{{DPI: 72}, {DPI: 144}, {MaxPixels: 333}} {
				paths, err := RenderPages(context.Background(), source, t.TempDir(), opt)
				if err != nil || len(paths) != 2 {
					t.Fatal(paths, err)
				}
				for _, path := range paths {
					encoded, err := os.ReadFile(path)
					if err != nil {
						t.Fatal(err)
					}
					img, err := png.Decode(bytes.NewReader(encoded))
					if err != nil {
						t.Fatal(err)
					}
					for col, x := range []int{218, 256, 293, 331} {
						want := []color.RGBA{{32, 32, 32, 255}, {96, 96, 96, 255}, {160, 160, 160, 255}, {224, 224, 224, 255}}[col]
						got := color.RGBAModel.Convert(img.At(img.Bounds().Dx()*x/400, img.Bounds().Dy()*350/400)).(color.RGBA)
						if got != want {
							t.Fatalf("%s col %d: got %v want %v", path, col, got, want)
						}
					}
				}
			}
			after, err := os.ReadFile(source)
			if err != nil || !bytes.Equal(data, after) {
				t.Fatal("source changed", err)
			}
		})
	}
}

func TestPreviewLZWSaturationResetImage(t *testing.T) {
	for _, rule := range []string{"normal"} {
		t.Run(rule, func(t *testing.T) {
			data, err := os.ReadFile("../../testdata/regression/pdf-lzw-saturation-reset-baseline/source.pdf")
			if err != nil {
				t.Fatal(err)
			}
			source := writeTemp(t, "shading.pdf", data)
			for _, opt := range []RenderOptions{{DPI: 72}, {DPI: 144}, {MaxPixels: 333}} {
				paths, err := RenderPages(context.Background(), source, t.TempDir(), opt)
				if err != nil || len(paths) != 2 {
					t.Fatal(paths, err)
				}
				for _, path := range paths {
					encoded, err := os.ReadFile(path)
					if err != nil {
						t.Fatal(err)
					}
					img, err := png.Decode(bytes.NewReader(encoded))
					if err != nil {
						t.Fatal(err)
					}
					for col, x := range []int{218, 256, 293, 331} {
						want := []color.RGBA{{32, 32, 32, 255}, {96, 96, 96, 255}, {160, 160, 160, 255}, {224, 224, 224, 255}}[col]
						got := color.RGBAModel.Convert(img.At(img.Bounds().Dx()*x/400, img.Bounds().Dy()*350/400)).(color.RGBA)
						if got != want {
							t.Fatalf("%s col %d: got %v want %v", path, col, got, want)
						}
					}
				}
			}
			after, err := os.ReadFile(source)
			if err != nil || !bytes.Equal(data, after) {
				t.Fatal("source changed", err)
			}
		})
	}
}

func TestPreviewLZWEarly0SaturationResetImage(t *testing.T) {
	for _, rule := range []string{"normal"} {
		t.Run(rule, func(t *testing.T) {
			data, err := os.ReadFile("../../testdata/regression/pdf-lzw-early0-saturation-reset-baseline/source.pdf")
			if err != nil {
				t.Fatal(err)
			}
			source := writeTemp(t, "shading.pdf", data)
			for _, opt := range []RenderOptions{{DPI: 72}, {DPI: 144}, {MaxPixels: 333}} {
				paths, err := RenderPages(context.Background(), source, t.TempDir(), opt)
				if err != nil || len(paths) != 2 {
					t.Fatal(paths, err)
				}
				for _, path := range paths {
					encoded, err := os.ReadFile(path)
					if err != nil {
						t.Fatal(err)
					}
					img, err := png.Decode(bytes.NewReader(encoded))
					if err != nil {
						t.Fatal(err)
					}
					for col, x := range []int{218, 256, 293, 331} {
						want := []color.RGBA{{32, 32, 32, 255}, {96, 96, 96, 255}, {160, 160, 160, 255}, {224, 224, 224, 255}}[col]
						got := color.RGBAModel.Convert(img.At(img.Bounds().Dx()*x/400, img.Bounds().Dy()*350/400)).(color.RGBA)
						if got != want {
							t.Fatalf("%s col %d: got %v want %v", path, col, got, want)
						}
					}
				}
			}
			after, err := os.ReadFile(source)
			if err != nil || !bytes.Equal(data, after) {
				t.Fatal("source changed", err)
			}
		})
	}
}

func TestPreviewLZWSaturatedRefImage(t *testing.T) {
	for _, rule := range []string{"normal"} {
		t.Run(rule, func(t *testing.T) {
			data, err := os.ReadFile("../../testdata/regression/pdf-lzw-saturated-ref-baseline/source.pdf")
			if err != nil {
				t.Fatal(err)
			}
			source := writeTemp(t, "shading.pdf", data)
			for _, opt := range []RenderOptions{{DPI: 72}, {DPI: 144}, {MaxPixels: 333}} {
				paths, err := RenderPages(context.Background(), source, t.TempDir(), opt)
				if err != nil || len(paths) != 2 {
					t.Fatal(paths, err)
				}
				for _, path := range paths {
					encoded, err := os.ReadFile(path)
					if err != nil {
						t.Fatal(err)
					}
					img, err := png.Decode(bytes.NewReader(encoded))
					if err != nil {
						t.Fatal(err)
					}
					for col, x := range []int{218, 256, 293, 331} {
						want := []color.RGBA{{32, 32, 32, 255}, {96, 96, 96, 255}, {96, 96, 96, 255}, {96, 96, 96, 255}}[col]
						got := color.RGBAModel.Convert(img.At(img.Bounds().Dx()*x/400, img.Bounds().Dy()*350/400)).(color.RGBA)
						if got != want {
							t.Fatalf("%s col %d: got %v want %v", path, col, got, want)
						}
					}
				}
			}
			after, err := os.ReadFile(source)
			if err != nil || !bytes.Equal(data, after) {
				t.Fatal("source changed", err)
			}
		})
	}
}
