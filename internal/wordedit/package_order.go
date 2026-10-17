package wordedit

import (
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"sort"
)

// reference DOCX library writes parts in relationship DFS order, with each owner's rels
// immediately after the owner. Apply this only when editing adds a part. Unlike
// reference DOCX library, retain unreachable source parts at the end rather than discard
// user content. Arbitrary orphan-package equivalence is therefore not claimed.
func (d *Doc) orderAddedParts() error {
	p := d.Pkg
	ct, err := p.XML("[Content_Types].xml")
	if err != nil {
		return err
	}
	children := ct.Root().ChildElements()
	sort.SliceStable(children, func(i, j int) bool {
		if children[i].Tag != children[j].Tag {
			return children[i].Tag < children[j].Tag
		}
		key := "PartName"
		if children[i].Tag == "Default" {
			key = "Extension"
		}
		return children[i].SelectAttrValue(key, "") < children[j].SelectAttrValue(key, "")
	})
	for _, c := range children {
		ct.Root().RemoveChild(c)
		ct.Root().AddChild(c)
	}
	if err := p.SetXML("[Content_Types].xml", ct); err != nil {
		return err
	}
	parts := map[string]*opc.Part{}
	for _, part := range p.Parts {
		parts[part.Name] = part
	}
	seen := map[string]bool{}
	ordered := make([]*opc.Part, 0, len(p.Parts))
	add := func(name string) {
		if part, ok := parts[name]; ok && !seen[name] {
			seen[name] = true
			ordered = append(ordered, part)
		}
	}
	add("[Content_Types].xml")
	add("_rels/.rels")
	var walk func(string) error
	walk = func(owner string) error {
		rels, err := p.Rels(owner)
		if err != nil {
			return err
		}
		for _, r := range rels {
			if r.TargetMode == "External" {
				continue
			}
			target := opc.Resolve(owner, r.Target)
			if seen[target] {
				continue
			}
			if _, ok := parts[target]; !ok {
				continue
			}
			add(target)
			add(opc.RelsName(target))
			if err := walk(target); err != nil {
				return err
			}
		}
		return nil
	}
	if err := walk(""); err != nil {
		return err
	}
	for _, part := range p.Parts {
		add(part.Name)
	}
	p.Parts = ordered
	return nil
}
