package office

import (
	"errors"
	"fmt"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/beevik/etree"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
)

const ptEMU = 12700.0

var hex6 = regexp.MustCompile(`^[0-9A-Fa-f]{6}$`)

// slideCanvas collects vector shapes that replace one ChartEx graphic frame.
type slideCanvas struct {
	shapes []*etree.Element
}

func pEl(local string, attrs ...string) *etree.Element {
	e := etree.NewElement("p:" + local)
	for i := 0; i+1 < len(attrs); i += 2 {
		e.CreateAttr(attrs[i], attrs[i+1])
	}
	return e
}

func aEl(parent *etree.Element, local string, attrs ...string) *etree.Element {
	e := parent.CreateElement("a:" + local)
	for i := 0; i+1 < len(attrs); i += 2 {
		e.CreateAttr(attrs[i], attrs[i+1])
	}
	return e
}

func ri(f float64) string { return strconv.FormatInt(int64(math.Round(f)), 10) }

// previewShapeStyle retains reference presentation library 1.0.2's editable theme defaults.
func previewShapeStyle(shape *etree.Element, line, fill, effect, fontColour string) {
	style := shape.CreateElement("p:style")
	for _, ref := range [][2]string{{"lnRef", line}, {"fillRef", fill}, {"effectRef", effect}} {
		aEl(aEl(style, ref[0], "idx", ref[1]), "schemeClr", "val", "accent1")
	}
	aEl(aEl(style, "fontRef", "idx", "minor"), "schemeClr", "val", fontColour)
}

// freeform adds a closed filled polygon with a thin white outline.
func (c *slideCanvas) freeform(points [][2]float64, colour, name string) {
	minX, minY := math.Inf(1), math.Inf(1)
	maxX, maxY := math.Inf(-1), math.Inf(-1)
	for _, p := range points {
		x, y := math.Round(p[0]), math.Round(p[1])
		minX, minY, maxX, maxY = math.Min(minX, x), math.Min(minY, y), math.Max(maxX, x), math.Max(maxY, y)
	}
	w, h := math.Max(1, maxX-minX), math.Max(1, maxY-minY)
	sp := pEl("sp")
	nv := sp.CreateElement("p:nvSpPr")
	if name == "" {
		name = fmt.Sprintf("Freeform %d", len(c.shapes)+1)
	}
	nv.CreateElement("p:cNvPr").CreateAttr("name", name)
	nv.SelectElement("p:cNvPr").CreateAttr("id", "0")
	nv.CreateElement("p:cNvSpPr")
	nv.CreateElement("p:nvPr")
	props := sp.CreateElement("p:spPr")
	xf := aEl(props, "xfrm")
	aEl(xf, "off", "x", ri(minX), "y", ri(minY))
	aEl(xf, "ext", "cx", ri(w), "cy", ri(h))
	geom := aEl(props, "custGeom")
	for _, t := range []string{"avLst", "gdLst", "ahLst", "cxnLst"} {
		aEl(geom, t)
	}
	aEl(geom, "rect", "l", "l", "t", "t", "r", "r", "b", "b")
	path := aEl(aEl(geom, "pathLst"), "path", "w", ri(w), "h", ri(h))
	for i, p := range points {
		tag := "lnTo"
		if i == 0 {
			tag = "moveTo"
		}
		aEl(aEl(path, tag), "pt", "x", ri(math.Round(p[0])-minX), "y", ri(math.Round(p[1])-minY))
	}
	aEl(path, "close")
	aEl(aEl(props, "solidFill"), "srgbClr", "val", strings.ToUpper(colour))
	ln := aEl(props, "ln", "w", ri(.7*ptEMU))
	aEl(aEl(ln, "solidFill"), "srgbClr", "val", "FFFFFF")
	aEl(props, "effectLst")
	previewShapeStyle(sp, "1", "3", "2", "lt1")
	body := sp.CreateElement("p:txBody")
	aEl(body, "bodyPr", "rtlCol", "0", "anchor", "ctr")
	aEl(body, "lstStyle")
	aEl(aEl(body, "p"), "pPr", "algn", "ctr")
	c.shapes = append(c.shapes, sp)
}

