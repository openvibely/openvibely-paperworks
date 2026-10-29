package office

import (
	"errors"
	"fmt"
	"math"
	"path"
	"regexp"
	"strconv"
	"strings"

	"github.com/beevik/etree"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
)

const (
	NSX14Main = "http://schemas.microsoft.com/office/excel/2006/main"
	NSXDR     = "http://schemas.openxmlformats.org/drawingml/2006/spreadsheetDrawing"
)

// StandardChartPreview makes implicit pie colors and marker-only scatter
// explicit so LibreOffice renders them like Office does.
func StandardChartPreview(root *etree.Element) bool {
	changed := false
	Iter(root, func(chart *etree.Element) bool {
		if (Is(chart, NSC, "pieChart") || Is(chart, NSC, "doughnutChart")) && Child(chart, NSC, "varyColors") == nil {
			v := etree.NewElement("varyColors")
			v.Space = chart.Space
			v.CreateAttr("val", "1")
			chart.InsertChildAt(0, v)
			changed = true
		}
		if !Is(chart, NSC, "scatterChart") {
			return true
		}
		style := Child(chart, NSC, "scatterStyle")
		if style == nil || style.SelectAttrValue("val", "") != "marker" {
			return true
		}
		for _, s := range Children(chart, NSC, "ser") {
			props := Child(s, NSC, "spPr")
			if props == nil {
				props = newNS(s, NSC, "spPr")
				pos := 0
				for i, c := range s.ChildElements() {
					if Is(c, NSC, "idx") || Is(c, NSC, "order") || Is(c, NSC, "tx") {
						pos = c.Index() + 1
						_ = i
					}
				}
				s.InsertChildAt(pos, props)
			}
			line := Child(props, NSA, "ln")
			if line == nil {
				line = newNS(props, NSA, "ln")
				props.AddChild(line)
			}
			colour := Child(props, NSA, "solidFill")
			if colour == nil {
				colour = Child(line, NSA, "solidFill")
			}
			if colour != nil {
				colour = colour.Copy()
			}
			for _, f := range line.ChildElements() {
				switch f.Tag {
				case "noFill", "solidFill", "gradFill", "pattFill":
					line.RemoveChild(f)
				}
			}
			line.InsertChildAt(0, newNS(line, NSA, "noFill"))
			marker := Child(s, NSC, "marker")
			if marker == nil {
				marker = newNS(s, NSC, "marker")
				s.InsertChildAt(props.Index()+1, marker)
				sym := newNS(marker, NSC, "symbol")
				sym.CreateAttr("val", "circle")
				marker.AddChild(sym)
				size := newNS(marker, NSC, "size")
				size.CreateAttr("val", "3")
				marker.AddChild(size)
			}
			mp := Child(marker, NSC, "spPr")
			if mp == nil {
				mp = newNS(marker, NSC, "spPr")
				marker.AddChild(mp)
			}
			hasFill := false
			for _, t := range []string{"noFill", "solidFill", "gradFill", "pattFill"} {
				hasFill = hasFill || Child(mp, NSA, t) != nil
			}
			if !hasFill {
				if colour != nil {
					mp.InsertChildAt(0, colour)
				} else {
					fill := newNS(mp, NSA, "solidFill")
					mp.AddChild(fill)
					idx := 0
					if e := Child(s, NSC, "idx"); e != nil {
						idx, _ = strconv.Atoi(e.SelectAttrValue("val", "0"))
					}
					sc := newNS(fill, NSA, "schemeClr")
					sc.CreateAttr("val", "accent"+strconv.Itoa(idx%6+1))
					fill.AddChild(sc)
				}
			}
			changed = true
		}
		return true
	})
	return changed
}

// newNS creates a detached element in ns, reusing an existing prefix from ctx.
func newNS(ctx *etree.Element, ns, local string) *etree.Element {
	e := etree.NewElement(local)
	if p := nsPrefix(ctx, ns); p != "" {
		e.Space = p
	} else {
		e.CreateAttr("xmlns", ns)
	}
	return e
}

