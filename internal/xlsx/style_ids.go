package xlsx

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/beevik/etree"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
)

// stabilizeStyleIDs assigns cell formats in worksheet encounter order, as the
// pinned writer does when serializing cells. Excelize's original IDs depend on
// map iteration while registering named styles. Keep unused formats at the end;
// removing their fonts/fills requires a separate style-table compaction pass.
func stabilizeStyleIDs(p *opc.Package) error {
	d, err := p.XML("xl/styles.xml")
	if err != nil {
		return err
	}
	xfs := d.Root().SelectElement("cellXfs")
	if xfs == nil {
		return fmt.Errorf("missing cell formats")
	}
	entries := xfs.ChildElements()
	if len(entries) == 0 {
		return fmt.Errorf("empty cell formats")
	}
	order := []int{0}
	ids := map[int]int{0: 0}
	type sheet struct {
		name string
		doc  *etree.Document
	}
	sheets := []sheet{}
	remap := func(e *etree.Element, key string) error {
		a := e.SelectAttr(key)
		if a == nil {
			return nil
		}
		old, err := strconv.Atoi(a.Value)
		if err != nil || old < 0 || old >= len(entries) {
			return fmt.Errorf("invalid cell style %q", a.Value)
		}
		id, ok := ids[old]
		if !ok {
			id = len(order)
			ids[old] = id
			order = append(order, old)
		}
		a.Value = strconv.Itoa(id)
		return nil
	}
	for _, name := range p.Names() {
		if !strings.HasPrefix(name, "xl/worksheets/sheet") || !strings.HasSuffix(name, ".xml") {
			continue
		}
		sd, err := p.XML(name)
		if err != nil {
			return err
		}
		sheets = append(sheets, sheet{name, sd})
		for _, e := range sd.Root().FindElements("cols/col") {
			if err = remap(e, "style"); err != nil {
				return err
			}
		}
		for _, row := range sd.Root().FindElements("sheetData/row") {
			if err = remap(row, "s"); err != nil {
				return err
			}
			for _, c := range row.SelectElements("c") {
				if err = remap(c, "s"); err != nil {
					return err
				}
			}
		}
	}
	for i := range entries {
		if _, ok := ids[i]; !ok {
			order = append(order, i)
		}
	}
	for _, e := range entries {
		xfs.RemoveChild(e)
	}
	for _, i := range order {
		xfs.AddChild(entries[i])
	}
	if err = p.SetXML("xl/styles.xml", d); err != nil {
		return err
	}
	for _, s := range sheets {
		if err = p.SetXML(s.name, s.doc); err != nil {
			return err
		}
	}
	return nil
}
