package pdf

import (
	"context"
	"fmt"
	"math"
	"strings"

	"github.com/pdfcpu/pdfcpu/pkg/font"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/primitives"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// FlowSpec uses points. Blocks are laid out top-to-bottom inside equal margins.
type FlowSpec struct {
	Margin   float64     `json:"margin"`
	FontSize int         `json:"font_size"`
	Leading  float64     `json:"leading"`
	Blocks   []FlowBlock `json:"blocks"`
}
type FlowBlock struct {
	Kind         string     `json:"kind"`
	Text         string     `json:"text,omitempty"`
	Height       float64    `json:"height,omitempty"`
	Width        float64    `json:"width,omitempty"`
	Src          string     `json:"src,omitempty"`
	Caption      string     `json:"caption,omitempty"`
	Header       []string   `json:"header,omitempty"`
	Rows         [][]string `json:"rows,omitempty"`
	ColumnWidths []int      `json:"column_widths,omitempty"`
}

func flowPages(ctx context.Context, paper string, flow *FlowSpec) ([]CreatePage, error) {
	dim, ok := types.PaperSize[paper]
	if !ok {
		return nil, fmt.Errorf("unknown flow paper %q", paper)
	}
	margin, size, leading := flow.Margin, flow.FontSize, flow.Leading
	if margin == 0 {
		margin = 72
	}
	if size == 0 {
		size = 12
	}
	if leading == 0 {
		leading = float64(size) * 1.4
	}
	finite := func(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }
	if !finite(margin) || !finite(leading) || margin < 0 || margin*2 >= dim.Width || margin*2 >= dim.Height || size < 6 || size > 72 || leading < float64(size)*1.2 || leading > 144 {
		return nil, fmt.Errorf("invalid flow margins, size or leading")
	}
	if len(flow.Blocks) < 1 || len(flow.Blocks) > 10000 {
		return nil, fmt.Errorf("flow requires 1..10000 blocks")
	}
	width, usable := dim.Width-2*margin, dim.Height-2*margin
	if usable < leading*2 {
		return nil, fmt.Errorf("flow content area too short")
	}
	pages := []CreatePage{{}}
	y := dim.Height - margin
	newPage := func() error {
		if len(pages) >= 1000 {
			return fmt.Errorf("flow exceeds 1000 pages")
		}
		pages = append(pages, CreatePage{})
		y = dim.Height - margin
		return nil
	}
	room := func(height float64) error {
		if height > usable {
			return fmt.Errorf("flow element exceeds usable page height")
		}
		if y-height < margin {
			return newPage()
		}
		return nil
	}
	count := 0
	for i, b := range flow.Blocks {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		switch b.Kind {
		case "paragraph", "heading":
			name, fs, step := "Helvetica", size, leading
			if b.Kind == "heading" {
				name = "Helvetica-Bold"
				fs = size + 6
				step = float64(fs) * 1.4
			}
			lines, err := wrapFlow(ctx, b.Text, name, fs, width)
			if err != nil {
				return nil, fmt.Errorf("block %d: %w", i+1, err)
			}
			if b.Kind == "heading" {
				reserve := float64(len(lines)) * step
				if i+1 < len(flow.Blocks) {
					next := flow.Blocks[i+1]
					switch next.Kind {
					case "paragraph":
						reserve += leading + leading/2
					case "heading":
						reserve += float64(size+6)*1.4 + leading/2
					case "table":
						table, err := prepareFlowTable(ctx, next, size, leading, width)
						if err != nil {
							return nil, err
						}
						reserve += float64(table.headerHeight+table.rowHeight) + leading/2
					}
				}
				if err := room(reserve); err != nil {
					return nil, err
				}
			}
			for _, line := range lines {
				if err := room(step); err != nil {
					return nil, err
				}
				y -= step
				p := &pages[len(pages)-1]
				p.Text = append(p.Text, &primitives.TextBox{Value: line, Position: [2]float64{margin, y}, Font: &primitives.FormFont{Name: name, Size: float64(fs)}})
				count++
				if count > 10000 {
					return nil, fmt.Errorf("flow exceeds 10000 elements")
				}
			}
			y -= leading / 2
		case "figure":
			if b.Src == "" || !finite(b.Width) || !finite(b.Height) || b.Width <= 0 || b.Width > width || b.Height <= 0 {
				return nil, fmt.Errorf("figure requires a local src and positive width/height within the content area")
			}
			var lines []string
			if b.Caption != "" {
				var err error
				lines, err = wrapFlow(ctx, b.Caption, "Helvetica", size, b.Width)
				if err != nil {
					return nil, fmt.Errorf("figure caption: %w", err)
				}
			}
			captionHeight := float64(len(lines)) * leading
			if len(lines) > 0 {
				captionHeight += leading / 2
			}
			if err := room(b.Height + captionHeight); err != nil {
				return nil, err
			}
			x := margin + (width-b.Width)/2
			y -= b.Height
			page := &pages[len(pages)-1]
			page.Images = append(page.Images, &primitives.ImageBox{Src: b.Src, Position: [2]float64{x, y}, Width: b.Width, Height: b.Height})
			if len(lines) > 0 {
				y -= leading / 2
			}
			for _, line := range lines {
				y -= leading
				page.Text = append(page.Text, &primitives.TextBox{Value: line, Position: [2]float64{x, y}, Font: &primitives.FormFont{Name: "Helvetica", Size: float64(size)}})
			}
			count += 1 + len(lines)
			if count > 10000 {
				return nil, fmt.Errorf("flow exceeds 10000 elements")
			}
			y -= leading / 2
		case "spacer":
			if !finite(b.Height) || b.Height < 0 || b.Height > usable {
				return nil, fmt.Errorf("invalid spacer height")
			}
			if err := room(b.Height); err != nil {
				return nil, err
			}
			y -= b.Height
		case "page_break":
			if err := newPage(); err != nil {
				return nil, err
			}
		case "table":
			prepared, err := prepareFlowTable(ctx, b, size, leading, width)
			if err != nil {
				return nil, err
			}
			cols, widths := len(b.Header), prepared.widths
			header, rows := prepared.header, prepared.rows
			headerHeight, rowHeight := prepared.headerHeight, prepared.rowHeight
			for start := 0; start < len(b.Rows); {
				if err := ctx.Err(); err != nil {
					return nil, err
				}
				if err := room(float64(headerHeight + rowHeight)); err != nil {
					return nil, err
				}
				n := min(len(b.Rows)-start, int((y-margin-float64(headerHeight))/float64(rowHeight)))
				height := float64(n*rowHeight + headerHeight)
				y -= height
				table := &primitives.Table{Position: [2]float64{margin, y}, Width: width, Rows: n, Cols: cols, ColWidths: widths, LineHeight: rowHeight, Grid: true, Font: &primitives.FormFont{Name: "Helvetica", Size: float64(size)}, Header: &primitives.TableHeader{Values: header, LineHeight: headerHeight, BackgroundColor: "#17324D", Font: &primitives.FormFont{Name: "Helvetica-Bold", Size: float64(size), Color: "#FFFFFF"}}, Values: rows[start : start+n]}
				pages[len(pages)-1].Tables = append(pages[len(pages)-1].Tables, table)
				start += n
				count++
				if count > 10000 {
					return nil, fmt.Errorf("flow exceeds 10000 elements")
				}
				if start < len(b.Rows) {
					if err := newPage(); err != nil {
						return nil, err
					}
				}
			}
			y -= leading / 2
		default:
			return nil, fmt.Errorf("unknown flow block kind %q", b.Kind)
		}
	}
	return pages, nil
}

func wrapFlow(ctx context.Context, text, name string, size int, width float64) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(text) > 1000000 {
		return nil, fmt.Errorf("paragraph exceeds 1 MB")
	}
	if err := supportedText(text); err != nil {
		return nil, err
	}
	var out []string
	appendLine := func(line string) error {
		if len(out) >= 10000 {
			return fmt.Errorf("flow text exceeds 10000 lines")
		}
		out = append(out, line)
		return nil
	}
	// Iterate without allocating a slice for every logical line up front.
	remaining := strings.ReplaceAll(text, "\r\n", "\n")
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		paragraph, rest, more := strings.Cut(remaining, "\n")
		line := ""
		for _, word := range strings.Fields(paragraph) {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			w, err := font.TextWidth(ctx, word, name, size)
			if err != nil {
				return nil, err
			}
			if w > width {
				return nil, fmt.Errorf("unbreakable word exceeds content width")
			}
			next := word
			if line != "" {
				next = line + " " + word
			}
			w, err = font.TextWidth(ctx, next, name, size)
			if err != nil {
				return nil, err
			}
			if w > width {
				if err := appendLine(line); err != nil {
					return nil, err
				}
				line = word
			} else {
				line = next
			}
		}
		if err := appendLine(line); err != nil {
			return nil, err
		}
		if !more {
			break
		}
		remaining = rest
	}
	return out, nil
}

