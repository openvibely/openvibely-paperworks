package pdf

import (
	"bytes"
	"context"
	"golang.org/x/image/font/gofont/gobold"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUnicodeCanvasAndLayoutExtraction(t *testing.T) {
	ctx := context.Background()
	out := filepath.Join(t.TempDir(), "unicode.pdf")
	s := CreateSpec{Canvas: &Canvas{Pages: []CanvasPage{{Width: 300, Height: 200, Elements: []CanvasElement{{Type: "text", X: 20, Y: 40, Text: "Café Ελληνικά", FontSize: 14}, {Type: "rect", X: 10, Y: 60, W: 50, H: 20, Fill: "FF0000"}}}}}}
	if err := Create(ctx, s, ".", out); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(out)
	pages, err := PageText(ctx, data)
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) != 1 || !strings.Contains(pages[0], "Café Ελληνικά") {
		t.Fatal(pages)
	}
	layout, err := ExtractLayout(ctx, data, LayoutOptions{Pages: []int{1}, Columns: []float64{0, 300}, Rows: []float64{0, 200}})
	if err != nil {
		t.Fatal(err)
	}
	if len(layout) != 1 || len(layout[0].Characters) == 0 || !strings.Contains(layout[0].Table[0][0], "Café") {
		t.Fatal(layout)
	}
}

