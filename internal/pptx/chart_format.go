package pptx

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/beevik/etree"
	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
)

// ChartFormat changes only selected native formatting properties, retaining data.
type ChartFormat struct {
	Axes   []ChartAxisFormat   `json:"axes,omitempty"`
	Legend *ChartLegendFormat  `json:"legend,omitempty"`
	Series []ChartSeriesFormat `json:"series,omitempty"`
}
type ChartAxisFormat struct {
	Position          string          `json:"position,omitempty"`
	TickLabelPosition string          `json:"tick_label_position,omitempty"`
	ID                string          `json:"id"` // c:axId from chart XML
	Visible           *bool           `json:"visible,omitempty"`
	Minimum           *float64        `json:"minimum,omitempty"`
	Maximum           *float64        `json:"maximum,omitempty"`
	MinorUnit         *float64        `json:"minor_unit,omitempty"`
	TextStyle         *ChartTextStyle `json:"text_style,omitempty"`
	MajorGridline     *TextOutline    `json:"major_gridline,omitempty"`
	MinorGridline     *TextOutline    `json:"minor_gridline,omitempty"`
	MajorUnit         *float64        `json:"major_unit,omitempty"`
	NumberFormat      *string         `json:"number_format,omitempty"`
	Title             *string         `json:"title,omitempty"`
}
type ChartLegendFormat struct {
	Visible   *bool           `json:"visible,omitempty"`
	Position  string          `json:"position,omitempty"` // b,t,l,r,tr
	TextStyle *ChartTextStyle `json:"text_style,omitempty"`
	Fill      *DrawingStyle   `json:"fill,omitempty"`
	Outline   *TextOutline    `json:"outline,omitempty"`
	Overlay   *bool           `json:"overlay,omitempty"`
}
type ChartSeriesFormat struct {
	Marker         *ChartMarkerFormat   `json:"marker,omitempty"`
	LabelOverrides []ChartLabelOverride `json:"label_overrides,omitempty"`
	Index          int                  `json:"index"` // zero-based order of c:ser in chart XML
	Outline        *TextOutline         `json:"outline,omitempty"`
	Fill           *DrawingStyle        `json:"fill,omitempty"`
	Points         []ChartPointFormat   `json:"points,omitempty"`
	Trendlines     *[]ChartTrendline    `json:"trendlines,omitempty"`
	ErrorBars      *ChartErrorBars      `json:"error_bars,omitempty"`
	Labels         *ChartLabelsFormat   `json:"labels,omitempty"`
}
type ChartLabelsFormat struct {
	TextStyle    *ChartTextStyle `json:"text_style,omitempty"`
	ShowValue    *bool           `json:"show_value,omitempty"`
	ShowCategory *bool           `json:"show_category,omitempty"`
	ShowSeries   *bool           `json:"show_series,omitempty"`
	ShowPercent  *bool           `json:"show_percent,omitempty"`
	Position     string          `json:"position,omitempty"`
	NumberFormat *string         `json:"number_format,omitempty"`
}

// Insert schema-ordered optional children without disturbing unrelated nodes.
func chartEnsure(parent *etree.Element, tag, order string) *etree.Element {
	if e := office.Child(parent, cns, tag); e != nil {
		return e
	}
	e := etree.NewElement("c:" + tag)
	e.CreateAttr("xmlns:c", cns)
	ranks := strings.Fields(order)
	rank := func(tag string) int {
		for i, v := range ranks {
			if tag == v {
				return i
			}
		}
		return len(ranks)
	}
	for _, other := range parent.ChildElements() {
		if rank(other.Tag) > rank(tag) {
			parent.InsertChildAt(other.Index(), e)
			return e
		}
	}
	parent.AddChild(e)
	return e
}
func chartBool(parent *etree.Element, tag string, value bool, order string) {
	v := "0"
	if value {
		v = "1"
	}
	chartEnsure(parent, tag, order).CreateAttr("val", v)
}

const axisOrder = "axId scaling delete axPos majorGridlines minorGridlines title numFmt majorTickMark minorTickMark tickLblPos spPr txPr crossAx crosses crossesAt auto lblAlgn lblOffset tickLblSkip tickMarkSkip noMultiLvlLbl crossBetween majorUnit minorUnit dispUnits extLst"