func sub(parent *etree.Element, ns, local string, attrs ...string) *etree.Element {
	e := newNS(parent, ns, local)
	for i := 0; i+1 < len(attrs); i += 2 {
		e.CreateAttr(attrs[i], attrs[i+1])
	}
	parent.AddChild(e)
	return e
}

// NeedsOfficePreview reports whether rendering needs a vector fallback copy.
func NeedsOfficePreview(source string) (bool, error) {
	e := strings.ToLower(path.Ext(source))
	if e != ".xlsx" && e != ".pptx" {
		return false, nil
	}
	p, err := opc.Open(source)
	if err != nil {
		return false, err
	}
	scan, err := detectOfficePreview(p, e)
	return scan != nil && scan.needs, err
}

type officePreviewScan struct {
	needs  bool
	charts []string
	parsed map[string]*etree.Document
	edited map[string]*etree.Document
}

// detectOfficePreview operates on an already-open package and retains parsed
// chart documents so the fallback copy can edit them without reparsing XML.
func detectOfficePreview(p *opc.Package, extension string) (*officePreviewScan, error) {
	charts, chartDocs, err := chartPartDocuments(p)
	if err != nil {
		return nil, err
	}
	scan := &officePreviewScan{
		charts: charts,
		parsed: chartDocs,
		edited: map[string]*etree.Document{},
	}
	for _, n := range charts {
		doc := chartDocs[n]
		if Is(doc.Root(), NSC, "chartSpace") && StandardChartPreview(doc.Root()) {
			scan.needs = true
			scan.edited[n] = doc
		}
	}
	if scan.needs {
		return scan, nil
	}
	if extension == ".xlsx" {
		for _, n := range p.Names() {
			if !strings.HasPrefix(n, "xl/worksheets/") || !strings.HasSuffix(n, ".xml") {
				continue
			}
			doc, err := p.XML(n)
			if err != nil {
				return nil, err
			}
			scan.parsed[n] = doc
			for _, g := range All(doc.Root(), NSX14, "sparklineGroup") {
				if g.SelectAttrValue("type", "line") == "line" {
					scan.needs = true
					return scan, nil
				}
			}
		}
		return scan, nil
	}
	slides, err := PresentationSlides(p)
	if err != nil {
		return nil, err
	}
	for _, s := range slides {
		doc, err := p.XML(s)
		if err != nil {
			return nil, err
		}
		scan.parsed[s] = doc
		if v := doc.Root().SelectAttrValue("show", ""); v == "0" || v == "false" {
			scan.needs = true
			return scan, nil
		}
	}
	for _, n := range charts {
		for _, s := range All(scan.parsed[n].Root(), NSCX, "series") {
			switch s.SelectAttrValue("layoutId", "") {
			case "treemap", "sunburst", "waterfall", "funnel":
				scan.needs = true
				return scan, nil
			case "clusteredColumn":
				if Child(Child(s, NSCX, "layoutPr"), NSCX, "binning") != nil {
					scan.needs = true
					return scan, nil
				}
			}
		}
	}
	return scan, nil
}

// RelTarget resolves an internal relationship of owner by ID.
func RelTarget(p *opc.Package, owner, id string) (string, error) {
	rels, err := p.Rels(owner)
	if err != nil {
		return "", err
	}
	for _, r := range rels {
		if r.ID == id {
			if r.TargetMode == "External" {
				break
			}
			return opc.Resolve(owner, r.Target), nil
		}
	}
	return "", fmt.Errorf("missing or external relationship %s", id)
}

