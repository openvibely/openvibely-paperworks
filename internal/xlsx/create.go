// Package xlsx authors new editable workbooks. Existing packages must use the
// narrow office edit operations to avoid rewriting unsupported native parts.
package xlsx

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"github.com/openvibely/openvibely-paperworks/internal/xmlresource"
	"github.com/xuri/excelize/v2"
)

type Spec struct {
	ThemeFile string                    `json:"theme_file,omitempty"`
	Sheets    []Sheet                   `json:"sheets"`
	Styles    map[string]excelize.Style `json:"styles"`
	Names     []excelize.DefinedName    `json:"names"`
}
type Cell struct {
	Value   any    `json:"value"`
	Formula string `json:"formula"`
	Date    string `json:"date"`
	Style   string `json:"style"`
}
type Format struct {
	Range string `json:"range"`
	Style string `json:"style"`
}
type Chart struct {
	Options    *excelize.Chart        `json:"options,omitempty"`
	VaryColors *bool                  `json:"vary_colors"`
	Type       string                 `json:"type"`
	Cell       string                 `json:"cell"`
	Title      string                 `json:"title"`
	Series     []excelize.ChartSeries `json:"series"`
	XAxis      excelize.ChartAxis     `json:"x_axis"`
	YAxis      excelize.ChartAxis     `json:"y_axis"`
	Width      uint                   `json:"width"`
	Height     uint                   `json:"height"`
}
type Conditional struct {
	Options  *excelize.ConditionalFormatOptions `json:"options,omitempty"`
	Range    string                             `json:"range"`
	Style    string                             `json:"style"`
	Type     string                             `json:"type"`
	Criteria string                             `json:"criteria"`
	Value    string                             `json:"value"`
}
type Picture struct {
	Cell    string                   `json:"cell"`
	Path    string                   `json:"path"`
	Options *excelize.GraphicOptions `json:"options"`
}
type Sheet struct {
	ColumnCharts  []ColumnChart                      `json:"column_charts,omitempty"`
	SheetProps    *excelize.SheetPropsOptions        `json:"sheet_props,omitempty"`
	PageMargins   *excelize.PageLayoutMarginsOptions `json:"page_margins,omitempty"`
	Name          string                             `json:"name"`
	DataTables    []DataTable                        `json:"data_tables"`
	Rows          [][]any                            `json:"rows"`
	Cells         map[string]Cell                    `json:"cells"`
	Formats       []Format                           `json:"formats"`
	Columns       map[string]float64                 `json:"columns"`
	RowHeights    map[int]float64                    `json:"row_heights"`
	Freeze        string                             `json:"freeze"`
	Merges        []string                           `json:"merges"`
	Tables        []excelize.Table                   `json:"tables"`
	Validations   []excelize.DataValidation          `json:"validations"`
	Conditional   []Conditional                      `json:"conditional"`
	Charts        []Chart                            `json:"charts"`
	Pictures      []Picture                          `json:"pictures"`
	ShowGridLines bool                               `json:"show_grid_lines"`
	Hidden        bool                               `json:"hidden"`
	PrintArea     string                             `json:"print_area"`
	PageLayout    *excelize.PageLayoutOptions        `json:"page_layout"`
}

func bounds(s string) (string, string, error) {
	a, b, ok := strings.Cut(s, ":")
	if !ok {
		b = a
	}
	x, y, e := excelize.CellNameToCoordinates(a)
	if e != nil {
		return "", "", e
	}
	xx, yy, e := excelize.CellNameToCoordinates(b)
	if e != nil {
		return "", "", e
	}
	if xx < x || yy < y || int64(xx-x+1)*int64(yy-y+1) > 1000000 {
		return "", "", fmt.Errorf("invalid or oversized range %q", s)
	}
	return a, b, nil
}
func literal(v any) error {
	switch v.(type) {
	case nil, string, bool, float64, int, int64:
		return nil
	default:
		return fmt.Errorf("cell values must be JSON strings, numbers, booleans or null")
	}
}

func Create(ctx context.Context, spec Spec, base, output string) error {
	return createWithClock(ctx, spec, base, output, time.Now)
}

