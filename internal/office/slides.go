package office

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/beevik/etree"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
)

// textShape is a p:sp with a text body and its effective geometry (EMU).
type textShape struct {
	el                     *etree.Element
	id                     string
	parent                 *etree.Element
	x, y, w, h             float64
	lIns, rIns, tIns, bIns float64
	rotated, title         bool
	body                   *etree.Element
}

func relOfType(p *opc.Package, owner, kind string) string {
	rels, err := p.Rels(owner)
	if err != nil {
		return ""
	}
	for _, r := range rels {
		if strings.HasSuffix(r.Type, "/"+kind) && r.TargetMode != "External" {
			return opc.Resolve(owner, r.Target)
		}
	}
	return ""
}

func placeholderKey(sp *etree.Element) (string, string, bool) {
	ph := Child(Child(Child(sp, NSP, "nvSpPr"), NSP, "nvPr"), NSP, "ph")
	if ph == nil {
		return "", "", false
	}
	return ph.SelectAttrValue("type", "body"), ph.SelectAttrValue("idx", "0"), true
}

// inheritedXfrm finds placeholder geometry on the layout, then the master.
func inheritedXfrm(p *opc.Package, slide string, sp *etree.Element) *etree.Element {
	typ, idx, ok := placeholderKey(sp)
	if !ok {
		return nil
	}
	owner := slide
	for _, kind := range []string{"slideLayout", "slideMaster"} {
		owner = relOfType(p, owner, kind)
		if owner == "" {
			return nil
		}
		doc, err := p.XML(owner)
		if err != nil {
			return nil
		}
		var byIdx, byType *etree.Element
		for _, o := range All(doc.Root(), NSP, "sp") {
			t, i, ok := placeholderKey(o)
			if !ok {
				continue
			}
			x := Child(Child(o, NSP, "spPr"), NSA, "xfrm")
			if x == nil {
				continue
			}
			if i == idx && idx != "0" && byIdx == nil {
				byIdx = x
			}
			if t == typ && byType == nil {
				byType = x
			}
		}
		if byIdx != nil {
			return byIdx
		}
		if byType != nil {
			return byType
		}
	}
	return nil
}

func emuAttr(e *etree.Element, key string, def float64) float64 {
	if e == nil {
		return def
	}
	v, err := strconv.ParseFloat(e.SelectAttrValue(key, ""), 64)
	if err != nil {
		return def
	}
	return v
}

func slideTextShapes(p *opc.Package, slide string, root *etree.Element) []*textShape {
	var out []*textShape
	for _, sp := range All(root, NSP, "sp") {
		body := Child(sp, NSP, "txBody")
		if body == nil {
			continue
		}
		xfrm := Child(Child(sp, NSP, "spPr"), NSA, "xfrm")
		if xfrm == nil {
			xfrm = inheritedXfrm(p, slide, sp)
		}
		off, ext := Child(xfrm, NSA, "off"), Child(xfrm, NSA, "ext")
		bp := Child(body, NSA, "bodyPr")
		typ, _, isPh := placeholderKey(sp)
		cnv := Child(Child(sp, NSP, "nvSpPr"), NSP, "cNvPr")
		out = append(out, &textShape{
			el: sp, id: AttrOr(cnv, "id", ""), parent: sp.Parent(),
			x: emuAttr(off, "x", 0), y: emuAttr(off, "y", 0), w: emuAttr(ext, "cx", 0), h: emuAttr(ext, "cy", 0),
			lIns: emuAttr(bp, "lIns", 91440), rIns: emuAttr(bp, "rIns", 91440),
			tIns: emuAttr(bp, "tIns", 45720), bIns: emuAttr(bp, "bIns", 45720),
			rotated: emuAttr(xfrm, "rot", 0) != 0,
			title:   isPh && (typ == "title" || typ == "ctrTitle"),
			body:    body,
		})
	}
	return out
}

