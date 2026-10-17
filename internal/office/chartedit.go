package office

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/beevik/etree"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"github.com/xuri/excelize/v2"
)

var cellAddrRe = regexp.MustCompile(`^[A-Z]+[1-9][0-9]*$`)

// WorkbookInputs patches literal cells of one sheet in raw XLSX bytes
// without discarding formulas, caches or extensions. It returns the new
// bytes and the changed worksheet part.
func WorkbookInputs(raw []byte, sheet string, cells map[string]any) ([]byte, string, error) {
	if len(cells) == 0 || len(cells) > 10000 {
		return nil, "", errors.New("provide 1–10000 literal input cells")
	}
	p, err := opc.Read(raw)
	if err != nil {
		return nil, "", err
	}
	_, _, byName, err := WorkbookSheets(p)
	if err != nil {
		return nil, "", err
	}
	part, ok := byName[sheet]
	if !ok {
		return nil, "", errors.New("unknown input sheet")
	}
	if part == "" || !p.Has(part) {
		return nil, "", errors.New("input sheet must be internal")
	}
	doc, err := p.XML(part)
	if err != nil {
		return nil, "", err
	}
	root := doc.Root()
	data := Child(root, NSS, "sheetData")
	if data == nil {
		return nil, "", errors.New("input worksheet has no sheet data")
	}
	var blocked []*etree.Element
	blocked = append(blocked, All(Child(root, NSS, "mergeCells"), NSS, "mergeCell")...)
	blocked = append(blocked, All(root, NSS, "f")...)
	addrs := make([]string, 0, len(cells))
	for a := range cells {
		addrs = append(addrs, a)
	}
	slices.Sort(addrs)
	for _, addr := range addrs {
		if err := setInput(data, blocked, addr, cells[addr]); err != nil {
			return nil, "", err
		}
	}
	if dim := Child(root, NSS, "dimension"); dim != nil {
		l, t, r, b := math.MaxInt, math.MaxInt, 0, 0
		for _, c := range All(data, NSS, "c") {
			col, row, err := SplitCell(c.SelectAttrValue("r", ""))
			if err != nil {
				continue
			}
			l, t, r, b = min(l, col), min(t, row), max(r, col), max(b, row)
		}
		if r > 0 {
			dim.CreateAttr("ref", fmt.Sprintf("%s%d:%s%d", ColumnName(l), t, ColumnName(r), b))
		}
	}
	dataXML, err := workbookInputXML(doc)
	if err != nil {
		return nil, "", err
	}
	p.Set(part, dataXML)
	out, err := p.Bytes()
	return out, part, err
}

// workbookInputXML matches the input editor's lxml element/declaration boundary.
// It serializes the worksheet root, excluding document-level siblings. This is
// distinct from the minidom document writer used when restoring formula caches.
// General parser and namespace-allocation equivalence remains separate.
func workbookInputXML(source *etree.Document) ([]byte, error) {
	if source.Root() == nil {
		return nil, errors.New("input worksheet has no root")
	}
	doc := etree.NewDocument()
	doc.SetRoot(source.Root().Copy())
	data, err := doc.WriteToBytes()
	if err != nil {
		return nil, err
	}
	return append([]byte("<?xml version='1.0' encoding='UTF-8' standalone='yes'?>\n"), data...), nil
}

