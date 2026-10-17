package xlsx

import (
	"sort"
	"strconv"
	"strings"

	"github.com/beevik/etree"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"github.com/xuri/excelize/v2"
)

// serializeWorkbook applies openpyxl's new-workbook defaults. This is only
// called by authoring; editing existing workbooks must preserve their settings.
func serializeWorkbook(p *opc.Package, spec Spec) error {
	d, err := p.XML("xl/workbook.xml")
	if err != nil {
		return err
	}
	r := d.Root()
	r.RemoveAttr("xmlns:mc")
	r.RemoveAttr("mc:Ignorable")
	r.RemoveAttr("xmlns:x15")
	r.RemoveAttr("xmlns:r")
	if e := r.SelectElement("fileVersion"); e != nil {
		r.RemoveChild(e)
	}
	props := r.SelectElement("workbookPr")
	if props != nil {
		props.RemoveAttr("filterPrivacy")
		props.RemoveAttr("defaultThemeVersion")
	}
	if r.SelectElement("workbookProtection") == nil {
		e := etree.NewElement("workbookProtection")
		r.InsertChildAt(1, e)
	}
	if views := r.SelectElement("bookViews"); views != nil {
		for _, v := range views.SelectElements("workbookView") {
			active := v.SelectAttrValue("activeTab", "0")
			// Excelize supplies window dimensions that were never specified by the
			// author. The pinned writer leaves those optional properties absent.
			for _, k := range []string{"xWindow", "yWindow", "windowWidth", "windowHeight"} {
				v.RemoveAttr(k)
			}
			for _, a := range [][2]string{{"visibility", "visible"}, {"minimized", "0"}, {"showHorizontalScroll", "1"}, {"showVerticalScroll", "1"}, {"showSheetTabs", "1"}, {"tabRatio", "600"}, {"firstSheet", "0"}, {"activeTab", active}, {"autoFilterDateGrouping", "1"}} {
				v.CreateAttr(a[0], a[1])
			}
			orderAttributes(v, "visibility", "minimized", "showHorizontalScroll", "showVerticalScroll", "showSheetTabs", "tabRatio", "firstSheet", "activeTab", "autoFilterDateGrouping")
		}
	}
	for _, s := range r.FindElements("sheets/sheet") {
		s.CreateAttr("xmlns:r", opc.NSOffR)
		if s.SelectAttr("state") == nil {
			s.CreateAttr("state", "visible")
		}
		orderAttributes(s, "xmlns:r", "name", "sheetId", "state", "r:id")
	}
	names := r.SelectElement("definedNames")
	if names == nil {
		names = etree.NewElement("definedNames")
		if c := r.SelectElement("calcPr"); c != nil {
			r.InsertChild(c, names)
		} else {
			r.AddChild(names)
		}
	}
	for _, n := range names.SelectElements("definedName") {
		orderAttributes(n, "name", "comment", "customMenu", "description", "help", "statusBar", "localSheetId", "hidden", "function", "vbProcedure", "xlm", "functionGroupId", "shortcutKey", "publishToServer", "workbookParameter")
		if n.SelectAttrValue("name", "") != "_xlnm.Print_Area" {
			continue
		}
		i, e := strconv.Atoi(n.SelectAttrValue("localSheetId", ""))
		if e != nil || i < 0 || i >= len(spec.Sheets) || spec.Sheets[i].PrintArea == "" {
			continue
		}
		a, b, e := bounds(spec.Sheets[i].PrintArea)
		if e != nil {
			return e
		}
		abs := func(s string) (string, error) {
			x, y, e := excelize.CellNameToCoordinates(s)
			if e != nil {
				return "", e
			}
			return excelize.CoordinatesToCellName(x, y, true)
		}
		a, e = abs(a)
		if e != nil {
			return e
		}
		b, e = abs(b)
		if e != nil {
			return e
		}
		n.SetText("'" + strings.ReplaceAll(spec.Sheets[i].Name, "'", "''") + "'!" + a + ":" + b)
	}
	if c := r.SelectElement("calcPr"); c != nil {
		c.CreateAttr("calcId", "124519")
		c.CreateAttr("fullCalcOnLoad", "1")
	}
	rels, err := p.XML("xl/_rels/workbook.xml.rels")
	if err != nil {
		return err
	}
	entries := rels.Root().ChildElements()
	rank := func(e *etree.Element) int {
		switch e.SelectAttrValue("Type", "") {
		case opc.NSOffR + "/worksheet":
			return 0
		case opc.NSOffR + "/styles":
			return 1
		case opc.NSOffR + "/theme":
			return 2
		}
		return 3
	}
	sort.SliceStable(entries, func(i, j int) bool { return rank(entries[i]) < rank(entries[j]) })
	ids := map[string]string{}
	for i, e := range entries {
		old := e.SelectAttrValue("Id", "")
		id := "rId" + strconv.Itoa(i+1)
		ids[old] = id
		e.CreateAttr("Id", id)
		rels.Root().RemoveChild(e)
		rels.Root().AddChild(e)
	}
	for _, e := range r.FindElements(".//*") {
		for i := range e.Attr {
			a := &e.Attr[i]
			if a.Space == "r" && a.Key == "id" {
				if id, ok := ids[a.Value]; ok {
					a.Value = id
				}
			}
		}
	}
	ns := names.ChildElements()
	sort.SliceStable(ns, func(i, j int) bool {
		return ns[i].SelectAttr("localSheetId") == nil && ns[j].SelectAttr("localSheetId") != nil
	})
	for _, n := range ns {
		names.RemoveChild(n)
		names.AddChild(n)
	}
	if err = writeWorkbookXML(p, "xl/workbook.xml", r); err != nil {
		return err
	}
	for _, e := range rels.Root().ChildElements() {
		if e.SelectAttrValue("Type", "") == opc.NSOffR+"/worksheet" {
			target := e.SelectAttrValue("Target", "")
			if !strings.HasPrefix(target, "/") {
				e.CreateAttr("Target", "/xl/"+target)
			}
		}
		orderAttributes(e, "Type", "Target", "Id", "TargetMode")
	}
	return writeWorkbookXML(p, "xl/_rels/workbook.xml.rels", rels.Root())
}

func writeWorkbookXML(p *opc.Package, name string, r *etree.Element) error {
	d := etree.NewDocument()
	d.SetRoot(r.Copy())
	raw, err := d.WriteToBytes()
	if err != nil {
		return err
	}
	// Element text uses literal quotes in lxml, while attributes retain escaping.
	var b strings.Builder
	unquote := strings.NewReplacer("&#39;", "'", "&apos;", "'", "&#34;", "\"", "&quot;", "\"")
	for _, segment := range strings.SplitAfter(string(raw), ">") {
		if i := strings.IndexByte(segment, '<'); i >= 0 {
			b.WriteString(unquote.Replace(segment[:i]))
			b.WriteString(segment[i:])
		} else {
			b.WriteString(unquote.Replace(segment))
		}
	}
	p.Set(name, []byte(strings.ReplaceAll(b.String(), "&apos;", "'")))
	return nil
}
