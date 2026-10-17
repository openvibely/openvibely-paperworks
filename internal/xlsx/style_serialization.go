package xlsx

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/beevik/etree"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
)

// serializeStyles is for newly authored packages only. Cell formats have already
// been ordered by first use; resources retain registration order after pruning.
func serializeStyles(p *opc.Package, spec Spec) error {
	// Font inheritance in the pinned writer differs from Excelize. Keep explicit
	// font family/charset configurations on the existing path until characterized.
	for _, s := range spec.Styles {
		if s.Font != nil && (s.Font.Family != "" || s.Font.Charset != nil) {
			return nil
		}
	}
	d, err := p.XML("xl/styles.xml")
	if err != nil {
		return err
	}
	r := d.Root()
	used := map[int]bool{0: true}
	for _, name := range p.Names() {
		if !strings.HasPrefix(name, "xl/worksheets/sheet") || !strings.HasSuffix(name, ".xml") {
			continue
		}
		sd, e := p.XML(name)
		if e != nil {
			return e
		}
		for _, el := range sd.Root().FindElements(".//*") {
			key := ""
			if el.Tag == "c" || el.Tag == "row" {
				key = "s"
			}
			if el.Tag == "col" {
				key = "style"
			}
			if key == "" {
				continue
			}
			if a := el.SelectAttr(key); a != nil {
				n, e := strconv.Atoi(a.Value)
				if e != nil {
					return e
				}
				used[n] = true
			}
		}
	}
	xfs := r.SelectElement("cellXfs")
	if xfs == nil {
		return fmt.Errorf("missing styles")
	}
	// stabilizeStyleIDs placed all referenced formats before unused ones. Do not
	// silently shift a referenced index if called on a different package layout.
	entries := xfs.ChildElements()
	gap := false
	for i, e := range entries {
		if used[i] {
			if gap {
				return fmt.Errorf("styles not in first-use order")
			}
		} else {
			gap = true
			xfs.RemoveChild(e)
		}
	}
	xfs.CreateAttr("count", strconv.Itoa(len(xfs.ChildElements())))
	for _, table := range []struct {
		name, key string
		defaults  int
	}{{"fonts", "fontId", 1}, {"fills", "fillId", 2}, {"borders", "borderId", 1}} {
		t := r.SelectElement(table.name)
		if t == nil {
			continue
		}
		refs := map[int]bool{}
		for i := 0; i < table.defaults; i++ {
			refs[i] = true
		}
		for _, xf := range r.FindElements(".//xf") {
			n, e := strconv.Atoi(xf.SelectAttrValue(table.key, "0"))
			if e != nil {
				return e
			}
			refs[n] = true
		}
		mapping := map[int]int{}
		next := 0
		for i, e := range t.ChildElements() {
			if refs[i] {
				mapping[i] = next
				next++
			} else {
				t.RemoveChild(e)
			}
		}
		for _, xf := range r.FindElements(".//xf") {
			n, _ := strconv.Atoi(xf.SelectAttrValue(table.key, "0"))
			v, ok := mapping[n]
			if !ok {
				return fmt.Errorf("invalid %s", table.key)
			}
			xf.CreateAttr(table.key, strconv.Itoa(v))
		}
		t.CreateAttr("count", strconv.Itoa(next))
	}
	for i, f := range r.SelectElement("fonts").ChildElements() {
		if i == 0 {
			if f.SelectElement("scheme") == nil {
				f.CreateElement("scheme").CreateAttr("val", "minor")
			}
		} else {
			if e := f.SelectElement("family"); e != nil {
				f.RemoveChild(e)
			}
		}
	}
	for _, f := range r.FindElements("dxfs/dxf/font") {
		if e := f.SelectElement("family"); e != nil {
			f.RemoveChild(e)
		}
	}
	for _, e := range r.FindElements(".//*") {
		if a := e.SelectAttr("rgb"); a != nil && strings.HasPrefix(a.Value, "FF") && len(a.Value) == 8 {
			a.Value = "00" + a.Value[2:]
		}
	}
	for _, f := range r.FindElements("fills/fill/patternFill") {
		if f.SelectAttrValue("patternType", "") == "none" {
			f.RemoveAttr("patternType")
		}
	}
	for _, xf := range xfs.ChildElements() {
		for _, key := range []string{"applyFont", "applyFill", "applyBorder", "applyNumberFormat", "applyAlignment", "applyProtection"} {
			xf.RemoveAttr(key)
		}
		for _, key := range []string{"alignment", "protection"} {
			if e := xf.SelectElement(key); e != nil {
				if len(e.Attr) == 0 && len(e.Child) == 0 {
					xf.RemoveChild(e)
				} else {
					xf.CreateAttr("apply"+strings.ToUpper(key[:1])+key[1:], "1")
				}
			}
		}
		xf.CreateAttr("pivotButton", "0")
		xf.CreateAttr("quotePrefix", "0")
		orderAttributes(xf, "numFmtId", "fontId", "fillId", "borderId", "applyAlignment", "applyProtection", "pivotButton", "quotePrefix", "xfId")
	}
	for _, s := range r.FindElements("cellStyles/cellStyle") {
		if s.SelectAttr("hidden") == nil {
			s.CreateAttr("hidden", "0")
		}
	}
	if t := r.SelectElement("tableStyles"); t != nil {
		t.CreateAttr("defaultTableStyle", "TableStyleMedium9")
		orderAttributes(t, "count", "defaultTableStyle", "defaultPivotStyle")
	}
	if r.SelectElement("colors") == nil {
		colors := r.CreateElement("colors").CreateElement("indexedColors")
		for _, rgb := range strings.Fields(indexedStyleColors) {
			colors.CreateElement("rgbColor").CreateAttr("rgb", "00"+rgb)
		}
	}
	r.Attr = nil
	r.CreateAttr("xmlns", "http://schemas.openxmlformats.org/spreadsheetml/2006/main")
	out := etree.NewDocument()
	out.SetRoot(r.Copy())
	raw, err := out.WriteToBytes()
	if err != nil {
		return err
	}
	p.Set("xl/styles.xml", raw)
	return nil
}

// ECMA indexed palette, also used by pinned openpyxl.styles.colors.COLOR_INDEXED.
const indexedStyleColors = `000000 FFFFFF FF0000 00FF00 0000FF FFFF00 FF00FF 00FFFF
000000 FFFFFF FF0000 00FF00 0000FF FFFF00 FF00FF 00FFFF
800000 008000 000080 808000 800080 008080 C0C0C0 808080
9999FF 993366 FFFFCC CCFFFF 660066 FF8080 0066CC CCCCFF
000080 FF00FF FFFF00 00FFFF 800080 800000 008080 0000FF
00CCFF CCFFFF CCFFCC FFFF99 99CCFF FF99CC CC99FF FFCC99
3366FF 33CCCC 99CC00 FFCC00 FF9900 FF6600 666699 969696
003366 339966 003300 333300 993300 993366 333399 333333`