// line adds a straight connector, honouring a saved a:ln when given.
func (c *slideCanvas) line(x1, y1, x2, y2 float64, colour string, props *etree.Element, saved func(*etree.Element, string) string) {
	if props != nil && Child(props, NSA, "noFill") != nil {
		return
	}
	width := ri(.5 * ptEMU)
	if props != nil {
		if w := props.SelectAttrValue("w", ""); w != "" {
			width = w
		}
		colour = saved(Child(props, NSA, "solidFill"), colour)
	}
	cx := pEl("cxnSp")
	nv := cx.CreateElement("p:nvCxnSpPr")
	nv.CreateElement("p:cNvPr").CreateAttr("id", "0")
	nv.SelectElement("p:cNvPr").CreateAttr("name", fmt.Sprintf("Connector %d", len(c.shapes)+1))
	nv.CreateElement("p:cNvCxnSpPr")
	nv.CreateElement("p:nvPr")
	sp := cx.CreateElement("p:spPr")
	xf := aEl(sp, "xfrm")
	if x2 < x1 {
		xf.CreateAttr("flipH", "1")
	}
	if y2 < y1 {
		xf.CreateAttr("flipV", "1")
	}
	aEl(xf, "off", "x", ri(math.Min(x1, x2)), "y", ri(math.Min(y1, y2)))
	aEl(xf, "ext", "cx", ri(math.Abs(x2-x1)), "cy", ri(math.Abs(y2-y1)))
	aEl(aEl(sp, "prstGeom", "prst", "line"), "avLst")
	ln := aEl(sp, "ln", "w", width)
	aEl(aEl(ln, "solidFill"), "srgbClr", "val", strings.ToUpper(colour))
	previewShapeStyle(cx, "2", "0", "1", "tx1")
	c.shapes = append(c.shapes, cx)
}

type textStyle struct {
	size   float64
	bold   bool
	italic *bool
	font   string
	colour string
	center bool
	vert   string
}

// textbox mirrors reference presentation library add_textbox with a single paragraph.
func (c *slideCanvas) textbox(x, y, w, h float64, text string, st textStyle) {
	sp := pEl("sp")
	nv := sp.CreateElement("p:nvSpPr")
	nv.CreateElement("p:cNvPr").CreateAttr("id", "0")
	nv.SelectElement("p:cNvPr").CreateAttr("name", fmt.Sprintf("TextBox %d", len(c.shapes)+1))
	nv.CreateElement("p:cNvSpPr").CreateAttr("txBox", "1")
	nv.CreateElement("p:nvPr")
	props := sp.CreateElement("p:spPr")
	xf := aEl(props, "xfrm")
	aEl(xf, "off", "x", ri(x), "y", ri(y))
	aEl(xf, "ext", "cx", ri(w), "cy", ri(h))
	aEl(aEl(props, "prstGeom", "prst", "rect"), "avLst")
	aEl(props, "noFill")
	body := sp.CreateElement("p:txBody")
	bp := aEl(body, "bodyPr", "wrap", "none", "lIns", ri(2*ptEMU), "rIns", ri(2*ptEMU), "tIns", ri(ptEMU), "bIns", ri(ptEMU))
	if st.vert != "" {
		bp.CreateAttr("vert", st.vert)
	}
	aEl(bp, "spAutoFit")
	aEl(body, "lstStyle")
	para := aEl(body, "p")
	if st.center {
		aEl(para, "pPr", "algn", "ctr")
	}
	run := aEl(para, "r")
	rp := etree.NewElement("a:defRPr")
	rp.CreateAttr("sz", ri(st.size*100))
	rp.CreateAttr("b", map[bool]string{true: "1", false: "0"}[st.bold])
	if st.italic != nil {
		rp.CreateAttr("i", map[bool]string{true: "1", false: "0"}[*st.italic])
	}
	aEl(aEl(rp, "solidFill"), "srgbClr", "val", strings.ToUpper(st.colour))
	if st.font != "" {
		aEl(rp, "latin", "typeface", st.font)
	}
	// Reference label_box sets paragraph.font, leaving runs to inherit.
	// Do not impose a language or duplicate direct run formatting.
	paragraphProps := para.SelectElement("a:pPr")
	if paragraphProps == nil {
		paragraphProps = aEl(para, "pPr")
		para.RemoveChild(paragraphProps)
		para.InsertChildAt(0, paragraphProps)
	}
	paragraphProps.AddChild(rp)
	aEl(run, "t").SetText(text)
	c.shapes = append(c.shapes, sp)
}

