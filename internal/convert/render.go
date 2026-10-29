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
	Page       int    `json:"page"`
	Status     string `json:"status"`
	BeforePage int    `json:"before_page,omitempty"`
	AfterPage  int    `json:"after_page,omitempty"`
	Bounds     []int  `json:"bounds,omitempty"`
	Image      string `json:"image,omitempty"`
}

func (p PageChange) MarshalJSON() ([]byte, error) {
	fields := map[string]any{"page": p.Page, "status": p.Status}
	if p.BeforePage != 0 {
		fields["before_page"] = p.BeforePage
	}
	if p.AfterPage != 0 {
		fields["after_page"] = p.AfterPage
	}
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

const pageFeatureGrid = 48
const pageFeatureSamples = 4

type pageFeature [pageFeatureGrid * pageFeatureGrid * 3]uint8

type pageAlignment struct {
	before int
	after  int
	kind   byte
}

const (
	pageMatch byte = iota + 1
	pageRemoved
	pageAdded
)

// pageImageFeature downsamples each page to a fixed grid for inexpensive,
// deterministic similarity comparisons. Small sample blocks retain the page's
// broad text and shape layout without requiring exact pixel identity.
func pageImageFeature(img image.Image) pageFeature {
	var feature pageFeature
	bounds := img.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	if width == 0 || height == 0 {
		return feature
	}
	for gy := 0; gy < pageFeatureGrid; gy++ {
		for gx := 0; gx < pageFeatureGrid; gx++ {
			var sums [3]uint32
			for sy := 0; sy < pageFeatureSamples; sy++ {
				for sx := 0; sx < pageFeatureSamples; sx++ {
					x := bounds.Min.X + ((gx*pageFeatureSamples+sx)*width+width/2)/(pageFeatureGrid*pageFeatureSamples)
					y := bounds.Min.Y + ((gy*pageFeatureSamples+sy)*height+height/2)/(pageFeatureGrid*pageFeatureSamples)
					c := color.RGBAModel.Convert(img.At(x, y)).(color.RGBA)
					alpha := uint32(c.A)
					sums[0] += (uint32(c.R)*alpha + 255*(255-alpha)) / 255
					sums[1] += (uint32(c.G)*alpha + 255*(255-alpha)) / 255
					sums[2] += (uint32(c.B)*alpha + 255*(255-alpha)) / 255
				}
			}
			i := (gy*pageFeatureGrid + gx) * 3
			for channel := range sums {
				feature[i+channel] = uint8(sums[channel] / (pageFeatureSamples * pageFeatureSamples))
			}
		}
	}
	return feature
}

func pageFeatureDistance(a, b pageFeature) float64 {
	var total uint64
	for i := range a {
		x, y := int(a[i]), int(b[i])
		if x > y {
			total += uint64(x - y)
		} else {
			total += uint64(y - x)
		}
	}
	return float64(total) / float64(len(a)*255)
}

// alignPages uses global sequence alignment with image distance as the match
// cost and a fixed cost for inserted or removed pages. Ties prefer matching
// earlier pages, which makes runs of identical pages stable across comparisons.
func alignPages(before, after []image.Image) []pageAlignment {
	const gapCost = 0.55
	n, m := len(before), len(after)
	beforeFeatures := make([]pageFeature, n)
	afterFeatures := make([]pageFeature, m)
	for i, page := range before {
		beforeFeatures[i] = pageImageFeature(page)
	}
	for i, page := range after {
		afterFeatures[i] = pageImageFeature(page)
	}

	cost := make([][]float64, n+1)
	next := make([][]byte, n+1)
	for i := range cost {
		cost[i] = make([]float64, m+1)
		next[i] = make([]byte, m+1)
	}
	for i := n; i >= 0; i-- {
		for j := m; j >= 0; j-- {
			if i == n && j == m {
				continue
			}
			best := 1e9
			var step byte
			if i < n && j < m {
				best = pageFeatureDistance(beforeFeatures[i], afterFeatures[j]) + cost[i+1][j+1]
				step = pageMatch
			}
			if i < n {
				candidate := gapCost + cost[i+1][j]
				if candidate < best {
					best, step = candidate, pageRemoved
				}
			}
			if j < m {
				candidate := gapCost + cost[i][j+1]
				if candidate < best {
					best, step = candidate, pageAdded
				}
			}
			cost[i][j], next[i][j] = best, step
		}
	}

	alignment := make([]pageAlignment, 0, max(n, m))
	for i, j := 0, 0; i < n || j < m; {
		switch next[i][j] {
		case pageMatch:
			alignment = append(alignment, pageAlignment{before: i, after: j, kind: pageMatch})
			i++
			j++
		case pageRemoved:
			alignment = append(alignment, pageAlignment{before: i, kind: pageRemoved})
			i++
		case pageAdded:
			alignment = append(alignment, pageAlignment{after: j, kind: pageAdded})
			j++
		default:
			return nil
		}
	}
	return alignment
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
	beforePages := make([]image.Image, len(left))
	for i, path := range left {
		beforePages[i], err = loadPNG(path)
		if err != nil {
			return nil, err
		}
	}
	afterPages := make([]image.Image, len(right))
	for i, path := range right {
		afterPages[i], err = loadPNG(path)
		if err != nil {
			return nil, err
		}
	}

	var changes []PageChange
	for _, pair := range alignPages(beforePages, afterPages) {
		switch pair.kind {
		case pageRemoved:
			changes = append(changes, PageChange{Page: pair.before + 1, Status: "removed"})
			continue
		case pageAdded:
			changes = append(changes, PageChange{Page: pair.after + 1, Status: "added"})
			continue
		}

		a, b := beforePages[pair.before], afterPages[pair.after]
		diff, bounds := diffImages(a, b)
		name := filepath.Join(output, fmt.Sprintf("diff-%04d.png", pair.after+1))
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
		change := PageChange{Page: pair.after + 1, Status: st, Bounds: bounds, Image: name}
		if pair.before != pair.after {
			change.BeforePage = pair.before + 1
			change.AfterPage = pair.after + 1
		}
		changes = append(changes, change)
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
