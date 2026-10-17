package office

import (
	"errors"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"

	"github.com/openvibely/openvibely-paperworks/internal/opc"
)

var a1Range = regexp.MustCompile(`^\$?([A-Za-z]{1,3})\$?([1-9][0-9]*)(?::\$?([A-Za-z]{1,3})\$?([1-9][0-9]*))?$`)

// ColumnIndex converts "A".."XFD" to 1-based column numbers.
func ColumnIndex(s string) int {
	n := 0
	for _, r := range strings.ToUpper(s) {
		n = n*26 + int(r-'A'+1)
	}
	return n
}

func ColumnName(n int) string {
	s := ""
	for n > 0 {
		n--
		s = string(rune('A'+n%26)) + s
		n /= 26
	}
	return s
}

// ParseRange returns left, top, right, bottom (1-based, inclusive).
func ParseRange(r string) (int, int, int, int, error) {
	return parseRange(r, true)
}

func parseRange(r string, normalize bool) (int, int, int, int, error) {
	m := a1Range.FindStringSubmatch(r)
	if m == nil {
		return 0, 0, 0, 0, errors.New("preview requires a single finite A1 cell range")
	}
	l, err := strconv.Atoi(m[2])
	if err != nil {
		return 0, 0, 0, 0, errors.New("cell row exceeds supported integer range")
	}
	left, top := ColumnIndex(m[1]), l
	right, bottom := left, top
	if m[3] != "" {
		right = ColumnIndex(m[3])
		bottom, err = strconv.Atoi(m[4])
		if err != nil {
			return 0, 0, 0, 0, errors.New("cell row exceeds supported integer range")
		}
	}
	if left > right || top > bottom {
		if !normalize {
			return 0, 0, 0, 0, errors.New("preview range endpoints must be in top-left to bottom-right order")
		}
		left, right = min(left, right), max(left, right)
		top, bottom = min(top, bottom), max(top, bottom)
	}
	return left, top, right, bottom, nil
}

