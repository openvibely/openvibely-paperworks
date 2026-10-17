package testsupport

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/beevik/etree"
)

// WorksheetStateAndGlobalName reads the workbook declaration selected by
// openpyxl 3.1.5 _find_workbook_part and DefinedNameList.by_sheet. This bounded
// metadata reader does not validate every workbook descriptor or load worksheets.
func WorksheetStateAndGlobalName(data []byte, sheetName, definedName string) (string, bool, error) {
	z, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", false, err
	}
	files := map[string]*zip.File{}
	for _, f := range z.File {
		if files[f.Name] != nil {
			return "", false, fmt.Errorf("duplicate part %s", f.Name)
		}
		files[f.Name] = f
	}
	read := func(name string) (*etree.Element, error) {
		f := files[name]
		if f == nil {
			return nil, fmt.Errorf("missing part %s", name)
		}
		r, e := f.Open()
		if e != nil {
			return nil, e
		}
		defer r.Close()
		raw, e := io.ReadAll(r)
		if e != nil {
			return nil, e
		}
		d := etree.NewDocument()
		if e = d.ReadFromBytes(raw); e != nil {
			return nil, e
		}
		if d.Root() == nil {
			return nil, fmt.Errorf("empty XML %s", name)
		}
		return d.Root(), nil
	}
	ct, err := read("[Content_Types].xml")
	if err != nil {
		return "", false, err
	}
	const ctns = "http://schemas.openxmlformats.org/package/2006/content-types"
	types := []string{"application/vnd.ms-excel.template.macroEnabled.main+xml", "application/vnd.openxmlformats-officedocument.spreadsheetml.template.main+xml", "application/vnd.ms-excel.sheet.macroEnabled.main+xml", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"}
	main := ""
	for _, kind := range types {
		for _, e := range ct.ChildElements() {
			if e.NamespaceURI() == ctns && e.Tag == "Override" && e.SelectAttrValue("ContentType", "") == kind {
				main = strings.TrimPrefix(e.SelectAttrValue("PartName", ""), "/")
				break
			}
		}
		if main != "" {
			break
		}
	}
	if main == "" {
		for _, e := range ct.ChildElements() {
			if e.NamespaceURI() == ctns && e.Tag == "Default" {
				for _, kind := range types {
					if e.SelectAttrValue("ContentType", "") == kind {
						main = "xl/workbook.xml"
					}
				}
			}
		}
	}
	if main == "" {
		return "", false, fmt.Errorf("no workbook content type")
	}
	book, err := read(main)
	if err != nil {
		return "", false, err
	}
	const s = "http://schemas.openxmlformats.org/spreadsheetml/2006/main"
	if book.NamespaceURI() != s || book.Tag != "workbook" {
		return "", false, fmt.Errorf("unsupported workbook root")
	}
	state := ""
	found := false
	for _, container := range book.ChildElements() {
		if container.NamespaceURI() != s {
			continue
		}
		switch container.Tag {
		case "sheets":
			for _, e := range container.ChildElements() {
				if e.NamespaceURI() == s && e.Tag == "sheet" && e.SelectAttrValue("name", "") == sheetName && state == "" {
					state = e.SelectAttrValue("state", "visible")
					if state != "visible" && state != "hidden" && state != "veryHidden" {
						return "", false, fmt.Errorf("invalid sheet state %q", state)
					}
				}
			}
		case "definedNames":
			for _, e := range container.ChildElements() {
				if e.NamespaceURI() != s || e.Tag != "definedName" {
					continue
				}
				if scope := e.SelectAttr("localSheetId"); scope != nil {
					if _, err := strconv.Atoi(scope.Value); err != nil {
						return "", false, err
					}
					continue
				}
				name := e.SelectAttrValue("name", "")
				if name == "_xlnm.Print_Titles" || name == "_xlnm.Print_Area" || name == "_xlnm._FilterDatabase" {
					continue
				}
				if name == definedName {
					found = true
				}
			}
		}
	}
	if state == "" {
		return "", false, fmt.Errorf("missing worksheet %s", sheetName)
	}
	return state, found, nil
}
