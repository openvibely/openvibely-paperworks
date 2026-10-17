package pdf

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// FixedTable is a point-based table with uniform rows. Position is the lower
// left corner; column widths are points, not percentages. Header is optional.
// Text uses Helvetica, left aligned at PaddingLeft; the baseline is BaselineOffset points
// from each cell's lower-left corner. No wrapping or autosizing is performed.
type FixedTable struct {
	Position       [2]float64 `json:"pos"`
	ColumnWidths   []float64  `json:"column_widths"`
	RowHeight      float64    `json:"row_height"`
	FontSize       float64    `json:"font_size"`
	PaddingLeft    float64    `json:"padding_left"`
	BaselineOffset float64    `json:"baseline_offset"`
	Header         []string   `json:"header,omitempty"`
	Rows           [][]string `json:"rows"`
	HeaderColor    [3]float64 `json:"header_color"`
	GridColor      [3]float64 `json:"grid_color"`
	GridWidth      float64    `json:"grid_width"`
}

func finite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }

func (t FixedTable) validate() error {
	if len(t.ColumnWidths) == 0 || len(t.ColumnWidths) > 1000 || len(t.Rows) == 0 || len(t.Rows) > 10000 || len(t.Rows)*len(t.ColumnWidths) > 100000 {
		return fmt.Errorf("fixed table exceeds limits or has no cells")
	}
	for _, v := range append(append([]float64{}, t.Position[:]...), t.RowHeight, t.FontSize, t.PaddingLeft, t.BaselineOffset, t.GridWidth) {
		if !finite(v) {
			return fmt.Errorf("fixed table coordinates must be finite")
		}
	}
	if t.RowHeight <= 0 || t.FontSize <= 0 || t.GridWidth < 0 || t.PaddingLeft < 0 || t.BaselineOffset < 0 {
		return fmt.Errorf("invalid fixed table dimensions")
	}
	width := 0.0
	for _, v := range t.ColumnWidths {
		if !finite(v) || v <= 0 {
			return fmt.Errorf("invalid fixed table column width")
		}
		width += v
	}
	if !finite(width+t.Position[0]+t.PaddingLeft) || !finite(t.RowHeight*float64(len(t.Rows)+1)+t.Position[1]+t.BaselineOffset) {
		return fmt.Errorf("fixed table dimensions overflow")
	}
	for _, rgb := range [][3]float64{t.HeaderColor, t.GridColor} {
		for _, v := range rgb {
			if !finite(v) || v < 0 || v > 1 {
				return fmt.Errorf("fixed table colors must be in 0..1")
			}
		}
	}
	rows := t.Rows
	if len(t.Header) > 0 {
		rows = append([][]string{t.Header}, rows...)
	}
	for _, r := range rows {
		if len(r) != len(t.ColumnWidths) {
			return fmt.Errorf("fixed table row has wrong column count")
		}
		for _, s := range r {
			if e := supportedText(s); e != nil {
				return e
			}
			if strings.ContainsAny(s, "\n\r\t") {
				return fmt.Errorf("fixed table cells must be single-line text")
			}
		}
	}
	return nil
}

