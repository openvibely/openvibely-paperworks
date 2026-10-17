// Package testsupport contains independent readers used only by native tests.
package testsupport

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"path"
	"strconv"
	"strings"

	"github.com/beevik/etree"
)

// ActiveWorksheetCharts follows openpyxl 3.1.5's workbook -> active worksheet ->
// drawing relationships -> anchored charts acquisition. It does not count orphan
// chart parts. Full openpyxl descriptor validation and chartsheets are unsupported;
// unsupported roots/chart types fail explicitly rather than establishing equivalence.
func ActiveWorksheetCharts(data []byte) ([]string, error) {
	return worksheetObjects(data, "", false, "", "", "")
}

// NamedWorksheetCharts follows the named worksheet rather than scanning all
// chart parts or assuming it is the active sheet.
func NamedWorksheetCharts(data []byte, name string) ([]string, error) {
	if name == "" {
		return nil, fmt.Errorf("worksheet name is required")
	}
	return worksheetObjects(data, name, false, "", "", "")
}

// NamedWorksheetTables reads tableParts in worksheet order, resolves their
// relationships (including absolute package paths), and returns openpyxl table
// dictionary keys. General descriptor validation remains outside this reader.
func NamedWorksheetTables(data []byte, name string) ([]string, error) {
	if name == "" {
		return nil, fmt.Errorf("missing worksheet name")
	}
	return worksheetObjects(data, name, true, "", "", "")
}

// NamedWorksheetTableRef resolves the table by its dictionary name, including
// absolute OPC relationship targets emitted by openpyxl.
func NamedWorksheetTableRef(data []byte, sheet, table string) (string, error) {
	if sheet == "" || table == "" {
		return "", fmt.Errorf("sheet and table are required")
	}
	refs, err := worksheetObjects(data, sheet, true, "", "", table)
	if err != nil {
		return "", err
	}
	if len(refs) != 1 || refs[0] == "" {
		return "", fmt.Errorf("missing table or reference: %s", table)
	}
	return refs[0], nil
}

// ActiveWorksheetArrayFormula follows workbook activeTab and worksheet relationships.
// This bounded reader requires explicit cell coordinates; it never guesses a part path.
func ActiveWorksheetArrayFormula(data []byte, cell string) (bool, error) {
	if cell == "" {
		return false, fmt.Errorf("missing cell coordinate")
	}
	values, err := worksheetObjects(data, "", false, cell, "", "")
	return len(values) == 1 && values[0] == "array", err
}

