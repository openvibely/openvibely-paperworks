package pptx

import (
	"fmt"
	"github.com/beevik/etree"
	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
)

// setLayoutMaster moves both sides of the layout/master relationship, retaining
// layout content and slide assignments. It refuses ambiguous input packages.
func setLayoutMaster(pkg *opc.Package, layout, master string) error {
	if pkg.ContentType(layout) != layoutContentType || pkg.ContentType(master) != masterContentType {
		return fmt.Errorf("part must identify an existing layout and parent_part an existing master")
	}
	rels, err := pkg.Rels(layout)
	if err != nil {
		return err
	}
	old := ""
	count := 0
	for _, r := range rels {
		if r.Type == rns+"/slideMaster" {
			count++
			if r.TargetMode != "External" {
				old = opc.Resolve(layout, r.Target)
			}
		}
	}
	if count != 1 || pkg.ContentType(old) != masterContentType {
		return fmt.Errorf("layout requires one valid internal master relationship")
	}
	// Check the reverse registration before modifying anything.
	od, err := pkg.XML(old)
	if err != nil {
		return err
	}
	ors, err := pkg.Rels(old)
	if err != nil {
		return err
	}
	rid := ""
	count = 0
	for _, r := range ors {
		if r.Type == rns+"/slideLayout" && r.TargetMode != "External" && opc.Resolve(old, r.Target) == layout {
			rid = r.ID
			count++
		}
	}
	var registration *etree.Element
	for _, el := range office.All(od.Root(), pns, "sldLayoutId") {
		for _, a := range el.Attr {
			if a.NamespaceURI() == rns && a.Key == "id" && a.Value == rid {
				if registration != nil {
					return fmt.Errorf("duplicate layout registration")
				}
				registration = el
			}
		}
	}
	if count != 1 || registration == nil {
		return fmt.Errorf("layout is not uniquely registered by its master")
	}
	if old == master {
		return nil
	}
	nd, err := pkg.XML(master)
	if err != nil {
		return err
	}
	nrs, err := pkg.Rels(master)
	if err != nil {
		return err
	}
	for _, r := range nrs {
		if r.Type == rns+"/slideLayout" && r.TargetMode != "External" && opc.Resolve(master, r.Target) == layout {
			return fmt.Errorf("destination master already references layout")
		}
	}
	list := office.Child(nd.Root(), pns, "sldLayoutIdLst")
	if list == nil {
		list = etree.NewElement("p:sldLayoutIdLst")
		list.CreateAttr("xmlns:p", pns)
		nd.Root().AddChild(list)
		for _, c := range nd.Root().ChildElements() {
			if c.Tag == "transition" || c.Tag == "timing" || c.Tag == "hf" || c.Tag == "txStyles" || c.Tag == "extLst" {
				nd.Root().RemoveChild(list)
				nd.Root().InsertChildAt(c.Index(), list)
				break
			}
		}
	}
	newID, err := pkg.AddRel(master, rns+"/slideLayout", opc.Relative(master, layout), false)
	if err != nil {
		return err
	}
	if err = registerStyleID(list, "sldLayoutId", newID); err != nil {
		return err
	}
	registration.Parent().RemoveChild(registration)
	if err = pkg.SetXML(old, od); err != nil {
		return err
	}
	if err = pkg.SetXML(master, nd); err != nil {
		return err
	}
	if err = removeRels(pkg, old, func(r *etree.Element) bool { return r.SelectAttrValue("Id", "") == rid }); err != nil {
		return err
	}
	if err = removeRels(pkg, layout, func(r *etree.Element) bool { return r.SelectAttrValue("Type", "") == rns+"/slideMaster" }); err != nil {
		return err
	}
	_, err = pkg.AddRel(layout, rns+"/slideMaster", opc.Relative(layout, master), false)
	return err
}