func paraText(par *etree.Element) string {
	var b strings.Builder
	for _, c := range par.ChildElements() {
		switch {
		case Is(c, NSA, "r") || Is(c, NSA, "fld"):
			b.WriteString(Text(c, NSA, "t"))
		case Is(c, NSA, "br"):
			b.WriteString("\v")
		}
	}
	return b.String()
}

func shapeText(s *textShape) string {
	var parts []string
	for _, par := range Children(s.body, NSA, "p") {
		parts = append(parts, paraText(par))
	}
	return strings.Join(parts, "\n")
}

func ptAttr(e *etree.Element, key string) (float64, bool) {
	if e == nil {
		return 0, false
	}
	v, err := strconv.ParseFloat(e.SelectAttrValue(key, ""), 64)
	return v / 100, err == nil
}

func spacingPt(pPr *etree.Element, name string) (float64, bool) {
	sp := Child(Child(pPr, NSA, name), NSA, "spcPts")
	return ptAttr(sp, "val")
}

type runFont struct {
	size         float64
	family       string
	bold, italic bool
	text         string
}

func paragraphRuns(par *etree.Element) []runFont {
	ppr := Child(par, NSA, "pPr")
	def := Child(ppr, NSA, "defRPr")
	var out []runFont
	for _, r := range Children(par, NSA, "r") {
		rpr := Child(r, NSA, "rPr")
		size, ok := ptAttr(rpr, "sz")
		if !ok {
			size, _ = ptAttr(def, "sz")
		}
		family := AttrOr(Child(rpr, NSA, "latin"), "typeface", "")
		if family == "" {
			family = AttrOr(Child(def, NSA, "latin"), "typeface", "")
		}
		flag := func(k string) bool {
			if v, ok := HasAttr(rpr, k); ok {
				return Truthy(v, false)
			}
			v, _ := HasAttr(def, k)
			return Truthy(v, false)
		}
		out = append(out, runFont{size, family, flag("b"), flag("i"), Text(r, NSA, "t")})
	}
	return out
}

var tokenRe = regexp.MustCompile(`\s+|\S+`)

// measuredHeight wraps text with actual glyph advances of exactly matching
// installed fonts; it returns false when any font is unknown.
func measuredHeight(s *textShape) (float64, bool) {
	avail := (s.w - s.lIns - s.rIns) / 12700
	if avail <= 0 {
		return 0, false
	}
	wrap := AttrOr(Child(s.body, NSA, "bodyPr"), "wrap", "square") != "none"
	total := 0.0
	for _, par := range Children(s.body, NSA, "p") {
		width, line, height := 0.0, 0.0, 0.0
		for _, r := range paragraphRuns(par) {
			if r.text == "" {
				continue
			}
			if r.size == 0 || r.family == "" || strings.HasPrefix(r.family, "+") {
				return 0, false
			}
			f := FindFont(r.family, r.bold, r.italic)
			if f == nil {
				return 0, false
			}
			for _, tok := range tokenRe.FindAllString(r.text, -1) {
				length := Advance(f, tok, r.size)
				if wrap && width > 0 && width+length > avail && strings.TrimSpace(tok) != "" {
					height += line
					line, width = 0, 0
				}
				for _, ch := range tok {
					adv := Advance(f, string(ch), r.size)
					if wrap && width > 0 && width+adv > avail {
						height += line
						line, width = 0, 0
					}
					width += adv
					line = max(line, r.size)
				}
			}
		}
		height += line
		ppr := Child(par, NSA, "pPr")
		if ln := Child(ppr, NSA, "lnSpc"); ln != nil {
			if pct := Child(ln, NSA, "spcPct"); pct != nil {
				v, _ := strconv.ParseFloat(pct.SelectAttrValue("val", "100000"), 64)
				height *= v / 100000
			} else if pts, ok := ptAttr(Child(ln, NSA, "spcPts"), "val"); ok && line > 0 {
				height = height / line * pts
			}
		}
		total += height
		if v, ok := spacingPt(ppr, "spcBef"); ok {
			total += v
		}
		if v, ok := spacingPt(ppr, "spcAft"); ok {
			total += v
		}
	}
	return total, true
}

