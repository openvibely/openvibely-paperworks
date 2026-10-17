package office

import (
	"fmt"
	"github.com/beevik/etree"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"github.com/xuri/excelize/v2"
	"math"
	"strconv"
)

// ReplaceCategorySeries replaces the data and entire embedded workbook of a
// single-series column chart. Unlike WorkbookInputs, it deliberately discards
// the old workbook's cells, charts and formatting. Other chart shapes are rejected.
// General reference presentation library replace_data and byte serialization equivalence remain separate.
func ReplaceCategorySeries(p *opc.Package, part, name string, categories []string, values []float64) error {
	if len(categories) == 0 || len(categories) != len(values) {
		return fmt.Errorf("equal nonempty categories and values required")
	}
	for _, v := range values {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return fmt.Errorf("finite values required")
		}
	}
	d, err := p.XML(part)
	if err != nil {
		return err
	}
	plots := All(d.Root(), NSC, "plotArea")
	if len(plots) != 1 {
		return fmt.Errorf("one plot area required")
	}
	var chart *etree.Element
	for _, e := range plots[0].ChildElements() {
		if e.NamespaceURI() == NSC && len(e.Tag) >= 5 && e.Tag[len(e.Tag)-5:] == "Chart" {
			if chart != nil {
				return fmt.Errorf("one chart required")
			}
			chart = e
		}
	}
	if chart == nil || chart.Tag != "barChart" || Child(chart, NSC, "barDir") == nil || Child(chart, NSC, "barDir").SelectAttrValue("val", "") != "col" {
		return fmt.Errorf("single column chart required")
	}
	series := Children(chart, NSC, "ser")
	if len(series) != 1 {
		return fmt.Errorf("single series required")
	}
	target, err := ChartWorkbookTarget(p, part, d.Root())
	if err != nil {
		return err
	}
	f := excelize.NewFile()
	defer f.Close()
	if err = f.SetCellValue("Sheet1", "B1", name); err != nil {
		return err
	}
	for i, c := range categories {
		row := strconv.Itoa(i + 2)
		if err = f.SetCellValue("Sheet1", "A"+row, c); err != nil {
			return err
		}
		if err = f.SetCellValue("Sheet1", "B"+row, values[i]); err != nil {
			return err
		}
	}
	raw, err := f.WriteToBuffer()
	if err != nil {
		return err
	}
	makeRef := func(tag, kind, formula string, text []string) *etree.Element {
		e := etree.NewElement("c:" + tag)
		e.CreateAttr("xmlns:c", NSC)
		r := e.CreateElement("c:" + kind + "Ref")
		r.CreateElement("c:f").SetText(formula)
		cache := r.CreateElement("c:" + kind + "Cache")
		if kind == "num" {
			cache.CreateElement("c:formatCode").SetText("General")
		}
		cache.CreateElement("c:ptCount").CreateAttr("val", strconv.Itoa(len(text)))
		for i, v := range text {
			pt := cache.CreateElement("c:pt")
			pt.CreateAttr("idx", strconv.Itoa(i))
			pt.CreateElement("c:v").SetText(v)
		}
		return e
	}
	nums := make([]string, len(values))
	for i, v := range values {
		nums[i] = strconv.FormatFloat(v, 'f', -1, 64)
	}
	end := strconv.Itoa(len(values) + 1)
	replacements := []*etree.Element{makeRef("tx", "str", "Sheet1!$B$1", []string{name}), makeRef("cat", "str", "Sheet1!$A$2:$A$"+end, categories), makeRef("val", "num", "Sheet1!$B$2:$B$"+end, nums)}
	order := []string{"idx", "order", "tx", "spPr", "invertIfNegative", "pictureOptions", "marker", "explosion", "dPt", "dLbls", "trendline", "errBars", "cat", "val", "xVal", "yVal", "shape", "smooth", "bubbleSize", "bubble3D", "extLst"}
	ranks := map[string]int{}
	for i, n := range order {
		ranks[n] = i
	}
	s := series[0]
	for _, tag := range []string{"tx", "cat", "val"} {
		for _, e := range Children(s, NSC, tag) {
			s.RemoveChild(e)
		}
	}
	for _, e := range replacements {
		inserted := false
		for _, tag := range order[ranks[e.Tag]+1:] {
			if next := Child(s, NSC, tag); next != nil {
				s.InsertChildAt(next.Index(), e)
				inserted = true
				break
			}
		}
		if !inserted {
			s.AddChild(e)
		}
	}
	if err = p.SetXML(part, d); err != nil {
		return err
	}
	p.Set(target, raw.Bytes())
	return nil
}
