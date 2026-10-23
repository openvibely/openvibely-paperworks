package pptx

import (
	"fmt"
	"github.com/beevik/etree"
	"github.com/openvibely/openvibely-paperworks/internal/office"
	"strconv"
)

// ChartTextStyle uses points, matching the rest of the presentation editor.
type ChartTextStyle struct {
	Underline string   `json:"underline,omitempty"`
	Alignment string   `json:"alignment,omitempty"`
	Font      string   `json:"font,omitempty"`
	FontSize  *float64 `json:"font_size,omitempty"`
	Color     string   `json:"color,omitempty"`
	Bold      *bool    `json:"bold,omitempty"`
	Italic    *bool    `json:"italic,omitempty"`
}
type ChartPointFormat struct {
	Index   int           `json:"index"`
	Fill    *DrawingStyle `json:"fill,omitempty"`
	Outline *TextOutline  `json:"outline,omitempty"`
}
type ChartTrendlineLabel struct {
	Text      *string         `json:"text,omitempty"`
	TextStyle *ChartTextStyle `json:"text_style,omitempty"`
	Fill      *DrawingStyle   `json:"fill,omitempty"`
	Outline   *TextOutline    `json:"outline,omitempty"`
}
type ChartTrendline struct {
	Label *ChartTrendlineLabel `json:"label,omitempty"`

	Type            string       `json:"type"`
	Name            string       `json:"name,omitempty"`
	Order           *int         `json:"order,omitempty"`
	Period          *int         `json:"period,omitempty"`
	Forward         *float64     `json:"forward,omitempty"`
	Backward        *float64     `json:"backward,omitempty"`
	Intercept       *float64     `json:"intercept,omitempty"`
	DisplayEquation *bool        `json:"display_equation,omitempty"`
	DisplayRSquared *bool        `json:"display_r_squared,omitempty"`
	Outline         *TextOutline `json:"outline,omitempty"`
}
type ChartErrorBars struct {
	Type      string       `json:"type"`                // standardError, percentage, standardDeviation, none
	Direction string       `json:"direction,omitempty"` // x or y; defaults to y
	Value     *float64     `json:"value,omitempty"`
	EndStyle  string       `json:"end_style,omitempty"` // cap or noCap
	Outline   *TextOutline `json:"outline,omitempty"`
}

const seriesOrder = "idx order tx spPr invertIfNegative pictureOptions marker explosion dPt dLbls trendline errBars cat val xVal yVal bubbleSize bubble3D shape smooth extLst"

func chartAppearance(parent *etree.Element, fill *DrawingStyle, outline *TextOutline, text *ChartTextStyle, order string) error {
	if fill != nil || outline != nil {
		pr := etree.NewElement("a:rPr")
		if err := paintTextProperties(pr, &TextPaint{Fill: fill, Outline: outline}); err != nil {
			return err
		}
		sp := chartEnsure(parent, "spPr", order)
		for _, child := range pr.ChildElements() {
			for _, old := range sp.ChildElements() {
				if old.NamespaceURI() == ans && (old.Tag == child.Tag || isChartFill(old.Tag) && isChartFill(child.Tag)) {
					sp.RemoveChild(old)
				}
			}
			if isChartFill(child.Tag) {
				sp.InsertChildAt(0, child.Copy())
			} else {
				i := 0
				for _, c := range sp.ChildElements() {
					if isChartFill(c.Tag) {
						i = c.Index() + 1
					}
				}
				sp.InsertChildAt(i, child.Copy())
			}
		}
	}
	if text != nil {
		return formatChartTextBody(chartEnsure(parent, "txPr", order), text)
	}
	return nil
}

