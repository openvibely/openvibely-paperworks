package office

import (
	"errors"
	"math"
	"slices"
	"strconv"
	"unicode/utf8"

	"github.com/beevik/etree"
)

// P converts points to EMU, truncating like reference presentation library's Pt().
func P(points float64) float64 { return math.Trunc(points * ptEMU) }

func runeLen(s string) float64 { return float64(utf8.RuneCountInString(s)) }

func rect(x, y, w, h float64) [][2]float64 {
	return [][2]float64{{x, y}, {x + w, y}, {x + w, y + h}, {x, y + h}}
}

func childPath(e *etree.Element, steps ...[2]string) *etree.Element {
	for _, s := range steps {
		e = Child(e, s[0], s[1])
	}
	return e
}

func truthyVal(e *etree.Element, key, def string) bool {
	v := e.SelectAttrValue(key, def)
	return v == "1" || v == "true"
}

func (pp *chartExPainter) paint(frame, chartRoot *etree.Element, series ChartExSeries) ([]*etree.Element, error) {
	dims := series.dims
	native := series.node
	histogram := series.Type == "histogram"
	var values []float64
	if histogram || series.Type == "waterfall" || series.Type == "funnel" {
		values = dims.Num["val"]
	} else {
		values = dims.Num["size"]
	}
	var paths [][]string
	switch {
	case histogram:
		for range values {
			paths = append(paths, []string{"Values"})
		}
	case dims.Paths != nil:
		paths = dims.Paths
	case dims.Str["cat"] != nil:
		for _, c := range dims.Str["cat"] {
			paths = append(paths, []string{c})
		}
	default:
		for i := range values {
			paths = append(paths, []string{strconv.Itoa(i + 1)})
		}
	}
	limit := 5000
	if histogram {
		limit = 100000
	}
	if len(values) > limit {
		return nil, errors.New("extended chart preview exceeds point limit")
	}
	tree := NewTree()
	for i := 0; i < min(len(paths), len(values)); i++ {
		cur := tree
		for j := len(paths[i]) - 1; j >= 0; j-- {
			n := cur.Get(paths[i][j], i)
			n.Value += values[i]
			cur = n.Children
		}
	}
	xf := Child(frame, NSP, "xfrm")
	off, ext := Child(xf, NSA, "off"), Child(xf, NSA, "ext")
	if off == nil || ext == nil {
		return nil, errors.New("chart frame has no position")
	}
	num := func(e *etree.Element, k string) float64 {
		v, _ := strconv.ParseFloat(e.SelectAttrValue(k, "0"), 64)
		return v
	}
	ox, oy, w, h := num(off, "x"), num(off, "y"), num(ext, "cx"), num(ext, "cy")
	chartXML := Child(chartRoot, NSCX, "chart")
	seriesColour := childPath(native, [2]string{NSCX, "spPr"}, [2]string{NSA, "solidFill"})
	var groups []string
	for _, p := range paths {
		if !slices.Contains(groups, p[len(p)-1]) {
			groups = append(groups, p[len(p)-1])
		}
	}
	colours := make([]string, len(paths))
	for i, p := range paths {
		colours[i] = pp.saved(seriesColour, pp.palette[slices.Index(groups, p[len(p)-1])%len(pp.palette)])
	}
	pointColour := func(i int, def string) string {
		for _, dp := range Children(native, NSCX, "dataPt") {
			if dp.SelectAttrValue("idx", "") == strconv.Itoa(i) {
				return pp.saved(childPath(dp, [2]string{NSCX, "spPr"}, [2]string{NSA, "solidFill"}), def)
			}
		}
		return def
	}
	for _, dp := range Children(native, NSCX, "dataPt") {
		if i, err := strconv.Atoi(dp.SelectAttrValue("idx", "-1")); err == nil && i >= 0 && i < len(colours) {
			colours[i] = pp.saved(childPath(dp, [2]string{NSCX, "spPr"}, [2]string{NSA, "solidFill"}), colours[i])
		}
	}
	title, legend := Child(chartXML, NSCX, "title"), Child(chartXML, NSCX, "legend")
	titleText := ""
	if title != nil {
		titleText = Text(childPath(title, [2]string{NSCX, "tx"}, [2]string{NSCX, "txData"}), NSCX, "v")
		if titleText == "" {
			if tx := Child(title, NSCX, "tx"); tx != nil {
				titleText = allText(tx)
			}
		}
	}
	padding := P(12)
	titleHeight := 0.0
	if titleText != "" {
		titleHeight = math.Min(h/4, P(16.5))
	}
	legendLabels := groups
	legendColours := map[string]string{}
	for _, g := range groups {
		for i, p := range paths {
			if p[len(p)-1] == g {
				legendColours[g] = colours[i]
				break
			}
		}
	}
	switch series.Type {
	case "sunburst", "histogram", "funnel":
		name := "Values"
		if v := childPath(native, [2]string{NSCX, "tx"}, [2]string{NSCX, "txData"}, [2]string{NSCX, "v"}); v != nil {
			name = v.Text()
		}
		legendLabels = []string{name}
		legendColours = map[string]string{name: pp.saved(seriesColour, pp.palette[0])}
	case "waterfall":
		legendLabels = []string{"Increase", "Decrease", "Total"}
		legendColours = map[string]string{"Increase": pp.palette[0], "Decrease": pp.palette[1], "Total": pp.palette[2]}
	}
	legendPos := ""
	if legend != nil {
		legendPos = legend.SelectAttrValue("pos", "b")
	}
	side := legendPos == "l" || legendPos == "r" || legendPos == "tr"
	legendExtent := 0.0
	if legend != nil {
		if side {
			longest := 0.0
			for _, l := range legendLabels {
				longest = math.Max(longest, runeLen(l))
			}
			legendExtent = math.Min(w/3, math.Max(P(80), longest*P(6)+P(24)))
		} else {
			total := 0.0
			for _, l := range legendLabels {
				total += runeLen(l)*6 + 28
			}
			legendExtent = math.Min(h/3, P(19.5)*math.Max(1, math.Ceil(total/math.Max(1, (w-2*padding)/12700))))
		}
	}
	plotX, plotY, plotW, plotH := ox+padding, oy+padding, w-2*padding, h-2*padding
	if titleText != "" && !truthyVal(title, "overlay", "0") {
		plotY += titleHeight
		plotH -= titleHeight
	}
	if legend != nil && !truthyVal(legend, "overlay", "0") {
		if side {
			plotW -= legendExtent
			if legendPos == "l" {
				plotX += legendExtent
			}
		} else {
			plotH -= legendExtent
			if legendPos == "t" {
				plotY += legendExtent
			}
		}
	}
	if plotW <= 0 || plotH <= 0 {
		return nil, errors.New("chart title/legend leave no plot space")
	}
	c := &pp.canvas
	plain := func(x, y, width, height float64, text string, style *etree.Element, size float64, center bool, colour string, bold bool) bool {
		return pp.label(x, y, width, height, text, style, size, center, colour, bold)
	}
	dataLabels := Child(native, NSCX, "dataLabels")
	plotLabel := func(x, y, width, height float64, label string, value float64, center bool) {
		if width < P(22) || height < P(20) {
			return
		}
		plain(x, y, width, P(12), label, dataLabels, 9, center, "FFFFFF", true)
		plain(x, y+P(12), width, P(11), gFmt(value), dataLabels, 7.5, center, "FFFFFF", false)
	}
	parentLayout := Text(Child(native, NSCX, "layoutPr"), NSCX, "parentLabelLayout")
	if n := childPath(native, [2]string{NSCX, "layoutPr"}, [2]string{NSCX, "parentLabelLayout"}); n != nil {
		parentLayout = n.SelectAttrValue("val", parentLayout)
	}
	var rectangles func(t *Tree, x, y, width, height, skip float64)
	rectangles = func(t *Tree, x, y, width, height, labelSkip float64) {
		for _, cell := range TreemapCells(t, x, y, width, height) {
			n := cell.Node
			if len(n.Children.Order) > 0 {
				if parentLayout == "banner" {
					c.freeform(rect(cell.X, cell.Y, cell.W, cell.H), colours[n.Index], "")
					plain(cell.X+P(1), cell.Y+P(1), cell.W-P(2), P(15), n.Label, nil, 9, false, "FFFFFF", true)
					rectangles(n.Children, cell.X, cell.Y+P(16.5), cell.W, math.Max(0, cell.H-P(16.5)), 0)
				} else {
					skip := 0.0
					if parentLayout == "overlapping" {
						skip = P(33)
					}
					rectangles(n.Children, cell.X, cell.Y, cell.W, cell.H, skip)
					if parentLayout == "overlapping" {
						plain(cell.X+P(1), cell.Y+P(1), cell.W-P(2), P(15), n.Label, nil, 9, false, "FFFFFF", true)
					}
				}
				continue
			}
			c.freeform(rect(cell.X, cell.Y, cell.W, cell.H), colours[n.Index], "")
			skip := 0.0
			if math.Abs(cell.Y-y) < 1 {
				skip = labelSkip
			}
			plotLabel(cell.X+P(1), cell.Y+P(1)+skip, cell.W-P(2), cell.H-P(2)-skip, n.Label, n.Value, false)
		}
	}
	depthLevels := 1.0
	if len(paths) > 0 {
		depthLevels = float64(len(paths[0]))
	}
	var rings func(t *Tree, start, end float64, depth int)
	rings = func(t *Tree, start, end float64, depth int) {
		total := 0.0
		for _, n := range t.Nodes {
			total += n.Value
		}
		angle := start
		radius := math.Max(0, math.Min(plotW, plotH)/2-P(.75))
		step := radius / (depthLevels + 1)
		cx, cy := plotX+plotW/2, plotY+plotH/2
		for _, n := range t.Sorted() {
			if total == 0 || n.Value == 0 {
				continue
			}
			stop := angle + (end-start)*n.Value/total
			segments := int(math.Max(2, math.Ceil((stop-angle)/2)))
			outer, inner := float64(depth+2)*step, float64(depth+1)*step
			var pts [][2]float64
			for i := 0; i <= segments; i++ {
				a := (angle + (stop-angle)*float64(i)/float64(segments) - 90) * math.Pi / 180
				pts = append(pts, [2]float64{cx + outer*math.Cos(a), cy + outer*math.Sin(a)})
			}
			for i := segments; i >= 0; i-- {
				a := (angle + (stop-angle)*float64(i)/float64(segments) - 90) * math.Pi / 180
				pts = append(pts, [2]float64{cx + inner*math.Cos(a), cy + inner*math.Sin(a)})
			}
			c.freeform(pts, colours[n.Index], "")
			mid := ((angle+stop)/2 - 90) * math.Pi / 180
			rad := (inner + outer) / 2
			if stop-angle >= 18 {
				plotLabel(cx+rad*math.Cos(mid)-P(30), cy+rad*math.Sin(mid)-P(11), P(60), P(24), n.Label, n.Value, true)
			}
			if len(n.Children.Order) > 0 {
				rings(n.Children, angle, stop, depth+1)
			}
			angle = stop
		}
	}
	if bg := childPath(chartRoot, [2]string{NSCX, "spPr"}, [2]string{NSA, "solidFill"}); bg != nil {
		c.freeform(rect(ox, oy, w, h), pp.saved(bg, "FFFFFF"), "")
	}
	var err error
	switch series.Type {
	case "histogram", "waterfall":
		err = pp.bars(chartXML, native, series, dims, values, plotX, plotY, plotW, plotH, pointColour)
	case "funnel":
		err = pp.funnel(chartXML, native, dims, values, plotX, plotY, plotW, plotH, pointColour)
	case "treemap":
		rectangles(tree, plotX, plotY, plotW, plotH, 0)
	default:
		rings(tree, 0, 360, 0)
	}
	if err != nil {
		return nil, err
	}
	if titleText != "" {
		plain(ox, oy+P(3), w, titleHeight, titleText, title, 13.5, true, "000000", false)
	}
	if legend != nil {
		lx := ox
		if legendPos != "l" && side {
			lx = ox + w - legendExtent
		}
		var ly float64
		if legendPos != "b" {
			ly = oy
			if legendPos == "t" || legendPos == "tr" {
				ly += titleHeight
			}
		} else {
			ly = oy + h - padding - legendExtent + P(4)
		}
		lineHeight := P(22)
		type item struct {
			label string
			width float64
		}
		rows := [][]item{{}}
		for _, l := range legendLabels {
			iw := math.Min(w, P(runeLen(l)*6+28))
			last := rows[len(rows)-1]
			used := 0.0
			for _, it := range last {
				used += it.width
			}
			if len(last) > 0 && (side || used+iw > w) {
				rows = append(rows, nil)
			}
			rows[len(rows)-1] = append(rows[len(rows)-1], item{l, iw})
		}
		for r, items := range rows {
			cursor := 0.0
			if !side {
				used := 0.0
				for _, it := range items {
					used += it.width
				}
				cursor = (w - used) / 2
			}
			for _, it := range items {
				x := lx + cursor
				if side {
					x = lx
				}
				y := ly + float64(r)*lineHeight
				if y+lineHeight > oy+h {
					return nil, errors.New("chart legend exceeds preview bounds")
				}
				c.freeform([][2]float64{{x + P(3), y + P(7)}, {x + P(11), y + P(7)}, {x + P(11), y + P(15)}, {x + P(3), y + P(15)}}, legendColours[it.label], "")
				extent := it.width
				if side {
					extent = legendExtent
				}
				plain(x+P(14), y, math.Max(P(22), extent-P(14)), lineHeight, it.label, legend, 7.5, false, "666666", false)
				cursor += it.width
			}
		}
	}
	return c.shapes, nil
}

