package pptx

import (
	"embed"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"github.com/xuri/excelize/v2"
)

//go:embed all:assets/chart-workbook
var chartWorkbookAssets embed.FS

// The bounded category writer follows reference presentation library's CategoryWorkbookWriter:
// categories first, then column-major series, General format, no A1 label.
// Formula, URL, control and empty strings retain the existing authoring path.
func categoryWorkbookSupported(c Chart) bool {
	if c.Type == "scatter" {
		return false
	}
	valid := func(s string) bool {
		if s == "" || len([]rune(s)) > 32767 || strings.HasPrefix(s, "=") || strings.Contains(s, ":") || strings.Contains(s, "_x") {
			return false
		}
		for _, r := range s {
			if unicode.IsControl(r) {
				return false
			}
		}
		return true
	}
	for _, s := range c.Categories {
		if !valid(s) {
			return false
		}
	}
	for _, s := range c.Series {
		if !valid(s.Name) {
			return false
		}
	}
	return len(c.Categories) > 0 && len(c.Series) > 0
}

func categoryWorkbook(c Chart, now time.Time) ([]byte, error) {
	const declaration = "<?xml version=\"1.0\" encoding=\"UTF-8\" standalone=\"yes\"?>\n"
	const main = "http://schemas.openxmlformats.org/spreadsheetml/2006/main"
	quote := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace
	ids := map[string]int{}
	var values []string
	count := 0
	intern := func(s string) int {
		count++
		if id, ok := ids[s]; ok {
			return id
		}
		id := len(values)
		ids[s] = id
		values = append(values, s)
		return id
	}
	cats := make([]int, len(c.Categories))
	names := make([]int, len(c.Series))
	for i, s := range c.Categories {
		cats[i] = intern(s)
	}
	for i, s := range c.Series {
		names[i] = intern(s.Name)
	}
	last, _ := excelize.ColumnNumberToName(len(c.Series) + 1)
	var sheet strings.Builder
	fmt.Fprintf(&sheet, `%s<worksheet xmlns="%s" xmlns:r="%s"><dimension ref="A1:%s%d"/><sheetViews><sheetView tabSelected="1" workbookViewId="0"/></sheetViews><sheetFormatPr defaultRowHeight="15"/><cols><col min="1" max="1" width="10.7109375" customWidth="1"/></cols><sheetData>`, declaration, main, rns, last, len(c.Categories)+1)
	for row := 0; row <= len(c.Categories); row++ {
		fmt.Fprintf(&sheet, `<row r="%d" spans="1:%d">`, row+1, len(c.Series)+1)
		if row > 0 {
			fmt.Fprintf(&sheet, `<c r="A%d" s="1" t="s"><v>%d</v></c>`, row+1, cats[row-1])
		}
		for col, s := range c.Series {
			letter, _ := excelize.ColumnNumberToName(col + 2)
			if row == 0 {
				fmt.Fprintf(&sheet, `<c r="%s1" t="s"><v>%d</v></c>`, letter, names[col])
			} else {
				fmt.Fprintf(&sheet, `<c r="%s%d" s="1"><v>%.16G</v></c>`, letter, row+1, s.Values[row-1])
			}
		}
		sheet.WriteString(`</row>`)
	}
	sheet.WriteString(`</sheetData><pageMargins left="0.7" right="0.7" top="0.75" bottom="0.75" header="0.3" footer="0.3"/></worksheet>`)
	var ss strings.Builder
	fmt.Fprintf(&ss, `%s<sst xmlns="%s" count="%d" uniqueCount="%d">`, declaration, main, count, len(values))
	for _, s := range values {
		ss.WriteString(`<si><t`)
		if strings.TrimSpace(s) != s {
			ss.WriteString(` xml:space="preserve"`)
		}
		ss.WriteString(`>` + quote(s) + `</t></si>`)
	}
	ss.WriteString(`</sst>`)
	p := &opc.Package{}
	order := []string{"[Content_Types].xml", "_rels/.rels", "xl/_rels/workbook.xml.rels", "xl/worksheets/sheet1.xml", "xl/workbook.xml", "xl/sharedStrings.xml", "xl/styles.xml", "xl/theme/theme1.xml", "docProps/core.xml", "docProps/app.xml"}
	for _, name := range order {
		var raw []byte
		var err error
		switch name {
		case "xl/worksheets/sheet1.xml":
			raw = []byte(sheet.String())
		case "xl/sharedStrings.xml":
			raw = []byte(ss.String())
		case "docProps/core.xml":
			raw, err = chartWorkbookAssets.ReadFile("assets/chart-workbook/core.xml")
			raw = []byte(strings.ReplaceAll(string(raw), "CLOCK", now.UTC().Format("2006-01-02T15:04:05Z")))
		default:
			raw, err = chartWorkbookAssets.ReadFile("assets/chart-workbook/" + name)
		}
		if err != nil {
			return nil, err
		}
		p.Parts = append(p.Parts, &opc.Part{Name: name, Data: raw})
	}
	// In-memory XlsxWriter fixes ZIP timestamps, independently of core properties.
	return p.DocumentBytes(func() time.Time { return time.Date(1980, 1, 1, 0, 0, 0, 0, time.Local) })
}
