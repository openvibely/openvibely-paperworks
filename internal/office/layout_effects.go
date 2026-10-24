package office

import "github.com/beevik/etree"

// Effects are native parameters, not a claim of raster-equivalent bounds.
func (p layoutPalette) shapeEffects(chain []*etree.Element) map[string]any {
	for _, shape := range chain {
		pr := Child(shape, NSP, "spPr")
		if shape.Tag == "grpSp" {
			pr = Child(shape, NSP, "grpSpPr")
		}
		if list := Child(pr, NSA, "effectLst"); list != nil {
			return p.effects(list)
		}
		if dag := Child(pr, NSA, "effectDag"); dag != nil {
			return map[string]any{"resolved": false, "xml": xmlString(dag), "reason": "effect graph requires renderer evaluation"}
		}
		ref := Child(Child(shape, NSP, "style"), NSA, "effectRef")
		if ref == nil {
			continue
		}
		idx := int(emuAttr(ref, "idx", 0))
		if idx == 0 {
			return map[string]any{"resolved": true, "effects": []map[string]any{}}
		}
		list := Child(Child(Child(p.theme, NSA, "themeElements"), NSA, "fmtScheme"), NSA, "effectStyleLst")
		entries := Children(list, NSA, "effectStyle")
		if idx < 1 || idx > len(entries) {
			return map[string]any{"resolved": false, "reference_xml": xmlString(ref)}
		}
		entry := copyLayoutPaintElement(entries[idx-1])
		for _, ph := range All(entry, NSA, "schemeClr") {
			if AttrOr(ph, "val", "") != "phClr" {
				continue
			}
			for _, c := range ref.ChildElements() {
				if c.NamespaceURI() != NSA {
					continue
				}
				switch c.Tag {
				case "srgbClr", "schemeClr", "sysClr", "prstClr", "hslClr", "scrgbClr":
					replacement := copyLayoutPaintElement(c)
					for _, tr := range ph.ChildElements() {
						replacement.AddChild(copyLayoutPaintElement(tr))
					}
					par := ph.Parent()
					at := ph.Index()
					par.RemoveChild(ph)
					par.InsertChildAt(at, replacement)
				}
			}
		}
		if effects := Child(entry, NSA, "effectLst"); effects != nil {
			return p.effects(effects)
		}
		return map[string]any{"resolved": false, "xml": xmlString(entry), "reason": "theme effect has no supported effect list"}
	}
	return nil
}
func (p layoutPalette) effects(list *etree.Element) map[string]any {
	effects := []map[string]any{}
	resolved := true
	for _, e := range list.ChildElements() {
		r := map[string]any{"kind": e.Tag, "properties": layoutAttributes(e), "xml": xmlString(e)}
		known := e.NamespaceURI() == NSA
		switch e.Tag {
		case "glow", "innerShdw", "outerShdw", "prstShdw":
			color := p.color(e)
			r["color"] = color
			known = known && color["resolved"] == true
		case "blur", "softEdge", "reflection":
		default:
			known = false
		}
		r["resolved"] = known
		resolved = resolved && known
		effects = append(effects, r)
	}
	return map[string]any{"resolved": resolved, "effects": effects, "units": "native DrawingML: distances EMU, angles 60000ths of a degree, percentages 100000ths"}
}