// PresentationTextChecks checks native bullets and explicit heading
// geometry; inherited or unmatched fonts are reported for rendered review.
func PresentationTextChecks(p *opc.Package, slide string, root *etree.Element, number int) ([]string, []map[string]any, []map[string]any) {
	var failures []string
	var headings, unknown []map[string]any
	shapes := slideTextShapes(p, slide, root)
	for _, s := range shapes {
		usableW := s.w - s.lIns - s.rIns
		var explicit []float64
		for _, par := range Children(s.body, NSA, "p") {
			ppr := Child(par, NSA, "pPr")
			if ppr != nil {
				marker, auto := Child(ppr, NSA, "buChar"), Child(ppr, NSA, "buAutoNum")
				if marker != nil || auto != nil {
					if marker != nil && strings.TrimSpace(AttrOr(marker, "char", "")) == "" {
						failures = append(failures, fmt.Sprintf("Slide %d: empty native bullet marker", number))
					}
					m, i := emuAttr(ppr, "marL", 0), emuAttr(ppr, "indent", 0)
					if i >= 0 || m+i < 0 || m >= usableW {
						failures = append(failures, fmt.Sprintf("Slide %d: invalid native bullet geometry", number))
					}
				}
			}
			for _, r := range Children(par, NSA, "r") {
				if v, ok := ptAttr(Child(r, NSA, "rPr"), "sz"); ok {
					explicit = append(explicit, v)
				}
			}
			if v, ok := ptAttr(Child(ppr, NSA, "defRPr"), "sz"); ok {
				explicit = append(explicit, v)
			}
		}
		maxExplicit := 0.0
		for _, v := range explicit {
			maxExplicit = max(maxExplicit, v)
		}
		if !s.title && maxExplicit < 20 {
			continue
		}
		if strings.TrimSpace(shapeText(s)) == "" {
			continue
		}
		usableH := (s.h - s.tIns - s.bIns) / 12700
		height, measured := 0.0, true
		for _, par := range Children(s.body, NSA, "p") {
			text := paraText(par)
			if text == "" {
				continue
			}
			size := 0.0
			for _, r := range paragraphRuns(par) {
				size = max(size, r.size)
			}
			if size == 0 {
				measured = false
				continue
			}
			height += float64(len(strings.Split(text, "\v"))) * size
			ppr := Child(par, NSA, "pPr")
			if v, ok := spacingPt(ppr, "spcBef"); ok {
				height += v
			}
			if v, ok := spacingPt(ppr, "spcAft"); ok {
				height += v
			}
			if usableW <= 0 {
				failures = append(failures, fmt.Sprintf("Slide %d: heading has no usable width", number))
			}
		}
		bp := Child(s.body, NSA, "bodyPr")
		autofit := Child(bp, NSA, "normAutofit") != nil || Child(bp, NSA, "spAutoFit") != nil
		mh, ok := measuredHeight(s)
		if ok {
			height = max(height, mh)
		}
		if !autofit && height > usableH+1 {
			failures = append(failures, fmt.Sprintf("Slide %d: heading text exceeds its frame", number))
		}
		if ok && !autofit && !s.rotated {
			inkBottom := s.y + s.tIns + height*12700
			for _, o := range shapes {
				if o == s || o.parent != s.parent || o.rotated || strings.TrimSpace(shapeText(o)) == "" || o.y <= s.y {
					continue
				}
				overlap := min(s.x+s.w, o.x+o.w) - max(s.x, o.x)
				if overlap > min(s.w, o.w)/2 && inkBottom > o.y+o.tIns+12700 {
					failures = append(failures, fmt.Sprintf("Slide %d: heading text overlaps following text", number))
				}
			}
		}
		id, _ := strconv.Atoi(s.id)
		headings = append(headings, map[string]any{"shape": id, "minimum_text_height_pt": height, "height_pt": usableH})
		if !measured || !ok {
			unknown = append(unknown, map[string]any{"slide": number, "shape": id, "reason": "Wrapped or inherited-font heading requires rendered inspection"})
		}
	}
	return failures, headings, unknown
}
