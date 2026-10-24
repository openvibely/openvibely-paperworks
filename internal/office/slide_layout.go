package office

import (
	"fmt"
	"github.com/beevik/etree"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"strconv"
	"strings"
)

// PresentationLayouts returns native object metadata in logical 96-DPI pixels.
// It does not infer glyph positions from raster images. Unresolved properties
// remain absent, with the original XML retained for inspection.
func PresentationLayouts(p *opc.Package) ([]map[string]any, error) {
	slides, err := PresentationSlides(p)
	if err != nil {
		return nil, err
	}
	main := ""
	rels, err := p.Rels("")
	if err != nil {
		return nil, err
	}
	for _, r := range rels {
		if r.Type == NSR+"/officeDocument" {
			main = opc.Resolve("", r.Target)
		}
	}
	if main == "" {
		return nil, fmt.Errorf("missing presentation relationship")
	}
	pres, err := p.XML(main)
	if err != nil {
		return nil, err
	}
	size := Child(pres.Root(), NSP, "sldSz")
	width, height := emuAttr(size, "cx", 0)/9525, emuAttr(size, "cy", 0)/9525
	if width <= 0 || height <= 0 {
		return nil, fmt.Errorf("invalid slide size")
	}
	var reports []map[string]any
	for i, part := range slides {
		owners := []string{part}
		scopes := []string{"slide"}
		for _, kind := range []string{"slideLayout", "slideMaster"} {
			next := relOfType(p, owners[len(owners)-1], kind)
			if next == "" {
				break
			}
			owners = append(owners, next)
			scopes = append(scopes, kind)
		}
		var roots []*etree.Element
		for _, owner := range owners {
			d, e := p.XML(owner)
			if e != nil {
				return nil, e
			}
			roots = append(roots, d.Root())
		}
		themeFonts := map[string]string{}
		var themeRoot *etree.Element
		if len(owners) == 3 {
			if tp := relOfType(p, owners[2], "theme"); tp != "" {
				td, e := p.XML(tp)
				if e != nil {
					return nil, e
				}
				themeRoot = td.Root()
				fs := Child(Child(td.Root(), NSA, "themeElements"), NSA, "fontScheme")
				for _, f := range []struct{ tag, token string }{{"majorFont", "+mj-lt"}, {"minorFont", "+mn-lt"}} {
					themeFonts[f.token] = AttrOr(Child(Child(fs, NSA, f.tag), NSA, "latin"), "typeface", "")
				}
			}
		}
		palette := newLayoutPalette(themeRoot, roots)
		var elements []map[string]any
		var layers []map[string]any
		for j, root := range roots {
			tree := Child(Child(root, NSP, "cSld"), NSP, "spTree")
			if j > 0 {
				layers = append(layers, map[string]any{"id": owners[j], "scope": scopes[j], "editable": false, "xml": xmlString(root)})
			}
			var walk func(*etree.Element, bool)
			walk = func(parent *etree.Element, grouped bool) {
				if parent == nil {
					return
				}
				for _, sh := range parent.ChildElements() {
					if sh.NamespaceURI() != NSP {
						continue
					}
					switch sh.Tag {
					case "sp", "pic", "cxnSp", "graphicFrame", "grpSp":
					default:
						continue
					}
					var nv *etree.Element
					for _, child := range sh.ChildElements() {
						if strings.HasPrefix(child.Tag, "nv") {
							nv = Child(child, NSP, "cNvPr")
							break
						}
					}
					id := AttrOr(nv, "id", "")
					record := map[string]any{"id": owners[j] + "#" + id, "shape_id": id, "part": owners[j], "name": AttrOr(nv, "name", ""), "kind": sh.Tag, "scope": scopes[j], "editable": j == 0, "xml": xmlString(sh)}
					if j == 0 {
						record["edit_target"] = map[string]any{"slide": i + 1, "shape_id": id}
					}
					var chain []*etree.Element
					chain = append(chain, sh)
					for k := j + 1; k < len(roots); k++ {
						if match := layoutPlaceholder(roots[k], chain[len(chain)-1], k == 2); match != nil {
							chain = append(chain, match)
						}
					}
					var xf *etree.Element
					for _, candidate := range chain {
						xf = Child(Child(candidate, NSP, "spPr"), NSA, "xfrm")
						if xf == nil {
							xf = Child(candidate, NSP, "xfrm")
						}
						if xf != nil {
							break
						}
					}
					if grouped || sh.Tag == "grpSp" {
						record["position_unresolved"] = "group transform; inspect native XML"
					} else if xf != nil {
						off, ext := Child(xf, NSA, "off"), Child(xf, NSA, "ext")
						if off != nil && ext != nil {
							record["position"] = map[string]float64{"left": emuAttr(off, "x", 0) / 9525, "top": emuAttr(off, "y", 0) / 9525, "width": emuAttr(ext, "cx", 0) / 9525, "height": emuAttr(ext, "cy", 0) / 9525}
							record["rotation"] = emuAttr(xf, "rot", 0) / 60000
							record["flip_h"] = Truthy(xf.SelectAttrValue("flipH", "0"), false)
							record["flip_v"] = Truthy(xf.SelectAttrValue("flipV", "0"), false)
						}
					}
					body := Child(sh, NSP, "txBody")
					var paras []map[string]any
					var text []string
					for _, par := range Children(body, NSA, "p") {
						text = append(text, paraText(par))
						pp := Child(par, NSA, "pPr")
						level := int(emuAttr(pp, "lvl", 0))
						var defaults []*etree.Element
						defaults = append(defaults, Child(pp, NSA, "defRPr"))
						for ci, candidate := range chain {
							if ci > 0 {
								for _, cp := range Children(Child(candidate, NSP, "txBody"), NSA, "p") {
									cpp := Child(cp, NSA, "pPr")
									if int(emuAttr(cpp, "lvl", 0)) == level {
										defaults = append(defaults, Child(cpp, NSA, "defRPr"))
										break
									}
								}
							}
							lst := Child(Child(candidate, NSP, "txBody"), NSA, "lstStyle")
							defaults = append(defaults, Child(Child(lst, NSA, fmt.Sprintf("lvl%dpPr", level+1)), NSA, "defRPr"), Child(Child(lst, NSA, "defPPr"), NSA, "defRPr"))
						}
						typ, _, _ := placeholderKey(sh)
						styleName := "otherStyle"
						if typ == "title" || typ == "ctrTitle" {
							styleName = "titleStyle"
						} else if typ == "body" {
							styleName = "bodyStyle"
						}
						if len(roots) == 3 {
							defaults = append(defaults, Child(Child(Child(roots[2], NSP, "txStyles"), NSP, styleName), NSA, fmt.Sprintf("lvl%dpPr", level+1)))
						}
						// Master level entries hold defRPr, unlike the earlier entries.
						for k, d := range defaults {
							if d != nil && d.Tag != "defRPr" {
								defaults[k] = Child(d, NSA, "defRPr")
							}
						}
						defaults = append(defaults, Child(Child(Child(pres.Root(), NSP, "defaultTextStyle"), NSA, fmt.Sprintf("lvl%dpPr", level+1)), NSA, "defRPr"))
						var runs []map[string]any
						for _, run := range par.ChildElements() {
							if !Is(run, NSA, "r") && !Is(run, NSA, "fld") {
								continue
							}
							ds := append([]*etree.Element{Child(run, NSA, "rPr")}, defaults...)
							style := map[string]any{}
							for _, d := range ds {
								if d == nil {
									continue
								}
								for _, a := range d.Attr {
									if _, ok := style[a.Key]; !ok {
										style[a.Key] = a.Value
									}
								}
								if _, ok := style["font_family"]; !ok {
									if latin := Child(d, NSA, "latin"); latin != nil {
										style["font_family"] = latin.SelectAttrValue("typeface", "")
									}
								}
								if _, ok := style["fill_xml"]; !ok {
									for _, tag := range []string{"solidFill", "gradFill", "noFill"} {
										if fill := Child(d, NSA, tag); fill != nil {
											style["fill_xml"] = xmlString(fill)
											style["fill"] = palette.fill(fill)
											break
										}
									}
								}
							}
							if sz, ok := style["sz"].(string); ok {
								if n, e := strconv.ParseFloat(sz, 64); e == nil {
									style["font_size_px"] = n / 100 * 96 / 72
								}
							}
							if family, ok := style["font_family"].(string); ok && themeFonts[family] != "" {
								style["font_token"] = family
								style["font_family"] = themeFonts[family]
							}
							runs = append(runs, map[string]any{"text": Text(run, NSA, "t"), "style": style})
						}
						paras = append(paras, map[string]any{"text": paraText(par), "runs": runs, "paragraph_xml": layoutXML(pp)})
					}
					record["fill"] = palette.shapeFill(chain)
					record["text"] = strings.Join(text, "\n")
					record["paragraphs"] = paras
					for _, candidate := range chain {
						pr := Child(candidate, NSP, "spPr")
						for _, tag := range []string{"solidFill", "gradFill", "pattFill", "blipFill", "noFill"} {
							if f := Child(pr, NSA, tag); f != nil {
								record["fill_xml"] = xmlString(f)
								break
							}
						}
						if record["fill_xml"] != nil {
							break
						}
					}
					if pic := Child(sh, NSP, "blipFill"); pic != nil {
						record["image_xml"] = xmlString(pic)
						layoutImage(record, sh, pic)
					}
					if frame := Child(Child(sh, NSA, "graphic"), NSA, "graphicData"); frame != nil {
						record["content_xml"] = xmlString(frame)
						layoutStyledTable(record, Child(frame, NSA, "tbl"), palette)
					}
					elements = append(elements, record)
					if sh.Tag == "grpSp" {
						walk(sh, true)
					}
				}
			}
			walk(tree, false)
		}
		reports = append(reports, map[string]any{"schema": "paperworks.slide-layout/v1", "slide": i + 1, "part": part, "units": "px", "pixels_per_inch": 96, "position": map[string]float64{"left": 0, "top": 0, "width": width, "height": height}, "elements": elements, "inherited_layers": layers, "limitations": []string{"Native geometry and cascading text properties, not raster-measured glyph bounds.", "Theme tokens and paint XML are retained; renderer font substitution, theme paint effects and group transforms are not resolved.", "Inherited records describe source layers, including placeholders; they are not a second set of visible slide objects."}})
	}
	return reports, nil
}

func layoutPlaceholder(root, shape *etree.Element, master bool) *etree.Element {
	var ph *etree.Element
	for _, n := range All(shape, NSP, "ph") {
		ph = n
		break
	}
	if ph == nil {
		return nil
	}
	typ, idx := ph.SelectAttrValue("type", "obj"), ph.SelectAttrValue("idx", "0")
	for _, candidate := range All(root, NSP, "sp") {
		other := Child(Child(Child(candidate, NSP, "nvSpPr"), NSP, "nvPr"), NSP, "ph")
		if other == nil {
			continue
		}
		ot := other.SelectAttrValue("type", "obj")
		if master {
			if ot == typ || (typ == "ctrTitle" && ot == "title") {
				return candidate
			}
		} else if (typ == "title" || typ == "ctrTitle") && (ot == "title" || ot == "ctrTitle") {
			return candidate
		} else if typ != "title" && typ != "ctrTitle" && ot != "title" && ot != "ctrTitle" && other.SelectAttrValue("idx", "0") == idx {
			return candidate
		}
	}
	return nil
}

func layoutXML(e *etree.Element) string {
	if e == nil {
		return ""
	}
	return xmlString(e)
}