func TestCanvasFontsAndGraphicsRender(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "bold.ttf"), gobold.TTF, 0600); err != nil {
		t.Fatal(err)
	}
	im := image.NewRGBA(image.Rect(0, 0, 20, 10))
	draw.Draw(im, im.Bounds(), &image.Uniform{color.RGBA{0, 0, 255, 255}}, image.Point{}, draw.Src)
	var buf bytes.Buffer
	if err := png.Encode(&buf, im); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "blue.png"), buf.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "canvas.pdf")
	canvas := Canvas{Fonts: map[string]string{"bold": "bold.ttf"}, Pages: []CanvasPage{
		{Width: 300, Height: 200, Elements: []CanvasElement{
			{Type: "text", X: 10, Y: 20, Text: "Café", Font: "bold"},
			{Type: "rect", X: 10, Y: 40, W: 40, H: 30, Fill: "FF0000"},
			{Type: "ellipse", X: 60, Y: 40, W: 40, H: 30, Fill: "00FF00"},
			{Type: "polygon", Points: [][2]float64{{110, 40}, {150, 40}, {130, 70}}, Fill: "FF00FF"},
			{Type: "image", X: 170, Y: 40, W: 40, Path: "blue.png"},
			{Type: "line", X: 10, Y: 90, W: 40, LineWidth: 4, Stroke: "0000FF"},
			{Type: "bezier", Points: [][2]float64{{60, 90}, {70, 90}, {90, 90}, {100, 90}}, LineWidth: 4, Stroke: "FF0000"},
			{Type: "rect", X: 20, Y: 50, W: 10, H: 10, Fill: "000000"},
		}},
		{Width: 300, Height: 200, Elements: []CanvasElement{
			{Type: "text", X: 10, Y: 20, Text: "Ελληνικά", Font: "bold"},
			{Type: "text", X: 10, Y: 40, W: 90, Text: "one two three four five six", FontSize: 14},
		}},
	}}
	if err := Create(ctx, CreateSpec{Canvas: &canvas}, dir, out); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	texts, err := PageText(ctx, data)
	if err != nil {
		t.Fatal(err)
	}
	if len(texts) != 2 || !strings.Contains(texts[0], "Café") || !strings.Contains(texts[1], "Ελληνικά") || !strings.Contains(texts[1], "six") {
		t.Fatalf("missing embedded font text: %q", texts)
	}
	count, err := RenderImages(ctx, data, RenderOptions{DPI: 72, Opaque: true}, func(page int, img *image.RGBA) error {
		if img.Bounds().Dx() != 300 || img.Bounds().Dy() != 200 {
			t.Fatalf("page size: %v", img.Bounds())
		}
		if page != 1 {
			return nil
		}
		for _, sample := range []struct {
			x, y int
			want color.RGBA
		}{
			{15, 45, color.RGBA{255, 0, 0, 255}}, {80, 55, color.RGBA{0, 255, 0, 255}},
			{130, 50, color.RGBA{255, 0, 255, 255}}, {190, 50, color.RGBA{0, 0, 255, 255}},
			{30, 90, color.RGBA{0, 0, 255, 255}}, {80, 90, color.RGBA{255, 0, 0, 255}},
			{25, 55, color.RGBA{0, 0, 0, 255}}, {190, 65, color.RGBA{255, 255, 255, 255}},
		} {
			if got := img.RGBAAt(sample.x, sample.y); got != sample.want {
				t.Errorf("pixel %d,%d = %v, want %v", sample.x, sample.y, got, sample.want)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("rendered %d pages", count)
	}
}

func TestLayoutTableJoinsWrappedLines(t *testing.T) {
	ctx := context.Background()
	text := func(x, y float64, s string) CanvasElement {
		return CanvasElement{Type: "text", X: x, Y: y, Text: s, FontSize: 10}
	}
	out := filepath.Join(t.TempDir(), "wrapped.pdf")
	canvas := Canvas{Pages: []CanvasPage{{Width: 400, Height: 200, Elements: []CanvasElement{
		text(10, 100, "Net sales"), text(10, 115, "2024"),
		text(110, 100, "Gross"), text(210, 100, "Margin"),
		text(310, 40, "alpha"), text(310, 55, "beta"), text(310, 70, "gamma"),
		text(10, 150, "Trailing "), text(10, 165, "space"),
		text(110, 150, "-"), text(110, 165, ".5"),
		text(210, 150, "tight"), text(210, 156, "lead"),
	}}}}
	if err := Create(ctx, CreateSpec{Canvas: &canvas}, ".", out); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	layout, err := ExtractLayout(ctx, data, LayoutOptions{Columns: []float64{0, 100, 200, 300, 400}, Rows: []float64{0, 80, 120, 200}})
	if err != nil {
		t.Fatal(err)
	}
	got := layout[0].Table
	want := [][]string{
		{"", "", "", "alpha beta gamma"},
		{"Net sales 2024", "Gross", "Margin", ""},
		{"Trailing space", "- .5", "tight lead", ""},
	}
	for i := range want {
		for j := range want[i] {
			if got[i][j] != want[i][j] {
				t.Errorf("cell %d,%d = %q, want %q", i, j, got[i][j], want[i][j])
			}
		}
	}
}

func TestLayoutTableKeepsSingleLinePunctuation(t *testing.T) {
	ctx := context.Background()
	cells := []string{`it's`, `x² m°`, `say "no"`, `e-mail ~x^`, `Net sales*`, `"quoted"`, `°C ²`, `'tis ^up`, `a b, c`, `~-*`, `-.5`, `'.'`, `*.`, `~,`, `°.`, `x₂ H₂O`}
	var elems []CanvasElement
	for i, s := range cells {
		elems = append(elems, CanvasElement{Type: "text", X: float64(i)*100 + 10, Y: 40, Text: s, FontSize: 10})
	}
	out := filepath.Join(t.TempDir(), "punct.pdf")
	canvas := Canvas{Pages: []CanvasPage{{Width: float64(len(cells)) * 100, Height: 100, Elements: elems}}}
	if err := Create(ctx, CreateSpec{Canvas: &canvas}, ".", out); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	cols := []float64{0}
	for i := range cells {
		cols = append(cols, float64(i+1)*100)
	}
	layout, err := ExtractLayout(ctx, data, LayoutOptions{Columns: cols, Rows: []float64{0, 100}})
	if err != nil {
		t.Fatal(err)
	}
	for i, want := range cells {
		if got := layout[0].Table[0][i]; got != want {
			t.Errorf("cell %d = %q, want %q", i, got, want)
		}
	}
}