func QuoteSheet(name string) string {
	if regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.]*$`).MatchString(name) && !a1Range.MatchString(name) {
		return name
	}
	return "'" + strings.ReplaceAll(name, "'", "''") + "'"
}

// WorkbookPreviewCopy prepares a disposable workbook copy whose printable
// scope is the selected sheet (and optionally an A1 range sized onto custom
// paper). Cells, formulas and drawings are untouched.
func WorkbookPreviewCopy(src, dst, sheet, cellRange string) error {
	if cellRange != "" && sheet == "" {
		return errors.New("spreadsheet preview selection requires XLSX and --sheet")
	}
	p, err := opc.Open(src)
	if err != nil {
		return err
	}
	book, err := p.XML("xl/workbook.xml")
	if err != nil {
		return err
	}
	root := book.Root()
	sheetsEl := Child(root, NSS, "sheets")
	sheets := Children(sheetsEl, NSS, "sheet")
	index := -1
	if sheet != "" {
		for i, s := range sheets {
			if s.SelectAttrValue("name", "") == sheet {
				index = i
			}
		}
		if index < 0 {
			return errors.New("unknown preview worksheet: " + sheet)
		}
		for i, s := range sheets {
			if i == index {
				s.CreateAttr("state", "visible")
			} else {
				s.CreateAttr("state", "hidden")
			}
		}
		for _, v := range All(Child(root, NSS, "bookViews"), NSS, "workbookView") {
			v.CreateAttr("activeTab", strconv.Itoa(index))
		}
	}
	names := Child(root, NSS, "definedNames")
	if names != nil {
		for _, n := range Children(names, NSS, "definedName") {
			if n.SelectAttrValue("name", "") == "_xlnm.Print_Area" {
				names.RemoveChild(n)
			}
		}
	}
	rels, err := p.Rels("xl/workbook.xml")
	if err != nil {
		return err
	}
	target := map[string]string{}
	for _, r := range rels {
		target[r.ID] = opc.Resolve("xl/workbook.xml", r.Target)
	}
	if cellRange == "" {
		for _, s := range sheets {
			part := target[AttrV(s, NSR, "id")]
			doc, err := p.XML(part)
			if err != nil || !Is(doc.Root(), NSS, "worksheet") {
				continue
			}
			ws := doc.Root()
			view := Child(Child(ws, NSS, "sheetViews"), NSS, "sheetView")
			opts := EnsureChild(ws, NSS, "printOptions", WorksheetOrder)
			opts.CreateAttr("headings", boolAttr(view == nil || Truthy(view.SelectAttrValue("showRowColHeaders", ""), true)))
			opts.CreateAttr("gridLines", boolAttr(view == nil || Truthy(view.SelectAttrValue("showGridLines", ""), true)))
			if err := p.SetXML(part, doc); err != nil {
				return err
			}
		}
		if err := p.SetXML("xl/workbook.xml", book); err != nil {
			return err
		}
		return p.SaveNew(dst)
	}
	left, top, right, bottom, err := parseRange(cellRange, false)
	if err != nil {
		return err
	}
	if right > 16384 || bottom > 1048576 || (right-left+1)*(bottom-top+1) > 10000 {
		return errors.New("preview range must contain at most 10000 cells within worksheet bounds")
	}
	if names == nil {
		names = NewChild(root, NSS, "definedNames", After(WorkbookOrder, "definedNames"))
	}
	for _, n := range Children(names, NSS, "definedName") {
		nm := n.SelectAttrValue("name", "")
		if (nm == "_xlnm.Print_Area" || nm == "_xlnm.Print_Titles") && n.SelectAttrValue("localSheetId", "") == strconv.Itoa(index) {
			names.RemoveChild(n)
		}
	}
	dn := NewChild(names, NSS, "definedName", nil)
	dn.CreateAttr("name", "_xlnm.Print_Area")
	dn.CreateAttr("localSheetId", strconv.Itoa(index))
	dn.SetText(fmt.Sprintf("%s!$%s$%d:$%s$%d", QuoteSheet(sheet), ColumnName(left), top, ColumnName(right), bottom))
	part := target[AttrV(sheets[index], NSR, "id")]
	doc, err := p.XML(part)
	if err != nil {
		return err
	}
	ws := doc.Root()
	defW, defH := 8.43, 15.0
	if f := Child(ws, NSS, "sheetFormatPr"); f != nil {
		if v, err := strconv.ParseFloat(f.SelectAttrValue("defaultColWidth", ""), 64); err == nil {
			defW = v
		}
		if v, err := strconv.ParseFloat(f.SelectAttrValue("defaultRowHeight", ""), 64); err == nil {
			defH = v
		}
	}
	cols := All(Child(ws, NSS, "cols"), NSS, "col")
	rows := map[int]map[string]string{}
	for _, r := range All(Child(ws, NSS, "sheetData"), NSS, "row") {
		n, _ := strconv.Atoi(r.SelectAttrValue("r", ""))
		rows[n] = attrMap(r)
	}
	width := 0.0
	for c := left; c <= right; c++ {
		units := defW
		hidden := false
		for _, d := range cols {
			lo, _ := strconv.Atoi(d.SelectAttrValue("min", "0"))
			hi, _ := strconv.Atoi(d.SelectAttrValue("max", "0"))
			if lo <= c && c <= hi {
				hidden = Truthy(d.SelectAttrValue("hidden", ""), false)
				if v, err := strconv.ParseFloat(d.SelectAttrValue("width", ""), 64); err == nil {
					units = v
				}
				break
			}
		}
		if !hidden {
			width += math.Floor((256*units + math.Floor(128.0/7)) / 256 * 7)
		}
	}
	height := 0.0
	for r := top; r <= bottom; r++ {
		a := rows[r]
		if a != nil && Truthy(a["hidden"], false) {
			continue
		}
		h := defH
		if a != nil {
			if v, err := strconv.ParseFloat(a["ht"], 64); err == nil {
				h = v
			}
		}
		height += h * 96 / 72
	}
	if !(width > 0 && width <= 20000) || !(height > 0 && height <= 20000) {
		return errors.New("preview dimensions must be between 1 and 20000 pixels")
	}
	view := Child(Child(ws, NSS, "sheetViews"), NSS, "sheetView")
	headings := view == nil || Truthy(view.SelectAttrValue("showRowColHeaders", ""), true)
	if headings {
		width += 40
		height += 22
	}
	for _, tag := range []string{"headerFooter", "rowBreaks", "colBreaks", "printOptions", "pageMargins", "pageSetup"} {
		RemoveAll(ws, NSS, tag)
	}
	po := NewChild(ws, NSS, "printOptions", After(WorksheetOrder, "printOptions"))
	po.CreateAttr("headings", boolAttr(headings))
	po.CreateAttr("gridLines", boolAttr(view == nil || Truthy(view.SelectAttrValue("showGridLines", ""), true)))
	pm := NewChild(ws, NSS, "pageMargins", After(WorksheetOrder, "pageMargins"))
	for _, k := range []string{"left", "right", "top", "bottom", "header", "footer"} {
		pm.CreateAttr(k, "0")
	}
	ps := NewChild(ws, NSS, "pageSetup", After(WorksheetOrder, "pageSetup"))
	ps.CreateAttr("paperSize", "0")
	ps.CreateAttr("paperWidth", fmt.Sprintf("%.6fmm", (width+2)*25.4/96))
	ps.CreateAttr("paperHeight", fmt.Sprintf("%.6fmm", (height+2)*25.4/96))
	ps.CreateAttr("fitToWidth", "1")
	ps.CreateAttr("fitToHeight", "1")
	ps.CreateAttr("orientation", "portrait")
	pr := Child(ws, NSS, "sheetPr")
	if pr == nil {
		pr = NewChild(ws, NSS, "sheetPr", WorksheetOrder[1:])
	}
	setup := Child(pr, NSS, "pageSetUpPr")
	if setup == nil {
		setup = NewChild(pr, NSS, "pageSetUpPr", nil)
	}
	setup.CreateAttr("fitToPage", "1")
	if err := p.SetXML(part, doc); err != nil {
		return err
	}
	if err := p.SetXML("xl/workbook.xml", book); err != nil {
		return err
	}
	return p.SaveNew(dst)
}

func boolAttr(b bool) string {
	if b {
		return "1"
	}
	return "0"
}

// VisibleSheetIndexes lists the zero-based indexes of visible sheets.
func VisibleSheetIndexes(path string) (int, []int, error) {
	p, err := opc.Open(path)
	if err != nil {
		return 0, nil, err
	}
	book, err := p.XML("xl/workbook.xml")
	if err != nil {
		return 0, nil, err
	}
	sheets := All(Child(book.Root(), NSS, "sheets"), NSS, "sheet")
	var vis []int
	for i, s := range sheets {
		if st := s.SelectAttrValue("state", "visible"); st == "visible" {
			vis = append(vis, i)
		}
	}
	return len(sheets), vis, nil
}
