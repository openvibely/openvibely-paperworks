package xlsx

import (
	"path"
	"sort"
	"strings"

	"github.com/beevik/etree"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
)

// serializePackageGraph follows the pinned openpyxl writer's manifest order for
// newly authored workbooks. It preserves unknown overrides and external links;
// it must not be used to normalize imported workbooks.
func serializePackageGraph(p *opc.Package) error {
	for _, name := range p.Names() {
		if name != "_rels/.rels" && !(strings.HasPrefix(name, "xl/worksheets/_rels/") && strings.HasSuffix(name, ".rels")) {
			continue
		}
		d, err := p.XML(name)
		if err != nil {
			return err
		}
		entries := d.Root().ChildElements()
		if name == "_rels/.rels" {
			rank := func(e *etree.Element) int {
				switch e.SelectAttrValue("Type", "") {
				case opc.NSOffR + "/officeDocument":
					return 0
				case "http://schemas.openxmlformats.org/package/2006/relationships/metadata/core-properties":
					return 1
				case opc.NSOffR + "/extended-properties":
					return 2
				}
				return 3
			}
			sort.SliceStable(entries, func(i, j int) bool { return rank(entries[i]) < rank(entries[j]) })
		}
		for _, e := range entries {
			if name != "_rels/.rels" && e.SelectAttrValue("TargetMode", "") != "External" {
				target := e.SelectAttrValue("Target", "")
				if target != "" && !strings.HasPrefix(target, "/") {
					e.CreateAttr("Target", "/"+path.Join("xl/worksheets", target))
				}
			}
			orderAttributes(e, "Type", "Target", "Id", "TargetMode")
			d.Root().RemoveChild(e)
			d.Root().AddChild(e)
		}
		if err = writeWorkbookXML(p, name, d.Root()); err != nil {
			return err
		}
	}
	d, err := p.XML("[Content_Types].xml")
	if err != nil {
		return err
	}
	root := d.Root()
	defaults := root.SelectElements("Default")
	overrides := root.SelectElements("Override")
	// Static workbook parts precede worksheet graphs. Worksheet/drawing pairs
	// follow sheet order; chart overrides are appended after those graphs.
	order := []string{"/xl/styles.xml", "/xl/theme/theme1.xml", "/docProps/core.xml", "/docProps/app.xml"}
	wb, err := p.XML("xl/_rels/workbook.xml.rels")
	if err != nil {
		return err
	}
	for _, rel := range wb.Root().ChildElements() {
		if rel.SelectAttrValue("Type", "") != opc.NSOffR+"/worksheet" {
			continue
		}
		target := rel.SelectAttrValue("Target", "")
		if !strings.HasPrefix(target, "/") {
			target = "/" + path.Join("xl", target)
		}
		order = append(order, target)
		relname := path.Join(path.Dir(strings.TrimPrefix(target, "/")), "_rels", path.Base(target)+".rels")
		if !p.Has(relname) {
			continue
		}
		rd, e := p.XML(relname)
		if e != nil {
			return e
		}
		for _, r := range rd.Root().ChildElements() {
			if r.SelectAttrValue("Type", "") == opc.NSOffR+"/drawing" {
				order = append(order, r.SelectAttrValue("Target", ""))
			}
		}
	}
	// Chart numbering follows creation order; use numeric suffixes, not lexical
	// sorting (chart10 must follow chart9).
	charts := []string{}
	for _, e := range overrides {
		n := e.SelectAttrValue("PartName", "")
		if strings.HasPrefix(n, "/xl/charts/") {
			charts = append(charts, n)
		}
	}
	sort.SliceStable(charts, func(i, j int) bool {
		a, b := charts[i], charts[j]
		if len(a) != len(b) {
			return len(a) < len(b)
		}
		return a < b
	})
	order = append(order, charts...)
	order = append(order, "/xl/workbook.xml")
	rank := map[string]int{}
	for i, n := range order {
		rank[n] = i
	}
	sort.SliceStable(overrides, func(i, j int) bool {
		a, ok := rank[overrides[i].SelectAttrValue("PartName", "")]
		if !ok {
			a = len(order)
		}
		b, ok := rank[overrides[j].SelectAttrValue("PartName", "")]
		if !ok {
			b = len(order)
		}
		return a < b
	})
	for _, e := range root.ChildElements() {
		root.RemoveChild(e)
	}
	for _, e := range defaults {
		orderAttributes(e, "Extension", "ContentType")
		root.AddChild(e)
	}
	for _, e := range overrides {
		orderAttributes(e, "PartName", "ContentType")
		root.AddChild(e)
	}
	return writeWorkbookXML(p, "[Content_Types].xml", root)
}
