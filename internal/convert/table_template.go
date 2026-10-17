package convert

import (
	"strconv"
	"strings"
	"unicode"

	"github.com/beevik/etree"
	"github.com/openvibely/openvibely-paperworks/internal/docxtemplate"
	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
)

// Table exchange and DOCX authoring share the pinned blank template.
// Each call reads an independent package; no mutation can alter the asset.

func exchangeTablePackage(rows, alignments [][]string, width int) (*opc.Package, error) {
	pkg, err := docxtemplate.Open()
	if err != nil {
		return nil, err
	}
	doc, err := pkg.XML("word/document.xml")
	if err != nil {
		return nil, err
	}
	body := office.Child(doc.Root(), office.NSW, "body")
	table := etree.NewElement("w:tbl")
	body.InsertChildAt(0, table)
	pr := table.CreateElement("w:tblPr")
	pr.CreateElement("w:tblStyle").CreateAttr("w:val", "TableGrid")
	w := pr.CreateElement("w:tblW")
	w.CreateAttr("w:type", "auto")
	w.CreateAttr("w:w", "0")
	pr.CreateElement("w:tblLayout").CreateAttr("w:type", "fixed")
	look := pr.CreateElement("w:tblLook")
	for _, a := range [][2]string{{"firstColumn", "1"}, {"firstRow", "1"}, {"lastColumn", "0"}, {"lastRow", "0"}, {"noHBand", "0"}, {"noVBand", "1"}, {"val", "04A0"}} {
		look.CreateAttr("w:"+a[0], a[1])
	}
	grid := table.CreateElement("w:tblGrid")
	for range rows[0] {
		grid.CreateElement("w:gridCol").CreateAttr("w:w", strconv.Itoa(width))
	}
	for i, row := range rows {
		tr := table.CreateElement("w:tr")
		if i == 0 {
			tr.CreateElement("w:trPr").CreateElement("w:tblHeader")
		}
		for j, text := range row {
			tc := tr.CreateElement("w:tc")
			cw := tc.CreateElement("w:tcPr").CreateElement("w:tcW")
			cw.CreateAttr("w:type", "dxa")
			cw.CreateAttr("w:w", strconv.Itoa(width))
			p := tc.CreateElement("w:p")
			p.CreateElement("w:pPr").CreateElement("w:jc").CreateAttr("w:val", alignments[i][j])
			run := p.CreateElement("w:r")
			if i == 0 {
				run.CreateElement("w:rPr").CreateElement("w:b")
			}
			exchangeCellRunText(run, text)
		}
	}
	// lxml leaves quotes literal in text while escaping them in attributes.
	doc.WriteSettings.CanonicalText = true
	doc.WriteSettings.CanonicalAttrVal = true
	if err := pkg.SetXML("word/document.xml", doc); err != nil {
		return nil, err
	}
	return pkg, nil
}

// reference DOCX library's run-text appender emits a tab/break for each control character
// and marks each individual text chunk whose ends have whitespace.
func exchangeCellRunText(run *etree.Element, text string) {
	var chunk strings.Builder
	flush := func() {
		if chunk.Len() == 0 {
			return
		}
		value := chunk.String()
		t := run.CreateElement("w:t")
		if strings.TrimFunc(value, unicode.IsSpace) != value {
			t.CreateAttr("xml:space", "preserve")
		}
		t.SetText(value)
		chunk.Reset()
	}
	for _, c := range text {
		switch c {
		case '\t':
			flush()
			run.CreateElement("w:tab")
		case '\r', '\n':
			flush()
			run.CreateElement("w:br")
		default:
			chunk.WriteRune(c)
		}
	}
	flush()
}
