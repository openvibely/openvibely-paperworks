package pptx

import (
	"fmt"
	"github.com/beevik/etree"
	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"strconv"
	"strings"
)

const masterContentType = "application/vnd.openxmlformats-officedocument.presentationml.slideMaster+xml"
const layoutContentType = "application/vnd.openxmlformats-officedocument.presentationml.slideLayout+xml"

type LayoutPlaceholder struct {
	Type  string  `json:"type"`
	Index int     `json:"index"`
	X     float64 `json:"x"`
	Y     float64 `json:"y"`
	W     float64 `json:"w"`
	H     float64 `json:"h"`
}

func stylePartDocument(base string, e DeckEdit, master bool) (*etree.Document, error) {
	tag := "sldLayout"
	if master {
		tag = "sldMaster"
	}
	var d *etree.Document
	if e.Path != "" {
		raw, err := readStyleResource(base, e.Path, pns, tag)
		if err != nil {
			return nil, err
		}
		d, err = opc.ParseXML(raw)
		if err != nil {
			return nil, err
		}
		if len(e.Placeholders) > 0 {
			return nil, fmt.Errorf("path and placeholders are mutually exclusive")
		}
	} else {
		raw := `<p:` + tag + ns + `><p:cSld><p:spTree><p:nvGrpSpPr><p:cNvPr id="1" name=""/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr><p:grpSpPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="0" cy="0"/><a:chOff x="0" y="0"/><a:chExt cx="0" cy="0"/></a:xfrm></p:grpSpPr></p:spTree></p:cSld>`
		if master {
			raw += `<p:clrMap bg1="lt1" tx1="dk1" bg2="lt2" tx2="dk2" accent1="accent1" accent2="accent2" accent3="accent3" accent4="accent4" accent5="accent5" accent6="accent6" hlink="hlink" folHlink="folHlink"/>`
		} else {
			raw += `<p:clrMapOvr><a:masterClrMapping/></p:clrMapOvr>`
		}
		raw += `</p:` + tag + `>`
		var err error
		d, err = opc.ParseXML([]byte(raw))
		if err != nil {
			return nil, err
		}
		tree := office.Child(office.Child(d.Root(), pns, "cSld"), pns, "spTree")
		seen := map[int]bool{}
		for i, p := range e.Placeholders {
			switch p.Type {
			case "title", "ctrTitle", "subTitle", "body", "obj", "pic", "chart", "tbl", "dt", "ftr", "sldNum":
			default:
				return nil, fmt.Errorf("unsupported placeholder type")
			}
			if p.Index < 0 || seen[p.Index] {
				return nil, fmt.Errorf("placeholder indices must be unique and nonnegative")
			}
			seen[p.Index] = true
			if !finite(p.X) || !finite(p.Y) || !finite(p.W) || !finite(p.H) || p.X < 0 || p.Y < 0 || p.W <= 0 || p.H <= 0 || p.X+p.W > 1000 || p.Y+p.H > 1000 {
				return nil, fmt.Errorf("invalid placeholder geometry")
			}
			el := Element{X: p.X, Y: p.Y, W: p.W, H: p.H, Name: p.Type}
			fragment, err := opc.ParseXML([]byte(`<p:sp` + ns + `>` + nv(i+2, el, "Sp") + `<p:spPr>` + xfrm(el, "a") + `<a:prstGeom prst="rect"><a:avLst/></a:prstGeom></p:spPr><p:txBody><a:bodyPr/><a:lstStyle/><a:p/></p:txBody></p:sp>`))
			if err != nil {
				return nil, err
			}
			ph := office.Child(office.Child(fragment.Root(), pns, "nvSpPr"), pns, "nvPr").CreateElement("p:ph")
			ph.CreateAttr("type", p.Type)
			ph.CreateAttr("idx", strconv.Itoa(p.Index))
			tree.AddChild(fragment.Root().Copy())
		}
	}
	common := office.Child(d.Root(), pns, "cSld")
	if common == nil || office.Child(common, pns, "spTree") == nil {
		return nil, fmt.Errorf("style part requires cSld/spTree")
	}
	if e.Name != "" {
		common.CreateAttr("name", e.Name)
	}
	return d, nil
}
func registerStyleID(list *etree.Element, tag, rid string) error {
	max := uint64(2147483647)
	for _, e := range list.ChildElements() {
		n, err := strconv.ParseUint(e.SelectAttrValue("id", ""), 10, 32)
		if err != nil {
			return fmt.Errorf("invalid existing style ID")
		}
		if n > max {
			max = n
		}
	}
	if max >= 4294967295 {
		return fmt.Errorf("style IDs exhausted")
	}
	child := list.CreateElement("p:" + tag)
	child.CreateAttr("xmlns:p", pns)
	child.CreateAttr("id", strconv.FormatUint(max+1, 10))
	child.CreateAttr("r:id", rid)
	child.CreateAttr("xmlns:r", rns)
	return nil
}
func addPresentationStylePart(pkg *opc.Package, main string, presentation *etree.Document, base string, e DeckEdit) error {
	master := e.Operation == "add-master"
	prefix, ctype := "ppt/slideLayouts/", layoutContentType
	if master {
		prefix, ctype = "ppt/slideMasters/", masterContentType
	}
	if !strings.HasPrefix(e.Part, prefix) || strings.Contains(strings.TrimPrefix(e.Part, prefix), "/") || !strings.HasSuffix(e.Part, ".xml") || strings.Contains(e.Part, "..") || pkg.Has(e.Part) || pkg.Has(opc.RelsName(e.Part)) {
		return fmt.Errorf("part must name a new XML part in %s", prefix)
	}
	d, err := stylePartDocument(base, e, master)
	if err != nil {
		return err
	}
	if master {
		if e.ParentPart != "" {
			return fmt.Errorf("master cannot have parent_part")
		}
		theme := e.ThemePart
		if theme == "" {
			for _, n := range pkg.SortedNames() {
				if pkg.ContentType(n) == "application/vnd.openxmlformats-officedocument.theme+xml" {
					theme = n
					break
				}
			}
		}
		if !pkg.Has(theme) || pkg.ContentType(theme) != "application/vnd.openxmlformats-officedocument.theme+xml" {
			return fmt.Errorf("theme_part must identify an existing theme")
		}
		// Imported master layout IDs are replaced by a registered blank layout.
		for _, old := range office.Children(d.Root(), pns, "sldLayoutIdLst") {
			d.Root().RemoveChild(old)
		}
		if err = pkg.SetXML(e.Part, d); err != nil {
			return err
		}
		if err = pkg.EnsureContentType(e.Part, ctype); err != nil {
			return err
		}
		if _, err = pkg.AddRel(e.Part, rns+"/theme", opc.Relative(e.Part, theme), false); err != nil {
			return err
		}
		rid, err := pkg.AddRel(main, rns+"/slideMaster", opc.Relative(main, e.Part), false)
		if err != nil {
			return err
		}
		list := office.Child(presentation.Root(), pns, "sldMasterIdLst")
		if list == nil {
			list = etree.NewElement("p:sldMasterIdLst")
			list.CreateAttr("xmlns:p", pns)
			presentation.Root().InsertChildAt(0, list)
		}
		if err = registerStyleID(list, "sldMasterId", rid); err != nil {
			return err
		}
		layout := pkg.UnusedName("ppt/slideLayouts/paperworksLayout", ".xml")
		return addPresentationStylePart(pkg, main, presentation, base, DeckEdit{Operation: "add-layout", Part: layout, ParentPart: e.Part, Name: e.Name + " Blank"})
	}
	if e.ThemePart != "" {
		return fmt.Errorf("layout inherits its master's theme")
	}
	if !pkg.Has(e.ParentPart) || pkg.ContentType(e.ParentPart) != masterContentType {
		return fmt.Errorf("parent_part must identify an existing master")
	}
	parent, err := pkg.XML(e.ParentPart)
	if err != nil {
		return err
	}
	if err = pkg.SetXML(e.Part, d); err != nil {
		return err
	}
	if err = pkg.EnsureContentType(e.Part, ctype); err != nil {
		return err
	}
	if _, err = pkg.AddRel(e.Part, rns+"/slideMaster", opc.Relative(e.Part, e.ParentPart), false); err != nil {
		return err
	}
	rid, err := pkg.AddRel(e.ParentPart, rns+"/slideLayout", opc.Relative(e.ParentPart, e.Part), false)
	if err != nil {
		return err
	}
	list := office.Child(parent.Root(), pns, "sldLayoutIdLst")
	if list == nil {
		list = etree.NewElement("p:sldLayoutIdLst")
		list.CreateAttr("xmlns:p", pns)
		inserted := false
		for _, c := range parent.Root().ChildElements() {
			if c.Tag == "transition" || c.Tag == "timing" || c.Tag == "hf" || c.Tag == "txStyles" || c.Tag == "extLst" {
				parent.Root().InsertChildAt(c.Index(), list)
				inserted = true
				break
			}
		}
		if !inserted {
			parent.Root().AddChild(list)
		}
	}
	if err = registerStyleID(list, "sldLayoutId", rid); err != nil {
		return err
	}
	return pkg.SetXML(e.ParentPart, parent)
}
