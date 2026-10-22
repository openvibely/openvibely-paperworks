package office

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"

	"github.com/beevik/etree"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
)

type ExtendedChartSpec struct {
	// CategoriesPresent distinguishes explicit JSON null from omission. A typed
	// nil Categories slice with this false retains the existing category labels.
	categoriesInvalid bool
	categoriesNotList bool
	binCountJSON      json.RawMessage // preserve JSON values until chart validation
	CategoriesPresent bool            `json:"-"`
	Action            string          `json:"action"`
	Type              string          `json:"type"`
	Slide             int             `json:"slide"`
	Name              string          `json:"name"`
	Part              string          `json:"chart_part"`
	Values            []float64       `json:"values"`
	Categories        []string        `json:"categories"`
	Paths             [][]string      `json:"category_paths"`
	Position          []int64         `json:"position_emu"`
	BinSize           *float64        `json:"bin_size"`
	BinCount          *int            `json:"bin_count"`
	Interval          *string         `json:"interval_closed"`
	Underflow         any             `json:"underflow"`
	Overflow          any             `json:"overflow"`
}

// UnmarshalJSON retains invalid JSON category shapes for application validation.
// The public typed API remains []string; no non-string label is coerced.
func (s *ExtendedChartSpec) UnmarshalJSON(data []byte) error {
	type plain ExtendedChartSpec
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	raw, present := fields["categories"]
	delete(fields, "categories")
	binCount := fields["bin_count"]
	delete(fields, "bin_count")
	rest, err := json.Marshal(fields)
	if err != nil {
		return err
	}
	var decoded plain
	if err := json.Unmarshal(rest, &decoded); err != nil {
		return err
	}
	decoded.binCountJSON = append(json.RawMessage(nil), binCount...)
	decoded.CategoriesPresent = present
	if present {
		var labels []json.RawMessage
		if string(raw) == "null" {
			decoded.categoriesNotList = true
		} else if err := json.Unmarshal(raw, &labels); err != nil {
			decoded.categoriesNotList = true
		} else {
			decoded.Categories = make([]string, len(labels))
			for i, label := range labels {
				var value any
				if err := json.Unmarshal(label, &value); err != nil {
					return err
				}
				text, ok := value.(string)
				if !ok {
					decoded.categoriesInvalid = true
					continue
				}
				decoded.Categories[i] = text
			}
			if decoded.categoriesInvalid {
				decoded.Categories = nil
			}
		}
	}
	*s = ExtendedChartSpec(decoded)
	return nil
}

func chartExReports(p *opc.Package) ([]*ChartExReport, error) {
	parts, e := ChartParts(p)
	if e != nil {
		return nil, e
	}
	reports := []*ChartExReport{}
	for _, part := range parts {
		d, e := p.XML(part)
		if e != nil {
			return nil, e
		}
		if !Is(d.Root(), NSCX, "chartSpace") {
			continue
		}
		r, e := VerifyExtendedChart(d.Root(), part, p)
		if e != nil {
			return nil, e
		}
		reports = append(reports, r)
	}
	return reports, nil
}

