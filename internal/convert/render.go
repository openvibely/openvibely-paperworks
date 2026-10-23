package convert

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/beevik/etree"
	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"github.com/openvibely/openvibely-paperworks/internal/pdf"
	"github.com/pmezard/go-difflib/difflib"
)

const unsupportedChartMarker = "one or more chart types are unsupported in this version of libreoffice"

// VerifyOfficeChartPreview rejects LibreOffice's placeholder page for ChartEx
// charts instead of letting it pass as a chart preview.
func VerifyOfficeChartPreview(ctx context.Context, source, pdfPath string) error {
	p, err := opc.Open(source)
	if err != nil {
		return err
	}
	charts, err := office.ChartParts(p)
	if err != nil {
		return err
	}
	extended := false
	for _, n := range charts {
		doc, err := p.XML(n)
		if err != nil {
			return err
		}
		extended = extended || office.Is(doc.Root(), office.NSCX, "chartSpace")
	}
	if !extended {
		return nil
	}
	var src strings.Builder
	for _, n := range p.Names() {
		if strings.HasPrefix(n, "ppt/slides/") && strings.HasSuffix(n, ".xml") && !strings.Contains(n, "/_rels/") {
			doc, err := p.XML(n)
			if err != nil {
				return err
			}
			office.Iter(doc.Root(), func(e *etree.Element) bool {
				src.WriteString(" " + e.Text())
				return true
			})
		}
	}
	// A deck discussing this error can legitimately contain these words.
	if strings.Contains(strings.ToLower(src.String()), unsupportedChartMarker) {
		return nil
	}
	data, err := os.ReadFile(pdfPath)
	if err != nil {
		return err
	}
	pages, err := pdf.PageText(ctx, data)
	if err != nil {
		return err
	}
	text := strings.Join(strings.Fields(strings.ToLower(strings.Join(pages, " "))), " ")
	if strings.Contains(text, unsupportedChartMarker) {
		return errors.New("LibreOffice could not render an extended chart. The native PPTX is intact; use a renderer supporting this chart type. No verified preview was produced")
	}
	return nil
}

// RenderFile renders a PDF or Office file to PNG pages, reusing an existing output directory if present.
func RenderFile(ctx context.Context, path, output, sheet, cellRange string, opt pdf.RenderOptions) ([]string, error) {
	e := ext(path)
	if (sheet != "" || cellRange != "") && (e != ".xlsx" || sheet == "") {
		return nil, errors.New("preview selection requires XLSX and --sheet")
	}
	if err := opt.Validate(); err != nil {
		return nil, err
	}
	var pdfPath string
	if e == ".pdf" {
		if err := os.MkdirAll(output, 0o755); err != nil {
			return nil, err
		}
		pdfPath = path
	} else {
		o := Options{reuseOutput: true, Sheet: sheet, CellRange: cellRange, WholeSheets: e == ".xlsx" && cellRange == ""}
		res, err := deps(ctx).convert(ctx, path, output, o)
		if err != nil {
			return nil, err
		}
		pdfPath = res["output"].(string)
		if e == ".pptx" {
			if err := VerifyOfficeChartPreview(ctx, path, pdfPath); err != nil {
				return nil, err
			}
		}
	}
	return deps(ctx).render(ctx, pdfPath, output, opt)
}

type PageChange struct {
	Page   int    `json:"page"`
	Status string `json:"status"`
	Bounds []int  `json:"bounds,omitempty"`
	Image  string `json:"image,omitempty"`
}

func (p PageChange) MarshalJSON() ([]byte, error) {
	fields := map[string]any{"page": p.Page, "status": p.Status}
	if p.Image != "" {
		fields["image"] = p.Image
		fields["bounds"] = p.Bounds
	}
	return json.Marshal(fields)
}

func loadPNG(p string) (image.Image, error) {
	f, err := os.Open(p)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return png.Decode(f)
}

// diffImages returns |a-b| per channel on a white canvas sized to both images
// and the bounding box of non-zero pixels (nil when identical).
func diffImages(a, b image.Image) (*image.RGBA, []int) {
	w := max(a.Bounds().Dx(), b.Bounds().Dx())
	h := max(a.Bounds().Dy(), b.Bounds().Dy())
	out := image.NewRGBA(image.Rect(0, 0, w, h))
	at := func(img image.Image, x, y int) (uint8, uint8, uint8) {
		bb := img.Bounds()
		if x >= bb.Dx() || y >= bb.Dy() {
			return 255, 255, 255
		}
		c := color.RGBAModel.Convert(img.At(bb.Min.X+x, bb.Min.Y+y)).(color.RGBA)
		// Composite onto white like converting RGBA to RGB after pasting.
		al := uint16(c.A)
		blend := func(v uint8) uint8 { return uint8((uint16(v)*255 + 255*(255-al)) / 255) }
		if c.A == 255 {
			return c.R, c.G, c.B
		}
		return blend(c.R), blend(c.G), blend(c.B)
	}
	d := func(x, y uint8) uint8 {
		if x > y {
			return x - y
		}
		return y - x
	}
	minX, minY, maxX, maxY := w, h, -1, -1
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			r1, g1, b1 := at(a, x, y)
			r2, g2, b2 := at(b, x, y)
			r, g, bl := d(r1, r2), d(g1, g2), d(b1, b2)
			i := out.PixOffset(x, y)
			out.Pix[i], out.Pix[i+1], out.Pix[i+2], out.Pix[i+3] = r, g, bl, 255
			if r|g|bl != 0 {
				minX, minY, maxX, maxY = min(minX, x), min(minY, y), max(maxX, x), max(maxY, y)
			}
		}
	}
	if maxX < 0 {
		return out, nil
	}
	return out, []int{minX, minY, maxX + 1, maxY + 1}
}

