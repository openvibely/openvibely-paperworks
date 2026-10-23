package office

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/beevik/etree"

	"github.com/openvibely/openvibely-paperworks/internal/opc"
)

// SharedStrings returns the plain text of every shared string item.
func SharedStrings(p *opc.Package) ([]string, error) {
	if !p.Has("xl/sharedStrings.xml") {
		return nil, nil
	}
	doc, err := p.XML("xl/sharedStrings.xml")
	if err != nil {
		return nil, err
	}
	var out []string
	for _, si := range Children(doc.Root(), NSS, "si") {
		out = append(out, spreadsheetText(si))
	}
	return out, nil
}

// SplitCell splits "AB12" into its column index and row.
func SplitCell(ref string) (int, int, error) {
	i := 0
	ref = strings.ReplaceAll(ref, "$", "")
	for i < len(ref) && (ref[i] >= 'A' && ref[i] <= 'Z' || ref[i] >= 'a' && ref[i] <= 'z') {
		i++
	}
	if i == 0 || i > 3 || i == len(ref) {
		return 0, 0, fmt.Errorf("invalid cell reference %q", ref)
	}
	row, err := strconv.Atoi(ref[i:])
	if err != nil || row < 1 {
		return 0, 0, fmt.Errorf("invalid cell reference %q", ref)
	}
	return ColumnIndex(ref[:i]), row, nil
}

// CellValue is the saved result of one worksheet cell.
type CellValue struct {
	Kind    string // n, str, b, e, or "" when empty
	Text    string
	Formula bool
}

// Number parses a numeric cell value.
func (v CellValue) Number() (float64, bool) {
	if v.Kind != "n" && v.Kind != "b" {
		return 0, false
	}
	f, err := strconv.ParseFloat(v.Text, 64)
	return f, err == nil
}

// Sheet indexes one worksheet's cells by reference.
type Sheet struct {
	Name  string
	Part  string
	Doc   *etree.Document
	Cells map[string]*etree.Element
}

// Workbook provides read access to sheet cells by name.
type Workbook struct {
	Pkg      *opc.Package
	Order    []string
	Sheets   map[string]*Sheet
	Shared   []string
	Date1904 bool
}

func OpenWorkbook(p *opc.Package) (*Workbook, error) {
	order, _, byName, err := WorkbookSheets(p)
	if err != nil {
		return nil, err
	}
	shared, err := SharedStrings(p)
	if err != nil {
		return nil, err
	}
	wb := &Workbook{Pkg: p, Order: order, Sheets: map[string]*Sheet{}, Shared: shared}
	if doc, err := p.XML("xl/workbook.xml"); err == nil {
		if pr := Child(doc.Root(), NSS, "workbookPr"); pr != nil {
			wb.Date1904 = Truthy(pr.SelectAttrValue("date1904", ""), false)
		}
	}
	for _, name := range order {
		part := byName[name]
		if part == "" || !p.Has(part) {
			continue
		}
		doc, err := p.XML(part)
		if err != nil {
			return nil, err
		}
		s := &Sheet{Name: name, Part: part, Doc: doc, Cells: map[string]*etree.Element{}}
		for _, c := range All(doc.Root(), NSS, "c") {
			s.Cells[strings.ToUpper(c.SelectAttrValue("r", ""))] = c
		}
		wb.Sheets[name] = s
	}
	return wb, nil
}

// Value reads the saved value of sheet!ref.
func (wb *Workbook) Value(sheet, ref string) (CellValue, error) {
	s := wb.Sheets[sheet]
	if s == nil {
		return CellValue{}, fmt.Errorf("unknown worksheet %q", sheet)
	}
	return wb.CellValueOf(s.Cells[strings.ToUpper(ref)])
}

func (wb *Workbook) CellValueOf(c *etree.Element) (CellValue, error) {
	if c == nil {
		return CellValue{}, nil
	}
	out := CellValue{Formula: Child(c, NSS, "f") != nil}
	kind := c.SelectAttrValue("t", "n")
	v := Child(c, NSS, "v")
	// The reference reader normalizes absent/empty <v> to None before type
	// conversion. A formula with no saved result is not an empty string or zero.
	// Inline strings have their own <is> representation and must remain distinct.
	if kind != "inlineStr" && (v == nil || v.Text() == "") {
		return out, nil
	}
	switch kind {
	case "inlineStr":
		if is := Child(c, NSS, "is"); is != nil {
			out.Kind, out.Text = "str", spreadsheetText(is)
		}
		return out, nil
	case "s":
		if v == nil {
			return out, nil
		}
		i, err := strconv.Atoi(strings.TrimSpace(v.Text()))
		if err != nil || i < 0 || i >= len(wb.Shared) {
			return out, fmt.Errorf("invalid shared string index in %s", c.SelectAttrValue("r", ""))
		}
		out.Kind, out.Text = "str", wb.Shared[i]
		return out, nil
	}
	if v == nil {
		return out, nil
	}
	out.Kind, out.Text = kind, v.Text()
	if kind == "d" {
		out.Kind = "str"
	}
	return out, nil
}