type flowTable struct {
	widths                  []int
	header                  []string
	rows                    [][]string
	headerHeight, rowHeight int
}

func prepareFlowTable(ctx context.Context, b FlowBlock, size int, leading, width float64) (flowTable, error) {
	cols := len(b.Header)
	if cols == 0 || cols > 50 || len(b.Rows) == 0 || len(b.Rows) > 10000 {
		return flowTable{}, fmt.Errorf("flow table requires 1..50 headers and 1..10000 rows")
	}
	widths := append([]int(nil), b.ColumnWidths...)
	if len(widths) == 0 {
		widths = make([]int, cols)
		for j := range widths {
			widths[j] = 100 / cols
		}
		widths[cols-1] += 100 - (100/cols)*cols
	}
	if len(widths) != cols {
		return flowTable{}, fmt.Errorf("column_widths count differs from header")
	}
	sum := 0
	for _, v := range widths {
		if v <= 0 || v > 100 {
			return flowTable{}, fmt.Errorf("column widths must be positive")
		}
		sum += v
	}
	if sum != 100 {
		return flowTable{}, fmt.Errorf("column widths must total 100 percent")
	}
	wrapRow := func(row []string, name string) ([]string, int, error) {
		if len(row) != cols {
			return nil, 0, fmt.Errorf("table row has wrong column count")
		}
		values := make([]string, cols)
		height := 1
		for j, text := range row {
			if len(text) > 20000 {
				return nil, 0, fmt.Errorf("table cell exceeds text limit")
			}
			lines, err := wrapFlow(ctx, text, name, size, width*float64(widths[j])/100-10)
			if err != nil {
				return nil, 0, fmt.Errorf("table column %d: %w", j+1, err)
			}
			values[j] = strings.Join(lines, "\n")
			height = max(height, len(lines))
		}
		return values, int(math.Ceil(float64(height)*leading + 10)), nil
	}
	header, headerHeight, err := wrapRow(b.Header, "Helvetica-Bold")
	if err != nil {
		return flowTable{}, err
	}
	rows := make([][]string, len(b.Rows))
	rowHeight := 0
	for j, row := range b.Rows {
		if err := ctx.Err(); err != nil {
			return flowTable{}, err
		}
		values, height, err := wrapRow(row, "Helvetica")
		if err != nil {
			return flowTable{}, err
		}
		rows[j] = values
		rowHeight = max(rowHeight, height)
	}
	return flowTable{widths, header, rows, headerHeight, rowHeight}, nil
}
