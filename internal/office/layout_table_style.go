package office

import (
	"fmt"
	"github.com/beevik/etree"
)

// Native properties remain explicit: absent values are not fabricated defaults.
func layoutAttributes(e *etree.Element) map[string]string {
	out := map[string]string{}
	if e != nil {
		for _, a := range e.Attr {
			if a.Space == "" && a.Key != "xmlns" {
				out[a.Key] = a.Value
			}
		}
	}
	return out
}
func layoutTableFormatting(cell *etree.Element, palette layoutPalette, textDefaults ...*etree.Element) map[string]any {
	body := Child(cell, NSA, "txBody")
	pr := Child(cell, NSA, "tcPr")
	result := map[string]any{"properties": layoutAttributes(pr), "body_properties": layoutAttributes(Child(body, NSA, "bodyPr"))}
	for _, tag := range []string{"solidFill", "gradFill", "pattFill", "blipFill", "noFill", "grpFill"} {
		if f := Child(pr, NSA, tag); f != nil {
			result["fill"] = palette.fill(f)
			break
		}
	}
	borders := map[string]any{}
	for _, tag := range []string{"lnL", "lnR", "lnT", "lnB", "lnTlToBr", "lnBlToTr"} {
		if line := Child(pr, NSA, tag); line != nil {
			b := map[string]any{"properties": layoutAttributes(line), "xml": xmlString(line)}
			for _, fill := range []string{"solidFill", "gradFill", "noFill", "pattFill"} {
				if f := Child(line, NSA, fill); f != nil {
					b["fill"] = palette.fill(f)
					break
				}
			}
			borders[tag] = b
		}
	}
	result["borders"] = borders
	var paragraphs []map[string]any
	for _, par := range Children(body, NSA, "p") {
		pp := Child(par, NSA, "pPr")
		level := int(emuAttr(pp, "lvl", 0))
		list := Child(body, NSA, "lstStyle")
		defaults := []*etree.Element{Child(pp, NSA, "defRPr"), Child(Child(list, NSA, fmt.Sprintf("lvl%dpPr", level+1)), NSA, "defRPr"), Child(Child(list, NSA, "defPPr"), NSA, "defRPr")}
		defaults = append(defaults, textDefaults...)
		var runs []map[string]any
		for _, run := range par.ChildElements() {
			if !Is(run, NSA, "r") && !Is(run, NSA, "fld") && !Is(run, NSA, "br") {
				continue
			}
			style := map[string]any{}
			attrs := map[string]string{}
			sources := append([]*etree.Element{Child(run, NSA, "rPr")}, defaults...)
			for _, src := range sources {
				for k, v := range layoutAttributes(src) {
					if _, ok := attrs[k]; !ok {
						attrs[k] = v
					}
				}
				fontSource := src
				if Is(src, NSA, "tcTxStyle") {
					fontSource = Child(src, NSA, "font")
				}
				for _, font := range []string{"latin", "ea", "cs", "sym"} {
					if _, ok := style[font]; !ok {
						if e := Child(fontSource, NSA, font); e != nil {
							style[font] = layoutAttributes(e)
						}
					}
				}
				if style["fill"] == nil {
					for _, tag := range []string{"solidFill", "gradFill", "noFill", "pattFill"} {
						if f := Child(src, NSA, tag); f != nil {
							style["fill"] = palette.fill(f)
							break
						}
					}
				}
			}
			style["properties"] = attrs
			text := Text(run, NSA, "t")
			if run.Tag == "br" {
				text = "\n"
			}
			runs = append(runs, map[string]any{"kind": run.Tag, "text": text, "style": style, "properties": layoutAttributes(run)})
		}
		paragraphs = append(paragraphs, map[string]any{"text": paraText(par), "properties": layoutAttributes(pp), "runs": runs, "end_properties": layoutAttributes(Child(par, NSA, "endParaRPr"))})
	}
	result["paragraphs"] = paragraphs
	// Table-style regions and renderer defaults may contribute additional styling.
	result["style_scope"] = "explicit cell properties and local text defaults"
	return result
}
