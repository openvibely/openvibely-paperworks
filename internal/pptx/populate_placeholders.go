package pptx

import (
	"context"
	"fmt"
	"strconv"

	"github.com/beevik/etree"
	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
)

// A slide matches layout placeholders by index, with title placeholders treated
// as a separate slot. Furniture remains inherited rather than copied locally.
func contentPlaceholderKey(shape *etree.Element) (string, error) {
	phs := office.All(shape, pns, "ph")
	if len(phs) == 0 {
		return "", nil
	}
	if len(phs) != 1 {
		return "", fmt.Errorf("ambiguous placeholder shape")
	}
	ph := phs[0]
	typ := ph.SelectAttrValue("type", "obj")
	switch typ {
	case "dt", "ftr", "hdr", "sldNum":
		return "", nil
	}
	idx, err := strconv.ParseUint(ph.SelectAttrValue("idx", "0"), 10, 32)
	if err != nil {
		return "", fmt.Errorf("invalid placeholder index")
	}
	if typ == "title" || typ == "ctrTitle" {
		return "title", nil
	}
	return strconv.FormatUint(idx, 10), nil
}

func populatePlaceholders(ctx context.Context, pkg *opc.Package, slide string) error {
	rels, err := pkg.Rels(slide)
	if err != nil {
		return err
	}
	layout := ""
	for _, r := range rels {
		if r.Type == rns+"/slideLayout" {
			if layout != "" || r.TargetMode == "External" {
				return fmt.Errorf("ambiguous or external slide layout")
			}
			layout = opc.Resolve(slide, r.Target)
		}
	}
	if layout == "" || pkg.ContentType(layout) != layoutContentType {
		return fmt.Errorf("slide requires an existing layout")
	}
	src, err := pkg.XML(layout)
	if err != nil {
		return err
	}
	dst, err := pkg.XML(slide)
	if err != nil {
		return err
	}
	srcTree := office.Child(office.Child(src.Root(), pns, "cSld"), pns, "spTree")
	dstTree := office.Child(office.Child(dst.Root(), pns, "cSld"), pns, "spTree")
	if srcTree == nil || dstTree == nil {
		return fmt.Errorf("missing shape tree")
	}
	existing := map[string]bool{}
	maxID := uint64(0)
	for _, pr := range office.All(dst.Root(), pns, "cNvPr") {
		id, err := strconv.ParseUint(pr.SelectAttrValue("id", ""), 10, 32)
		if err != nil {
			return fmt.Errorf("invalid shape ID")
		}
		if id > maxID {
			maxID = id
		}
	}
	for _, shape := range dstTree.ChildElements() {
		key, err := contentPlaceholderKey(shape)
		if err != nil {
			return err
		}
		if key != "" {
			if existing[key] {
				return fmt.Errorf("ambiguous existing placeholder index")
			}
			existing[key] = true
		}
	}
	layoutRels, err := pkg.Rels(layout)
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, shape := range srcTree.ChildElements() {
		if err := ctx.Err(); err != nil {
			return err
		}
		key, err := contentPlaceholderKey(shape)
		if err != nil {
			return err
		}
		if key == "" {
			continue
		}
		if seen[key] {
			return fmt.Errorf("ambiguous layout placeholder index")
		}
		seen[key] = true
		if existing[key] {
			continue
		}
		copy := shape.Copy()
		// Preserve all inherited namespace declarations, including imported aliases.
		bound := map[string]bool{}
		for node := shape; node != nil; node = node.Parent() {
			for _, a := range node.Attr {
				if a.Space == "xmlns" || a.FullKey() == "xmlns" {
					if !bound[a.FullKey()] {
						copy.CreateAttr(a.FullKey(), a.Value)
						bound[a.FullKey()] = true
					}
				}
			}
		}
		for _, pr := range office.All(copy, pns, "cNvPr") {
			maxID++
			if maxID > 4294967295 {
				return fmt.Errorf("shape IDs exhausted")
			}
			pr.CreateAttr("id", strconv.FormatUint(maxID, 10))
		}
		// Layout prompt text is not authored slide content. Retain its text style.
		if body := office.Child(copy, pns, "txBody"); body != nil {
			replaceBody(body, "")
		}
		needed := map[string]bool{}
		office.Iter(copy, func(e *etree.Element) bool {
			for _, a := range e.Attr {
				if a.NamespaceURI() == rns {
					needed[a.Value] = true
				}
			}
			return true
		})
		remap := map[string]string{}
		for _, r := range layoutRels {
			if !needed[r.ID] {
				continue
			}
			target := r.Target
			if r.TargetMode != "External" {
				resolved := opc.Resolve(layout, target)
				if !pkg.Has(resolved) {
					return fmt.Errorf("missing placeholder dependency %s", resolved)
				}
				target = opc.Relative(slide, resolved)
			}
			id, err := pkg.AddRel(slide, r.Type, target, r.TargetMode == "External")
			if err != nil {
				return err
			}
			remap[r.ID] = id
		}
		for id := range needed {
			if remap[id] == "" {
				return fmt.Errorf("placeholder references missing relationship %s", id)
			}
		}
		rewriteRelIDs(copy, remap)
		if ext := office.Child(dstTree, pns, "extLst"); ext != nil {
			dstTree.InsertChildAt(ext.Index(), copy)
		} else {
			dstTree.AddChild(copy)
		}
	}
	return pkg.SetXML(slide, dst)
}
