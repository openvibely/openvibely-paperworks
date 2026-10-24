package office

import (
	"github.com/beevik/etree"
	"strconv"
	"strings"
)

// Resolve supported paint explicitly; never label unsupported transforms as RGB.
type layoutPalette struct {
	colors  map[string]string
	aliases map[string]string
	theme   *etree.Element
}

func newLayoutPalette(theme *etree.Element, roots []*etree.Element) layoutPalette {
	p := layoutPalette{colors: map[string]string{}, aliases: map[string]string{"tx1": "dk1", "bg1": "lt1", "tx2": "dk2", "bg2": "lt2"}, theme: theme}
	scheme := Child(Child(theme, NSA, "themeElements"), NSA, "clrScheme")
	if scheme != nil {
		for _, c := range scheme.ChildElements() {
			if kids := c.ChildElements(); len(kids) > 0 {
				p.colors[c.Tag] = AttrOr(kids[0], "lastClr", AttrOr(kids[0], "val", ""))
			}
		}
	}
	masterAliases := map[string]string{}
	for k, v := range p.aliases {
		masterAliases[k] = v
	}
	for _, root := range roots {
		if Is(root, NSP, "sldMaster") {
			if mapping := Child(root, NSP, "clrMap"); mapping != nil {
				for _, a := range mapping.Attr {
					if a.Space == "" && a.Key != "xmlns" {
						masterAliases[a.Key] = a.Value
					}
				}
			}
		}
	}
	for i := len(roots) - 1; i >= 0; i-- {
		mapping := Child(roots[i], NSP, "clrMap")
		if o := Child(roots[i], NSP, "clrMapOvr"); o != nil {
			if Child(o, NSA, "masterClrMapping") != nil {
				p.aliases = map[string]string{}
				for k, v := range masterAliases {
					p.aliases[k] = v
				}
			}
			mapping = Child(o, NSA, "overrideClrMapping")
		}
		if mapping != nil {
			for _, a := range mapping.Attr {
				if a.Space == "" && a.Key != "xmlns" {
					p.aliases[a.Key] = a.Value
				}
			}
		}
	}
	return p
}
func (p layoutPalette) color(parent *etree.Element) map[string]any {
	result := map[string]any{"resolved": false, "xml": layoutXML(parent)}
	if parent == nil {
		return result
	}
	for _, c := range parent.ChildElements() {
		if c.NamespaceURI() != NSA {
			continue
		}
		if c.Tag != "srgbClr" && c.Tag != "schemeClr" && c.Tag != "sysClr" {
			continue
		}
		v := AttrOr(c, "val", "")
		if c.Tag == "schemeClr" {
			if alias := p.aliases[v]; alias != "" {
				v = alias
			}
			v = p.colors[v]
		}
		if c.Tag == "sysClr" {
			v = AttrOr(c, "lastClr", "")
		}
		if !hex6.MatchString(v) {
			return result
		}
		alpha := 1.0
		for _, tr := range c.ChildElements() {
			if !Is(tr, NSA, "alpha") {
				return result
			}
			n, e := strconv.ParseFloat(AttrOr(tr, "val", ""), 64)
			if e != nil {
				return result
			}
			alpha = n / 100000
		}
		result["resolved"] = true
		result["rgb"] = "#" + strings.ToUpper(v)
		result["alpha"] = alpha
		return result
	}
	return result
}
func (p layoutPalette) fill(f *etree.Element) map[string]any {
	if f == nil {
		return nil
	}
	r := map[string]any{"kind": f.Tag, "xml": xmlString(f)}
	switch f.Tag {
	case "noFill":
		r["resolved"] = true
	case "solidFill":
		r["color"] = p.color(f)
	case "gradFill":
		var stops []map[string]any
		for _, s := range Children(Child(f, NSA, "gsLst"), NSA, "gs") {
			stops = append(stops, map[string]any{"position": emuAttr(s, "pos", 0) / 100000, "color": p.color(s)})
		}
		r["stops"] = stops
	case "pattFill":
		r["preset"] = AttrOr(f, "prst", "")
		r["foreground"] = p.color(Child(f, NSA, "fgClr"))
		r["background"] = p.color(Child(f, NSA, "bgClr"))
	}
	return r
}
func (p layoutPalette) shapeFill(chain []*etree.Element) map[string]any {
	for _, sh := range chain {
		pr := Child(sh, NSP, "spPr")
		for _, tag := range []string{"solidFill", "gradFill", "pattFill", "blipFill", "noFill", "grpFill"} {
			if f := Child(pr, NSA, tag); f != nil {
				return p.fill(f)
			}
		}
		ref := Child(Child(sh, NSP, "style"), NSA, "fillRef")
		if ref == nil {
			continue
		}
		idx := int(emuAttr(ref, "idx", 0))
		if idx == 0 {
			return map[string]any{"kind": "noFill", "resolved": true}
		}
		tag := "fillStyleLst"
		if idx >= 1001 {
			tag = "bgFillStyleLst"
			idx -= 1000
		}
		list := Child(Child(Child(p.theme, NSA, "themeElements"), NSA, "fmtScheme"), NSA, tag)
		if list == nil || idx < 1 || idx > len(list.ChildElements()) {
			return map[string]any{"resolved": false, "reference_xml": xmlString(ref)}
		}
		source := list.ChildElements()[idx-1]
		f := source.Copy()
		// Copies lose ancestor namespace declarations. Preserve the in-scope
		// bindings before namespace-aware traversal and serialization.
		seen := map[string]bool{}
		for ancestor := source; ancestor != nil; ancestor = ancestor.Parent() {
			for _, a := range ancestor.Attr {
				if a.Space == "xmlns" || (a.Space == "" && a.Key == "xmlns") {
					key := a.FullKey()
					if !seen[key] {
						f.CreateAttr(key, a.Value)
						seen[key] = true
					}
				}
			}
		}
		// phClr inherits the color (and its transforms) from the style reference.
		for _, ph := range All(f, NSA, "schemeClr") {
			if AttrOr(ph, "val", "") != "phClr" {
				continue
			}
			for _, c := range ref.ChildElements() {
				if strings.HasSuffix(c.Tag, "Clr") {
					replacement := c.Copy()
					for _, tr := range ph.ChildElements() {
						replacement.AddChild(tr.Copy())
					}
					par := ph.Parent()
					at := ph.Index()
					par.RemoveChild(ph)
					par.InsertChildAt(at, replacement)
					break
				}
			}
		}
		return p.fill(f)
	}
	return nil
}
