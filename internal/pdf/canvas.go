package pdf

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"github.com/pdfcpu/pdfcpu/pkg/font"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/color"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/create"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/sfnt"
)

// Canvas is an alternate fixed-layout authoring surface. Coordinates are points
// from the top left. Text remains searchable and fonts are embedded.
type Canvas struct {
	Fonts map[string]string `json:"fonts,omitempty"`
	Pages []CanvasPage      `json:"pages"`
}
type CanvasPage struct {
	Width    float64         `json:"width"`
	Height   float64         `json:"height"`
	Elements []CanvasElement `json:"elements"`
}
type CanvasElement struct {
	Type      string       `json:"type"`
	X         float64      `json:"x"`
	Y         float64      `json:"y"`
	W         float64      `json:"w,omitempty"`
	H         float64      `json:"h,omitempty"`
	Text      string       `json:"text,omitempty"`
	Font      string       `json:"font,omitempty"`
	FontSize  float64      `json:"font_size,omitempty"`
	Fill      string       `json:"fill,omitempty"`
	Stroke    string       `json:"stroke,omitempty"`
	LineWidth float64      `json:"line_width,omitempty"`
	Path      string       `json:"path,omitempty"`
	Points    [][2]float64 `json:"points,omitempty"`
}

func createCanvasPDF(ctx context.Context, s Canvas, base, out string) error {
	if len(s.Pages) == 0 || len(s.Pages) > 1000 {
		return fmt.Errorf("canvas pages must contain 1..1000 entries")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	// Keep font preparation private to this invocation, without changing global
	// pdfcpu configuration or installing fonts into the user's environment.
	dir, err := os.MkdirTemp("", "paperworks-canvas-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	conf, err := model.LoadConfiguration(dir)
	if err != nil {
		return err
	}
	conf.Offline = true
	fontDir, _ := conf.UserFontStore()
	fonts := map[string]string{}
	fontData := map[string][]byte{}
	install := func(alias string, data []byte) error {
		parsed, err := sfnt.Parse(data)
		if err != nil {
			return err
		}
		name, err := parsed.Name(nil, sfnt.NameIDPostScript)
		if err != nil || name == "" {
			return fmt.Errorf("font %s needs a PostScript name", alias)
		}
		if prior, ok := fontData[name]; ok && !bytes.Equal(prior, data) {
			return fmt.Errorf("different fonts share PostScript name %q", name)
		}
		if err := font.InstallFontFromBytesQuiet(fontDir, "font.ttf", data); err != nil {
			return err
		}
		fontData[name] = data
		fonts[alias] = name
		return nil
	}
	if err := install("default", goregular.TTF); err != nil {
		return err
	}
	for alias, file := range s.Fonts {
		if alias == "" || alias == "default" {
			return fmt.Errorf("invalid custom font name")
		}
		if !filepath.IsAbs(file) {
			file = filepath.Join(base, file)
		}
		data, err := canvasAsset(file, "font")
		if err != nil {
			return err
		}
		if err := install(alias, data); err != nil {
			return err
		}
	}
	c, err := pdfcpu.CreateContextWithXRefTable(conf, types.PaperSize["A4"])
	if err != nil {
		return err
	}
	pages := []*model.Page{}
	fontMap := model.FontMap{}
	total := 0
	for _, page := range s.Pages {
		if !finite(page.Width) || !finite(page.Height) || page.Width <= 0 || page.Height <= 0 || page.Width > 14400 || page.Height > 14400 {
			return fmt.Errorf("invalid canvas page size")
		}
		p := model.NewPage(types.RectForDim(page.Width, page.Height), types.RectForDim(page.Width, page.Height))
		for _, e := range page.Elements {
			total++
			if total > 100000 {
				return fmt.Errorf("canvas exceeds 100000 elements")
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			for _, v := range []float64{e.X, e.Y, e.W, e.H, e.FontSize, e.LineWidth} {
				if !finite(v) {
					return fmt.Errorf("nonfinite canvas coordinate")
				}
			}
			if e.W < 0 || e.H < 0 || e.FontSize < 0 || e.LineWidth < 0 {
				return fmt.Errorf("negative canvas size")
			}
			stroke := e.Stroke
			if stroke == "" {
				stroke = "000000"
			}
			r, g, b, err := canvasColor(stroke)
			if err != nil {
				return err
			}
			col := color.SimpleColor{R: float32(r) / 255, G: float32(g) / 255, B: float32(b) / 255}
			width := e.LineWidth
			if width == 0 {
				width = .5
			}
			fmt.Fprintf(p.Buf, "q %.6f %.6f %.6f RG %.6f w\n", col.R, col.G, col.B, width)
			paint := "S"
			if e.Fill != "" {
				r, g, b, err := canvasColor(e.Fill)
				if err != nil {
					return err
				}
				fmt.Fprintf(p.Buf, "%.6f %.6f %.6f rg\n", float64(r)/255, float64(g)/255, float64(b)/255)
				paint = "f"
				if e.Stroke != "" {
					paint = "B"
				}
			}
			switch e.Type {
			case "text":
				alias := e.Font
				if alias == "" {
					alias = "default"
				}
				name, ok := fonts[alias]
				if !ok {
					return fmt.Errorf("unknown font %q", alias)
				}
				size := e.FontSize
				if size == 0 {
					size = 12
				}
				key := p.Fm.EnsureKey(name)
				fontMap.EnsureKey(name)
				lines := []string{e.Text}
				x, baseline := e.X, e.Y
				leading := e.H
				if leading == 0 {
					leading = size * 1.2
				}
				if e.W > 0 {
					// Preserve the previous text box's inset and line-height semantics.
					const inset = 72.0 / 25.4
					if e.W <= 2*inset {
						return fmt.Errorf("text box too narrow")
					}
					lines = nil
					for _, paragraph := range strings.Split(strings.ReplaceAll(e.Text, "\r", ""), "\n") {
						wrapped, err := c.WordWrapFloat(ctx, paragraph, name, size, e.W-2*inset)
						if err != nil {
							return err
						}
						if len(wrapped) == 0 {
							wrapped = []string{""}
						}
						lines = append(lines, wrapped...)
					}
					x += inset
					baseline += leading/2 + size*.3
				}
				for i, line := range lines {
					td := model.TextDescriptor{Text: line, FontName: name, FontKey: key, FontSize: size, Embed: true,
						X: x, Y: page.Height - baseline - float64(i)*leading, ScaleAbs: true, Scale: 1, FillCol: col, StrokeCol: col}
					if _, err := model.WriteMultiLine(ctx, c.XRefTable, p.Buf, p.MediaBox, nil, td); err != nil {
						return err
					}
				}
			case "rect":
				fmt.Fprintf(p.Buf, "%.6f %.6f %.6f %.6f re %s\n", e.X, page.Height-e.Y-e.H, e.W, e.H, paint)
			case "ellipse":
				x, y, rx, ry := e.X+e.W/2, page.Height-e.Y-e.H/2, e.W/2, e.H/2
				const k = .5522847498307936
				fmt.Fprintf(p.Buf, "%.6f %.6f m\n", x+rx, y)
				for _, v := range [][6]float64{{x + rx, y + k*ry, x + k*rx, y + ry, x, y + ry}, {x - k*rx, y + ry, x - rx, y + k*ry, x - rx, y}, {x - rx, y - k*ry, x - k*rx, y - ry, x, y - ry}, {x + k*rx, y - ry, x + rx, y - k*ry, x + rx, y}} {
					fmt.Fprintf(p.Buf, "%.6f %.6f %.6f %.6f %.6f %.6f c\n", v[0], v[1], v[2], v[3], v[4], v[5])
				}
				fmt.Fprintf(p.Buf, "h %s\n", paint)
			case "line":
				fmt.Fprintf(p.Buf, "%.6f %.6f m %.6f %.6f l S\n", e.X, page.Height-e.Y, e.X+e.W, page.Height-e.Y-e.H)
			case "polygon", "bezier":
				if e.Type == "polygon" && (len(e.Points) < 3 || len(e.Points) > 100000) {
					return fmt.Errorf("polygon needs 3..100000 points")
				}
				if e.Type == "bezier" && len(e.Points) != 4 {
					return fmt.Errorf("bezier needs four points")
				}
				for _, point := range e.Points {
					if !finite(point[0]) || !finite(point[1]) {
						return fmt.Errorf("invalid point")
					}
				}
				v := e.Points
				fmt.Fprintf(p.Buf, "%.6f %.6f m\n", v[0][0], page.Height-v[0][1])
				if e.Type == "polygon" {
					for _, point := range v[1:] {
						fmt.Fprintf(p.Buf, "%.6f %.6f l\n", point[0], page.Height-point[1])
					}
					fmt.Fprintf(p.Buf, "h %s\n", paint)
				} else {
					fmt.Fprintf(p.Buf, "%.6f %.6f %.6f %.6f %.6f %.6f c %s\n", v[1][0], page.Height-v[1][1], v[2][0], page.Height-v[2][1], v[3][0], page.Height-v[3][1], paint)
				}
			case "image":
				file := e.Path
				if !filepath.IsAbs(file) {
					file = filepath.Join(base, file)
				}
				data, err := canvasAsset(file, "image")
				if err != nil {
					return err
				}
				ir, w, h, err := model.CreateImageResource(c.XRefTable, bytes.NewReader(data))
				if err != nil {
					return err
				}
				key := fmt.Sprintf("Im%d", len(p.Im))
				p.Im[key] = model.ImageResource{Res: model.Resource{ID: key, IndRef: ir}, Width: w, Height: h}
				width, height := e.W, e.H
				if width == 0 && height == 0 {
					width, height = float64(w)*.75, float64(h)*.75
				}
				if width == 0 {
					width = height * float64(w) / float64(h)
				}
				if height == 0 {
					height = width * float64(h) / float64(w)
				}
				fmt.Fprintf(p.Buf, "%.6f 0 0 %.6f %.6f %.6f cm /%s Do\n", width, height, e.X, page.Height-e.Y-height, key)
			default:
				return fmt.Errorf("unsupported canvas element %q", e.Type)
			}
			p.Buf.WriteString("Q\n")
		}
		pages = append(pages, &p)
	}
	if _, _, err := create.UpdatePageTree(ctx, c, pages, fontMap); err != nil {
		return err
	}
	data, err := Serialize(ctx, c)
	if err != nil {
		return err
	}
	if _, err := Load(ctx, data); err != nil {
		return err
	}
	return opc.WriteNew(out, data)
}

func canvasAsset(file, kind string) ([]byte, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > 32<<20 {
		return nil, fmt.Errorf("%s must be a regular file under 32 MiB", kind)
	}
	data, err := io.ReadAll(io.LimitReader(f, (32<<20)+1))
	if len(data) > 32<<20 {
		return nil, fmt.Errorf("%s exceeds 32 MiB", kind)
	}
	return data, err
}
func canvasColor(s string) (int, int, int, error) {
	if len(s) != 6 {
		return 0, 0, 0, fmt.Errorf("color must be six hex digits")
	}
	v, err := strconv.ParseUint(s, 16, 24)
	return int(v >> 16), int(v >> 8 & 255), int(v & 255), err
}