func createWithClock(ctx context.Context, spec Spec, base, output string, clock func() time.Time) error {
	created := clock()

	if len(spec.Sheets) == 0 || len(spec.Sheets) > 256 {
		return fmt.Errorf("sheets must contain 1..256 entries")
	}
	f := excelize.NewFile()
	defer f.Close()
	styles := map[string]int{}
	for name, style := range spec.Styles {
		id, e := f.NewStyle(&style)
		if e != nil {
			return fmt.Errorf("style %s: %w", name, e)
		}
		styles[name] = id
	}
	// Create all sheets before adding cross-sheet formulas and chart references.
	seen := map[string]bool{}
	for i, s := range spec.Sheets {
		key := strings.ToLower(s.Name)
		if s.Name == "" || seen[key] {
			return fmt.Errorf("empty or duplicate sheet name %q", s.Name)
		}
		seen[key] = true
		var e error
		if i == 0 {
			e = f.SetSheetName("Sheet1", s.Name)
		} else {
			_, e = f.NewSheet(s.Name)
		}
		if e != nil {
			return e
		}
	}
	count := 0
	visible := false
	for _, s := range spec.Sheets {
		if s.SheetProps != nil {
			if e := f.SetSheetProps(s.Name, s.SheetProps); e != nil {
				return e
			}
		}
		if s.PageMargins != nil {
			if e := f.SetPageMargins(s.Name, s.PageMargins); e != nil {
				return e
			}
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if !s.Hidden {
			visible = true
		}
		if e := f.SetSheetView(s.Name, 0, &excelize.ViewOptions{ShowGridLines: &s.ShowGridLines}); e != nil {
			return e
		}
		for r, row := range s.Rows {
			for c, v := range row {
				count++
				if count > 1000000 {
					return fmt.Errorf("workbook exceeds one million specified cells")
				}
				if c%1024 == 0 {
					if e := ctx.Err(); e != nil {
						return e
					}
				}
				address, e := excelize.CoordinatesToCellName(c+1, r+1)
				if e != nil {
					return e
				}
				// Strings, including those beginning '=', are always literal here.
				if e := literal(v); e != nil {
					return fmt.Errorf("%s!%s: %w", s.Name, address, e)
				}
				if e = f.SetCellValue(s.Name, address, v); e != nil {
					return e
				}
			}
		}
		for address, c := range s.Cells {
			if e := literal(c.Value); e != nil {
				return fmt.Errorf("%s!%s: %w", s.Name, address, e)
			}
			count++
			if count > 1000000 {
				return fmt.Errorf("workbook exceeds one million specified cells")
			}
			if c.Formula != "" && (c.Date != "" || c.Value != nil) || c.Date != "" && c.Value != nil {
				return fmt.Errorf("%s!%s: choose value, formula, or date", s.Name, address)
			}
			var e error
			switch {
			case c.Formula != "":
				e = f.SetCellFormula(s.Name, address, strings.TrimPrefix(c.Formula, "="))
			case c.Date != "":
				var d time.Time
				d, e = time.Parse(time.RFC3339, c.Date)
				if e == nil {
					e = f.SetCellValue(s.Name, address, d)
				}
			default:
				e = f.SetCellValue(s.Name, address, c.Value)
			}
			if e != nil {
				return fmt.Errorf("%s!%s: %w", s.Name, address, e)
			}
			if c.Style != "" {
				id, ok := styles[c.Style]
				if !ok {
					return fmt.Errorf("unknown style %q", c.Style)
				}
				if e = f.SetCellStyle(s.Name, address, address, id); e != nil {
					return e
				}
			}
		}
		for _, v := range s.Formats {
			a, b, e := bounds(v.Range)
			if e != nil {
				return e
			}
			id, ok := styles[v.Style]
			if !ok {
				return fmt.Errorf("unknown style %q", v.Style)
			}
			if e = f.SetCellStyle(s.Name, a, b, id); e != nil {
				return e
			}
		}
		for col, width := range s.Columns {
			a, b, ok := strings.Cut(col, ":")
			if !ok {
				b = a
			}
			if e := f.SetColWidth(s.Name, a, b, width); e != nil {
				return e
			}
		}
		for row, height := range s.RowHeights {
			if e := f.SetRowHeight(s.Name, row, height); e != nil {
				return e
			}
		}
		for _, v := range s.Merges {
			a, b, e := bounds(v)
			if e != nil {
				return e
			}
			if e = f.MergeCell(s.Name, a, b); e != nil {
				return e
			}
		}
		if s.Freeze != "" {
			x, y, e := excelize.CellNameToCoordinates(s.Freeze)
			if e != nil {
				return e
			}
			pane := "bottomRight"
			if x == 1 {
				pane = "bottomLeft"
			}
			if y == 1 {
				pane = "topRight"
			}
			if e = f.SetPanes(s.Name, &excelize.Panes{Freeze: true, XSplit: x - 1, YSplit: y - 1, TopLeftCell: s.Freeze, ActivePane: pane}); e != nil {
				return e
			}
		}
		for _, v := range s.Tables {
			if e := f.AddTable(s.Name, &v); e != nil {
				return e
			}
		}
		for _, v := range s.Validations {
			if e := f.AddDataValidation(s.Name, &v); e != nil {
				return e
			}
		}
		for _, v := range s.Conditional {
			style, ok := spec.Styles[v.Style]
			if !ok {
				return fmt.Errorf("unknown conditional style %q", v.Style)
			}
			id, e := f.NewConditionalStyle(&style)
			if e != nil {
				return e
			}
			option := excelize.ConditionalFormatOptions{Type: v.Type, Criteria: v.Criteria, Value: v.Value, Format: &id}
			if v.Options != nil {
				option = *v.Options
				option.Format = &id
			}
			if e = f.SetConditionalFormat(s.Name, v.Range, []excelize.ConditionalFormatOptions{option}); e != nil {
				return e
			}
		}
		types := map[string]excelize.ChartType{"bar": excelize.Bar, "column": excelize.Col, "line": excelize.Line, "scatter": excelize.Scatter, "pie": excelize.Pie, "doughnut": excelize.Doughnut, "area": excelize.Area, "radar": excelize.Radar, "bubble": excelize.Bubble, "stock-hlc": excelize.StockHighLowClose, "stock-ohlc": excelize.StockOpenHighLowClose}
		for _, v := range s.Charts {
			typ, ok := types[v.Type]
			if !ok {
				return fmt.Errorf("unsupported chart type %q", v.Type)
			}
			chart := &excelize.Chart{Type: typ, VaryColors: v.VaryColors, Series: v.Series, Title: excelize.ChartTitle{Paragraph: []excelize.RichTextRun{{Text: v.Title}}}, XAxis: v.XAxis, YAxis: v.YAxis}
			if v.Options != nil {
				copy := *v.Options
				chart = &copy
				chart.Type = typ
				chart.Series = v.Series
			}
			if v.Width > 0 {
				chart.Dimension.Width = v.Width
			}
			if v.Height > 0 {
				chart.Dimension.Height = v.Height
			}
			if e := f.AddChart(s.Name, v.Cell, chart); e != nil {
				return e
			}
		}
		for _, v := range s.Pictures {
			p := v.Path
			if !filepath.IsAbs(p) {
				p = filepath.Join(base, p)
			}
			if e := f.AddPicture(s.Name, v.Cell, p, v.Options); e != nil {
				return e
			}
		}
		if s.PrintArea != "" {
			if _, _, e := bounds(s.PrintArea); e != nil {
				return e
			}
			ref := "'" + strings.ReplaceAll(s.Name, "'", "''") + "'!" + s.PrintArea
			if e := f.SetDefinedName(&excelize.DefinedName{Name: "_xlnm.Print_Area", Scope: s.Name, RefersTo: ref}); e != nil {
				return e
			}
		}
		if s.PageLayout != nil {
			if e := f.SetPageLayout(s.Name, s.PageLayout); e != nil {
				return e
			}
			fit := true
			if e := f.SetSheetProps(s.Name, &excelize.SheetPropsOptions{FitToPage: &fit}); e != nil {
				return e
			}
		}
	}
	if !visible {
		return fmt.Errorf("at least one sheet must be visible")
	}
	for i, s := range spec.Sheets {
		if !s.Hidden {
			f.SetActiveSheet(i)
			break
		}
	}
	for _, s := range spec.Sheets {
		if s.Hidden {
			if e := f.SetSheetVisible(s.Name, false); e != nil {
				return e
			}
		}
	}
	for _, v := range spec.Names {
		if e := f.SetDefinedName(&v); e != nil {
			return e
		}
	}
	if e := ctx.Err(); e != nil {
		return e
	}
	if e := prepareDataTables(ctx, f, spec); e != nil {
		return e
	}
	data, e := f.WriteToBuffer()
	if e != nil {
		return e
	}
	if hasDataTables(spec) || hasColumnCharts(spec) || spec.ThemeFile != "" {
		p, e := opc.Read(data.Bytes())
		if e != nil {
			return e
		}
		if spec.ThemeFile != "" {
			raw, _, err := xmlresource.Read(base, spec.ThemeFile, "http://schemas.openxmlformats.org/drawingml/2006/main", "theme")
			if err != nil {
				var rootErr *xmlresource.RootError
				if errors.As(err, &rootErr) {
					return fmt.Errorf("theme_file must contain a DrawingML theme")
				}
				return err
			}
			p.Set("xl/theme/theme1.xml", raw)
		}
		if e = patchColumnCharts(p, spec); e != nil {
			return e
		}
		if e = patchDataTables(p, spec); e != nil {
			return e
		}
		if e = stabilizeStyleIDs(p); e != nil {
			return e
		}
		if e = serializeStyles(p, spec); e != nil {
			return e
		}
		if e = serializeWorksheets(p); e != nil {
			return e
		}
		if e = serializeWorkbook(p, spec); e != nil {
			return e
		}
		if e = serializePackageGraph(p); e != nil {
			return e
		}
		raw, err := spreadsheetArchive(p, created, clock)
		if err != nil {
			return err
		}
		return opc.WriteNew(output, raw)
	}
	return opc.WriteNew(output, data.Bytes())
}