func setInput(data *etree.Element, blocked []*etree.Element, addr string, value any) error {
	if !cellAddrRe.MatchString(addr) {
		return errors.New("input must be an A1 cell address")
	}
	col, row, err := SplitCell(addr)
	if err != nil || row > 1048576 || col > 16384 {
		return errors.New("cell outside worksheet limits")
	}
	for _, area := range blocked {
		ref := area.SelectAttrValue("ref", "")
		if ref == "" {
			continue
		}
		l, t, r, b, err := ParseRange(ref)
		if err == nil && l <= col && col <= r && t <= row && row <= b {
			return errors.New("chart input cannot intersect merged cells or array/table formula ranges")
		}
	}
	var rowEl *etree.Element
	for _, e := range Children(data, NSS, "row") {
		n, _ := strconv.Atoi(e.SelectAttrValue("r", "0"))
		if n == row {
			rowEl = e
			break
		}
	}
	if rowEl == nil {
		rowEl = etree.NewElement(qual(data.Space, "row"))
		rowEl.CreateAttr("r", strconv.Itoa(row))
		insertSorted(data, rowEl, func(e *etree.Element) bool {
			n, _ := strconv.Atoi(e.SelectAttrValue("r", "0"))
			return n > row
		})
	}
	var cell *etree.Element
	for _, e := range Children(rowEl, NSS, "c") {
		if e.SelectAttrValue("r", "") == addr {
			cell = e
			break
		}
	}
	if cell == nil {
		cell = etree.NewElement(qual(rowEl.Space, "c"))
		cell.CreateAttr("r", addr)
		insertSorted(rowEl, cell, func(e *etree.Element) bool {
			if !Is(e, NSS, "c") {
				return true
			}
			c, _, _ := SplitCell(e.SelectAttrValue("r", ""))
			return c > col
		})
	}
	if Child(cell, NSS, "f") != nil {
		return errors.New("chart input edits must not overwrite original formulas")
	}
	text, kind, err := inputText(value)
	if err != nil {
		return err
	}
	RemoveAll(cell, NSS, "v")
	RemoveAll(cell, NSS, "is")
	cell.RemoveAttr("t")
	switch kind {
	case "":
	case "inlineStr":
		cell.CreateAttr("t", "inlineStr")
		t := NewChild(NewChild(cell, NSS, "is", []string{"extLst"}), NSS, "t", nil)
		t.CreateAttr("xml:space", "preserve")
		t.SetText(text)
	default:
		cell.CreateAttr("t", kind)
		NewChild(cell, NSS, "v", []string{"extLst"}).SetText(text)
	}
	return nil
}

func inputText(value any) (string, string, error) {
	switch v := value.(type) {
	case nil:
		return "", "", nil
	case bool:
		if v {
			return "1", "b", nil
		}
		return "0", "b", nil
	case string:
		if strings.HasPrefix(v, "=") {
			return "", "", errors.New("chart input edits require literal values")
		}
		return v, "inlineStr", nil
	case json.Number:
		if i, err := strconv.ParseInt(string(v), 10, 64); err == nil {
			return strconv.FormatInt(i, 10), "n", nil
		}
		f, err := v.Float64()
		if err != nil || math.IsInf(f, 0) || math.IsNaN(f) {
			return "", "", errors.New("nonfinite input value")
		}
		return pyRepr(f), "n", nil
	case float64:
		if math.IsInf(v, 0) || math.IsNaN(v) {
			return "", "", errors.New("nonfinite input value")
		}
		if v == math.Trunc(v) && math.Abs(v) < 1e15 {
			return strconv.FormatInt(int64(v), 10), "n", nil
		}
		return pyRepr(v), "n", nil
	}
	return "", "", errors.New("input value must be scalar")
}

func insertSorted(parent, e *etree.Element, after func(*etree.Element) bool) {
	for _, c := range parent.ChildElements() {
		if after(c) {
			parent.InsertChildAt(c.Index(), e)
			return
		}
	}
	parent.AddChild(e)
}