func formatNativeChart(p *opc.Package, owner string, shape *etree.Element, f *ChartFormat) error {
	if f == nil {
		return fmt.Errorf("chart_format required")
	}
	refs := office.All(shape, cns, "chart")
	if len(refs) != 1 {
		return fmt.Errorf("select one standard chart; ChartEx formatting is not supported")
	}
	rid := office.AttrV(refs[0], office.NSR, "id")
	rels, err := p.Rels(owner)
	if err != nil {
		return err
	}
	part := ""
	for _, r := range rels {
		if r.ID == rid && r.Type == rns+"/chart" && r.TargetMode != "External" {
			part = opc.Resolve(owner, r.Target)
		}
	}
	if part == "" {
		return fmt.Errorf("chart relationship missing")
	}
	d, err := p.XML(part)
	if err != nil {
		return err
	}
	chart := office.Child(d.Root(), cns, "chart")
	plot := office.Child(chart, cns, "plotArea")
	if plot == nil {
		return fmt.Errorf("missing chart plot area")
	}
	for _, a := range f.Axes {
		var axis *etree.Element
		for _, v := range plot.ChildElements() {
			switch v.Tag {
			case "catAx", "valAx", "dateAx", "serAx":
			default:
				continue
			}
			id := office.Child(v, cns, "axId")
			if id != nil && id.SelectAttrValue("val", "") == a.ID {
				if axis != nil {
					return fmt.Errorf("ambiguous axis id")
				}
				axis = v
			}
		}
		if axis == nil || a.ID == "" {
			return fmt.Errorf("unknown axis id %q", a.ID)
		}
		if (a.Minimum != nil || a.Maximum != nil || a.MajorUnit != nil || a.MinorUnit != nil) && axis.Tag != "valAx" && axis.Tag != "dateAx" {
			return fmt.Errorf("numeric bounds require a value or date axis")
		}
		scaling := office.Child(axis, cns, "scaling")
		bound := func(tag string, change *float64) (*float64, error) {
			if change != nil {
				if !finite(*change) {
					return nil, fmt.Errorf("non-finite axis bound")
				}
				return change, nil
			}
			if v := office.Child(scaling, cns, tag); v != nil {
				n, e := strconv.ParseFloat(v.SelectAttrValue("val", ""), 64)
				if e != nil {
					return nil, e
				}
				return &n, nil
			}
			return nil, nil
		}
		lo, e := bound("min", a.Minimum)
		if e != nil {
			return e
		}
		hi, e := bound("max", a.Maximum)
		if e != nil {
			return e
		}
		if lo != nil && hi != nil && *lo >= *hi {
			return fmt.Errorf("axis minimum must be less than maximum")
		}
		if office.Child(scaling, cns, "logBase") != nil && (lo != nil && *lo <= 0 || hi != nil && *hi <= 0) {
			return fmt.Errorf("logarithmic bounds must be positive")
		}
		for _, v := range []struct {
			tag   string
			value *float64
		}{{"min", a.Minimum}, {"max", a.Maximum}} {
			if v.value != nil {
				scaling = chartEnsure(axis, "scaling", axisOrder)
				chartEnsure(scaling, v.tag, "logBase orientation max min extLst").CreateAttr("val", strconv.FormatFloat(*v.value, 'g', -1, 64))
			}
		}
		if a.MajorUnit != nil {
			if !finite(*a.MajorUnit) || *a.MajorUnit <= 0 {
				return fmt.Errorf("major_unit must be positive")
			}
			chartEnsure(axis, "majorUnit", axisOrder).CreateAttr("val", strconv.FormatFloat(*a.MajorUnit, 'g', -1, 64))
		}
		if err := formatAxisExtras(axis, a); err != nil {
			return err
		}
		if a.Visible != nil {
			chartBool(axis, "delete", !*a.Visible, axisOrder)
		}
		if a.NumberFormat != nil {
			n := chartEnsure(axis, "numFmt", axisOrder)
			n.CreateAttr("formatCode", *a.NumberFormat)
			n.CreateAttr("sourceLinked", "0")
		}
		if a.Title != nil {
			old := office.Child(axis, cns, "title")
			if *a.Title == "" {
				if old != nil {
					axis.RemoveChild(old)
				}
			} else {
				title := chartEnsure(axis, "title", axisOrder)
				tx := chartEnsure(title, "tx", "tx layout overlay spPr txPr extLst")
				for _, c := range tx.ChildElements() {
					tx.RemoveChild(c)
				}
				rich := tx.CreateElement("c:rich")
				rich.CreateAttr("xmlns:c", cns)
				rich.CreateAttr("xmlns:a", ans)
				rich.CreateElement("a:bodyPr")
				rich.CreateElement("a:lstStyle")
				rich.CreateElement("a:p").CreateElement("a:r").CreateElement("a:t").SetText(*a.Title)
			}
		}
	}
	if l := f.Legend; l != nil {
		if l.Visible != nil && !*l.Visible {
			if l.Position != "" || l.Overlay != nil || l.TextStyle != nil || l.Fill != nil || l.Outline != nil {
				return fmt.Errorf("hidden legend cannot specify position or overlay")
			}
			if old := office.Child(chart, cns, "legend"); old != nil {
				chart.RemoveChild(old)
			}
		} else {
			legend := chartEnsure(chart, "legend", "title autoTitleDeleted pivotFmts view3D floor sideWall backWall plotArea legend plotVisOnly dispBlanksAs showDLblsOverMax extLst")
			if l.Position != "" {
				switch l.Position {
				case "b", "t", "l", "r", "tr":
				default:
					return fmt.Errorf("invalid legend position")
				}
				chartEnsure(legend, "legendPos", "legendPos legendEntry layout overlay spPr txPr extLst").CreateAttr("val", l.Position)
			}
			if err := chartAppearance(legend, l.Fill, l.Outline, l.TextStyle, "legendPos legendEntry layout overlay spPr txPr extLst"); err != nil {
				return err
			}
			if l.Overlay != nil {
				chartBool(legend, "overlay", *l.Overlay, "legendPos legendEntry layout overlay spPr txPr extLst")
			}
		}
	}
	series := office.All(plot, cns, "ser")
	for _, s := range f.Series {
		if s.Index < 0 || s.Index >= len(series) {
			return fmt.Errorf("series index out of bounds")
		}
		ser := series[s.Index]
		// Surface and stock series have different capabilities; do not invent labels.
		kind := ser.Parent().Tag
		if s.Fill != nil {
			fill, e := paintFill(s.Fill)
			if e != nil {
				return e
			}
			sp := chartEnsure(ser, "spPr", "idx order tx spPr invertIfNegative pictureOptions marker explosion dPt dLbls trendline errBars cat val xVal yVal bubbleSize bubble3D shape smooth extLst")
			for _, c := range sp.ChildElements() {
				if strings.HasSuffix(c.Tag, "Fill") {
					sp.RemoveChild(c)
				}
			}
			sp.InsertChildAt(0, fill)
		}
		if s.Outline != nil {
			pr := etree.NewElement("a:rPr")
			pr.CreateAttr("xmlns:a", ans)
			if err := paintTextProperties(pr, &TextPaint{Outline: s.Outline}); err != nil {
				return err
			}
			sp := chartEnsure(ser, "spPr", "idx order tx spPr invertIfNegative pictureOptions marker explosion dPt dLbls trendline errBars cat val xVal yVal bubbleSize bubble3D shape smooth extLst")
			if old := office.Child(sp, ans, "ln"); old != nil {
				sp.RemoveChild(old)
			}
			line := office.Child(pr, ans, "ln").Copy()
			index := 0
			for _, c := range sp.ChildElements() {
				if strings.HasSuffix(c.Tag, "Fill") {
					index = c.Index() + 1
				}
			}
			sp.InsertChildAt(index, line)
		}
		if err := formatLabelOverrides(ser, s.LabelOverrides); err != nil {
			return err
		}
		if err := formatSeriesExtras(ser, s); err != nil {
			return err
		}
		if l := s.Labels; l != nil {
			switch kind {
			case "barChart", "bar3DChart", "lineChart", "line3DChart", "pieChart", "pie3DChart", "doughnutChart", "ofPieChart", "areaChart", "area3DChart", "scatterChart", "bubbleChart", "radarChart":
			default:
				return fmt.Errorf("data labels not supported for %s", kind)
			}
			if l.ShowPercent != nil && *l.ShowPercent && kind != "pieChart" && kind != "pie3DChart" && kind != "doughnutChart" && kind != "ofPieChart" {
				return fmt.Errorf("percent labels require pie or doughnut")
			}
			labels := chartEnsure(ser, "dLbls", "idx order tx spPr invertIfNegative pictureOptions marker explosion dPt dLbls trendline errBars cat val xVal yVal bubbleSize bubble3D shape smooth extLst")
			order := "dLbl delete numFmt spPr txPr dLblPos showLegendKey showVal showCatName showSerName showPercent showBubbleSize separator showLeaderLines leaderLines extLst"
			if err := chartAppearance(labels, nil, nil, l.TextStyle, order); err != nil {
				return err
			}
			if l.Position != "" {
				switch l.Position {
				case "bestFit", "b", "ctr", "inBase", "inEnd", "l", "outEnd", "r", "t":
				default:
					return fmt.Errorf("invalid label position")
				}
				chartEnsure(labels, "dLblPos", order).CreateAttr("val", l.Position)
			}
			if l.NumberFormat != nil {
				n := chartEnsure(labels, "numFmt", order)
				n.CreateAttr("formatCode", *l.NumberFormat)
				n.CreateAttr("sourceLinked", "0")
			}
			for _, v := range []struct {
				tag   string
				value *bool
			}{{"showVal", l.ShowValue}, {"showCatName", l.ShowCategory}, {"showSerName", l.ShowSeries}, {"showPercent", l.ShowPercent}} {
				if v.value != nil {
					chartBool(labels, v.tag, *v.value, order)
				}
			}
		}
	}
	return p.SetXML(part, d)
}