// PresentationSlides lists slide parts in presentation order.
func PresentationSlides(p *opc.Package) ([]string, error) {
	doc, err := p.XML("ppt/presentation.xml")
	if err != nil {
		return nil, err
	}
	var out []string
	for _, s := range Children(Child(doc.Root(), NSP, "sldIdLst"), NSP, "sldId") {
		part, err := RelTarget(p, "ppt/presentation.xml", AttrV(s, NSR, "id"))
		if err != nil {
			return nil, err
		}
		out = append(out, part)
	}
	return out, nil
}

// editSet caches parsed parts so several edits to one part are coherent.
type editSet struct {
	p     *opc.Package
	docs  map[string]*etree.Document
	cache map[string]*etree.Document
}

// read returns a parsed document without marking the part for serialization.
func (s *editSet) read(part string) (*etree.Document, error) {
	if d := s.docs[part]; d != nil {
		return d, nil
	}
	if d := s.cache[part]; d != nil {
		return d, nil
	}
	d, err := s.p.XML(part)
	if err != nil {
		return nil, err
	}
	s.cache[part] = d
	return d, nil
}

// get returns a parsed document and marks it for serialization after edits.
func (s *editSet) get(part string) (*etree.Document, error) {
	if d := s.docs[part]; d != nil {
		return d, nil
	}
	d, err := s.read(part)
	if err != nil {
		return nil, err
	}
	s.docs[part] = d
	return d, nil
}

func (s *editSet) flush() error {
	for name, d := range s.docs {
		if err := s.p.SetXML(name, d); err != nil {
			return err
		}
	}
	return nil
}

// OfficePreviewCopy writes a disposable rendering copy that uses ordinary
// vector drawings for native features LibreOffice cannot print. It returns
// false without writing when no fallback is needed.
func OfficePreviewCopy(source, destination string) (bool, error) {
	extension := strings.ToLower(path.Ext(source))
	if extension != ".xlsx" && extension != ".pptx" {
		return false, nil
	}
	p, err := opc.Open(source)
	if err != nil {
		return false, err
	}
	scan, err := detectOfficePreview(p, extension)
	if err != nil || !scan.needs {
		return false, err
	}
	set := &editSet{p: p, docs: scan.edited, cache: scan.parsed}
	switch extension {
	case ".xlsx":
		if err := sparklinePreview(set); err != nil {
			return false, err
		}
	case ".pptx":
		if err := chartExPreview(set, scan.charts); err != nil {
			return false, err
		}
	}
	if len(set.docs) == 0 {
		return false, nil
	}
	if err := set.flush(); err != nil {
		return false, err
	}
	return true, p.SaveNew(destination)
}

// sheetGeometry answers column widths and row heights in pixels like Excel.
type sheetGeometry struct {
	root       *etree.Element
	defaultCol float64
	defaultRow float64
	rows       map[int]*etree.Element
}

func newGeometry(root *etree.Element) *sheetGeometry {
	g := &sheetGeometry{root: root, defaultCol: 8.43, defaultRow: 15, rows: map[int]*etree.Element{}}
	if f := Child(root, NSS, "sheetFormatPr"); f != nil {
		if v, err := strconv.ParseFloat(f.SelectAttrValue("defaultColWidth", ""), 64); err == nil && v > 0 {
			g.defaultCol = v
		}
		if v, err := strconv.ParseFloat(f.SelectAttrValue("defaultRowHeight", ""), 64); err == nil && v > 0 {
			g.defaultRow = v
		}
	}
	for _, r := range All(Child(root, NSS, "sheetData"), NSS, "row") {
		n, _ := strconv.Atoi(r.SelectAttrValue("r", ""))
		g.rows[n] = r
	}
	return g
}

func (g *sheetGeometry) col(index int) *etree.Element {
	for _, c := range All(Child(g.root, NSS, "cols"), NSS, "col") {
		lo, _ := strconv.Atoi(c.SelectAttrValue("min", "0"))
		hi, _ := strconv.Atoi(c.SelectAttrValue("max", "0"))
		if lo <= index && index <= hi {
			return c
		}
	}
	return nil
}

