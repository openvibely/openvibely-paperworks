package office

import (
	"archive/zip"
	"encoding/xml"
	"fmt"
	"io"
	"strconv"
	"testing"
)

// Independent cached-value acquisition for the unstyled single-sheet array
// fixtures. Keep numeric/null/text distinct, unlike GetCellValue's strings.
// Deliberately fail on shared strings, dates or styled cells: those require
// separate reader semantics, not a guessed conversion.
func referenceCachedScalar(t *testing.T, file, part, address string) any {
	t.Helper()
	z, err := zip.OpenReader(file)
	if err != nil {
		t.Fatal(err)
	}
	defer z.Close()
	var data []byte
	for _, f := range z.File {
		if f.Name == part {
			r, e := f.Open()
			if e != nil {
				t.Fatal(e)
			}
			data, e = io.ReadAll(r)
			r.Close()
			if e != nil {
				t.Fatal(e)
			}
		}
	}
	if data == nil {
		t.Fatal("missing worksheet", part)
	}
	value, err := referenceCachedScalarXML(data, address)
	if err != nil {
		t.Fatal(err)
	}
	return value
}
func referenceCachedScalarXML(data []byte, address string) (any, error) {
	var root struct {
		Data struct {
			Rows []struct {
				Cells []struct {
					Address string  `xml:"r,attr"`
					Kind    string  `xml:"t,attr"`
					Style   string  `xml:"s,attr"`
					Value   *string `xml:"http://schemas.openxmlformats.org/spreadsheetml/2006/main v"`
					Inline  struct {
						Text string `xml:"http://schemas.openxmlformats.org/spreadsheetml/2006/main t"`
					} `xml:"http://schemas.openxmlformats.org/spreadsheetml/2006/main is"`
				} `xml:"http://schemas.openxmlformats.org/spreadsheetml/2006/main c"`
			} `xml:"http://schemas.openxmlformats.org/spreadsheetml/2006/main row"`
		} `xml:"http://schemas.openxmlformats.org/spreadsheetml/2006/main sheetData"`
	}
	if err := xml.Unmarshal(data, &root); err != nil {
		return nil, err
	}
	for _, row := range root.Data.Rows {
		for _, c := range row.Cells {
			if c.Address != address {
				continue
			}
			if c.Style != "" && c.Style != "0" {
				return nil, fmt.Errorf("styled cell %s unsupported", address)
			}
			switch c.Kind {
			case "", "n":
				if c.Value == nil || *c.Value == "" {
					return nil, nil
				}
				v, e := strconv.ParseFloat(*c.Value, 64)
				return v, e
			case "inlineStr":
				return c.Inline.Text, nil
			case "str":
				if c.Value == nil || *c.Value == "" {
					return nil, nil
				}
				return *c.Value, nil
			default:
				return nil, fmt.Errorf("unsupported cached kind %q at %s", c.Kind, address)
			}
		}
	}
	return nil, nil
}
func TestReferenceCachedScalarTypes(t *testing.T) {
	for _, tc := range []struct {
		cell string
		want any
	}{
		{`<c r="B1"><v>1</v></c>`, float64(1)},
		{`<c r="B1" t="inlineStr"><is><t>1</t></is></c>`, "1"},
		{`<c r="B1"><v/></c>`, nil},
		{`<c r="B1" t="inlineStr"><is><t/></is></c>`, ""},
		{`<c r="B1" t="str"><v/></c>`, nil},
		{``, nil},
	} {
		raw := []byte(`<worksheet xmlns="` + NSS + `"><sheetData><row r="1">` + tc.cell + `</row></sheetData></worksheet>`)
		got, e := referenceCachedScalarXML(raw, "B1")
		if e != nil || got != tc.want {
			t.Fatalf("%s: %#v want %#v (%v)", tc.cell, got, tc.want, e)
		}
	}
}
