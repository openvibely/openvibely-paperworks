package xlsx

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/beevik/etree"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"github.com/xuri/excelize/v2"
)

// serializeWorksheets follows openpyxl's worksheet/cell writer for new workbooks.
// It preserves cells and non-cell extension elements rather than rebuilding a
// worksheet from the subset of properties represented by the authoring spec.
func serializeWorksheets(p *opc.Package) error {
	var shared []*etree.Element
	if p.Has("xl/sharedStrings.xml") {
		d, err := p.XML("xl/sharedStrings.xml")
		if err != nil {
			return err
		}
		shared = d.Root().SelectElements("si")
	}
	for _, part := range p.Parts {
		if !strings.HasPrefix(part.Name, "xl/worksheets/sheet") || !strings.HasSuffix(part.Name, ".xml") {
			continue
		}
		d, err := p.XML(part.Name)
		if err != nil {
			return err
		}
		r := d.Root()
		for _, el := range r.FindElements(".//*") {
			for i := range el.Attr {
				switch el.Attr[i].Key {
				case "fitToPage", "summaryBelow", "summaryRight", "showGridLines", "tabSelected", "customWidth", "hidden", "bestFit", "collapsed", "customHeight", "showDropDown", "showInputMessage", "showErrorMessage", "allowBlank", "stopIfTrue", "aboveAverage", "equalAverage", "percent", "bottom", "useFirstPageNumber", "usePrinterDefaults", "blackAndWhite", "draft":
				default:
					continue
				}
				if el.Attr[i].Value == "true" {
					el.Attr[i].Value = "1"
				}
				if el.Attr[i].Value == "false" {
					el.Attr[i].Value = "0"
				}
			}
		}
		minX, minY, maxX, maxY := 16385, 1048577, 0, 0
		for _, c := range r.FindElements("sheetData/row/c") {
			x, y, e := excelize.CellNameToCoordinates(c.SelectAttrValue("r", ""))
			if e != nil {
				return e
			}
			if x < minX {
				minX = x
			}
			if y < minY {
				minY = y
			}
			if x > maxX {
				maxX = x
			}
			if y > maxY {
				maxY = y
			}
			typ := c.SelectAttrValue("t", "")
			if typ == "s" {
				v := c.SelectElement("v")
				if v == nil {
					return fmt.Errorf("shared string without index")
				}
				n, e := strconv.Atoi(v.Text())
				if e != nil || n < 0 || n >= len(shared) {
					return fmt.Errorf("invalid shared string index %q", v.Text())
				}
				c.RemoveChild(v)
				is := shared[n].Copy()
				is.Tag = "is"
				c.AddChild(is)
				c.CreateAttr("t", "inlineStr")
			} else if c.SelectElement("f") != nil {
				if typ == "str" {
					c.RemoveAttr("t")
				}
				if c.SelectElement("v") == nil && c.SelectElement("f").SelectAttrValue("t", "") != "dataTable" {
					c.CreateElement("v")
				}
			} else if typ == "" {
				c.CreateAttr("t", "n")
			}
			orderAttributes(c, "r", "s", "t")
		}
		if dim := r.SelectElement("dimension"); dim != nil && maxX > 0 {
			a, _ := excelize.CoordinatesToCellName(minX, minY)
			b, _ := excelize.CoordinatesToCellName(maxX, maxY)
			if a != b {
				a += ":" + b
			}
			dim.CreateAttr("ref", a)
		}
		if pr := r.SelectElement("sheetPr"); pr != nil && pr.SelectElement("outlinePr") == nil {
			o := etree.NewElement("outlinePr")
			o.CreateAttr("summaryBelow", "1")
			o.CreateAttr("summaryRight", "1")
			pr.InsertChildAt(0, o)
		}
		for _, view := range r.FindElements("sheetViews/sheetView") {
			if view.SelectAttrValue("showGridLines", "") == "1" {
				view.RemoveAttr("showGridLines")
			}
			view.RemoveAttr("tabSelected")
			if pane := view.SelectElement("pane"); pane != nil {
				orderAttributes(pane, "xSplit", "ySplit", "topLeftCell", "activePane", "state")
				if len(view.SelectElements("selection")) == 0 {
					active := pane.SelectAttrValue("activePane", "bottomRight")
					if active == "bottomRight" {
						for _, name := range []string{"topRight", "bottomLeft"} {
							view.CreateElement("selection").CreateAttr("pane", name)
						}
					}
					s := view.CreateElement("selection")
					s.CreateAttr("pane", active)
					s.CreateAttr("activeCell", "A1")
					s.CreateAttr("sqref", "A1")
				}
			}
		}
		if cols := r.SelectElement("cols"); cols != nil {
			for _, c := range cols.SelectElements("col") {
				lo, _ := strconv.Atoi(c.SelectAttrValue("min", "0"))
				hi, _ := strconv.Atoi(c.SelectAttrValue("max", "0"))
				if lo < 1 || hi > 16384 || hi < lo {
					return fmt.Errorf("invalid column bounds")
				}
				pos := c.Index()
				cols.RemoveChild(c)
				for i := lo; i <= hi; i++ {
					v := c.Copy()
					v.CreateAttr("min", strconv.Itoa(i))
					v.CreateAttr("max", strconv.Itoa(i))
					orderAttributes(v, "width", "style", "hidden", "bestFit", "customWidth", "outlineLevel", "collapsed", "min", "max")
					cols.InsertChildAt(pos+i-lo, v)
				}
			}
		}
		for _, e := range r.FindElements("conditionalFormatting/cfRule") {
			orderAttributes(e, "type", "rank", "priority", "equalAverage", "operator", "aboveAverage", "dxfId", "stdDev", "stopIfTrue", "timePeriod", "text", "percent", "bottom")
		}
		for _, e := range r.FindElements("dataValidations/dataValidation") {
			for _, k := range []string{"showDropDown", "showInputMessage", "showErrorMessage", "allowBlank"} {
				if e.SelectAttr(k) == nil {
					e.CreateAttr(k, "0")
				}
			}
			orderAttributes(e, "sqref", "showDropDown", "showInputMessage", "showErrorMessage", "allowBlank", "errorTitle", "error", "promptTitle", "prompt", "type", "errorStyle", "imeMode", "operator")
		}
		if e := r.SelectElement("pageSetup"); e != nil {
			orderAttributes(e, "orientation", "paperSize", "scale", "fitToHeight", "fitToWidth", "firstPageNumber", "useFirstPageNumber", "paperHeight", "paperWidth", "pageOrder", "usePrinterDefaults", "blackAndWhite", "draft", "cellComments", "errors", "horizontalDpi", "verticalDpi", "copies", "r:id")
		}
		// Keep the relationship namespace local to references, as the reference writer does.
		r.RemoveAttr("xmlns:r")
		for _, e := range r.FindElements(".//*") {
			if e.SelectAttr("r:id") != nil && e.SelectAttr("xmlns:r") == nil {
				e.CreateAttr("xmlns:r", opc.NSOffR)
				orderAttributes(e, "xmlns:r", "r:id")
			}
		}
		out := etree.NewDocument()
		out.SetRoot(r.Copy())
		raw, e := out.WriteToBytes()
		if e != nil {
			return e
		}
		// lxml's streaming cell/value contexts emit explicit closing tags.
		text := strings.ReplaceAll(string(raw), "<v/>", "<v></v>")
		for start := 0; ; {
			i := strings.Index(text[start:], "<c ")
			if i < 0 {
				break
			}
			i += start
			j := strings.IndexByte(text[i:], '>') + i
			if text[j-1] == '/' {
				text = text[:j-1] + "></c>" + text[j+1:]
				start = j + 4
			} else {
				start = j + 1
			}
		}
		p.Set(part.Name, []byte(text))
	}
	// All worksheet cells have been expanded; retain no dangling shared-string link.
	if p.Has("xl/sharedStrings.xml") {
		for _, name := range []string{"xl/_rels/workbook.xml.rels", "[Content_Types].xml"} {
			d, err := p.XML(name)
			if err != nil {
				return err
			}
			for _, e := range d.Root().ChildElements() {
				if e.SelectAttrValue("Type", "") == opc.NSOffR+"/sharedStrings" || e.SelectAttrValue("PartName", "") == "/xl/sharedStrings.xml" {
					d.Root().RemoveChild(e)
				}
			}
			if err = p.SetXML(name, d); err != nil {
				return err
			}
		}
		p.Remove("xl/sharedStrings.xml")
	}
	return nil
}

func orderAttributes(e *etree.Element, names ...string) {
	old := append([]etree.Attr(nil), e.Attr...)
	e.Attr = nil
	for _, name := range names {
		for _, a := range old {
			key := a.Key
			if a.Space != "" {
				key = a.Space + ":" + key
			}
			if key == name {
				e.CreateAttr(key, a.Value)
			}
		}
	}
	for _, a := range old {
		key := a.Key
		if a.Space != "" {
			key = a.Space + ":" + key
		}
		if e.SelectAttr(key) == nil {
			e.CreateAttr(key, a.Value)
		}
	}
}
