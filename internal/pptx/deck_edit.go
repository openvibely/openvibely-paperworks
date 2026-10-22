package pptx

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/beevik/etree"
	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
)

type DeckEditSpec struct {
	Edits []DeckEdit `json:"edits"`
}
type DeckEdit struct {
	Part      string   `json:"part,omitempty"`
	Path      string   `json:"path,omitempty"`
	Operation string   `json:"operation"`
	ShapeID   string   `json:"shape_id,omitempty"`
	Slide     int      `json:"slide"`
	Position  int      `json:"position,omitempty"`
	Content   *Slide   `json:"content,omitempty"`
	Element   *Element `json:"element,omitempty"`
}

// DeckEditFile uses current one-based slide positions. Imported dependency
// graphs are copied with remapped relationships, not flattened into pictures.
func DeckEditFile(ctx context.Context, input, output, base string, spec DeckEditSpec) error {
	if len(spec.Edits) == 0 || len(spec.Edits) > 1000 {
		return fmt.Errorf("edits must contain 1..1000 operations")
	}
	pkg, err := opc.Open(input)
	if err != nil {
		return err
	}
	main, err := presentationPart(pkg)
	if err != nil {
		return err
	}
	d, err := pkg.XML(main)
	if err != nil {
		return err
	}
	list := office.Child(d.Root(), pns, "sldIdLst")
	if list == nil {
		return fmt.Errorf("missing slide list")
	}
	for i, e := range spec.Edits {
		if err = ctx.Err(); err != nil {
			return err
		}
		if err = pkg.SetXML(main, d); err != nil {
			return err
		}
		slides, err := office.PresentationSlides(pkg)
		if err != nil {
			return err
		}
		ids := office.Children(list, pns, "sldId")
		if e.Operation != "add-slide" && e.Operation != "replace-theme" && (e.Slide < 1 || e.Slide > len(slides)) {
			return fmt.Errorf("edit %d: slide out of range", i)
		}
		switch e.Operation {
		case "set-layout":
			if pkg.ContentType(e.Part) != "application/vnd.openxmlformats-officedocument.presentationml.slideLayout+xml" {
				return fmt.Errorf("part must identify an existing slide layout")
			}
			relPart := opc.RelsName(slides[e.Slide-1])
			rels, err := pkg.XML(relPart)
			if err != nil {
				return err
			}
			for _, r := range rels.Root().ChildElements() {
				if r.SelectAttrValue("Type", "") == rns+"/slideLayout" {
					rels.Root().RemoveChild(r)
				}
			}
			if err = pkg.SetXML(relPart, rels); err != nil {
				return err
			}
			target := opc.Relative(slides[e.Slide-1], e.Part)
			if _, err = pkg.AddRel(slides[e.Slide-1], rns+"/slideLayout", target, false); err != nil {
				return err
			}
		case "replace-theme":
			if pkg.ContentType(e.Part) != "application/vnd.openxmlformats-officedocument.theme+xml" {
				return fmt.Errorf("part must identify an existing theme")
			}
			file := e.Path
			if !filepath.IsAbs(file) {
				file = filepath.Join(base, file)
			}
			raw, err := os.ReadFile(file)
			if err != nil {
				return err
			}
			theme, err := opc.ParseXML(raw)
			if err != nil {
				return err
			}
			if !office.Is(theme.Root(), ans, "theme") {
				return fmt.Errorf("expected DrawingML theme root")
			}
			if err = pkg.SetXML(e.Part, theme); err != nil {
				return err
			}
		case "delete-slide":
			if len(ids) <= 1 {
				return fmt.Errorf("cannot remove last slide")
			}
			removed := ids[e.Slide-1]
			rid := removed.SelectAttrValue("r:id", "")
			numericID := removed.SelectAttrValue("id", "")
			// Custom shows reference relationship IDs; sections reference numeric
			// slide IDs. Neither should keep a deleted slide in its visible list.
			for _, show := range office.All(d.Root(), pns, "custShow") {
				for _, ref := range office.All(show, pns, "sld") {
					if ref.SelectAttrValue("r:id", "") == rid {
						ref.Parent().RemoveChild(ref)
					}
				}
				if len(office.All(show, pns, "sld")) == 0 {
					show.Parent().RemoveChild(show)
				}
			}
			for _, ref := range office.All(d.Root(), "http://schemas.microsoft.com/office/powerpoint/2010/main", "sldId") {
				if ref.SelectAttrValue("id", "") == numericID {
					ref.Parent().RemoveChild(ref)
				}
			}
			list.RemoveChild(removed)
		case "move-slide":
			if e.Position < 1 || e.Position > len(ids) {
				return fmt.Errorf("position out of range")
			}
			item := ids[e.Slide-1]
			list.RemoveChild(item)
			remaining := office.Children(list, pns, "sldId")
			if e.Position > len(remaining) {
				list.AddChild(item)
			} else {
				list.InsertChildAt(remaining[e.Position-1].Index(), item)
			}
		case "duplicate-slide", "add-slide", "add-element", "replace-element":
			source := pkg
			sourcePart := ""
			reuse := e.Operation == "duplicate-slide"
			var cleanup func()
			if reuse {
				sourcePart = slides[e.Slide-1]
			} else {
				slide := Slide{}
				if e.Operation == "add-slide" {
					if e.Content == nil {
						return fmt.Errorf("content required")
					}
					slide = *e.Content
				} else {
					if e.Element == nil {
						return fmt.Errorf("element required")
					}
					element := *e.Element
					if element.Connector != nil {
						connector := *element.Connector
						connector.Start, connector.End = nil, nil
						element.Connector = &connector
					}
					slide.Elements = []Element{element}
				}
				size := office.Child(d.Root(), pns, "sldSz")
				w, h := 13.333333, 7.5
				if size != nil {
					x, _ := strconv.ParseFloat(size.SelectAttrValue("cx", ""), 64)
					y, _ := strconv.ParseFloat(size.SelectAttrValue("cy", ""), 64)
					if x > 0 && y > 0 {
						w = x / 914400
						h = y / 914400
					}
				}
				dir, err := os.MkdirTemp("", "paperworks-deck-import-")
				if err != nil {
					return err
				}
				cleanup = func() { os.RemoveAll(dir) }
				file := filepath.Join(dir, "slide.pptx")
				if err = Create(ctx, Spec{Width: w, Height: h, Font: "Arial", Slides: []Slide{slide}}, base, file); err != nil {
					cleanup()
					return err
				}
				source, err = opc.Open(file)
				cleanup()
				if err != nil {
					return err
				}
				parts, err := office.PresentationSlides(source)
				if err != nil {
					return err
				}
				sourcePart = parts[0]
			}
			imported, err := importPartGraph(ctx, pkg, source, sourcePart, reuse, map[string]string{})
			if err != nil {
				return err
			}
			if e.Operation == "add-element" || e.Operation == "replace-element" {
				src, err := pkg.XML(imported)
				if err != nil {
					return err
				}
				dest, err := pkg.XML(slides[e.Slide-1])
				if err != nil {
					return err
				}
				srcTree := office.All(src.Root(), pns, "spTree")[0]
				destTree := office.All(dest.Root(), pns, "spTree")[0]
				var replace *etree.Element
				if e.Operation == "replace-element" {
					for _, pr := range office.All(dest.Root(), pns, "cNvPr") {
						if pr.SelectAttrValue("id", "") == e.ShapeID {
							if replace != nil {
								return fmt.Errorf("ambiguous shape_id")
							}
							replace = pr.Parent().Parent()
						}
					}
					if replace == nil {
						return fmt.Errorf("shape_id not found")
					}
				}
				maxID := 1
				for _, p := range office.All(dest.Root(), pns, "cNvPr") {
					id, _ := strconv.Atoi(p.SelectAttrValue("id", "0"))
					maxID = max(maxID, id)
				}
				rels, err := pkg.Rels(imported)
				if err != nil {
					return err
				}
				// Transfer only relationships referenced by the copied objects.
				// A temporary slide's layout and notes belong to that slide,
				// not to the destination receiving one shape.
				needed := map[string]bool{}
				office.Iter(srcTree, func(element *etree.Element) bool {
					for _, attr := range element.Attr {
						if attr.NamespaceURI() == rns {
							needed[attr.Value] = true
						}
					}
					return true
				})
				remap := map[string]string{}
				for _, r := range rels {
					if !needed[r.ID] {
						continue
					}
					target := r.Target
					if r.TargetMode != "External" {
						target = opc.Relative(slides[e.Slide-1], opc.Resolve(imported, r.Target))
					}
					id, err := pkg.AddRel(slides[e.Slide-1], r.Type, target, r.TargetMode == "External")
					if err != nil {
						return err
					}
					remap[r.ID] = id
				}
				for _, shape := range srcTree.ChildElements() {
					if shape.Tag == "nvGrpSpPr" || shape.Tag == "grpSpPr" {
						continue
					}
					copy := shape.Copy()
					replacementID := ""
					if replace != nil {
						if props := office.All(replace, pns, "cNvPr"); len(props) > 0 {
							replacementID = props[0].SelectAttrValue("id", "")
						}
					}
					if replace != nil {
						replace.Parent().InsertChildAt(replace.Index(), copy)
						replace.Parent().RemoveChild(replace)
						replace = nil
					} else {
						destTree.AddChild(copy)
					}
					for _, p := range office.All(copy, pns, "cNvPr") {
						if replacementID != "" {
							p.CreateAttr("id", replacementID)
							replacementID = ""
						} else {
							maxID++
							p.CreateAttr("id", strconv.Itoa(maxID))
						}
					}
					rewriteRelIDs(copy, remap)
					if e.Element != nil && e.Element.Connector != nil {
						properties := office.All(copy, pns, "cNvCxnSpPr")
						if len(properties) != 1 {
							return fmt.Errorf("connector properties missing")
						}
						for _, end := range []struct {
							tag string
							c   *Connection
						}{{"stCxn", e.Element.Connector.Start}, {"endCxn", e.Element.Connector.End}} {
							if end.c == nil {
								continue
							}
							found := false
							for _, prop := range office.All(destTree, pns, "cNvPr") {
								if prop.SelectAttrValue("id", "") == end.c.ShapeID {
									found = true
								}
							}
							if !found || end.c.Site < 0 || end.c.Site > 255 {
								return fmt.Errorf("invalid connector endpoint")
							}
							ref := properties[0].CreateElement("a:" + end.tag)
							ref.CreateAttr("id", end.c.ShapeID)
							ref.CreateAttr("idx", strconv.Itoa(end.c.Site))
						}
					}
				}
				if err = pkg.SetXML(slides[e.Slide-1], dest); err != nil {
					return err
				}
				continue
			}
			pos := e.Position
			if pos == 0 {
				pos = len(ids) + 1
			}
			if pos < 1 || pos > len(ids)+1 {
				return fmt.Errorf("position must be 1..%d", len(ids)+1)
			}
			target := opc.Relative(main, imported)
			rid, err := pkg.AddRel(main, rns+"/slide", target, false)
			if err != nil {
				return err
			}
			maxID := 255
			for _, id := range ids {
				n, _ := strconv.Atoi(id.SelectAttrValue("id", "0"))
				maxID = max(maxID, n)
			}
			item := etree.NewElement("p:sldId")
			item.CreateAttr("id", strconv.Itoa(maxID+1))
			item.CreateAttr("r:id", rid)
			if pos > len(ids) {
				list.AddChild(item)
			} else {
				list.InsertChildAt(ids[pos-1].Index(), item)
			}
			// Register imported slide masters with the presentation.
			masters := office.Child(d.Root(), pns, "sldMasterIdLst")
			if masters == nil {
				masters = etree.NewElement("p:sldMasterIdLst")
				d.Root().InsertChildAt(0, masters)
			}
			existing, _ := pkg.Rels(main)
			known := map[string]bool{}
			for _, r := range existing {
				known[opc.Resolve(main, r.Target)] = true
			}
			next := uint64(2147483648)
			for _, x := range masters.ChildElements() {
				v, _ := strconv.ParseUint(x.SelectAttrValue("id", "0"), 10, 64)
				if v >= next {
					next = v + 1
				}
			}
			for _, n := range pkg.SortedNames() {
				if pkg.ContentType(n) != "application/vnd.openxmlformats-officedocument.presentationml.slideMaster+xml" || known[n] {
					continue
				}
				target := opc.Relative(main, n)
				id, err := pkg.AddRel(main, rns+"/slideMaster", target, false)
				if err != nil {
					return err
				}
				m := masters.CreateElement("p:sldMasterId")
				m.CreateAttr("id", strconv.FormatUint(next, 10))
				next++
				m.CreateAttr("r:id", id)
			}
		default:
			return fmt.Errorf("unsupported deck operation %q", e.Operation)
		}
	}
	if err = pkg.SetXML(main, d); err != nil {
		return err
	}
	return pkg.SaveNew(output)
}
func presentationPart(p *opc.Package) (string, error) {
	rels, err := p.Rels("")
	if err != nil {
		return "", err
	}
	for _, r := range rels {
		if r.Type == rns+"/officeDocument" && r.TargetMode != "External" {
			return opc.Resolve("", r.Target), nil
		}
	}
	return "", fmt.Errorf("missing presentation relationship")
}
func rewriteRelIDs(e *etree.Element, ids map[string]string) {
	for i, a := range e.Attr {
		if a.NamespaceURI() == rns {
			if v, ok := ids[a.Value]; ok {
				e.Attr[i].Value = v
			}
		}
	}
	for _, c := range e.ChildElements() {
		rewriteRelIDs(c, ids)
	}
}
func importPartGraph(ctx context.Context, dest, src *opc.Package, name string, reuse bool, memo map[string]string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if n := memo[name]; n != "" {
		return n, nil
	}
	if !src.Has(name) {
		return "", fmt.Errorf("missing dependency %s", name)
	}
	typ := src.ContentType(name)
	if reuse && (strings.Contains(typ, "slideLayout") || strings.Contains(typ, "slideMaster") || strings.Contains(typ, "theme")) {
		return name, nil
	}
	ext := path.Ext(name)
	prefix := path.Join(path.Dir(name), "imported-")
	target := dest.UnusedName(prefix, ext)
	memo[name] = target
	dest.Set(target, src.Get(name))
	if err := dest.EnsureContentType(target, typ); err != nil {
		return "", err
	}
	rels, err := src.Rels(name)
	if err != nil {
		return "", err
	}
	for _, r := range rels {
		to := r.Target
		if r.TargetMode != "External" {
			resolved := opc.Resolve(name, r.Target)
			n := memo[resolved]
			if n == "" {
				if reuse && r.Type == rns+"/slide" {
					// Navigation to another existing slide stays in this deck.
					// Back-links/self-links already in memo target the new copy.
					n = resolved
				} else {
					n, err = importPartGraph(ctx, dest, src, resolved, reuse, memo)
					if err != nil {
						return "", err
					}
				}
			}
			to = opc.Relative(target, n)
		}
		// Keep IDs referenced by the copied XML; AddRel generates different IDs.
		relPart := opc.RelsName(target)
		var doc *etree.Document
		if dest.Has(relPart) {
			doc, err = dest.XML(relPart)
			if err != nil {
				return "", err
			}
		} else {
			doc = etree.NewDocument()
			root := doc.CreateElement("Relationships")
			root.CreateAttr("xmlns", opc.NSRel)
		}
		x := doc.Root().CreateElement("Relationship")
		x.CreateAttr("Id", r.ID)
		x.CreateAttr("Type", r.Type)
		x.CreateAttr("Target", to)
		if r.TargetMode != "" {
			x.CreateAttr("TargetMode", r.TargetMode)
		}
		if err = dest.SetXML(relPart, doc); err != nil {
			return "", err
		}
	}
	return target, nil
}
