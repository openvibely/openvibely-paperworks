package office

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/beevik/etree"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"github.com/xuri/excelize/v2"
)

// These inputs reproduce DocumentGapChecks' preview cases without keeping generated workbooks.
func referencePreviewBook(t *testing.T, names []string, bookExtra string, sheets []string) string {
	t.Helper()
	f := excelize.NewFile()
	defer f.Close()
	if err := f.SetSheetName("Sheet1", names[0]); err != nil {
		t.Fatal(err)
	}
	for _, n := range names[1:] {
		if _, err := f.NewSheet(n); err != nil {
			t.Fatal(err)
		}
	}
	b, err := f.WriteToBuffer()
	if err != nil {
		t.Fatal(err)
	}
	p, err := opc.Read(b.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	book, err := p.XML("xl/workbook.xml")
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"definedNames"} {
		RemoveAll(book.Root(), NSS, n)
	}
	if bookExtra != "" {
		d := etree.NewDocument()
		if err := d.ReadFromString(`<definedNames xmlns="` + NSS + `">` + bookExtra + `</definedNames>`); err != nil {
			t.Fatal(err)
		}
		book.Root().AddChild(d.Root().Copy())
	}
	if err := p.SetXML("xl/workbook.xml", book); err != nil {
		t.Fatal(err)
	}
	for i, s := range sheets {
		p.Set(fmt.Sprintf("xl/worksheets/sheet%d.xml", i+1), []byte(`<worksheet xmlns="`+NSS+`">`+s+`</worksheet>`))
	}
	source := filepath.Join(t.TempDir(), "source.xlsx")
	if err := p.SaveNew(source); err != nil {
		t.Fatal(err)
	}
	return source
}
func previewXML(t *testing.T, p *opc.Package, part string) *etree.Element {
	t.Helper()
	d := etree.NewDocument()
	if e := referencePreviewReadXML(d, p.Get(part)); e != nil {
		t.Fatal(e)
	}
	return d.Root()
}
func previewSerialize(t *testing.T, e *etree.Element) string {
	t.Helper()
	if e == nil {
		t.Fatal("missing XML element")
	}
	raw, err := referencePreviewSerializeElement(e)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}
func previewUnchanged(t *testing.T, source string, before []byte) {
	t.Helper()
	after, e := os.ReadFile(source)
	if e != nil || !bytes.Equal(before, after) {
		t.Fatal("source changed", e)
	}
}
func previewRead(t *testing.T, source string) []byte {
	t.Helper()
	b, e := os.ReadFile(source)
	if e != nil {
		t.Fatal(e)
	}
	return b
}
func previewOpen(t *testing.T, source string) *opc.Package {
	t.Helper()
	p, e := opc.Open(source)
	if e != nil {
		t.Fatal(e)
	}
	return p
}
func previewCell(t *testing.T, f *excelize.File, sheet, cell, want string) {
	t.Helper()
	got, e := f.GetCellValue(sheet, cell)
	if e != nil || got != want {
		t.Errorf("%s!%s = %q, %v; want %q", sheet, cell, got, e, want)
	}
}

func TestReferenceWorkbookPreviewRangeDependencies(t *testing.T) {
	source := referenceOriginalWorkbookFixture(t, "worksheet-preview-fixtures.zip", "range")
	before := previewRead(t, source)
	out := filepath.Join(t.TempDir(), "preview.xlsx")
	if e := WorkbookPreviewCopy(source, out, "Sales '26", "B2:C3"); e != nil {
		t.Fatal(e)
	}
	previewUnchanged(t, source, before)
	f, e := excelize.OpenFile(out)
	if e != nil {
		t.Fatal(e)
	}
	defer f.Close()
	if got := referenceCachedScalar(t, out, "xl/worksheets/sheet1.xml", "A1"); got != float64(7) {
		t.Fatalf("Inputs!A1 = %#v, want numeric 7", got)
	}
	previewCell(t, f, "Sales '26", "Z99", "Outside preview")
	formula, e := f.GetCellFormula("Sales '26", "C2")
	if e != nil || formula != "Inputs!A1*2" {
		t.Error(formula, e)
	}
	if state := referenceWorksheetState(t, out, "Inputs"); state != "hidden" {
		t.Error("dependency state", state)
	}
	if f.GetSheetName(f.GetActiveSheetIndex()) != "Sales '26" {
		t.Error("wrong active sheet")
	}
	a, b := previewOpen(t, source), previewOpen(t, out)
	if !reflect.DeepEqual(a.Names(), b.Names()) {
		t.Fatal("package members changed")
	}
	for _, n := range a.Names() {
		if n != "xl/workbook.xml" && n != "xl/worksheets/sheet2.xml" && !bytes.Equal(a.Get(n), b.Get(n)) {
			t.Error("changed part", n)
		}
	}
	book := previewXML(t, b, "xl/workbook.xml")
	area := false
	for _, n := range All(book, NSS, "definedName") {
		switch n.SelectAttrValue("name", "") {
		case "_xlnm.Print_Area":
			area = strings.Contains(n.Text(), "$B$2:$C$3")
		case "_xlnm.Print_Titles":
			t.Error("print titles retained")
		}
	}
	if !area {
		t.Error("missing selected print area")
	}
	ws := previewXML(t, b, "xl/worksheets/sheet2.xml")
	old := previewXML(t, a, "xl/worksheets/sheet2.xml")
	for _, n := range []string{"sheetData", "mergeCells", "cols"} {
		if previewSerialize(t, Child(ws, NSS, n)) != previewSerialize(t, Child(old, NSS, n)) {
			t.Error("changed subtree", n)
		}
	}
	if !referenceOddHeaderCenterIsNone(t, ws) {
		t.Error("odd header center is not None")
	}
	for _, c := range []struct {
		element    *etree.Element
		attr, want string
	}{{Child(ws, NSS, "pageMargins"), "left", "0"}, {Child(Child(ws, NSS, "sheetPr"), NSS, "pageSetUpPr"), "fitToPage", "1"}, {Child(ws, NSS, "printOptions"), "headings", "1"}, {Child(ws, NSS, "printOptions"), "gridLines", "0"}} {
		if c.element == nil || c.element.SelectAttrValue(c.attr, "") != c.want {
			t.Errorf("%s want %s", c.attr, c.want)
		}
	}
}