type chartExPainter struct {
	canvas  slideCanvas
	theme   map[string]string
	palette []string
}

func (pp *chartExPainter) saved(e *etree.Element, def string) string {
	if e == nil {
		return def
	}
	var colour *etree.Element
	for _, c := range e.ChildElements() {
		if Is(c, NSA, "srgbClr") || Is(c, NSA, "schemeClr") || Is(c, NSA, "sysClr") {
			colour = c
			break
		}
	}
	if colour == nil {
		return def
	}
	v := colour.SelectAttrValue("val", "")
	switch {
	case Is(colour, NSA, "schemeClr"):
		alias := map[string]string{"tx1": "dk1", "bg1": "lt1", "tx2": "dk2", "bg2": "lt2"}
		if a, ok := alias[v]; ok {
			v = a
		}
		v = pp.theme[v]
	case Is(colour, NSA, "sysClr"):
		v = colour.SelectAttrValue("lastClr", "")
	}
	if !hex6.MatchString(v) {
		return def
	}
	ch := make([]float64, 3)
	for i := range ch {
		n, _ := strconv.ParseUint(v[i*2:i*2+2], 16, 8)
		ch[i] = float64(n) / 255
	}
	for _, t := range colour.ChildElements() {
		amount, _ := strconv.Atoi(t.SelectAttrValue("val", "0"))
		a := float64(amount) / 100000
		for i := range ch {
			switch {
			case Is(t, NSA, "tint"):
				ch[i] = ch[i] + (1-ch[i])*a
			case Is(t, NSA, "shade"), Is(t, NSA, "lumMod"):
				ch[i] *= a
			case Is(t, NSA, "lumOff"):
				ch[i] += a
			}
		}
	}
	return fmt.Sprintf("%02X%02X%02X", int(math.Round(math.Min(1, math.Max(0, ch[0]))*255)), int(math.Round(math.Min(1, math.Max(0, ch[1]))*255)), int(math.Round(math.Min(1, math.Max(0, ch[2]))*255)))
}

// label mirrors label_box: tiny boxes are skipped and saved run properties win.
func (pp *chartExPainter) label(x, y, w, h float64, text string, style *etree.Element, size float64, center bool, colour string, bold bool) bool {
	if w < 250000 || h < 130000 {
		return false
	}
	st := textStyle{size: size, bold: bold, colour: colour, center: center}
	var props *etree.Element
	if style != nil {
		props = firstDesc(style, NSA, "defRPr")
		if props == nil {
			props = firstDesc(style, NSA, "rPr")
		}
	}
	if props != nil {
		if s := props.SelectAttrValue("sz", ""); s != "" {
			if n, err := strconv.Atoi(s); err == nil {
				st.size = float64(n) / 100
			}
		}
		if b, ok := HasAttr(props, "b"); ok {
			st.bold = b == "1" || b == "true"
		}
		if i, ok := HasAttr(props, "i"); ok {
			v := i == "1" || i == "true"
			st.italic = &v
		}
		if f := Child(props, NSA, "latin"); f != nil {
			st.font = f.SelectAttrValue("typeface", "")
		}
		st.colour = pp.saved(Child(props, NSA, "solidFill"), "000000")
	}
	pp.canvas.textbox(x, y, w, h, text, st)
	return true
}

func firstDesc(e *etree.Element, ns, local string) *etree.Element {
	var found *etree.Element
	Iter(e, func(x *etree.Element) bool {
		if found == nil && x != e && Is(x, ns, local) {
			found = x
		}
		return found == nil
	})
	return found
}

func allText(e *etree.Element) string {
	var b strings.Builder
	Iter(e, func(x *etree.Element) bool {
		for _, t := range x.Child {
			if cd, ok := t.(*etree.CharData); ok {
				b.WriteString(cd.Data)
			}
		}
		return true
	})
	return b.String()
}

func gFmt(v float64) string { return pyG(v, 6) }