func pdfNumber(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }
func fixedString(s string) string {
	return strings.NewReplacer(`\`, `\\`, `(`, `\(`, `)`, `\)`).Replace(s)
}

func (t FixedTable) paint(b *bytes.Buffer, font string) {
	n := reportNumber
	x, y := t.Position[0], t.Position[1]
	w := 0.0
	for _, v := range t.ColumnWidths {
		w += v
	}
	rows := t.Rows
	if len(t.Header) > 0 {
		rows = append([][]string{t.Header}, rows...)
	}
	h := float64(len(rows)) * t.RowHeight
	fmt.Fprintf(b, "q\n1 0 0 1 %s %s cm\nq\n", n(x), n(y))
	x, y = 0, 0
	if len(t.Header) > 0 {
		c := t.HeaderColor
		fmt.Fprintf(b, "%s %s %s rg\nn %s %s %s %s re f*\n", n(c[0]), n(c[1]), n(c[2]), n(x), n(y+h), n(w), n(-t.RowHeight))
	}
	for i, row := range rows {
		if i == 0 && len(t.Header) > 0 {
			fmt.Fprintln(b, "1 1 1 rg")
		} else if i == 0 || (i == 1 && len(t.Header) > 0) {
			fmt.Fprintln(b, "0 0 0 rg")
		}
		if i == 0 {
			fmt.Fprintf(b, "BT /%s %s Tf %s TL ET\n", font, n(t.FontSize), n(t.FontSize*1.2))
		}
		cx := x
		for j, s := range row {
			text := ""
			if s != "" {
				text = "(" + fixedString(s) + ") Tj"
			}
			fmt.Fprintf(b, "BT 1 0 0 1 %s %s Tm %s T* ET\n", n(cx+t.PaddingLeft), n(y+h-float64(i+1)*t.RowHeight+t.BaselineOffset), text)
			cx += t.ColumnWidths[j]
		}
	}
	c := t.GridColor
	fmt.Fprintf(b, "q\n1 J\n1 j\n%s %s %s RG\n%s w\n", n(c[0]), n(c[1]), n(c[2]), n(t.GridWidth))
	line := func(x0, y0, x1, y1 float64) { fmt.Fprintf(b, "n %s %s m %s %s l S\n", n(x0), n(y0), n(x1), n(y1)) }
	line(0, h, w, h)
	line(0, 0, w, 0)
	line(0, 0, 0, h)
	line(w, 0, w, h)
	for i := 1; i < len(rows); i++ {
		cy := h - float64(i)*t.RowHeight
		line(0, cy, w, cy)
	}
	cx := 0.0
	for i := 0; i < len(t.ColumnWidths)-1; i++ {
		cx += t.ColumnWidths[i]
		line(cx, 0, cx, h)
	}

	fmt.Fprintln(b, "Q\nQ\nQ")
}

// applyFixedLayout adds explicitly positioned tables after the existing fixed
// content. Size changes the final media/crop boxes, never scales existing ink.
func applyFixedLayout(ctx context.Context, c *model.Context, pages []CreatePage) (bool, error) {
	changed := false
	for i, p := range pages {
		if err := ctx.Err(); err != nil {
			return false, err
		}
		if p.Size == nil && len(p.FixedTables) == 0 && len(p.FixedParagraphs) == 0 && len(p.FixedSpacers) == 0 {
			continue
		}
		pd, inh, err := pageDict(ctx, c, i+1)
		if err != nil {
			return false, err
		}
		if p.Size != nil {
			box := types.Array{types.Integer(0), types.Integer(0), types.Float(p.Size[0]), types.Float(p.Size[1])}
			pd["MediaBox"] = box
			pd["CropBox"] = box.Clone()
		}
		if len(p.FixedTables)+len(p.FixedParagraphs)+len(p.FixedSpacers) > 0 {
			res, _, err := writableResources(c.XRefTable, pd, inh)
			if err != nil {
				return false, err
			}
			fonts := dictOf(c.XRefTable, res["Font"]).d
			if fonts == nil {
				fonts = types.Dict{}
			} else {
				fonts = fonts.Clone().(types.Dict)
			}
			res["Font"] = fonts
			// Reuse only an identical built-in resource; custom encodings and
			// width overrides must never change the table's glyph metrics.
			want := types.Dict{"Type": types.Name("Font"), "Subtype": types.Name("Type1"), "BaseFont": types.Name("Helvetica"), "Encoding": types.Name("WinAnsiEncoding")}
			names := make([]string, 0, len(fonts))
			for key := range fonts {
				names = append(names, key)
			}
			sort.Strings(names)
			name := ""
			for _, key := range names {
				d := dictOf(c.XRefTable, fonts[key]).d
				if len(d) != len(want) {
					continue
				}
				same := true
				for k, v := range want {
					if n, ok := d[k].(types.Name); !ok || n != v.(types.Name) {
						same = false
						break
					}
				}
				if same {
					name = key
					break
				}
			}
			if name == "" {
				name = "FixedTableFont"
				if len(p.FixedParagraphs) > 0 {
					name = "F1"
				}
				for j := 1; fonts[name] != nil; j++ {
					name = fmt.Sprintf("FixedTableFont%d", j)
				}
				fonts[name] = want
			}
			var b bytes.Buffer
			standalone := len(p.FixedParagraphs) > 0 && len(p.Text)+len(p.Tables)+len(p.Images)+len(p.TextFields)+len(p.Checkboxes)+len(p.RadioGroups)+len(p.ComboBoxes) == 0
			if standalone {
				fmt.Fprintf(&b, "1 0 0 1 0 0 cm  BT /%s 12 Tf 14.4 TL ET\n", name)
			}
			for _, para := range p.FixedParagraphs {
				fn := fixedFont(c, fonts, para.Font)
				if err := para.paint(ctx, &b, fn); err != nil {
					return false, err
				}
			}
			for _, pos := range p.FixedSpacers {
				fmt.Fprintf(&b, "q\n1 0 0 1 %s %s cm\nQ\n", reportNumber(pos[0]), reportNumber(pos[1]))
			}
			for _, t := range p.FixedTables {
				t.paint(&b, name)
			}
			if standalone {
				fmt.Fprint(&b, " \n")
				sd, e := reportStream(b.Bytes())
				if e != nil {
					return false, e
				}
				ref, e := c.IndRefForNewObject(sd)
				if e != nil {
					return false, e
				}
				pd["Contents"] = *ref
				// Standalone fixed pages have only the built-in resources created
				// above. Publish the ReportLab font dictionary indirection without
				// changing resource handling on mixed, image or interactive pages.
				published := types.Dict{}
				for _, key := range sortedFontKeys(fonts) {
					font := dictOf(c.XRefTable, fonts[key]).d.Clone().(types.Dict)
					font["Name"] = types.Name(key)
					fr, err := c.IndRefForNewObject(font)
					if err != nil {
						return false, err
					}
					published[key] = *fr
				}
				fr, err := c.IndRefForNewObject(published)
				if err != nil {
					return false, err
				}
				pd["Resources"] = types.Dict{"Font": *fr, "ProcSet": types.Array{types.Name("PDF"), types.Name("Text"), types.Name("ImageB"), types.Name("ImageC"), types.Name("ImageI")}}
				pd["Rotate"] = types.Integer(0)
				pd["Trans"] = types.Dict{}
				delete(pd, "CropBox")

			} else if err = wrapContents(c.XRefTable, pd, b.Bytes()); err != nil {
				return false, err
			}
		}
		changed = true
	}
	return changed, nil
}
