package convert

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/beevik/etree"
	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
)

// TableExchange converts one explicitly selected rectangular table: XLSX
// range to DOCX, or DOCX table to CSV. It never overwrites files.
func TableExchange(source, destination, sheet, cellRange string, tableIndex int) (map[string]any, error) {
	if _, err := os.Stat(destination); err == nil {
		return nil, fmt.Errorf("output exists: %s: %w", destination, os.ErrExist)
	}
	switch {
	case ext(source) == ".docx" && ext(destination) == ".csv":
		return docxTableCSV(source, destination, tableIndex)
	case ext(source) == ".xlsx" && ext(destination) == ".docx":
		return xlsxTableDocx(source, destination, sheet, cellRange)
	}
	return nil, errors.New("use XLSX to DOCX or DOCX to CSV")
}

func bodyTables(root *etree.Element) []*etree.Element {
	var out []*etree.Element
	office.Iter(root, func(e *etree.Element) bool {
		if office.Is(e, office.NSW, "tbl") {
			out = append(out, e)
		}
		return true
	})
	return out
}

func cellText(tc *etree.Element) string {
	var paras []string
	for _, p := range office.Children(tc, office.NSW, "p") {
		var b strings.Builder
		office.Iter(p, func(e *etree.Element) bool {
			switch {
			case office.Is(e, office.NSW, "del"), office.Is(e, office.NSW, "tbl"):
				return false
			case office.Is(e, office.NSW, "t"):
				b.WriteString(e.Text())
			case office.Is(e, office.NSW, "tab"):
				b.WriteString("\t")
			case office.Is(e, office.NSW, "br"), office.Is(e, office.NSW, "cr"):
				b.WriteString("\n")
			}
			return true
		})
		paras = append(paras, b.String())
	}
	return strings.Join(paras, "\n")
}

func docxTableCSV(source, destination string, index int) (map[string]any, error) {
	p, err := opc.Open(source)
	if err != nil {
		return nil, err
	}
	doc, err := p.XML("word/document.xml")
	if err != nil {
		return nil, err
	}
	tables := bodyTables(doc.Root())
	if index < 0 || index >= len(tables) {
		return nil, errors.New("unknown table index")
	}
	tbl := tables[index]
	if len(office.All(tbl, office.NSW, "gridSpan"))+len(office.All(tbl, office.NSW, "vMerge")) > 0 {
		return nil, errors.New("merged cells require an explicit flattening decision")
	}
	var buf bytes.Buffer
	rows := office.Children(tbl, office.NSW, "tr")
	for _, tr := range rows {
		var rec []string
		for _, tc := range office.Children(tr, office.NSW, "tc") {
			rec = append(rec, cellText(tc))
		}
		writeExchangeCSVRecord(&buf, rec)
	}
	if err := opc.WriteNew(destination, buf.Bytes()); err != nil {
		return nil, err
	}
	return map[string]any{"output": destination, "rows": len(rows)}, nil
}

// Match Reference csv.writer's default QUOTE_MINIMAL dialect. encoding/csv
// additionally quotes leading whitespace and the psql terminator \., and its
// CRLF mode rewrites newlines inside fields; those are different byte contracts.
func writeExchangeCSVRecord(out *bytes.Buffer, fields []string) {
	for i, text := range fields {
		if i != 0 {
			out.WriteByte(',')
		}
		if strings.ContainsAny(text, ",\"\r\n") || len(fields) == 1 && text == "" {
			out.WriteByte('"')
			out.WriteString(strings.ReplaceAll(text, "\"", "\"\""))
			out.WriteByte('"')
		} else {
			out.WriteString(text)
		}
	}
	out.WriteString("\r\n")
}

func xlsxTableDocx(source, destination, sheet, cellRange string) (map[string]any, error) {
	if sheet == "" || cellRange == "" {
		return nil, errors.New("choose an explicit sheet and cell range")
	}
	l, t, r, b, err := office.ParseRange(cellRange)
	if err != nil || (r-l+1)*(b-t+1) > 10000 {
		return nil, errors.New("choose a finite range of at most 10000 cells")
	}
	p, err := opc.Open(source)
	if err != nil {
		return nil, err
	}
	wb, err := office.OpenWorkbook(p)
	if err != nil {
		return nil, err
	}
	s := wb.Sheets[sheet]
	if s == nil {
		return nil, fmt.Errorf("unknown worksheet %q", sheet)
	}
	for _, m := range office.All(office.Child(s.Doc.Root(), office.NSS, "mergeCells"), office.NSS, "mergeCell") {
		ml, mt, mr, mb, err := office.ParseRange(m.SelectAttrValue("ref", ""))
		if err == nil && !(mr < l || ml > r || mb < t || mt > b) {
			return nil, errors.New("merged source cells need an explicit mapping")
		}
	}
	styles := wb.Styles()
	var rows [][]string
	var alignments [][]string
	cols := r - l + 1
	// reference DOCX library's default Letter section has 1.25-inch side margins.
	usable := 8640
	width := usable / cols

	for row := t; row <= b; row++ {
		var texts, aligns []string
		for col := l; col <= r; col++ {
			ref := office.ColumnName(col) + strconv.Itoa(row)
			el := s.Cells[ref]
			v, err := wb.CellValueOf(el)
			if err != nil {
				return nil, err
			}
			if v.Formula && v.Kind == "" {
				return nil, fmt.Errorf("Missing saved formula result at %s!%s; recalculate before importing", sheet, ref)
			}
			if v.Kind == "e" {
				return nil, errors.New("source contains spreadsheet errors")
			}
			st := office.CellStyle{}
			if el != nil {
				st = office.StyleOf(styles, el)
			}
			text := exchangeText(v, st, wb.Date1904)
			align := "left"
			switch {
			case st.Horizontal == "center" || st.Horizontal == "right":
				align = st.Horizontal
			case v.Kind == "n" && !st.Date:
				align = "right"
			}
			texts = append(texts, text)
			aligns = append(aligns, align)
		}
		rows = append(rows, texts)
		alignments = append(alignments, aligns)
	}
	out, err := exchangeTablePackage(rows, alignments, width)
	if err != nil {
		return nil, err
	}
	if err := out.SaveNew(destination); err != nil {
		return nil, err
	}
	return map[string]any{"output": destination, "source_sheet": sheet, "source_range": cellRange, "rows": b - t + 1,
		"limits": "Values and basic alignment only; spreadsheet formulas, custom number formats and charts are not transferred."}, nil
}

func exchangeText(v office.CellValue, st office.CellStyle, date1904 bool) string {
	switch v.Kind {
	case "":
		return ""
	case "n":
		if st.Date {
			if f, ok := v.Number(); ok {
				return office.ISODate(office.SerialToTime(f, date1904))
			}
		}
	}
	return office.PyStr(v)
}