// Style the attached text body so inherited namespace bindings remain available.
func formatChartTextBody(tx *etree.Element, text *ChartTextStyle) error {
	if text.Underline != "" {
		switch text.Underline {
		case "none", "words", "sng", "dbl", "heavy", "dotted", "dottedHeavy", "dash", "dashHeavy", "dashLong", "dashLongHeavy", "dotDash", "dotDashHeavy", "dotDotDash", "dotDotDashHeavy", "wavy", "wavyHeavy", "wavyDbl":
		default:
			return fmt.Errorf("invalid chart underline")
		}
	}
	align := map[string]string{"left": "l", "center": "ctr", "right": "r", "justify": "just"}[text.Alignment]
	if text.Alignment != "" && align == "" {
		return fmt.Errorf("invalid chart text alignment")
	}

	if text.FontSize != nil && (!finite(*text.FontSize) || *text.FontSize <= 0 || *text.FontSize > 400) {
		return fmt.Errorf("chart font_size must be 0..400 points, exclusive of zero")
	}
	if text.Color != "" && !hex.MatchString(text.Color) {
		return fmt.Errorf("chart text color must be six hex digits")
	}
	tx.CreateAttr("xmlns:a", ans)
	if office.Child(tx, ans, "bodyPr") == nil {
		tx.InsertChildAt(0, etree.NewElement("a:bodyPr"))
	}
	if office.Child(tx, ans, "lstStyle") == nil {
		tx.InsertChildAt(1, etree.NewElement("a:lstStyle"))
	}
	ps := office.Children(tx, ans, "p")
	if len(ps) == 0 {
		ps = []*etree.Element{tx.CreateElement("a:p")}
	}
	for _, p := range ps {
		pp := office.Child(p, ans, "pPr")
		if pp == nil {
			pp = etree.NewElement("a:pPr")
			p.InsertChildAt(0, pp)
		}
		if align != "" {
			pp.CreateAttr("algn", align)
		}
		def := office.Child(pp, ans, "defRPr")
		if def == nil {
			def = etree.NewElement("a:defRPr")
			if ext := office.Child(pp, ans, "extLst"); ext != nil {
				pp.InsertChildAt(ext.Index(), def)
			} else {
				pp.AddChild(def)
			}
		}
		props := []*etree.Element{def}
		props = append(props, office.All(p, ans, "rPr")...)
		props = append(props, office.Children(p, ans, "endParaRPr")...)
		for _, pr := range props {
			pr.CreateAttr("xmlns:a", ans)
			if text.Underline != "" {
				pr.CreateAttr("u", text.Underline)
			}
			if text.FontSize != nil {
				pr.CreateAttr("sz", strconv.Itoa(int(*text.FontSize*100)))
			}
			for _, b := range []struct {
				k string
				v *bool
			}{{"b", text.Bold}, {"i", text.Italic}} {
				if b.v != nil {
					v := "0"
					if *b.v {
						v = "1"
					}
					pr.CreateAttr(b.k, v)
				}
			}
			if text.Color != "" {
				if err := paintTextProperties(pr, &TextPaint{Fill: &DrawingStyle{Fill: text.Color}}); err != nil {
					return err
				}
			}
			if text.Font != "" {
				latin := office.Child(pr, ans, "latin")
				if latin == nil {
					latin = etree.NewElement("a:latin")
					i := len(pr.Child)
					for _, c := range pr.ChildElements() {
						switch c.Tag {
						case "ea", "cs", "sym", "hlinkClick", "hlinkMouseOver", "rtl", "extLst":
							i = c.Index()
							goto insertFont
						}
					}
				insertFont:
					pr.InsertChildAt(i, latin)
				}
				latin.CreateAttr("typeface", text.Font)
			}
		}
	}
	return nil
}