func (pp *chartExPainter) bars(chartXML, native *etree.Element, series ChartExSeries, dims *ChartExDims, values []float64, plotX, plotY, plotW, plotH float64, pointColour func(int, string) string) error {
	c := &pp.canvas
	waterfall := series.Type == "waterfall"
	var bins []Bin
	if waterfall {
		cats := dims.Str["cat"]
		for i, v := range values {
			l := ""
			if i < len(cats) {
				l = cats[i]
			}
			bins = append(bins, Bin{l, v})
		}
	} else {
		var err error
		if bins, err = HistogramBins(values, childPath(native, [2]string{NSCX, "layoutPr"}, [2]string{NSCX, "binning"})); err != nil {
			return err
		}
	}
	totals := map[int]bool{}
	if waterfall {
		for _, e := range Children(childPath(native, [2]string{NSCX, "layoutPr"}, [2]string{NSCX, "subtotals"}), NSCX, "idx") {
			i, err := strconv.Atoi(e.SelectAttrValue("val", ""))
			if err != nil || i < 0 || i >= len(values) {
				return errors.New("waterfall subtotal index is outside its data")
			}
			totals[i] = true
		}
	}
	starts, ends := make([]float64, len(bins)), make([]float64, len(bins))
	running := 0.0
	for i, b := range bins {
		start := 0.0
		if waterfall && !totals[i] {
			start = running
		}
		starts[i], ends[i] = start, start+b.Count
		if waterfall {
			running = ends[i]
		}
	}
	var valueAxis, categoryAxis *etree.Element
	for _, a := range Children(Child(chartXML, NSCX, "plotArea"), NSCX, "axis") {
		if valueAxis == nil && Child(a, NSCX, "valScaling") != nil {
			valueAxis = a
		}
		if categoryAxis == nil && Child(a, NSCX, "catScaling") != nil {
			categoryAxis = a
		}
	}
	visible := func(a *etree.Element) bool { return a == nil || !truthyVal(a, "hidden", "0") }
	axisTitle := func(a *etree.Element) (*etree.Element, string) {
		if a == nil || !visible(a) {
			return nil, ""
		}
		t := Child(a, NSCX, "title")
		if t == nil {
			return nil, ""
		}
		return t, allText(t)
	}
	xt, xtitle := axisTitle(categoryAxis)
	yt, ytitle := axisTitle(valueAxis)
	all := append(append([]float64{}, starts...), ends...)
	scaleAxis := valueAxis
	if waterfall && minOf(all) < 0 {
		// The scale reader only needs valScaling. A detached axis copy loses
		// inherited namespace bindings, silently resetting negative minima.
		scaleAxis = etree.NewElement("axis")
		scaleAxis.CreateAttr("xmlns", NSCX)
		scaling := NewChild(scaleAxis, NSCX, "valScaling", nil)
		if original := Child(valueAxis, NSCX, "valScaling"); original != nil {
			for _, attr := range original.Attr {
				scaling.CreateAttr(attr.FullKey(), attr.Value)
			}
		}
		if v, ok := HasAttr(scaling, "min"); !ok || v == "auto" {
			scaling.CreateAttr("min", strconv.FormatFloat(minOf(all), 'g', -1, 64))
		}
	}
	minimum, maximum, ticks, err := HistogramScale(scaleAxis, maxOf(all))
	if err != nil {
		return err
	}
	code := "General"
	if f := Child(valueAxis, NSCX, "numFmt"); f != nil {
		code = f.SelectAttrValue("formatCode", "General")
	}
	var tickLabels []string
	longest := 0.0
	for _, t := range ticks {
		s, err := ChartNumber(t, code)
		if err != nil {
			return err
		}
		tickLabels = append(tickLabels, s)
		longest = math.Max(longest, runeLen(s))
	}
	ylabels := visible(valueAxis) && (valueAxis == nil || Child(valueAxis, NSCX, "tickLabels") != nil)
	xlabels := visible(categoryAxis) && (categoryAxis == nil || Child(categoryAxis, NSCX, "tickLabels") != nil)
	labelWidth, titleWidth := 0.0, 0.0
	if ylabels {
		labelWidth = math.Max(P(28), P(longest*5+4))
	}
	if ytitle != "" {
		titleWidth = P(26)
	}
	bx, by := plotX+labelWidth+titleWidth, plotY+P(14)
	bw := plotW - labelWidth - titleWidth - P(4)
	bh := plotH - P(20)
	if xlabels {
		bh -= P(26)
	}
	if xtitle != "" {
		bh -= P(24)
	}
	if bw <= 0 || bh <= 0 {
		return errors.New("histogram frame is too small for axes and labels")
	}
	y := func(v float64) float64 {
		return by + bh - bh*(math.Min(maximum, math.Max(minimum, v))-minimum)/(maximum-minimum)
	}
	grid := Child(valueAxis, NSCX, "majorGridlines")
	gridLine := childPath(grid, [2]string{NSCX, "spPr"}, [2]string{NSA, "ln"})
	showGrid := grid != nil && (gridLine == nil || Child(gridLine, NSA, "noFill") == nil)
	labelsStyle := Child(native, NSCX, "dataLabels")
	vis := Child(labelsStyle, NSCX, "visibility")
	showValues := vis != nil && truthyVal(vis, "value", "")
	axisLine := func(a *etree.Element) *etree.Element {
		return childPath(a, [2]string{NSCX, "spPr"}, [2]string{NSA, "ln"})
	}
	for i, t := range ticks {
		yy := y(t)
		if showGrid {
			c.line(bx, yy, bx+bw, yy, "D9D9D9", gridLine, pp.saved)
		}
		if ylabels {
			pp.label(plotX+titleWidth, yy-P(7), labelWidth, P(20), tickLabels[i], Child(valueAxis, NSCX, "txPr"), 8, true, "000000", false)
		}
	}
	if visible(valueAxis) {
		c.line(bx, by, bx, by+bh, "666666", axisLine(valueAxis), pp.saved)
	}
	if visible(categoryAxis) {
		c.line(bx, y(0), bx+bw, y(0), "666666", axisLine(categoryAxis), pp.saved)
	}
	slot := bw / float64(len(bins))
	gap := 0.0
	if waterfall {
		gw := .25
		if s := Child(categoryAxis, NSCX, "catScaling"); s != nil {
			v, err := parseFloatStrict(s.SelectAttrValue("gapWidth", ".25"))
			if err != nil {
				return errors.New("invalid waterfall gap width")
			}
			gw = v
		}
		if math.IsInf(gw, 0) || math.IsNaN(gw) || gw < 0 {
			return errors.New("invalid waterfall gap width")
		}
		gap = slot * (gw / (1 + gw)) / 2
	}
	longestBin := 0.0
	for _, b := range bins {
		longestBin = math.Max(longestBin, runeLen(b.Label))
	}
	stride := int(math.Max(1, math.Ceil(P(longestBin*4.5)/slot)))
	for i, b := range bins {
		xx := bx + float64(i)*slot
		top, baseline := math.Min(y(starts[i]), y(ends[i])), math.Max(y(starts[i]), y(ends[i]))
		def := pp.palette[0]
		if totals[i] {
			def = pp.palette[2]
		} else if waterfall && b.Count < 0 {
			def = pp.palette[1]
		}
		colour := pointColour(i, pp.saved(childPath(native, [2]string{NSCX, "spPr"}, [2]string{NSA, "solidFill"}), def))
		if top < baseline {
			name := "Histogram bin " + strconv.Itoa(i+1) + ": " + b.Label + ", frequency " + strconv.Itoa(int(b.Count))
			if waterfall {
				name = "Waterfall step " + strconv.Itoa(i+1) + ": " + b.Label + ", " + gFmt(starts[i]) + " to " + gFmt(ends[i])
			}
			c.freeform([][2]float64{{xx + gap, baseline}, {xx + slot - gap, baseline}, {xx + slot - gap, top}, {xx + gap, top}}, colour, name)
		}
		if waterfall && i < len(bins)-1 {
			v := childPath(native, [2]string{NSCX, "layoutPr"}, [2]string{NSCX, "visibility"})
			if v == nil || truthyVal(v, "connectorLines", "1") {
				c.line(xx+slot-gap, y(ends[i]), xx+slot+gap, y(ends[i]), "D9D9D9", nil, pp.saved)
			}
		}
		if xlabels && i%stride == 0 {
			pp.label(xx, by+bh+P(4), math.Min(slot*float64(stride), bx+bw-xx), P(24), b.Label, Child(categoryAxis, NSCX, "txPr"), 8, true, "000000", false)
		}
		if showValues && slot >= P(24) && minimum <= b.Count && b.Count <= maximum {
			ly := top - P(14)
			switch labelsStyle.SelectAttrValue("pos", "") {
			case "ctr":
				ly = (top+baseline)/2 - P(7)
			case "inEnd":
				ly = top + P(2)
			}
			text := strconv.Itoa(int(b.Count))
			if waterfall {
				text = pyRepr(b.Count)
			}
			pp.label(xx, ly, slot, P(18), text, labelsStyle, 8, true, "000000", false)
		}
	}
	if xtitle != "" {
		off := P(4)
		if xlabels {
			off = P(27)
		}
		pp.label(bx, by+bh+off, bw, P(24), xtitle, xt, 11, true, "000000", false)
	}
	if ytitle != "" && pp.label(plotX, by, titleWidth, bh, ytitle, yt, 11, true, "000000", false) {
		last := c.shapes[len(c.shapes)-1]
		Iter(last, func(e *etree.Element) bool {
			if e.Tag == "bodyPr" {
				e.CreateAttr("vert", "vert270")
			}
			return true
		})
	}
	return nil
}