func (g *sheetGeometry) colHidden(index int) bool {
	c := g.col(index)
	return c != nil && Truthy(c.SelectAttrValue("hidden", ""), false)
}

func (g *sheetGeometry) rowHidden(n int) bool {
	r := g.rows[n]
	return r != nil && Truthy(r.SelectAttrValue("hidden", ""), false)
}

func (g *sheetGeometry) colPixels(index int) float64 {
	c := g.col(index)
	if c != nil && Truthy(c.SelectAttrValue("hidden", ""), false) {
		return 0
	}
	w := g.defaultCol
	if c != nil {
		if v, err := strconv.ParseFloat(c.SelectAttrValue("width", ""), 64); err == nil {
			w = v
		}
	}
	return math.Floor((256*w + math.Floor(128.0/7)) / 256 * 7)
}

func (g *sheetGeometry) rowPixels(n int) float64 {
	if g.rowHidden(n) {
		return 0
	}
	h := g.defaultRow
	if r := g.rows[n]; r != nil {
		if v, err := strconv.ParseFloat(r.SelectAttrValue("ht", ""), 64); err == nil && v != 0 {
			h = v
		}
	}
	// openpyxl points_to_pixels rounds each row upward before summing a
	// merged-cell height. Keeping fractional pixels shrinks preview geometry.
	return math.Ceil(h * 96 / 72)
}

func (g *sheetGeometry) mergedBounds(cell string) (int, int, int, int, bool) {
	c, r, err := SplitCell(cell)
	if err != nil {
		return 0, 0, 0, 0, false
	}
	for _, m := range All(Child(g.root, NSS, "mergeCells"), NSS, "mergeCell") {
		l, t, rr, b, err := ParseRange(m.SelectAttrValue("ref", ""))
		if err == nil && l <= c && c <= rr && t <= r && r <= b {
			return l, t, rr, b, true
		}
	}
	return 0, 0, 0, 0, false
}

const emuPerPixel = 9525