var docxTextPart = regexp.MustCompile(`^word/(document|header\d+|footer\d+|footnotes|endnotes|comments)\.xml$`)

// docxLines extracts text and separators in document order. Deleted text remains
// included as before. Tabs remain tabs; untyped/textWrapping breaks and
// carriage returns become line feeds. Page and column breaks use explicit
// printable markers so they cannot be mistaken for text wrapping; other typed
// breaks retain their type in a marker.
func docxLines(path string) ([]string, error) {
	p, err := opc.Open(path)
	if err != nil {
		return nil, err
	}
	names := p.Names()
	slices.Sort(names)
	var lines []string
	for _, n := range names {
		if !docxTextPart.MatchString(n) {
			continue
		}
		doc, err := p.XML(n)
		if err != nil {
			return nil, err
		}
		lines = append(lines, "["+n+"]\n")
		for _, para := range office.All(doc.Root(), office.NSW, "p") {
			var b strings.Builder
			office.Iter(para, func(e *etree.Element) bool {
				switch {
				case office.Is(e, office.NSW, "t"), office.Is(e, office.NSW, "delText"):
					b.WriteString(e.Text())
				case office.Is(e, office.NSW, "tab"):
					b.WriteByte('\t')
				case office.Is(e, office.NSW, "cr"):
					b.WriteByte('\n')
				case office.Is(e, office.NSW, "br"):
					switch office.AttrV(e, office.NSW, "type") {
					case "", "textWrapping":
						b.WriteByte('\n')
					case "page":
						b.WriteString("[page break]")
					case "column":
						b.WriteString("[column break]")
					default:
						b.WriteString("[break type=")
						b.WriteString(office.AttrV(e, office.NSW, "type"))
						b.WriteByte(']')
					}
				}
				return true
			})
			appendDocxDiffLines(&lines, b.String()+"\n")
		}
	}
	return lines, nil
}

// appendDocxDiffLines splits text at line feeds while retaining each line's
// newline. This keeps the artifact text byte-for-byte faithful and gives the
// unified diff line boundaries for both paragraphs and inline text breaks.
func appendDocxDiffLines(lines *[]string, text string) {
	for len(text) > 0 {
		end := strings.IndexByte(text, '\n')
		if end < 0 {
			*lines = append(*lines, text)
			return
		}
		*lines = append(*lines, text[:end+1])
		text = text[end+1:]
	}
}

// Compare renders both files and writes per-page pixel diffs, plus a text
// diff for DOCX pairs.
func Compare(ctx context.Context, before, after, output string, opt pdf.RenderOptions) (map[string]any, error) {
	if err := NewOutputDir(output); err != nil {
		return nil, err
	}
	left, err := RenderFile(ctx, before, filepath.Join(output, "before"), "", "", opt)
	if err != nil {
		return nil, err
	}
	right, err := RenderFile(ctx, after, filepath.Join(output, "after"), "", "", opt)
	if err != nil {
		return nil, err
	}
	var changes []PageChange
	for i := 0; i < max(len(left), len(right)); i++ {
		if i >= min(len(left), len(right)) {
			st := "removed"
			if i >= len(left) {
				st = "added"
			}
			changes = append(changes, PageChange{Page: i + 1, Status: st})
			continue
		}
		a, err := loadPNG(left[i])
		if err != nil {
			return nil, err
		}
		b, err := loadPNG(right[i])
		if err != nil {
			return nil, err
		}
		diff, bounds := diffImages(a, b)
		name := filepath.Join(output, fmt.Sprintf("diff-%04d.png", i+1))
		f, err := os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err != nil {
			return nil, err
		}
		err = png.Encode(f, diff)
		f.Close()
		if err != nil {
			return nil, err
		}
		st := "same"
		if bounds != nil || a.Bounds().Size() != b.Bounds().Size() {
			st = "changed"
		}
		changes = append(changes, PageChange{Page: i + 1, Status: st, Bounds: bounds, Image: name})
	}
	result := map[string]any{"pages": changes, "visually_inspected": false}
	if ext(before) == ".docx" && ext(after) == ".docx" {
		first, err := docxLines(before)
		if err != nil {
			return nil, err
		}
		second, err := docxLines(after)
		if err != nil {
			return nil, err
		}
		os.WriteFile(filepath.Join(output, "text-before.txt"), []byte(strings.Join(first, "")), 0o644)
		os.WriteFile(filepath.Join(output, "text-after.txt"), []byte(strings.Join(second, "")), 0o644)
		diff, err := difflib.GetUnifiedDiffString(difflib.UnifiedDiff{A: first, B: second, FromFile: filepath.Base(before), ToFile: filepath.Base(after), Context: 3})
		if err != nil {
			return nil, err
		}
		dest := filepath.Join(output, "text-diff.txt")
		if err := os.WriteFile(dest, []byte(diff), 0o644); err != nil {
			return nil, err
		}
		result["text_changed"] = diff != ""
		result["text_diff"] = dest
	}
	return result, nil
}