// NamedWorksheetCommentText reads the comment attached to one worksheet cell,
// including rich-text runs that Excelize exposes separately from Comment.Text.
func NamedWorksheetCommentText(data []byte, name, cell string) (string, bool, error) {
	if name == "" || cell == "" {
		return "", false, fmt.Errorf("missing worksheet or cell")
	}
	values, err := worksheetObjects(data, name, false, "", cell, "")
	if err != nil || len(values) == 0 {
		return "", false, err
	}
	return values[0], true, nil
}
func worksheetObjects(data []byte, name string, tables bool, arrayCell, commentCell, tableRef string) ([]string, error) {
	const r = "http://schemas.openxmlformats.org/officeDocument/2006/relationships"
	const s = "http://schemas.openxmlformats.org/spreadsheetml/2006/main"
	const d = "http://schemas.openxmlformats.org/drawingml/2006/spreadsheetDrawing"
	const a = "http://schemas.openxmlformats.org/drawingml/2006/main"
	const c = "http://schemas.openxmlformats.org/drawingml/2006/chart"
	const pr = "http://schemas.openxmlformats.org/package/2006/relationships"
	z, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, err
	}
	files := map[string]*zip.File{}
	for _, f := range z.File {
		if files[f.Name] != nil {
			return nil, fmt.Errorf("duplicate part %s", f.Name)
		}
		files[f.Name] = f
	}
	xml := func(n string) (*etree.Element, error) {
		f := files[n]
		if f == nil {
			return nil, fmt.Errorf("missing part %s", n)
		}
		stream, e := f.Open()
		if e != nil {
			return nil, e
		}
		defer stream.Close()
		raw, e := io.ReadAll(stream)
		if e != nil {
			return nil, e
		}
		doc := etree.NewDocument()
		if e = doc.ReadFromBytes(raw); e != nil {
			return nil, e
		}
		if doc.Root() == nil {
			return nil, fmt.Errorf("empty XML %s", n)
		}
		return doc.Root(), nil
	}
	child := func(e *etree.Element, ns, tag string) *etree.Element {
		if e != nil {
			for _, v := range e.ChildElements() {
				if v.NamespaceURI() == ns && v.Tag == tag {
					return v
				}
			}
		}
		return nil
	}
	attr := func(e *etree.Element, ns, key string) string {
		if e != nil {
			for _, v := range e.Attr {
				if v.Key == key && v.NamespaceURI() == ns {
					return v.Value
				}
			}
		}
		return ""
	}
	type rel struct{ id, kind, target string }
	relations := func(owner string) ([]rel, error) {
		name := "_rels/.rels"
		if owner != "" {
			name = path.Join(path.Dir(owner), "_rels", path.Base(owner)+".rels")
		}
		if files[name] == nil {
			return nil, nil
		}
		root, e := xml(name)
		if e != nil {
			return nil, e
		}
		var rows []rel
		for _, v := range root.ChildElements() {
			if v.NamespaceURI() != pr || v.Tag != "Relationship" {
				continue
			}
			if v.SelectAttrValue("TargetMode", "") == "External" {
				continue
			}
			target := v.SelectAttrValue("Target", "")
			if strings.HasPrefix(target, "/") {
				target = strings.TrimPrefix(path.Clean(target), "/")
			} else {
				target = path.Clean(path.Join(path.Dir(owner), target))
			}
			rows = append(rows, rel{v.SelectAttrValue("Id", ""), v.SelectAttrValue("Type", ""), target})
		}
		return rows, nil
	}
	resolve := func(rows []rel, id, kind string) (string, error) {
		for _, v := range rows {
			if v.kind == kind && (id == "" || v.id == id) {
				return v.target, nil
			}
		}
		return "", fmt.Errorf("missing relationship %s %s", id, kind)
	}
	rows, err := relations("")
	if err != nil {
		return nil, err
	}
	main, err := resolve(rows, "", r+"/officeDocument")
	if err != nil {
		return nil, err
	}
	book, err := xml(main)
	if err != nil {
		return nil, err
	}
	active := 0
	if view := child(child(book, s, "bookViews"), s, "workbookView"); view != nil {
		active, err = strconv.Atoi(view.SelectAttrValue("activeTab", "0"))
		if err != nil {
			return nil, err
		}
	}
	sheets := child(book, s, "sheets")
	if sheets == nil {
		return nil, fmt.Errorf("missing sheets")
	}
	var entries []*etree.Element
	for _, e := range sheets.ChildElements() {
		if e.NamespaceURI() == s && e.Tag == "sheet" {
			entries = append(entries, e)
		}
	}
	if name != "" {
		active = -1
		for i, e := range entries {
			if e.SelectAttrValue("name", "") == name {
				active = i
				break
			}
		}
	}
	if active < 0 || active >= len(entries) {
		return nil, fmt.Errorf("active sheet out of range")
	}
	rows, err = relations(main)
	if err != nil {
		return nil, err
	}
	sheet, err := resolve(rows, attr(entries[active], r, "id"), r+"/worksheet")
	if err != nil {
		return nil, err
	}
	worksheet, err := xml(sheet)
	if err != nil {
		return nil, err
	}
	if worksheet.NamespaceURI() != s || worksheet.Tag != "worksheet" {
		return nil, fmt.Errorf("unsupported worksheet root")
	}
	if arrayCell != "" {
		kind := ""
		if data := child(worksheet, s, "sheetData"); data != nil {
			for _, row := range data.ChildElements() {
				if row.NamespaceURI() != s || row.Tag != "row" {
					continue
				}
				for _, cell := range row.ChildElements() {
					if cell.NamespaceURI() != s || cell.Tag != "c" {
						continue
					}
					coordinate := cell.SelectAttrValue("r", "")
					if coordinate == "" {
						return nil, fmt.Errorf("implicit cell coordinates unsupported")
					}
					if coordinate == arrayCell {
						kind = ""
						if f := child(cell, s, "f"); f != nil {
							kind = f.SelectAttrValue("t", "normal")
						}
					}
				}
			}
		}
		return []string{kind}, nil
	}
	// openpyxl uses worksheet relationship types, not just the drawing element.
	rows, err = relations(sheet)
	if err != nil {
		return nil, err
	}
	if commentCell != "" {
		var value []string
		for _, relation := range rows {
			if relation.kind != r+"/comments" {
				continue
			}
			comments, e := xml(relation.target)
			if e != nil {
				return nil, e
			}
			if comments.NamespaceURI() != s || comments.Tag != "comments" {
				return nil, fmt.Errorf("unsupported comments root")
			}
			if list := child(comments, s, "commentList"); list != nil {
				for _, comment := range list.ChildElements() {
					if comment.NamespaceURI() != s || comment.Tag != "comment" || comment.SelectAttrValue("ref", "") != commentCell {
						continue
					}
					var result strings.Builder
					if text := child(comment, s, "text"); text != nil {
						// openpyxl Text.content concatenates the plain text then all rich runs.
						if plain := child(text, s, "t"); plain != nil {
							result.WriteString(plain.Text())
						}
						for _, run := range text.ChildElements() {
							if run.NamespaceURI() == s && run.Tag == "r" {
								if text := child(run, s, "t"); text != nil {
									result.WriteString(text.Text())
								}
							}
						}
					}
					value = []string{result.String()}
				}
			}
		}
		return value, nil
	}
	if tables {
		var names []string
		seen := map[string]bool{}
		parts := child(worksheet, s, "tableParts")
		if parts == nil {
			return names, nil
		}
		for _, item := range parts.ChildElements() {
			if item.NamespaceURI() != s || item.Tag != "tablePart" {
				continue
			}
			target, e := resolve(rows, attr(item, r, "id"), r+"/table")
			if e != nil {
				return nil, e
			}
			table, e := xml(target)
			if e != nil {
				return nil, e
			}
			if table.NamespaceURI() != s || table.Tag != "table" {
				return nil, fmt.Errorf("unsupported table root")
			}
			name := table.SelectAttrValue("name", table.SelectAttrValue("displayName", ""))
			if name == "" || seen[strings.ToLower(name)] {
				return nil, fmt.Errorf("missing or duplicate table name")
			}
			seen[strings.ToLower(name)] = true
			if tableRef == "" {
				names = append(names, name)
			} else if name == tableRef {
				names = append(names, table.SelectAttrValue("ref", ""))
			}
		}
		return names, nil
	}
	var charts []string
	for _, drawing := range rows {
		if drawing.kind != r+"/drawing" {
			continue
		}
		root, e := xml(drawing.target)
		if e != nil {
			return nil, e
		}
		if root.NamespaceURI() != d || root.Tag != "wsDr" {
			return nil, fmt.Errorf("unsupported drawing root")
		}
		deps, e := relations(drawing.target)
		if e != nil {
			return nil, e
		}
		for _, kind := range []string{"absoluteAnchor", "oneCellAnchor", "twoCellAnchor"} {
			for _, anchor := range root.ChildElements() {
				if anchor.NamespaceURI() != d || anchor.Tag != kind {
					continue
				}
				frame := child(anchor, d, "graphicFrame")
				ref := child(child(child(frame, a, "graphic"), a, "graphicData"), c, "chart")
				if ref == nil {
					continue
				}
				target, e := resolve(deps, attr(ref, r, "id"), r+"/chart")
				if e != nil {
					return nil, e
				}
				chart, e := xml(target)
				if e != nil {
					return nil, e
				}
				if chart.NamespaceURI() != c || chart.Tag != "chartSpace" {
					return nil, fmt.Errorf("unsupported chart root")
				}
				plot := child(child(chart, c, "chart"), c, "plotArea")
				if plot == nil {
					return nil, fmt.Errorf("missing plot area")
				}
				supported := false
				for _, p := range plot.ChildElements() {
					if p.NamespaceURI() == c && strings.Contains("|barChart|lineChart|pieChart|doughnutChart|scatterChart|areaChart|bubbleChart|radarChart|", "|"+p.Tag+"|") {
						supported = true
					}
				}
				if !supported {
					return nil, fmt.Errorf("unsupported chart type")
				}
				charts = append(charts, target)
			}
		}
	}
	return charts, nil
}