// ExtendedCharts creates native ChartEx objects or updates literal dimensions.
// Linked workbooks always require chart-inputs, preserving source formulas.
func ExtendedCharts(p *opc.Package, s ExtendedChartSpec) ([]*ChartExReport, error) {
	reports, e := chartExReports(p)
	if e != nil {
		return nil, e
	}
	if s.Action == "" || s.Action == "list" {
		return reports, nil
	}
	if s.Action != "add" && s.Action != "replace-data" {
		return nil, fmt.Errorf("unknown extended chart action")
	}
	if len(s.Values) < 1 || len(s.Values) > 100000 {
		return nil, fmt.Errorf("values must contain 1..100000 numbers")
	}
	for _, v := range s.Values {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return nil, fmt.Errorf("non-finite chart value")
		}
	}
	var d *etree.Document
	var data *etree.Element
	kind := s.Type
	part := s.Part
	child := func(parent *etree.Element, tag string) *etree.Element {
		v := parent.CreateElement("cx:" + tag)
		v.CreateAttr("xmlns:cx", NSCX)
		return v
	}
	if s.Action == "add" {
		if kind != "treemap" && kind != "sunburst" && kind != "histogram" {
			return nil, fmt.Errorf("supported types: treemap, sunburst, histogram")
		}
		slides, e := PresentationSlides(p)
		if e != nil {
			return nil, e
		}
		if s.Slide < 1 || s.Slide > len(slides) {
			return nil, fmt.Errorf("select a valid one-based slide")
		}
		pres, e := p.XML("ppt/presentation.xml")
		if e != nil {
			return nil, e
		}
		sz := Child(pres.Root(), NSP, "sldSz")
		pos := s.Position
		if len(pos) != 4 || pos[0] < 0 || pos[1] < 0 || pos[2] <= 0 || pos[3] <= 0 || pos[0] > int64(emuAttr(sz, "cx", 0))-pos[2] || pos[1] > int64(emuAttr(sz, "cy", 0))-pos[3] {
			return nil, fmt.Errorf("position_emu must be [left,top,width,height] within slide")
		}
		part = uniquePart(p, "ppt/charts", "extendedChart")
		d = newRoot("chartSpace", NSCX)
		root := d.Root()
		data = child(child(root, "chartData"), "data")
		data.CreateAttr("id", "0")
		chart := child(root, "chart")
		plot := child(chart, "plotArea")
		region := child(plot, "plotAreaRegion")
		series := child(region, "series")
		layout := kind
		if kind == "histogram" {
			layout = "clusteredColumn"
		}
		series.CreateAttr("layoutId", layout)
		name := s.Name
		if name == "" {
			name = "Values"
		}
		child(child(child(series, "tx"), "txData"), "v").SetText(name)
		child(series, "dataId").CreateAttr("val", "0")
		if kind == "histogram" {
			child(child(series, "layoutPr"), "binning")
			axis := child(plot, "axis")
			axis.CreateAttr("id", "0")
			child(axis, "catScaling").CreateAttr("gapWidth", "0")
			child(axis, "tickLabels")
			axis = child(plot, "axis")
			axis.CreateAttr("id", "1")
			child(axis, "valScaling")
			child(axis, "tickLabels")
		}
		legend := child(chart, "legend")
		legend.CreateAttr("pos", "b")
		legend.CreateAttr("overlay", "0")
		owner := slides[s.Slide-1]
		target := opc.Relative(owner, part)
		rid, e := p.AddRel(owner, "http://schemas.microsoft.com/office/2014/relationships/chartEx", target, false)
		if e != nil {
			return nil, e
		}
		if e = p.EnsureContentType(part, "application/vnd.ms-office.chartex+xml"); e != nil {
			return nil, e
		}
		slide, e := p.XML(owner)
		if e != nil {
			return nil, e
		}
		tree := Child(Child(slide.Root(), NSP, "cSld"), NSP, "spTree")
		if tree == nil {
			return nil, fmt.Errorf("missing slide shape tree")
		}
		id := 1
		for _, prop := range All(tree, NSP, "cNvPr") {
			n, e := strconv.Atoi(prop.SelectAttrValue("id", ""))
			if e != nil {
				return nil, fmt.Errorf("invalid shape ID")
			}
			if n > id {
				id = n
			}
		}
		frame := etree.NewElement("p:graphicFrame")
		frame.CreateAttr("xmlns:p", NSP)
		frame.CreateAttr("xmlns:a", NSA)
		frame.CreateAttr("xmlns:r", NSR)
		nv := frame.CreateElement("p:nvGraphicFramePr")
		prop := nv.CreateElement("p:cNvPr")
		prop.CreateAttr("id", strconv.Itoa(id+1))
		prop.CreateAttr("name", name)
		nv.CreateElement("p:cNvGraphicFramePr")
		nv.CreateElement("p:nvPr")
		xf := frame.CreateElement("p:xfrm")
		off := xf.CreateElement("a:off")
		off.CreateAttr("x", strconv.FormatInt(pos[0], 10))
		off.CreateAttr("y", strconv.FormatInt(pos[1], 10))
		ext := xf.CreateElement("a:ext")
		ext.CreateAttr("cx", strconv.FormatInt(pos[2], 10))
		ext.CreateAttr("cy", strconv.FormatInt(pos[3], 10))
		gd := frame.CreateElement("a:graphic").CreateElement("a:graphicData")
		gd.CreateAttr("uri", NSCX)
		child(gd, "chart").CreateAttr("r:id", rid)
		insertBeforeExt(tree, frame)
		if e = p.SetXML(owner, slide); e != nil {
			return nil, e
		}
	} else {
		var report *ChartExReport
		for _, r := range reports {
			if r.Chart == part {
				report = r
			}
		}
		if report == nil {
			return nil, fmt.Errorf("unknown extended chart part")
		}
		if report.WorkbookVerified {
			return nil, fmt.Errorf("use chart-inputs for linked workbook data")
		}
		if len(report.Series) != 1 {
			return nil, fmt.Errorf("replacement requires a single-series chart")
		}
		series := report.Series[0]
		kind = series.Type
		if kind != "treemap" && kind != "sunburst" && kind != "histogram" {
			return nil, fmt.Errorf("data replacement is not supported for %s", kind)
		}
		if kind == "histogram" && s.categoriesInvalid {
			return nil, fmt.Errorf("Histogram categories must match values")
		}
		if kind == "histogram" && (s.categoriesNotList || s.CategoriesPresent && s.Categories == nil) {
			return nil, fmt.Errorf("Histogram categories must be a list matching values")
		}
		if kind == "histogram" && s.Categories == nil {
			s.Categories = series.dims.Str["cat"]
		}
		d, e = p.XML(part)
		if e != nil {
			return nil, e
		}
		for _, v := range Children(Child(d.Root(), NSCX, "chartData"), NSCX, "data") {
			if v.SelectAttrValue("id", "") == series.DataID {
				data = v
			}
		}
		if data == nil {
			return nil, fmt.Errorf("missing chart data")
		}
		for _, v := range data.ChildElements() {
			data.RemoveChild(v)
		}
	}
	levels := [][]string{}
	if kind == "histogram" && s.categoriesInvalid {
		return nil, fmt.Errorf("Histogram categories must match values")
	}
	if kind == "histogram" && (s.categoriesNotList || s.CategoriesPresent && s.Categories == nil) {
		return nil, fmt.Errorf("Histogram categories must be a list matching values")
	}
	if kind == "histogram" {
		if s.Categories != nil {
			levels = append(levels, s.Categories)
		}
	} else {
		positive := false
		for _, v := range s.Values {
			if v < 0 {
				return nil, fmt.Errorf("hierarchy values must be nonnegative")
			}
			positive = positive || v > 0
		}
		if !positive {
			return nil, fmt.Errorf("hierarchy total must be positive")
		}
		if s.Paths != nil {
			if len(s.Paths) != len(s.Values) || len(s.Paths[0]) == 0 || len(s.Paths[0]) > 100 {
				return nil, fmt.Errorf("category_paths must match values with depth 1..100")
			}
			for k := range s.Paths[0] {
				var level []string
				for _, v := range s.Paths {
					if len(v) != len(s.Paths[0]) {
						return nil, fmt.Errorf("category paths must be equal-depth")
					}
					level = append(level, v[k])
				}
				levels = append(levels, level)
			}
		} else {
			levels = append(levels, s.Categories)
		}
	}
	if len(levels) > 0 {
		dim := child(data, "strDim")
		dim.CreateAttr("type", "cat")
		for _, labels := range levels {
			if len(labels) != len(s.Values) {
				if kind == "histogram" {
					return nil, fmt.Errorf("Histogram categories must match values")
				}
				return nil, fmt.Errorf("categories must match values")
			}
			lvl := child(dim, "lvl")
			writeChartExPoints(lvl, labels, func() *etree.Element { return child(lvl, "pt") })
		}
	}
	dim := child(data, "numDim")
	typ := "val"
	if kind != "histogram" {
		typ = "size"
	}
	dim.CreateAttr("type", typ)
	lvl := child(dim, "lvl")
	texts := make([]string, len(s.Values))
	for i, v := range s.Values {
		texts[i] = strconv.FormatFloat(v, 'g', -1, 64)
	}
	writeChartExPoints(lvl, texts, func() *etree.Element { return child(lvl, "pt") })
	if kind == "histogram" {
		bins := All(d.Root(), NSCX, "binning")
		if len(bins) != 1 {
			return nil, fmt.Errorf("histogram requires one binning node")
		}
		bin := bins[0]
		if s.BinSize != nil && (s.BinCount != nil || s.binCountJSON != nil) {
			return nil, fmt.Errorf("select bin_size or bin_count")
		}
		if s.binCountJSON != nil {
			var value any
			if err := json.Unmarshal(s.binCountJSON, &value); err != nil {
				return nil, err
			}
			number, ok := value.(float64)
			if !ok {
				return nil, fmt.Errorf("numeric histogram bin setting required")
			}
			if number <= 0 || number != math.Trunc(number) || number > 4294967295 {
				return nil, fmt.Errorf("invalid bin_count")
			}
			count := int(number)
			s.BinCount = &count
		}
		if s.BinSize != nil || s.BinCount != nil {
			for _, v := range bin.ChildElements() {
				bin.RemoveChild(v)
			}
			if s.BinSize != nil {
				if *s.BinSize <= 0 || math.IsNaN(*s.BinSize) || math.IsInf(*s.BinSize, 0) {
					return nil, fmt.Errorf("invalid bin_size")
				}
				child(bin, "binSize").SetText(strconv.FormatFloat(*s.BinSize, 'g', -1, 64))
			} else {
				if *s.BinCount <= 0 {
					return nil, fmt.Errorf("invalid bin_count")
				}
				child(bin, "binCount").SetText(strconv.Itoa(*s.BinCount))
			}
		}
		if s.Interval != nil {
			if *s.Interval != "l" && *s.Interval != "r" {
				return nil, fmt.Errorf("interval_closed must be l or r")
			}
			bin.CreateAttr("intervalClosed", *s.Interval)
		}
		for k, v := range map[string]any{"underflow": s.Underflow, "overflow": s.Overflow} {
			if v == nil {
				continue
			}
			switch v := v.(type) {
			case string:
				if v != "auto" {
					return nil, fmt.Errorf("%s must be finite or auto", k)
				}
				bin.CreateAttr(k, v)
			case float64:
				if math.IsNaN(v) || math.IsInf(v, 0) {
					return nil, fmt.Errorf("invalid boundary")
				}
				bin.CreateAttr(k, strconv.FormatFloat(v, 'g', -1, 64))
			default:
				return nil, fmt.Errorf("%s must be finite or auto", k)
			}
		}
	}
	if _, e = VerifyExtendedChart(d.Root(), part, nil); e != nil {
		return nil, e
	}
	if e = p.SetXML(part, d); e != nil {
		return nil, e
	}
	return chartExReports(p)
}
