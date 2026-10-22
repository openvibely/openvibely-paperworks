package xlsx

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"path"
	"strings"

	"github.com/beevik/etree"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"github.com/xuri/excelize/v2"
)

// ColumnChart authors a clustered column chart with inherited fonts and colors.
// References remain live worksheet formulas, not frozen chart caches.
type ColumnChart struct {
	Cell       string         `json:"cell"`
	Title      string         `json:"title"`
	ValueTitle string         `json:"value_title"`
	Series     []ColumnSeries `json:"series"`
	WidthEMU   int64          `json:"width_emu"`
	HeightEMU  int64          `json:"height_emu"`
}
type ColumnSeries struct {
	Name       string `json:"name"`
	Categories string `json:"categories"`
	Values     string `json:"values"`
}

func hasColumnCharts(s Spec) bool {
	for _, sh := range s.Sheets {
		if len(sh.ColumnCharts) > 0 {
			return true
		}
	}
	return false
}
func chartEscape(s string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(s))
	return strings.NewReplacer("&#39;", "'", "&#34;", `"`).Replace(b.String())
}
func inheritedTitle(s string) string {
	return `<title><tx><rich><a:bodyPr xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main"/><a:p xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main"><a:pPr><a:defRPr/></a:pPr><a:r><a:t>` + chartEscape(s) + `</a:t></a:r></a:p></rich></tx></title>`
}
func inheritedColumnXML(c ColumnChart) ([]byte, error) {
	if len(c.Series) == 0 || len(c.Series) > 255 {
		return nil, fmt.Errorf("column chart requires 1..255 series")
	}
	var b strings.Builder
	b.WriteString(`<chartSpace xmlns="http://schemas.openxmlformats.org/drawingml/2006/chart"><chart>`)
	if c.Title != "" {
		b.WriteString(inheritedTitle(c.Title))
	}
	b.WriteString(`<plotArea><barChart><barDir val="col"/><grouping val="clustered"/>`)
	for i, s := range c.Series {
		if s.Name == "" || s.Categories == "" || s.Values == "" {
			return nil, fmt.Errorf("column chart series requires name, categories and values references")
		}
		fmt.Fprintf(&b, `<ser><idx val="%d"/><order val="%d"/><tx><strRef><f>%s</f></strRef></tx><spPr><a:ln xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main"><a:prstDash val="solid"/></a:ln></spPr><cat><numRef><f>%s</f></numRef></cat><val><numRef><f>%s</f></numRef></val></ser>`, i, i, chartEscape(s.Name), chartEscape(s.Categories), chartEscape(s.Values))
	}
	b.WriteString(`<gapWidth val="150"/><axId val="10"/><axId val="100"/></barChart><catAx><axId val="10"/><scaling><orientation val="minMax"/></scaling><axPos val="l"/><majorTickMark val="none"/><minorTickMark val="none"/><crossAx val="100"/><lblOffset val="100"/></catAx><valAx><axId val="100"/><scaling><orientation val="minMax"/></scaling><axPos val="l"/><majorGridlines/>`)
	if c.ValueTitle != "" {
		b.WriteString(inheritedTitle(c.ValueTitle))
	}
	b.WriteString(`<majorTickMark val="none"/><minorTickMark val="none"/><crossAx val="10"/></valAx></plotArea><legend><legendPos val="r"/></legend><plotVisOnly val="1"/><dispBlanksAs val="gap"/></chart></chartSpace>`)
	return []byte(b.String()), nil
}
func patchColumnCharts(p *opc.Package, spec Spec) error {
	for i, s := range spec.Sheets {
		if len(s.ColumnCharts) == 0 {
			continue
		}
		if len(s.Charts) > 0 || len(s.Pictures) > 0 {
			return fmt.Errorf("column_charts cannot share a sheet with charts or pictures")
		}
		sheet := fmt.Sprintf("xl/worksheets/sheet%d.xml", i+1)
		doc, e := p.XML(sheet)
		if e != nil {
			return e
		}
		drawing := p.UnusedName("xl/drawings/drawing", ".xml")
		rels := etree.NewDocument()
		rr := rels.CreateElement("Relationships")
		rr.CreateAttr("xmlns", opc.NSRel)
		d := etree.NewDocument()
		root := d.CreateElement("wsDr")
		root.CreateAttr("xmlns", "http://schemas.openxmlformats.org/drawingml/2006/spreadsheetDrawing")
		for j, c := range s.ColumnCharts {
			col, row, e := excelize.CellNameToCoordinates(c.Cell)
			if e != nil {
				return e
			}
			if c.WidthEMU <= 0 || c.HeightEMU <= 0 || c.WidthEMU > 2147483647 || c.HeightEMU > 2147483647 {
				return fmt.Errorf("column chart dimensions must be positive EMUs <=2147483647")
			}
			raw, e := inheritedColumnXML(c)
			if e != nil {
				return e
			}
			chart := p.UnusedName("xl/charts/chart", ".xml")
			p.Set(chart, raw)
			if e = p.EnsureContentType(chart, "application/vnd.openxmlformats-officedocument.drawingml.chart+xml"); e != nil {
				return e
			}
			id := fmt.Sprintf("rId%d", j+1)
			rel := rr.CreateElement("Relationship")
			rel.CreateAttr("Type", opc.NSOffR+"/chart")
			rel.CreateAttr("Target", "/"+chart)
			rel.CreateAttr("Id", id)
			fragment := fmt.Sprintf(`<oneCellAnchor><from><col>%d</col><colOff>0</colOff><row>%d</row><rowOff>0</rowOff></from><ext cx="%d" cy="%d"/><graphicFrame><nvGraphicFramePr><cNvPr id="%d" name="Chart %d"/><cNvGraphicFramePr/></nvGraphicFramePr><xfrm/><a:graphic xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main"><a:graphicData uri="http://schemas.openxmlformats.org/drawingml/2006/chart"><c:chart xmlns:c="http://schemas.openxmlformats.org/drawingml/2006/chart" xmlns:r="%s" r:id="%s"/></a:graphicData></a:graphic></graphicFrame><clientData/></oneCellAnchor>`, col-1, row-1, c.WidthEMU, c.HeightEMU, j+1, j+1, opc.NSOffR, id)
			f := etree.NewDocument()
			if e = f.ReadFromString(fragment); e != nil {
				return e
			}
			root.AddChild(f.Root())
		}
		// openpyxl writes drawing roots directly, without an XML declaration.
		raw, err := d.WriteToBytes()
		if err != nil {
			return err
		}
		p.Set(drawing, raw)
		raw, err = rels.WriteToBytes()
		if err != nil {
			return err
		}
		p.Set(opc.RelsName(drawing), raw)
		if e = p.EnsureContentType(drawing, "application/vnd.openxmlformats-officedocument.drawing+xml"); e != nil {
			return e
		}
		id, e := p.AddRel(sheet, opc.NSOffR+"/drawing", "../drawings/"+path.Base(drawing), false)
		if e != nil {
			return e
		}
		n := doc.Root().CreateElement("drawing")
		n.CreateAttr("xmlns:r", opc.NSOffR)
		n.CreateAttr("r:id", id)
		if e = p.SetXML(sheet, doc); e != nil {
			return e
		}
	}
	return nil
}