func chartExPreview(set *editSet, charts []string) error {
	p := set.p
	reports := map[string]*ChartExReport{}
	for _, n := range charts {
		doc, err := p.XML(n)
		if err != nil {
			return err
		}
		if Is(doc.Root(), NSCX, "chartSpace") {
			r, err := VerifyExtendedChart(doc.Root(), n, p)
			if err != nil {
				return err
			}
			reports[n] = r
		}
	}
	slides, err := PresentationSlides(p)
	if err != nil {
		return err
	}
	for _, part := range slides {
		doc, err := set.get(part)
		if err != nil {
			return err
		}
		root := doc.Root()
		changed := false
		if v := root.SelectAttrValue("show", ""); v == "0" || v == "false" {
			root.CreateAttr("show", "1")
			changed = true
		}
		frames := All(root, NSP, "graphicFrame")
		for _, frame := range frames {
			chart := firstDesc(frame, NSCX, "chart")
			if chart == nil {
				continue
			}
			changed = true
			cp, err := RelTarget(p, part, AttrV(chart, NSR, "id"))
			if err != nil {
				return err
			}
			report := reports[cp]
			if report == nil {
				return errors.New("missing ChartEx report for " + cp)
			}
			supported := true
			for _, s := range report.Series {
				if !slices.Contains([]string{"treemap", "sunburst", "histogram", "waterfall", "funnel"}, s.Type) {
					supported = false
				}
			}
			if !supported {
				continue
			}
			if len(report.Series) != 1 {
				return errors.New("extended chart preview needs one series")
			}
			chartDoc, err := p.XML(cp)
			if err != nil {
				return err
			}
			shapes, err := paintChartEx(p, part, frame, chartDoc.Root(), report.Series[0])
			if err != nil {
				return err
			}
			parent := frame.Parent()
			pos := frame.Index()
			parent.RemoveChild(frame)
			ident := 0
			Iter(root, func(e *etree.Element) bool {
				if Is(e, NSP, "cNvPr") {
					if n, err := strconv.Atoi(e.SelectAttrValue("id", "0")); err == nil && n > ident {
						ident = n
					}
				}
				return true
			})
			pfx := nsPrefix(root, NSP)
			apfx := nsPrefix(root, NSA)
			if apfx == "" {
				root.CreateAttr("xmlns:a", NSA)
				apfx = "a"
			}
			for _, sh := range shapes {
				ident++
				Iter(sh, func(e *etree.Element) bool {
					switch e.Space {
					case "p":
						e.Space = pfx
					case "a":
						e.Space = apfx
					}
					if e.Tag == "cNvPr" {
						e.CreateAttr("id", strconv.Itoa(ident))
					}
					return true
				})
				parent.InsertChildAt(pos, sh)
				pos++
			}
		}
		if !changed {
			delete(set.docs, part)
		}
	}
	return nil
}

func paintChartEx(p *opc.Package, part string, frame, chartRoot *etree.Element, series ChartExSeries) ([]*etree.Element, error) {
	pp := &chartExPainter{theme: map[string]string{}}
	owner := part
	for _, kind := range []string{"slideLayout", "slideMaster", "theme"} {
		rels, err := p.Rels(owner)
		if err != nil {
			return nil, err
		}
		next := ""
		for _, r := range rels {
			if strings.HasSuffix(r.Type, "/"+kind) {
				t, err := RelTarget(p, owner, r.ID)
				if err != nil {
					return nil, err
				}
				next = t
				break
			}
		}
		if next == "" {
			break
		}
		owner = next
		if kind == "theme" {
			doc, err := p.XML(owner)
			if err != nil {
				return nil, err
			}
			for _, e := range Child(Child(doc.Root(), NSA, "themeElements"), NSA, "clrScheme").ChildElements() {
				if c := e.ChildElements(); len(c) > 0 {
					v := c[0].SelectAttrValue("lastClr", "")
					if v == "" {
						v = c[0].SelectAttrValue("val", "")
					}
					pp.theme[e.Tag] = v
				}
			}
		}
	}
	for i, fb := range []string{"4472C4", "ED7D31", "A5A5A5", "FFC000", "5B9BD5", "70AD47"} {
		v := pp.theme["accent"+strconv.Itoa(i+1)]
		if v == "" {
			v = fb
		}
		pp.palette = append(pp.palette, v)
	}
	return pp.paint(frame, chartRoot, series)
}