func (pp *chartExPainter) funnel(chartXML, native *etree.Element, dims *ChartExDims, values []float64, plotX, plotY, plotW, plotH float64, pointColour func(int, string) string) error {
	labels := dims.Str["cat"]
	if labels == nil {
		labels = make([]string, len(values))
	}
	labelWidth, longest, any := 0.0, 0.0, false
	for _, l := range labels {
		longest = math.Max(longest, runeLen(l))
		any = any || l != ""
	}
	if any {
		labelWidth = P(math.Max(28, longest*5+10))
	}
	bw, bx := plotW-labelWidth, plotX+labelWidth
	if bw <= 0 {
		return errors.New("funnel labels leave no plot space")
	}
	slot := plotH / float64(len(values))
	maximum := maxOf(values)
	gap := .05
	if s := childPath(chartXML, [2]string{NSCX, "plotArea"}, [2]string{NSCX, "axis"}, [2]string{NSCX, "catScaling"}); s != nil {
		v, err := parseFloatStrict(s.SelectAttrValue("gapWidth", ".05"))
		if err != nil {
			return errors.New("invalid funnel gap width")
		}
		gap = v
	}
	if math.IsInf(gap, 0) || math.IsNaN(gap) || gap < 0 {
		return errors.New("invalid funnel gap width")
	}
	gap = slot * gap / (1 + gap)
	style := Child(native, NSCX, "dataLabels")
	vis := Child(style, NSCX, "visibility")
	for i, v := range values {
		label := ""
		if i < len(labels) {
			label = labels[i]
		}
		width := bw * v / maximum
		left := bx + (bw-width)/2
		top := plotY + float64(i)*slot + gap/2
		colour := pointColour(i, pp.saved(childPath(native, [2]string{NSCX, "spPr"}, [2]string{NSA, "solidFill"}), pp.palette[0]))
		if width != 0 {
			pp.canvas.freeform(rect(left, top, width, slot-gap), colour, "Funnel stage "+strconv.Itoa(i+1)+": "+label+", value "+gFmt(v))
		}
		if label != "" {
			pp.label(plotX, top+(slot-gap)/2-P(8), labelWidth, P(20), label, nil, 8, false, "000000", false)
		}
		if vis != nil && truthyVal(vis, "value", "") {
			pp.label(left, top+(slot-gap)/2-P(8), width, P(20), gFmt(v), style, 8, true, "000000", false)
		}
	}
	return nil
}