func TestReferenceWholeSheetIgnoresPrintArea(t *testing.T) {
	source := referenceOriginalWorkbookFixture(t, "worksheet-preview-fixtures.zip", "whole")
	before := previewRead(t, source)
	for _, selected := range []string{"Report", ""} {
		t.Run("sheet="+selected, func(t *testing.T) {
			out := filepath.Join(t.TempDir(), "preview.xlsx")
			if e := WorkbookPreviewCopy(source, out, selected, ""); e != nil {
				t.Fatal(e)
			}
			f, e := excelize.OpenFile(out)
			if e != nil {
				t.Fatal(e)
			}
			defer f.Close()
			previewCell(t, f, "Report", "A20", "Outside print area")
			previewCell(t, f, "Other", "C30", "Other content")
			for _, v := range []struct {
				name string
				want string
			}{{"Other", map[bool]string{true: "visible", false: "hidden"}[selected == ""]}, {"Hidden", "hidden"}} {
				if got := referenceWorksheetState(t, out, v.name); got != v.want {
					t.Error(v, got)
				}
			}
			a, b := previewOpen(t, source), previewOpen(t, out)
			for _, n := range All(previewXML(t, b, "xl/workbook.xml"), NSS, "definedName") {
				if n.SelectAttrValue("name", "") == "_xlnm.Print_Area" {
					t.Error("print area retained", n.Text())
				}
			}
			for _, n := range a.Names() {
				if strings.HasPrefix(n, "xl/worksheets/sheet") && strings.HasSuffix(n, ".xml") {
					old, ws := previewXML(t, a, n), previewXML(t, b, n)
					opts := Child(ws, NSS, "printOptions")
					if opts == nil {
						t.Fatal("missing print options")
					}
					for _, attr := range []string{"headings", "gridLines"} {
						if opts.SelectAttrValue(attr, "") != "1" {
							t.Error(n, attr)
						}
					}
					ws.RemoveChild(opts)
					if previewSerialize(t, old) != previewSerialize(t, ws) {
						t.Error("changed worksheet", n)
					}
				} else if n != "xl/workbook.xml" && !bytes.Equal(a.Get(n), b.Get(n)) {
					t.Error("changed part", n)
				}
			}
			previewUnchanged(t, source, before)
		})
	}
}

func TestReferenceWholeSheetUsesViewSettings(t *testing.T) {
	source := referenceOriginalWorkbookFixture(t, "worksheet-preview-fixtures.zip", "views")
	before := previewRead(t, source)
	out := filepath.Join(t.TempDir(), "preview.xlsx")
	if err := WorkbookPreviewCopy(source, out, "", ""); err != nil {
		t.Fatal(err)
	}
	a, b := previewOpen(t, source), previewOpen(t, out)
	// Reference compares all four XML roots before independently opening the workbook.
	for i := 1; i <= 4; i++ {
		part := fmt.Sprintf("xl/worksheets/sheet%d.xml", i)
		old, ws := previewXML(t, a, part), previewXML(t, b, part)
		oldOptions, newOptions := Child(old, NSS, "printOptions"), Child(ws, NSS, "printOptions")
		if oldOptions == nil || newOptions == nil {
			t.Fatal("missing print options", part)
		}
		old.RemoveChild(oldOptions)
		ws.RemoveChild(newOptions)
		if previewSerialize(t, old) != previewSerialize(t, ws) {
			t.Fatal("changed worksheet", part)
		}
	}
	f, err := excelize.OpenFile(out)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	// Reference iterates the reopened result, not the expected input-name list.
	// Resolve saved worksheet relationships, then decode openpyxl booleans.
	parts := referenceWorksheetParts(t, out)
	for _, name := range f.GetSheetList() {
		part, ok := parts[name]
		if !ok {
			t.Fatal("missing worksheet relationship", name)
		}
		ws := previewXML(t, b, part)
		view := Child(Child(ws, NSS, "sheetViews"), NSS, "sheetView")
		opts := Child(ws, NSS, "printOptions")
		if opts == nil || view == nil {
			t.Fatal("missing options or view")
		}
		// Original order: headings, grid lines, then preserved cell value.
		for _, c := range []struct{ attr, setting string }{{"headings", "showRowColHeaders"}, {"gridLines", "showGridLines"}} {
			if referenceWorksheetBool(opts, c.attr, false) != referenceWorksheetBool(view, c.setting, true) {
				t.Fatal(c.attr, "does not follow view")
			}
		}
		value, err := f.GetCellValue(name, "B2")
		if err != nil || value != "Preserved" {
			t.Fatal(name, value, err)
		}
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	previewUnchanged(t, source, before)
}
