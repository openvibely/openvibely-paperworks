package wordedit

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/beevik/etree"
)

func headerFooterContent(s map[string]any) ([]*etree.Element, error) {
	raw, hasRows := s["rows"]
	if !hasRows {
		text, ok := s["text"].(string)
		if !ok {
			return nil, fmt.Errorf("text or rows required")
		}
		p := W("p")
		r := W("r")
		r.AddChild(WT("t", text))
		p.AddChild(r)
		return []*etree.Element{p}, nil
	}
	if _, ok := s["text"]; ok {
		return nil, fmt.Errorf("text and rows are mutually exclusive")
	}
	var rows [][]string
	data, err := json.Marshal(raw)
	if err != nil {
		return nil, err
	}
	if err = json.Unmarshal(data, &rows); err != nil || len(rows) == 0 || len(rows) > 1000 {
		return nil, fmt.Errorf("rows must be a nonempty string matrix")
	}
	cols := len(rows[0])
	if cols == 0 || cols > 63 || len(rows)*cols > 10000 {
		return nil, fmt.Errorf("table exceeds row/column limits")
	}
	for _, r := range rows {
		if len(r) != cols {
			return nil, fmt.Errorf("table rows must be rectangular")
		}
	}
	var widths []int
	data, err = json.Marshal(s["widths_twips"])
	if err != nil {
		return nil, err
	}
	if err = json.Unmarshal(data, &widths); err != nil || len(widths) != cols {
		return nil, fmt.Errorf("widths_twips needs one positive integer per column")
	}
	total := 0
	for _, w := range widths {
		if w <= 0 || w > 31680 {
			return nil, fmt.Errorf("invalid table width")
		}
		total += w
	}
	if total > 31680 {
		return nil, fmt.Errorf("table too wide")
	}
	table := W("tbl")
	pr := W("tblPr")
	table.AddChild(pr)
	tw := W("tblW")
	setW(tw, "type", "dxa")
	setW(tw, "w", strconv.Itoa(total))
	pr.AddChild(tw)
	layout := W("tblLayout")
	setW(layout, "type", "fixed")
	pr.AddChild(layout)
	grid := W("tblGrid")
	table.AddChild(grid)
	for _, w := range widths {
		c := W("gridCol")
		setW(c, "w", strconv.Itoa(w))
		grid.AddChild(c)
	}
	for _, row := range rows {
		tr := W("tr")
		table.AddChild(tr)
		for i, text := range row {
			tc := W("tc")
			tr.AddChild(tc)
			pr := W("tcPr")
			tc.AddChild(pr)
			cw := W("tcW")
			setW(cw, "type", "dxa")
			setW(cw, "w", strconv.Itoa(widths[i]))
			pr.AddChild(cw)
			p := W("p")
			tc.AddChild(p)
			r := W("r")
			p.AddChild(r)
			// Keep line breaks and tabs as Word controls rather than literal XML text.
			var buf []rune
			flush := func() {
				if len(buf) > 0 {
					t := WT("t", string(buf))
					t.CreateAttr("xml:space", "preserve")
					r.AddChild(t)
					buf = nil
				}
			}
			for _, ch := range text {
				switch ch {
				case '\n':
					flush()
					r.AddChild(W("br"))
				case '\t':
					flush()
					r.AddChild(W("tab"))
				default:
					buf = append(buf, ch)
				}
			}
			flush()
		}
	}
	return []*etree.Element{table, W("p")}, nil
}
