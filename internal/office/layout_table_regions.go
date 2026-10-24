package office

import (
	"fmt"
	"github.com/beevik/etree"
)

func layoutTableRegions(table, styles *etree.Element, row, col, rows, cols int) ([]*etree.Element, string, bool) {
	pr := Child(table, NSA, "tblPr")
	id := Text(pr, NSA, "tableStyleId")
	if id == "" {
		id = AttrOr(styles, "def", "")
	}
	var style *etree.Element
	for _, s := range Children(styles, NSA, "tblStyle") {
		if AttrOr(s, "styleId", "") == id {
			style = s
			break
		}
	}
	if style == nil {
		return nil, id, false
	}
	flag := func(k string) bool { return Truthy(AttrOr(pr, k, "0"), false) }
	names := []string{"wholeTbl"}
	if flag("bandCol") {
		n := col
		if flag("firstCol") {
			n--
		}
		if n >= 0 {
			names = append(names, fmt.Sprintf("band%dV", n%2+1))
		}
	}
	if flag("bandRow") {
		n := row
		if flag("firstRow") {
			n--
		}
		if n >= 0 {
			names = append(names, fmt.Sprintf("band%dH", n%2+1))
		}
	}
	if flag("firstCol") && col == 0 {
		names = append(names, "firstCol")
	}
	if flag("lastCol") && col == cols-1 {
		names = append(names, "lastCol")
	}
	if flag("firstRow") && row == 0 {
		names = append(names, "firstRow")
	}
	if flag("lastRow") && row == rows-1 {
		names = append(names, "lastRow")
	}
	if flag("firstRow") && row == 0 {
		if flag("firstCol") && col == 0 {
			names = append(names, "nwCell")
		}
		if flag("lastCol") && col == cols-1 {
			names = append(names, "neCell")
		}
	}
	if flag("lastRow") && row == rows-1 {
		if flag("firstCol") && col == 0 {
			names = append(names, "swCell")
		}
		if flag("lastCol") && col == cols-1 {
			names = append(names, "seCell")
		}
	}
	var result []*etree.Element
	for _, name := range names {
		if region := Child(style, NSA, name); region != nil {
			result = append(result, region)
		}
	}
	return result, id, true
}
func layoutTableStyleCell(cell *etree.Element, regions []*etree.Element) (*etree.Element, []*etree.Element) {
	copy := copyLayoutPaintElement(cell)
	pr := Child(copy, NSA, "tcPr")
	if pr == nil {
		pr = copy.CreateElement("a:tcPr")
		pr.CreateAttr("xmlns:a", NSA)
	}
	// Most-specific region wins. Direct cell properties always win.
	var defaults []*etree.Element
	for i := len(regions) - 1; i >= 0; i-- {
		region := regions[i]
		if text := Child(region, NSA, "tcTxStyle"); text != nil {
			rp := copyLayoutPaintElement(text)
			for _, color := range text.ChildElements() {
				switch color.Tag {
				case "srgbClr", "schemeClr", "sysClr", "prstClr", "hslClr", "scrgbClr":
					f := rp.CreateElement("a:solidFill")
					f.CreateAttr("xmlns:a", NSA)
					f.AddChild(copyLayoutPaintElement(color))
				}
			}
			defaults = append(defaults, rp)
		}
		style := Child(region, NSA, "tcStyle")
		if fill := Child(style, NSA, "fill"); fill != nil {
			has := false
			for _, tag := range []string{"solidFill", "gradFill", "pattFill", "blipFill", "noFill", "grpFill"} {
				has = has || Child(pr, NSA, tag) != nil
			}
			if !has {
				for _, f := range fill.ChildElements() {
					pr.AddChild(copyLayoutPaintElement(f))
				}
			}
		}
		borders := Child(style, NSA, "tcBdr")
		for _, pair := range [][2]string{{"left", "lnL"}, {"right", "lnR"}, {"top", "lnT"}, {"bottom", "lnB"}, {"tl2br", "lnTlToBr"}, {"tr2bl", "lnBlToTr"}} {
			if Child(pr, NSA, pair[1]) != nil {
				continue
			}
			if line := Child(Child(borders, NSA, pair[0]), NSA, "ln"); line != nil {
				ln := copyLayoutPaintElement(line)
				ln.Tag = pair[1]
				pr.AddChild(ln)
			}
		}
	}
	return copy, defaults
}
