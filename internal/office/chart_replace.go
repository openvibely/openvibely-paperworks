package office

import (
	"bytes"
	"fmt"
	"math"
	"path"
	"strconv"
	"strings"

	"github.com/beevik/etree"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"github.com/xuri/excelize/v2"
)

const embeddedWorkbookContentType = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"

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
	shared, err := chartWorkbookIsShared(p, part, target)
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

	chartXML, err := replacementXMLBytes(d)
	if err != nil {
		return err
	}
	newTarget := target
	var relsXML, contentTypesXML []byte
	if shared {
		newTarget = unusedWorkbookPart(p, target)
		relsXML, err = chartWorkbookRelationshipBytes(p, part, d.Root(), target, newTarget)
		if err != nil {
			return err
		}
		contentTypesXML, err = workbookContentTypeBytes(p, newTarget)
		if err != nil {
			return err
		}
	}

	// All fallible parsing and serialization is complete. Apply the staged changes
	// together so an isolation error cannot leave a partially edited package.
	p.Set(part, chartXML)
	if shared {
		p.Set(opc.RelsName(part), relsXML)
		if contentTypesXML != nil {
			p.Set("[Content_Types].xml", contentTypesXML)
		}
	}
	p.Set(newTarget, raw.Bytes())
	return nil
}

func chartWorkbookIsShared(p *opc.Package, editedPart, target string) (bool, error) {
	parts, err := ChartParts(p)
	if err != nil {
		return false, fmt.Errorf("cannot determine whether embedded workbook %s is shared: %w", target, err)
	}
	for _, part := range parts {
		if part == editedPart {
			continue
		}
		doc, err := p.XML(part)
		if err != nil {
			return false, fmt.Errorf("cannot determine whether embedded workbook %s is shared: %w", target, err)
		}
		root := doc.Root()
		if Is(root, NSCX, "chartSpace") {
			if Child(Child(root, NSCX, "chartData"), NSCX, "externalData") == nil {
				continue
			}
		} else if Child(root, NSC, "externalData") == nil {
			continue
		}
		otherTarget, err := ChartWorkbookTarget(p, part, root)
		if err != nil {
			return false, fmt.Errorf("cannot determine whether embedded workbook %s is shared: %w", target, err)
		}
		if otherTarget == target {
			return true, nil
		}
	}
	return false, nil
}

func unusedWorkbookPart(p *opc.Package, target string) string {
	ext := path.Ext(target)
	base := strings.TrimSuffix(path.Base(target), ext)
	return p.UnusedName(path.Join(path.Dir(target), base+"-"), ext)
}

func chartWorkbookRelationshipBytes(p *opc.Package, part string, root *etree.Element, oldTarget, newTarget string) ([]byte, error) {
	ext := Child(root, NSC, "externalData")
	if ext == nil {
		return nil, fmt.Errorf("cannot isolate shared embedded workbook: chart has no externalData relationship")
	}
	relID := AttrV(ext, NSR, "id")
	relsName := opc.RelsName(part)
	doc, err := p.XML(relsName)
	if err != nil {
		return nil, fmt.Errorf("cannot isolate shared embedded workbook: %w", err)
	}
	var match *etree.Element
	for _, rel := range doc.Root().ChildElements() {
		if rel.SelectAttrValue("Id", "") != relID {
			continue
		}
		if match != nil {
			return nil, fmt.Errorf("cannot isolate shared embedded workbook: relationship %s is ambiguous", relID)
		}
		match = rel
	}
	if match == nil || match.SelectAttrValue("TargetMode", "") == "External" || !strings.HasSuffix(match.SelectAttrValue("Type", ""), "/package") {
		return nil, fmt.Errorf("cannot isolate shared embedded workbook: package relationship %s is unavailable", relID)
	}
	if got := opc.Resolve(part, match.SelectAttrValue("Target", "")); got != oldTarget {
		return nil, fmt.Errorf("cannot isolate shared embedded workbook: relationship %s no longer targets %s", relID, oldTarget)
	}
	if attr := match.SelectAttr("Target"); attr != nil {
		attr.Value = opc.Relative(part, newTarget)
	} else {
		match.CreateAttr("Target", opc.Relative(part, newTarget))
	}
	return replacementXMLBytes(doc)
}

func workbookContentTypeBytes(p *opc.Package, target string) ([]byte, error) {
	if p.ContentType(target) == embeddedWorkbookContentType {
		return nil, nil
	}
	doc, err := p.XML("[Content_Types].xml")
	if err != nil {
		return nil, fmt.Errorf("cannot register isolated embedded workbook: %w", err)
	}
	override := doc.Root().CreateElement("Override")
	override.CreateAttr("PartName", "/"+strings.TrimPrefix(target, "/"))
	override.CreateAttr("ContentType", embeddedWorkbookContentType)
	data, err := replacementXMLBytes(doc)
	if err != nil {
		return nil, fmt.Errorf("cannot register isolated embedded workbook: %w", err)
	}
	return data, nil
}

func replacementXMLBytes(doc *etree.Document) ([]byte, error) {
	doc.WriteSettings.CanonicalEndTags = false
	data, err := doc.WriteToBytes()
	if err != nil {
		return nil, err
	}
	data = bytes.TrimPrefix(data, []byte{0xef, 0xbb, 0xbf})
	if !bytes.HasPrefix(data, []byte("<?xml")) {
		data = append([]byte(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`+"\n"), data...)
	}
	return data, nil
}
