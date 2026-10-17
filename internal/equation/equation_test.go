package equation

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func decode(t *testing.T, b []byte) image.Image {
	t.Helper()
	im, e := png.Decode(bytes.NewReader(b))
	if e != nil {
		t.Fatal(e)
	}
	return im
}
func imageFor(t *testing.T, s string, o Options) image.Image {
	t.Helper()
	b, e := PNG(context.Background(), s, o)
	if e != nil {
		t.Fatal(e)
	}
	return decode(t, b)
}
func TestReferenceHelperContract(t *testing.T) {
	root := t.TempDir()
	type rendered struct {
		result Result
		im     image.Image
	}
	var supplemental []rendered
	for i, expression := range []string{`\int_{-\infty}^{\infty} e^{-x^2}\,dx=\sqrt{\pi}`, `\begin{aligned}f(x)&=\sum_{n=0}^{\infty}\frac{x^n}{n!}\\A&=\begin{pmatrix}1&2\\3&4\end{pmatrix}\end{aligned}`} {
		dest := filepath.Join(root, fmt.Sprintf("%d.png", i))
		result, err := Write(context.Background(), expression, dest, Defaults())
		if err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(dest)
		if err != nil {
			t.Fatal(err)
		}
		// Decode independently of the renderer's reported dimensions.
		im, format, err := image.Decode(bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}
		if format != "png" {
			t.Fatal("expected PNG", format)
		}
		if !equationPNGHasAlpha(t, data) {
			t.Fatal("PNG lacks alpha")
		}
		width, height := im.Bounds().Dx(), im.Bounds().Dy()
		if width <= 100 || height <= 50 {
			t.Fatal(width, height)
		}
		ink := 0
		for y := 0; y < height; y++ {
			for x := 0; x < width; x++ {
				_, _, _, alpha := im.At(x, y).RGBA()
				if alpha > 0 {
					ink++
				}
			}
		}
		if ink <= 100 || 2*ink >= width*height {
			t.Fatalf("ink %d of %dx%d", ink, width, height)
		}
		for x := 0; x < width; x++ {
			_, _, _, top := im.At(x, 0).RGBA()
			if top != 0 {
				t.Fatalf("top edge clips at %d", x)
			}
			_, _, _, bottom := im.At(x, height-1).RGBA()
			if bottom != 0 {
				t.Fatalf("bottom edge clips at %d", x)
			}
		}
		original, err := os.ReadFile(dest)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := Write(context.Background(), "x", dest, Defaults()); !errors.Is(err, os.ErrExist) {
			t.Fatalf("overwrite must preserve EEXIST identity: %v", err)
		}
		after, err := os.ReadFile(dest)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(original, after) {
			t.Fatal("overwrite changed original")
		}
		supplemental = append(supplemental, rendered{result, im})
	}
	invalidDest := filepath.Join(root, "invalid.png")
	for _, expression := range []string{"", `\frac{`, `\includegraphics{https://example.com/x}`, `\require{html}`} {
		if _, err := Write(context.Background(), expression, invalidDest, Defaults()); err == nil {
			t.Fatalf("accepted %q", expression)
		}
		if _, err := os.Stat(invalidDest); !os.IsNotExist(err) {
			t.Fatal("invalid output is accessible", err)
		}
	}
	inline := Defaults()
	inline.Inline = true
	inlinePath, displayPath := filepath.Join(root, "inline.png"), filepath.Join(root, "display.png")
	if _, err := Write(context.Background(), `\frac{1}{2}`, inlinePath, inline); err != nil {
		t.Fatal(err)
	}
	if _, err := Write(context.Background(), `\frac{1}{2}`, displayPath, Defaults()); err != nil {
		t.Fatal(err)
	}
	height := func(path string) int {
		t.Helper()
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		return decode(t, data).Bounds().Dy()
	}
	if height(inlinePath) >= height(displayPath) {
		t.Fatal("inline not compact")
	}
	// Supplemental native assertions follow the entire original contract.
	for _, r := range supplemental {
		width, height := r.im.Bounds().Dx(), r.im.Bounds().Dy()
		if width != r.result.Width || height != r.result.Height {
			t.Fatal("reported dimensions differ from PNG")
		}
		for y := 0; y < height; y++ {
			for _, x := range []int{0, width - 1} {
				_, _, _, alpha := r.im.At(x, y).RGBA()
				if alpha > 0 {
					t.Fatalf("side clips at %d,%d", x, y)
				}
			}
		}
	}
	for _, expression := range []string{`x\newcommand{\foo}{\foo}\foo`, strings.Repeat("{", 300) + "x" + strings.Repeat("}", 300), strings.Repeat("x", 20001)} {
		if _, err := Write(context.Background(), expression, invalidDest, Defaults()); err == nil {
			t.Fatal("accepted supplemental invalid expression")
		}
		if _, err := os.Stat(invalidDest); !os.IsNotExist(err) {
			t.Fatal("invalid created output", err)
		}
	}
}

// Alpha metadata includes PNG tRNS transparency, not just IHDR types 4 and 6.
// The image decoder above validates the PNG before this chunk walk.
func equationPNGHasAlpha(t *testing.T, data []byte) bool {
	t.Helper()
	for offset := 8; offset+12 <= len(data); {
		size := uint64(binary.BigEndian.Uint32(data[offset : offset+4]))
		end := uint64(offset) + 12 + size
		if end > uint64(len(data)) {
			t.Fatal("truncated PNG chunk")
		}
		kind := string(data[offset+4 : offset+8])
		if kind == "IHDR" && size == 13 && (data[offset+17] == 4 || data[offset+17] == 6) {
			return true
		}
		if kind == "tRNS" {
			return true
		}
		offset = int(end)
	}
	return false
}
func TestCancellationAndOptions(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e := PNG(ctx, "x", Defaults()); e != context.Canceled {
		t.Fatal(e)
	}
	o := Defaults()
	o.Scale = 16
	o.FontSize = 1000
	if _, e := PNG(context.Background(), strings.Repeat("x", 100), o); e == nil {
		t.Fatal("allocation not bounded")
	}
}
func TestBothArrowLabels(t *testing.T) {
	im := imageFor(t, `A\xrightarrow[below]{above}B`, Defaults())
	if im.Bounds().Dy() < 100 {
		t.Fatal(im.Bounds())
	}
}

// Rendering acceptance is distinct from the reference-pixel assertions.
// Three original-corpus inputs are rejected by the original helper too.
func TestOriginal37Baseline(t *testing.T) {
	data, e := os.ReadFile("../../testdata/equations/original-37.json")
	if e != nil {
		t.Fatal(e)
	}
	var cases map[string]string
	if e = json.Unmarshal(data, &cases); e != nil {
		t.Fatal(e)
	}
	if len(cases) != 37 {
		t.Fatal(len(cases))
	}
	for name, s := range cases {
		t.Run(name, func(t *testing.T) {
			pngData, e := PNG(context.Background(), s, Defaults())
			if name == "quantity" || name == "derivative" || name == "tag" {
				if e == nil {
					t.Fatal("expected documented unsupported input")
				}
				return
			}
			if e != nil {
				t.Fatal(e)
			}
			im := decode(t, pngData)
			b := im.Bounds()
			for y := 0; y < b.Dy(); y++ {
				for x := 0; x < b.Dx(); x++ {
					if x != 0 && y != 0 && x != b.Dx()-1 && y != b.Dy()-1 {
						continue
					}
					_, _, _, a := im.At(x, y).RGBA()
					if a > 0 {
						t.Fatalf("edge clipping at %d,%d", x, y)
					}
				}
			}
		})
	}
}