func sparklinePreview(set *editSet) error {
	p := set.p
	order, _, byName, err := WorkbookSheets(p)
	if err != nil {
		return err
	}
	wb, err := OpenWorkbook(p)
	if err != nil {
		return err
	}
	themeColours := map[int]string{}
	if doc, err := p.XML("xl/theme/theme1.xml"); err == nil {
		scheme := Child(Child(doc.Root(), NSA, "themeElements"), NSA, "clrScheme")
		for i, e := range scheme.ChildElements() {
			if c := e.ChildElements(); len(c) > 0 {
				v := c[0].SelectAttrValue("lastClr", "")
				if v == "" {
					v = c[0].SelectAttrValue("val", "")
				}
				themeColours[i] = v
			}
		}
	}
	colour := func(group *etree.Element, key, def string) string {
		e := Child(group, NSX14, key)
		if e == nil {
			return def
		}
		if rgb := e.SelectAttrValue("rgb", ""); rgb != "" {
			return rgb[max(0, len(rgb)-6):]
		}
		if t := e.SelectAttrValue("theme", ""); t != "" {
			i, _ := strconv.Atoi(t)
			if v := themeColours[i]; v != "" {
				return v
			}
		}
		return def
	}
	geoms := map[string]*sheetGeometry{}
	for _, name := range order {
		part := byName[name]
		doc, err := set.get(part)
		if err != nil {
			return err
		}
		geoms[name] = newGeometry(doc.Root())
	}
	for _, name := range order {
		part := byName[name]
		doc, _ := set.get(part)
		root := doc.Root()
		var groups []*etree.Element
		for _, g := range All(root, NSX14, "sparklineGroup") {
			if g.SelectAttrValue("type", "line") == "line" {
				groups = append(groups, g)
			}
		}
		if len(groups) == 0 {
			// Geometry reads are not edits. Preserve original worksheet bytes
			// when this sheet needs no sparkline drawing.
			delete(set.docs, part)
			continue
		}
		dr, err := ensureDrawing(set, part, root)
		if err != nil {
			return err
		}
		ident := 0
		Iter(dr, func(e *etree.Element) bool {
			if Is(e, NSXDR, "cNvPr") {
				if n, err := strconv.Atoi(e.SelectAttrValue("id", "0")); err == nil && n > ident {
					ident = n
				}
			}
			return true
		})
		shape := func(cell string, width, height float64, points [][2]float64, lineColour string, weight float64, fill bool) error {
			ident++
			c, r, err := SplitCell(cell)
			if err != nil {
				return err
			}
			anchor := sub(dr, NSXDR, "oneCellAnchor")
			from := sub(anchor, NSXDR, "from")
			for _, kv := range [][2]string{{"col", strconv.Itoa(c - 1)}, {"colOff", "0"}, {"row", strconv.Itoa(r - 1)}, {"rowOff", "0"}} {
				sub(from, NSXDR, kv[0]).SetText(kv[1])
			}
			sub(anchor, NSXDR, "ext", "cx", ftoa(width), "cy", ftoa(height))
			sp := sub(anchor, NSXDR, "sp")
			nv := sub(sp, NSXDR, "nvSpPr")
			sub(nv, NSXDR, "cNvPr", "id", strconv.Itoa(ident), "name", "Sparkline preview "+cell)
			sub(nv, NSXDR, "cNvSpPr")
			props := sub(sp, NSXDR, "spPr")
			geom := sub(props, NSA, "custGeom")
			for _, t := range []string{"avLst", "gdLst", "ahLst", "cxnLst"} {
				sub(geom, NSA, t)
			}
			sub(geom, NSA, "rect", "l", "0", "t", "0", "r", "r", "b", "b")
			fillMode := "none"
			if fill {
				fillMode = "norm"
			}
			pth := sub(sub(geom, NSA, "pathLst"), NSA, "path", "w", ftoa(width), "h", ftoa(height), "fill", fillMode)
			for i, pt := range points {
				tag := "lnTo"
				if i == 0 {
					tag = "moveTo"
				}
				sub(sub(pth, NSA, tag), NSA, "pt", "x", sparklineRoundedEMU(pt[0]), "y", sparklineRoundedEMU(pt[1]))
			}
			if fill {
				sub(pth, NSA, "close")
				sub(sub(props, NSA, "solidFill"), NSA, "srgbClr", "val", lineColour)
			} else {
				sub(props, NSA, "noFill")
			}
			ln := sub(props, NSA, "ln", "w", sparklineRoundedEMU(weight*12700))
			sub(sub(ln, NSA, "solidFill"), NSA, "srgbClr", "val", lineColour)
			sub(anchor, NSXDR, "clientData")
			return nil
		}
		geo := geoms[name]
		for _, group := range groups {
			type entry struct {
				cell string
				data []*float64
			}
			var entries []entry
			attr := func(k string) string { return group.SelectAttrValue(k, "") }
			for _, spark := range All(Child(group, NSX14, "sparklines"), NSX14, "sparkline") {
				ref := Text(spark, NSX14Main, "f")
				srcSheet, rng, ok := splitSheetRef(ref, sparkRefRe)
				if !ok {
					return errors.New("unsupported sparkline preview source")
				}
				l, t, r, b, err := ParseRange(rng)
				if err != nil || (r-l+1)*(b-t+1) > 100000 {
					return errors.New("sparkline preview range exceeds limits")
				}
				sg := geoms[srcSheet]
				if sg == nil {
					return errors.New("unknown sparkline source sheet " + srcSheet)
				}
				var data []*float64
				for row := t; row <= b; row++ {
					for c := l; c <= r; c++ {
						if attr("displayHidden") != "1" && (sg.rowHidden(row) || sg.colHidden(c)) {
							continue
						}
						ref := ColumnName(c) + strconv.Itoa(row)
						v, err := wb.Value(srcSheet, ref)
						if err != nil {
							return err
						}
						if v.Formula && (v.Kind == "" || (v.Kind != "str" && v.Text == "")) {
							return fmt.Errorf("Recalculate sparkline source before rendering: %s!%s", srcSheet, ref)
						}
						var val *float64
						if v.Kind == "n" {
							if f, err := strconv.ParseFloat(v.Text, 64); err == nil && !math.IsInf(f, 0) && !math.IsNaN(f) {
								val = &f
							}
						}
						if val == nil && attr("displayEmptyCellsAs") == "zero" {
							z := 0.0
							val = &z
						}
						data = append(data, val)
					}
				}
				if attr("rightToLeft") == "1" {
					for i, j := 0, len(data)-1; i < j; i, j = i+1, j-1 {
						data[i], data[j] = data[j], data[i]
					}
				}
				entries = append(entries, entry{Text(spark, NSX14Main, "sqref"), data})
			}
			var groupValues []float64
			for _, e := range entries {
				for _, v := range e.data {
					if v != nil {
						groupValues = append(groupValues, *v)
					}
				}
			}
			for _, e := range entries {
				var numeric []float64
				for _, v := range e.data {
					if v != nil {
						numeric = append(numeric, *v)
					}
				}
				if len(numeric) == 0 {
					continue
				}
				l, t, r, b, err := ParseRange(e.cell)
				if err != nil {
					return err
				}
				if ml, mt, mr, mb, ok := geo.mergedBounds(e.cell); ok {
					l, t, r, b = ml, mt, mr, mb
				}
				width, height := 0.0, 0.0
				for c := l; c <= r; c++ {
					width += geo.colPixels(c)
				}
				for row := t; row <= b; row++ {
					height += geo.rowPixels(row)
				}
				width, height = math.Round(width*emuPerPixel), math.Round(height*emuPerPixel)
				if width == 0 || height == 0 {
					continue
				}
				pad := math.Min(28575, math.Min(width/4, height/4))
				pick := func(kind string, def []float64, minimum bool) (float64, error) {
					if attr(kind+"AxisType") == "custom" {
						key := "manualMax"
						if minimum {
							key = "manualMin"
						}
						return parseFloatStrict(attr(key))
					}
					src := numeric
					if attr(kind+"AxisType") == "group" {
						src = def
					}
					if minimum {
						return minOf(src), nil
					}
					return maxOf(src), nil
				}
				low, err := pick("min", groupValues, true)
				if err != nil {
					return err
				}
				high, err := pick("max", groupValues, false)
				if err != nil {
					return err
				}
				y := func(v float64) float64 {
					if high == low {
						return height / 2
					}
					return height - pad - (math.Min(high, math.Max(low, v))-low)/(high-low)*(height-2*pad)
				}
				if attr("displayXAxis") == "1" && low <= 0 && 0 <= high {
					if err := shape(e.cell, width, height, [][2]float64{{pad, y(0)}, {width - pad, y(0)}}, colour(group, "colorAxis", "000000"), .5, false); err != nil {
						return err
					}
				}
				var segments [][][2]float64
				var current [][2]float64
				firstValueIndex, lastValueIndex := -1, -1
				for i, vp := range e.data {
					if vp != nil {
						if firstValueIndex == -1 {
							firstValueIndex = i
						}
						lastValueIndex = i
					}
				}
				hi, lo := maxOf(numeric), minOf(numeric)
				for i, vp := range e.data {
					if vp == nil {
						if attr("displayEmptyCellsAs") == "span" {
							continue
						}
						if len(current) > 1 {
							segments = append(segments, current)
						}
						current = nil
						continue
					}
					v := *vp
					pt := [2]float64{pad + float64(i)/math.Max(1, float64(len(e.data)-1))*(width-2*pad), y(v)}
					current = append(current, pt)
					marker := ""
					if attr("markers") == "1" {
						marker = "colorMarkers"
					}
					for _, f := range []struct {
						flag string
						cond bool
						key  string
					}{{"negative", v < 0, "colorNegative"}, {"first", i == firstValueIndex, "colorFirst"}, {"last", i == lastValueIndex, "colorLast"}, {"high", v == hi, "colorHigh"}, {"low", v == lo, "colorLow"}} {
						if f.cond && attr(f.flag) == "1" {
							marker = f.key
						}
					}
					if marker != "" {
						rad := math.Min(pad, 19050)
						px, py := pt[0], pt[1]
						if err := shape(e.cell, width, height, [][2]float64{{px - rad, py}, {px, py - rad}, {px + rad, py}, {px, py + rad}}, colour(group, marker, "376092"), .5, true); err != nil {
							return err
						}
					}
				}
				if len(current) > 1 {
					segments = append(segments, current)
				}
				weight := 0.75
				if w, err := strconv.ParseFloat(attr("lineWeight"), 64); err == nil {
					weight = w
				}
				for _, seg := range segments {
					if err := shape(e.cell, width, height, seg, colour(group, "colorSeries", "376092"), weight, false); err != nil {
						return err
					}
				}
			}
			group.Parent().RemoveChild(group)
		}
	}
	return nil
}