func isChartFill(tag string) bool {
	switch tag {
	case "noFill", "solidFill", "gradFill", "blipFill", "pattFill", "grpFill":
		return true
	}
	return false
}
func formatAxisExtras(axis *etree.Element, a ChartAxisFormat) error {
	if a.CrossesAt != nil {
		if !finite(*a.CrossesAt) {
			return fmt.Errorf("crosses_at must be finite")
		}
		for _, old := range office.Children(axis, cns, "crosses") {
			axis.RemoveChild(old)
		}
		chartEnsure(axis, "crossesAt", axisOrder).CreateAttr("val", strconv.FormatFloat(*a.CrossesAt, 'g', -1, 64))
	}
	if a.LabelOffsetPercent != nil {
		if (axis.Tag != "catAx" && axis.Tag != "dateAx") || *a.LabelOffsetPercent < 0 || *a.LabelOffsetPercent > 1000 {
			return fmt.Errorf("label_offset_percent requires a category/date axis and 0..1000")
		}
		chartEnsure(axis, "lblOffset", axisOrder).CreateAttr("val", strconv.Itoa(*a.LabelOffsetPercent))
	}

	if a.Position != "" {
		pos := map[string]string{"bottom": "b", "left": "l", "right": "r", "top": "t"}[a.Position]
		if pos == "" {
			return fmt.Errorf("axis position must be bottom, left, right or top")
		}
		chartEnsure(axis, "axPos", axisOrder).CreateAttr("val", pos)
	}
	if a.TickLabelPosition != "" {
		switch a.TickLabelPosition {
		case "nextTo", "high", "low", "none":
		default:
			return fmt.Errorf("invalid tick_label_position")
		}
		chartEnsure(axis, "tickLblPos", axisOrder).CreateAttr("val", a.TickLabelPosition)
	}

	if a.MinorUnit != nil {
		if !finite(*a.MinorUnit) || *a.MinorUnit <= 0 {
			return fmt.Errorf("minor_unit must be positive")
		}
		chartEnsure(axis, "minorUnit", axisOrder).CreateAttr("val", strconv.FormatFloat(*a.MinorUnit, 'g', -1, 64))
	}
	if err := chartAppearance(axis, nil, a.Outline, a.TextStyle, axisOrder); err != nil {
		return err
	}
	for _, v := range []struct {
		tag  string
		line *TextOutline
	}{{"majorGridlines", a.MajorGridline}, {"minorGridlines", a.MinorGridline}} {
		if v.line != nil {
			if err := chartAppearance(chartEnsure(axis, v.tag, axisOrder), nil, v.line, nil, "spPr"); err != nil {
				return err
			}
		}
	}
	return nil
}
func formatSeriesExtras(ser *etree.Element, s ChartSeriesFormat) error {
	kind := ser.Parent().Tag
	if s.Marker != nil {
		if err := formatChartMarker(ser, s.Marker); err != nil {
			return err
		}
	}
	if len(s.Points) > 0 {
		switch kind {
		case "barChart", "bar3DChart", "lineChart", "line3DChart", "pieChart", "pie3DChart", "doughnutChart", "ofPieChart", "areaChart", "area3DChart", "scatterChart", "bubbleChart", "radarChart":
		default:
			return fmt.Errorf("point formatting unsupported for %s", kind)
		}
		// Validate against cached values, not category labels (scatter uses xVal/yVal).
		values := office.Child(ser, cns, "val")
		if values == nil {
			values = office.Child(ser, cns, "yVal")
		}
		counts := office.All(values, cns, "ptCount")
		if len(counts) != 1 {
			return fmt.Errorf("point formatting requires a value cache")
		}
		count, err := strconv.Atoi(counts[0].SelectAttrValue("val", ""))
		if err != nil {
			return err
		}
		seen := map[int]bool{}
		for _, point := range s.Points {
			if point.Index < 0 || point.Index >= count || seen[point.Index] {
				return fmt.Errorf("invalid or duplicate point index")
			}
			seen[point.Index] = true
			var target *etree.Element
			for _, p := range office.Children(ser, cns, "dPt") {
				if idx := office.Child(p, cns, "idx"); idx != nil && idx.SelectAttrValue("val", "") == strconv.Itoa(point.Index) {
					if target != nil {
						return fmt.Errorf("ambiguous point index")
					}
					target = p
				}
			}
			if target == nil {
				target = newChartChild(ser, "dPt", seriesOrder)
				target.CreateElement("c:idx").CreateAttr("val", strconv.Itoa(point.Index))
			}
			appearance := target
			order := "idx invertIfNegative marker bubble3D explosion spPr pictureOptions extLst"
			switch kind {
			case "lineChart", "line3DChart", "scatterChart", "radarChart":
				if err := chartAppearance(target, point.Fill, point.Outline, nil, order); err != nil {
					return err
				}
				appearance = chartEnsure(target, "marker", order)
				if office.Child(appearance, cns, "symbol") == nil {
					symbol := "auto"
					if inherited := office.Child(office.Child(ser, cns, "marker"), cns, "symbol"); inherited != nil {
						symbol = inherited.SelectAttrValue("val", "auto")
					}
					chartEnsure(appearance, "symbol", "symbol size spPr extLst").CreateAttr("val", symbol)
				}
				order = "symbol size spPr extLst"
			}
			if err := chartAppearance(appearance, point.Fill, point.Outline, nil, order); err != nil {
				return err
			}
		}
	}
	if s.Trendlines != nil || s.ErrorBars != nil && s.ErrorBars.Type != "none" {
		switch kind {
		case "barChart", "lineChart", "areaChart", "scatterChart", "bubbleChart":
		default:
			return fmt.Errorf("trendlines/error bars unsupported for %s", kind)
		}
		if group := office.Child(ser.Parent(), cns, "grouping"); group != nil {
			v := group.SelectAttrValue("val", "")
			if v == "stacked" || v == "percentStacked" {
				return fmt.Errorf("trendlines/error bars unsupported for stacked charts")
			}
		}
	}
	if s.Trendlines != nil {
		if len(*s.Trendlines) > 100 {
			return fmt.Errorf("too many trendlines")
		}
		for _, old := range office.Children(ser, cns, "trendline") {
			ser.RemoveChild(old)
		}
		for _, t := range *s.Trendlines {
			typ := map[string]string{"linear": "linear", "exponential": "exp", "logarithmic": "log", "polynomial": "poly", "power": "power", "movingAverage": "movingAvg"}[t.Type]
			if typ == "" {
				return fmt.Errorf("invalid trendline type")
			}
			if t.Order != nil && (typ != "poly" || *t.Order < 2 || *t.Order > 6) {
				return fmt.Errorf("order requires polynomial trendline and 2..6")
			}
			if t.Period != nil && (typ != "movingAvg" || *t.Period < 2 || *t.Period > 255) {
				return fmt.Errorf("period requires movingAverage trendline and 2..255")
			}
			if typ == "movingAvg" && (t.Intercept != nil || t.Forward != nil || t.Backward != nil || t.DisplayEquation != nil && *t.DisplayEquation || t.DisplayRSquared != nil && *t.DisplayRSquared) {
				return fmt.Errorf("movingAverage cannot specify intercept, forecast or equation")
			}
			if t.Intercept != nil && typ != "linear" && typ != "poly" && typ != "exp" {
				return fmt.Errorf("intercept not supported for this trendline")
			}
			n := newChartChild(ser, "trendline", seriesOrder)
			if t.Name != "" {
				n.CreateElement("c:name").SetText(t.Name)
			}
			if err := chartAppearance(n, nil, t.Outline, nil, "name spPr trendlineType order period forward backward intercept dispRSqr dispEq trendlineLbl extLst"); err != nil {
				return err
			}
			n.CreateElement("c:trendlineType").CreateAttr("val", typ)
			if typ == "poly" {
				order := 2
				if t.Order != nil {
					order = *t.Order
				}
				n.CreateElement("c:order").CreateAttr("val", strconv.Itoa(order))
			}
			if typ == "movingAvg" {
				period := 2
				if t.Period != nil {
					period = *t.Period
				}
				n.CreateElement("c:period").CreateAttr("val", strconv.Itoa(period))
			}
			for _, v := range []struct {
				tag string
				n   *float64
			}{{"forward", t.Forward}, {"backward", t.Backward}, {"intercept", t.Intercept}} {
				if v.n != nil {
					if !finite(*v.n) || (v.tag != "intercept" && *v.n < 0) {
						return fmt.Errorf("invalid trendline %s", v.tag)
					}
					n.CreateElement("c:"+v.tag).CreateAttr("val", strconv.FormatFloat(*v.n, 'g', -1, 64))
				}
			}
			for _, v := range []struct {
				tag string
				n   *bool
			}{{"dispRSqr", t.DisplayRSquared}, {"dispEq", t.DisplayEquation}} {
				if v.n != nil {
					chartBool(n, v.tag, *v.n, "name spPr trendlineType order period forward backward intercept dispRSqr dispEq trendlineLbl extLst")
				}
			}
			if label := t.Label; label != nil {
				nl := n.CreateElement("c:trendlineLbl")
				if label.Text != nil {
					rich := nl.CreateElement("c:tx").CreateElement("c:rich")
					rich.CreateAttr("xmlns:a", ans)
					rich.CreateElement("a:bodyPr")
					rich.CreateElement("a:lstStyle")
					rich.CreateElement("a:p").CreateElement("a:r").CreateElement("a:t").SetText(*label.Text)
				}
				if err := chartAppearance(nl, label.Fill, label.Outline, nil, "layout tx numFmt spPr txPr extLst"); err != nil {
					return err
				}
				if err := formatChartRichText(nl, label.TextStyle, "layout tx numFmt spPr txPr extLst"); err != nil {
					return err
				}
			}
		}
	}
	if e := s.ErrorBars; e != nil {
		direction := e.Direction
		if direction == "" {
			direction = "y"
		}
		if direction != "x" && direction != "y" {
			return fmt.Errorf("error bar direction must be x or y")
		}
		if direction == "x" && kind != "scatterChart" && kind != "bubbleChart" {
			return fmt.Errorf("x error bars require scatter or bubble chart")
		}
		for _, old := range office.Children(ser, cns, "errBars") {
			d := office.Child(old, cns, "errDir")
			if d == nil || d.SelectAttrValue("val", "") == direction {
				ser.RemoveChild(old)
			}
		}
		if e.Type == "none" {
			if e.Value != nil || e.Outline != nil || e.EndStyle != "" {
				return fmt.Errorf("removed error bars cannot specify formatting")
			}
			return nil
		}
		typ := map[string]string{"standardError": "stdErr", "percentage": "percentage", "standardDeviation": "stdDev"}[e.Type]
		if typ == "" {
			return fmt.Errorf("invalid error bar type")
		}
		if e.Value != nil && (!finite(*e.Value) || *e.Value < 0 || typ == "stdErr") {
			return fmt.Errorf("invalid error bar value")
		}
		if e.EndStyle != "" && e.EndStyle != "cap" && e.EndStyle != "noCap" {
			return fmt.Errorf("invalid error bar end_style")
		}
		n := newChartChild(ser, "errBars", seriesOrder)
		n.CreateElement("c:errDir").CreateAttr("val", direction)
		n.CreateElement("c:errBarType").CreateAttr("val", "both")
		n.CreateElement("c:errValType").CreateAttr("val", typ)
		chartBool(n, "noEndCap", e.EndStyle == "noCap", "errDir errBarType errValType noEndCap plus minus val spPr extLst")
		if typ != "stdErr" {
			value := 1.0
			if e.Value != nil {
				value = *e.Value
			}
			n.CreateElement("c:val").CreateAttr("val", strconv.FormatFloat(value, 'g', -1, 64))
		}
		if err := chartAppearance(n, nil, e.Outline, nil, "errDir errBarType errValType noEndCap plus minus val spPr extLst"); err != nil {
			return err
		}
	}
	return nil
}
func newChartChild(parent *etree.Element, tag, order string) *etree.Element {
	siblings := office.Children(parent, cns, tag)
	if len(siblings) == 0 {
		return chartEnsure(parent, tag, order)
	}
	child := etree.NewElement("c:" + tag)
	child.CreateAttr("xmlns:c", cns)
	parent.InsertChildAt(siblings[len(siblings)-1].Index()+1, child)
	return child
}

// Apply both fallback text properties and any explicit rich-text run formatting.
func formatChartRichText(parent *etree.Element, style *ChartTextStyle, order string) error {
	if style == nil {
		return nil
	}
	if err := chartAppearance(parent, nil, nil, style, order); err != nil {
		return err
	}
	if rich := office.Child(office.Child(parent, cns, "tx"), cns, "rich"); rich != nil {
		return formatChartTextBody(rich, style)
	}
	return nil
}
