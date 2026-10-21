package pdf

import (
	"context"
	"fmt"
	"github.com/klippa-app/go-pdfium/requests"
	"sort"
	"strings"
	"unicode"
)

type Character struct {
	Text   string  `json:"text"`
	Left   float64 `json:"left"`
	Bottom float64 `json:"bottom"`
	Right  float64 `json:"right"`
	Top    float64 `json:"top"`
}
type PageLayout struct {
	Page       int         `json:"page"`
	Characters []Character `json:"characters"`
	Table      [][]string  `json:"table,omitempty"`
}
type LayoutOptions struct {
	Pages    []int       `json:"pages,omitempty"`
	Bounds   *[4]float64 `json:"bounds,omitempty"`
	Columns  []float64   `json:"columns,omitempty"`
	Rows     []float64   `json:"rows,omitempty"`
	MaxChars int         `json:"max_chars,omitempty"`
}

// ExtractLayout exposes text geometry in PDF points with a bottom-left origin.
// Optional ascending row/column boundaries define table cells explicitly; this
// is deterministic extraction, not a claim of arbitrary table detection/OCR.
func ExtractLayout(ctx context.Context, data []byte, opt LayoutOptions) ([]PageLayout, error) {
	if opt.MaxChars == 0 {
		opt.MaxChars = 100000
	}
	if opt.MaxChars < 1 || opt.MaxChars > 1000000 {
		return nil, fmt.Errorf("max_chars must be 1..1000000")
	}
	for _, edges := range [][]float64{opt.Columns, opt.Rows} {
		for i, v := range edges {
			if !finite(v) || (i > 0 && v <= edges[i-1]) {
				return nil, fmt.Errorf("table boundaries must be finite and strictly ascending")
			}
		}
	}
	if (len(opt.Columns) > 0 || len(opt.Rows) > 0) && (len(opt.Columns) < 2 || len(opt.Rows) < 2) {
		return nil, fmt.Errorf("both rows and columns need at least two boundaries")
	}
	if len(opt.Columns)*len(opt.Rows) > 100000 {
		return nil, fmt.Errorf("table exceeds 100000 boundary intersections")
	}
	if opt.Bounds != nil {
		b := opt.Bounds
		for _, v := range b {
			if !finite(v) {
				return nil, fmt.Errorf("invalid bounds")
			}
		}
		if b[2] <= b[0] || b[3] <= b[1] {
			return nil, fmt.Errorf("bounds must be left,bottom,right,top")
		}
	}
	inst, done, err := engine(ctx)
	if err != nil {
		return nil, err
	}
	defer done()
	doc, err := inst.OpenDocument(&requests.OpenDocument{File: &data})
	if err != nil {
		return nil, err
	}
	defer inst.FPDF_CloseDocument(&requests.FPDF_CloseDocument{Document: doc.Document})
	count, err := inst.FPDF_GetPageCount(&requests.FPDF_GetPageCount{Document: doc.Document})
	if err != nil {
		return nil, err
	}
	pages := opt.Pages
	if len(pages) == 0 {
		for i := 1; i <= count.PageCount; i++ {
			pages = append(pages, i)
		}
	}
	var out []PageLayout
	total := 0
	for _, page := range pages {
		if page < 1 || page > count.PageCount {
			return nil, fmt.Errorf("page out of range")
		}
		if err = ctx.Err(); err != nil {
			return nil, err
		}
		tp, err := inst.FPDFText_LoadPage(&requests.FPDFText_LoadPage{Page: requests.Page{ByIndex: &requests.PageByIndex{Document: doc.Document, Index: page - 1}}})
		if err != nil {
			return nil, err
		}
		result, err := func() (PageLayout, error) {
			defer inst.FPDFText_ClosePage(&requests.FPDFText_ClosePage{TextPage: tp.TextPage})
			r := PageLayout{Page: page}
			n, err := inst.FPDFText_CountChars(&requests.FPDFText_CountChars{TextPage: tp.TextPage})
			if err != nil {
				return r, err
			}
			if n.Count > 1000000 {
				return r, fmt.Errorf("page exceeds character inspection limit")
			}
			for i := 0; i < n.Count; i++ {
				if i%256 == 0 {
					if err = ctx.Err(); err != nil {
						return r, err
					}
				}
				u, err := inst.FPDFText_GetUnicode(&requests.FPDFText_GetUnicode{TextPage: tp.TextPage, Index: i})
				if err != nil {
					return r, err
				}
				if u.Unicode == 0 || unicode.IsControl(rune(u.Unicode)) {
					continue
				}
				b, err := inst.FPDFText_GetCharBox(&requests.FPDFText_GetCharBox{TextPage: tp.TextPage, Index: i})
				if err != nil {
					return r, err
				}
				if opt.Bounds != nil {
					a := opt.Bounds
					if b.Right < a[0] || b.Left > a[2] || b.Top < a[1] || b.Bottom > a[3] {
						continue
					}
				}
				total++
				if total > opt.MaxChars {
					return r, fmt.Errorf("selection exceeds max_chars; narrow pages or bounds")
				}
				r.Characters = append(r.Characters, Character{string(rune(u.Unicode)), b.Left, b.Bottom, b.Right, b.Top})
			}
			if len(opt.Columns) > 0 {
				r.Table = make([][]string, len(opt.Rows)-1)
				for i := range r.Table {
					r.Table[i] = make([]string, len(opt.Columns)-1)
				}
				for _, c := range r.Characters {
					x, y := (c.Left+c.Right)/2, (c.Bottom+c.Top)/2
					col := sort.SearchFloat64s(opt.Columns, x) - 1
					row := sort.SearchFloat64s(opt.Rows, y) - 1
					if col >= 0 && col < len(opt.Columns)-1 && row >= 0 && row < len(opt.Rows)-1 {
						r.Table[len(r.Table)-1-row][col] += c.Text
					}
				}
				for i := range r.Table {
					for j := range r.Table[i] {
						r.Table[i][j] = strings.TrimSpace(r.Table[i][j])
					}
				}
			}
			return r, nil
		}()
		if err != nil {
			return nil, err
		}
		out = append(out, result)
	}
	return out, nil
}