// MaterializeLiteralCharts snapshots literal chart series into new embedded
// workbooks; it never replaces existing workbook lineage.
func MaterializeLiteralCharts(p *opc.Package) ([]string, error) {
	names, err := ChartParts(p)
	if err != nil {
		return nil, err
	}
	var made []string
	for _, name := range names {
		doc, err := p.XML(name)
		if err != nil {
			return nil, err
		}
		root := doc.Root()
		literals := append(All(root, NSC, "numLit"), All(root, NSC, "strLit")...)
		if len(literals) == 0 {
			continue
		}
		if Child(root, NSC, "externalData") != nil || len(chartRefs(root)) > 0 || len(All(root, NSC, "multiLvlStrRef")) > 0 {
			return nil, errors.New("literal materialization cannot replace existing chart workbook lineage")
		}
		f := excelize.NewFile()
		const title = "Chart data"
		if err := f.SetSheetName("Sheet1", title); err != nil {
			return nil, err
		}
		for col, lit := range literals {
			pts, err := ChartLiteral(lit)
			if err != nil {
				return nil, err
			}
			numeric := lit.Tag == "numLit"
			letter := ColumnName(col + 1)
			for row, v := range pts {
				if v == nil {
					continue
				}
				cell := letter + strconv.Itoa(row+1)
				if numeric {
					n, _ := strconv.ParseFloat(strings.TrimSpace(*v), 64)
					err = f.SetCellFloat(title, cell, n, -1, 64)
				} else {
					err = f.SetCellStr(title, cell, *v)
				}
				if err != nil {
					return nil, err
				}
			}
			tag, cacheTag := "strRef", "strCache"
			if numeric {
				tag, cacheTag = "numRef", "numCache"
			}
			ref := etree.NewElement(qual(lit.Space, tag))
			ref.CreateElement(qual(lit.Space, "f")).SetText(fmt.Sprintf("'%s'!$%s$1:$%s$%d", title, letter, letter, len(pts)))
			cache := ref.CreateElement(qual(lit.Space, cacheTag))
			for _, c := range lit.ChildElements() {
				cache.AddChild(c)
			}
			parent := lit.Parent()
			parent.InsertChildAt(lit.Index(), ref)
			parent.RemoveChild(lit)
		}
		stem := strings.TrimSuffix(path.Base(name), path.Ext(name))
		target := "ppt/embeddings/" + stem + "-literal.xlsx"
		if p.Has(target) {
			return nil, errors.New("literal workbook destination already exists")
		}
		buf, err := f.WriteToBuffer()
		if err != nil {
			return nil, err
		}
		p.Set(target, buf.Bytes())
		rel, _ := relPath(name, target)
		id, err := literalWorkbookRelationship(p, name, rel)
		if err != nil {
			return nil, err
		}
		ext := etree.NewElement(qual(root.Space, "externalData"))
		ext.CreateAttr(qual(nsPrefixOrR(root), "id"), id)
		ext.CreateElement(qual(root.Space, "autoUpdate")).CreateAttr("val", "0")
		InsertBefore(root, ext, []string{"printSettings", "userShapes", "extLst"})
		if err := p.SetXML(name, doc); err != nil {
			return nil, err
		}
		if err := p.EnsureContentType(target, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"); err != nil {
			return nil, err
		}
		made = append(made, name)
	}
	return made, nil
}

func nsPrefixOrR(root *etree.Element) string {
	if pre := nsPrefix(root, NSR); pre != "" {
		return pre
	}
	root.CreateAttr("xmlns:r", NSR)
	return "r"
}

// literalWorkbookRelationship follows the reference materializer's stable ID
// policy, preserving all existing relationships and their order.
func literalWorkbookRelationship(p *opc.Package, chart, target string) (string, error) {
	name := opc.RelsName(chart)
	doc := etree.NewDocument()
	if p.Has(name) {
		var err error
		doc, err = p.XML(name)
		if err != nil {
			return "", err
		}
	} else {
		doc.CreateElement("Relationships").CreateAttr("xmlns", opc.NSRel)
	}
	root := doc.Root()
	if root == nil {
		return "", errors.New("chart relationships root missing")
	}
	used := map[string]bool{}
	for _, child := range root.ChildElements() {
		used[child.SelectAttrValue("Id", "")] = true
	}
	id := "literalWorkbook"
	for used[id] {
		id += "1"
	}
	rel := root.CreateElement(qual(root.Space, "Relationship"))
	rel.CreateAttr("Id", id)
	rel.CreateAttr("Type", opc.NSOffR+"/package")
	rel.CreateAttr("Target", target)
	return id, p.SetXML(name, doc)
}