var sparkRefRe = regexp.MustCompile(`^(?:'((?:[^']|'')+)'|([^!]+))!(.+)$`)

func ftoa(f float64) string { return strconv.FormatInt(int64(math.Round(f)), 10) }

func minOf(v []float64) float64 {
	m := math.Inf(1)
	for _, x := range v {
		m = math.Min(m, x)
	}
	return m
}

func maxOf(v []float64) float64 {
	m := math.Inf(-1)
	for _, x := range v {
		m = math.Max(m, x)
	}
	return m
}

// ensureDrawing returns the worksheet's drawing root, creating the part and
// relationship when the sheet has none.
func ensureDrawing(set *editSet, part string, root *etree.Element) (*etree.Element, error) {
	p := set.p
	if d := Child(root, NSS, "drawing"); d != nil {
		dp, err := RelTarget(p, part, AttrV(d, NSR, "id"))
		if err != nil {
			return nil, err
		}
		doc, err := set.get(dp)
		if err != nil {
			return nil, err
		}
		return doc.Root(), nil
	}
	dp := ""
	for i := 1; ; i++ {
		dp = fmt.Sprintf("xl/drawings/preview%d.xml", i)
		if !p.Has(dp) && set.docs[dp] == nil {
			break
		}
	}
	rel := opc.Relative(part, dp)
	id, err := p.AddRel(part, opc.NSOffR+"/drawing", rel, false)
	if err != nil {
		return nil, err
	}
	d := newNS(root, NSS, "drawing")
	rp := nsPrefix(root, NSR)
	if rp == "" {
		rp = "r"
		root.CreateAttr("xmlns:r", NSR)
	}
	d.CreateAttr(rp+":id", id)
	InsertBefore(root, d, []string{"legacyDrawing", "legacyDrawingHF", "picture", "oleObjects", "controls", "webPublishItems", "tableParts", "extLst"})
	if err := p.EnsureContentType(dp, "application/vnd.openxmlformats-officedocument.drawing+xml"); err != nil {
		return nil, err
	}
	doc := etree.NewDocument()
	doc.CreateProcInst("xml", `version="1.0" encoding="UTF-8" standalone="yes"`)
	ws := doc.CreateElement("xdr:wsDr")
	ws.CreateAttr("xmlns:xdr", NSXDR)
	ws.CreateAttr("xmlns:a", NSA)
	set.docs[dp] = doc
	return ws, nil
}

// Reference round uses ties-to-even for the sparkline point and line-width writers.
func sparklineRoundedEMU(v float64) string { return strconv.FormatInt(int64(math.RoundToEven(v)), 10) }
