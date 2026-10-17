package convert

import (
	"bytes"
	"github.com/beevik/etree"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"github.com/openvibely/openvibely-paperworks/internal/testsupport"
	"reflect"
	"strings"
	"testing"
)

// Original openpyxl packages, deduplicated by uncompressed member SHA. The
// original method fixtures include serialization/order details used by cache
// preservation checks; they must not be regenerated with Excelize defaults.
func referenceRuntimeOriginalWorkbookFixture(t *testing.T, archive, name string) string {
	t.Helper()
	return referenceRuntimeOriginalPackageFixture(t, archive, name, ".xlsx")
}

// Reconstruct the original ordered ZIP members without regenerating OOXML defaults.
func referenceRuntimeOriginalPackageFixture(t *testing.T, archive, name, extension string) string {
	t.Helper()
	return testsupport.OriginalPackageFixture(t, archive, name, extension)
}

// Original feature fixture uses an absolute table target; a filename scan or
// Excelize's relative-target shortcut would give a false negative here.
func TestReferenceRuntimeWorksheetTableRelationships(t *testing.T) {
	src := referenceRuntimeOriginalWorkbookFixture(t, "runtime-feature-array-fixtures.zip", "feature-preservation")
	for _, tc := range []struct {
		name string
		want []string
	}{
		{"absolute", []string{"InputsTable"}}, {"relative", []string{"InputsTable"}},
		{"orphan", nil}, {"unused-relationship", []string{"InputsTable"}},
		{"name-fallback", []string{"InputsTable"}}, {"name-not-display", []string{"InternalName"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, e := opc.Open(src)
			if e != nil {
				t.Fatal(e)
			}
			switch tc.name {
			case "relative":
				d, e := p.XML("xl/worksheets/_rels/sheet1.xml.rels")
				if e != nil {
					t.Fatal(e)
				}
				for _, r := range d.Root().ChildElements() {
					if strings.HasSuffix(r.SelectAttrValue("Type", ""), "/table") {
						r.CreateAttr("Target", "../tables/table1.xml")
					}
				}
				if e = p.SetXML("xl/worksheets/_rels/sheet1.xml.rels", d); e != nil {
					t.Fatal(e)
				}
			case "orphan":
				d, e := p.XML("xl/worksheets/sheet1.xml")
				if e != nil {
					t.Fatal(e)
				}
				d.Root().RemoveChild(d.Root().SelectElement("tableParts"))
				if e = p.SetXML("xl/worksheets/sheet1.xml", d); e != nil {
					t.Fatal(e)
				}
			case "unused-relationship":
				p.Set("xl/tables/unused.xml", p.Get("xl/tables/table1.xml"))
				if _, e = p.AddRel("xl/worksheets/sheet1.xml", opc.NSOffR+"/table", "../tables/unused.xml", false); e != nil {
					t.Fatal(e)
				}
			case "name-fallback", "name-not-display":
				d, e := p.XML("xl/tables/table1.xml")
				if e != nil {
					t.Fatal(e)
				}
				if tc.name == "name-fallback" {
					d.Root().RemoveAttr("name")
				} else {
					d.Root().CreateAttr("name", "InternalName")
				}
				if e = p.SetXML("xl/tables/table1.xml", d); e != nil {
					t.Fatal(e)
				}
			}
			data, e := p.Bytes()
			if e != nil {
				t.Fatal(e)
			}
			got, e := testsupport.NamedWorksheetTables(data, "Inputs")
			if e != nil || !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %v, %v; want %v", got, e, tc.want)
			}
		})
	}
}

func TestReferenceRuntimeArrayAndCommentAcquisition(t *testing.T) {
	for _, name := range []string{"original", "relocated", "scalar"} {
		t.Run("array-"+name, func(t *testing.T) {
			src := referenceRuntimeOriginalWorkbookFixture(t, "runtime-feature-array-fixtures.zip", "array")
			p, e := opc.Open(src)
			if e != nil {
				t.Fatal(e)
			}
			if name == "relocated" {
				p.Set("xl/worksheets/moved.xml", p.Get("xl/worksheets/sheet1.xml"))
				p.Remove("xl/worksheets/sheet1.xml")
				p.Set("xl/_rels/workbook.xml.rels", bytes.ReplaceAll(p.Get("xl/_rels/workbook.xml.rels"), []byte("sheet1.xml"), []byte("moved.xml")))
			}
			if name == "scalar" {
				p.Set("xl/worksheets/sheet1.xml", bytes.ReplaceAll(p.Get("xl/worksheets/sheet1.xml"), []byte(`t="array"`), []byte(`t="normal"`)))
			}
			raw, e := p.Bytes()
			if e != nil {
				t.Fatal(e)
			}
			got, e := testsupport.ActiveWorksheetArrayFormula(raw, "B1")
			if e != nil {
				t.Fatal(e)
			}
			if got != (name != "scalar") {
				t.Fatalf("array=%v", got)
			}
		})
	}
	for _, rich := range []bool{false, true} {
		t.Run(map[bool]string{false: "plain-comment", true: "rich-comment"}[rich], func(t *testing.T) {
			src := referenceRuntimeOriginalWorkbookFixture(t, "runtime-feature-array-fixtures.zip", "feature-preservation")
			p, e := opc.Open(src)
			if e != nil {
				t.Fatal(e)
			}
			if rich {
				changed := false
				for _, part := range p.Names() {
					if strings.Contains(part, "comments") && strings.HasSuffix(part, ".xml") {
						raw := p.Get(part)
						old := []byte(`<t>Preserve this source</t>`)
						if bytes.Contains(raw, old) {
							p.Set(part, bytes.ReplaceAll(raw, old, []byte(`<r><t xml:space="preserve">Preserve </t></r><r><t>this source</t></r>`)))
							changed = true
						}
					}
				}
				if !changed {
					t.Fatal("fixture comment was not changed")
				}
			}
			raw, e := p.Bytes()
			if e != nil {
				t.Fatal(e)
			}
			got, ok, e := testsupport.NamedWorksheetCommentText(raw, "Inputs", "B2")
			if e != nil {
				t.Fatal(e)
			}
			if !ok || got != "Preserve this source" {
				t.Fatalf("comment=%q present=%v", got, ok)
			}
		})
	}
}

func TestReferenceRuntimeWorkbookMetadata(t *testing.T) {
	src := referenceRuntimeOriginalWorkbookFixture(t, "runtime-feature-array-fixtures.zip", "feature-preservation")
	for _, tc := range []struct {
		name, state string
		global      bool
	}{
		{"base", "hidden", true}, {"very-hidden", "veryHidden", true}, {"visible", "visible", true}, {"default-state", "visible", true},
		{"local-name", "hidden", false}, {"missing-name", "hidden", false}, {"global-and-local", "hidden", true},
		{"relocated", "hidden", true}, {"relocated-stale-root-rel", "hidden", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, err := opc.Open(src)
			if err != nil {
				t.Fatal(err)
			}
			book, err := p.XML("xl/workbook.xml")
			if err != nil {
				t.Fatal(err)
			}
			sheets := book.Root().FindElement("sheets")
			var raw *etree.Element
			for _, e := range sheets.ChildElements() {
				if e.SelectAttrValue("name", "") == "Raw" {
					raw = e
				}
			}
			if raw == nil {
				t.Fatal("Raw missing")
			}
			switch tc.name {
			case "very-hidden", "visible":
				raw.CreateAttr("state", tc.state)
			case "default-state":
				raw.RemoveAttr("state")
			case "local-name":
				book.Root().FindElement("definedNames/definedName").CreateAttr("localSheetId", "0")
			case "missing-name":
				names := book.Root().FindElement("definedNames")
				names.RemoveChild(names.ChildElements()[0])
			case "global-and-local":
				names := book.Root().FindElement("definedNames")
				local := names.ChildElements()[0].Copy()
				local.CreateAttr("localSheetId", "0")
				names.AddChild(local)
			}
			if err := p.SetXML("xl/workbook.xml", book); err != nil {
				t.Fatal(err)
			}
			if strings.HasPrefix(tc.name, "relocated") {
				p.Set("xl/renamed.xml", p.Get("xl/workbook.xml"))
				p.Remove("xl/workbook.xml")
				p.Set("xl/_rels/renamed.xml.rels", p.Get("xl/_rels/workbook.xml.rels"))
				p.Remove("xl/_rels/workbook.xml.rels")
				ct, err := p.XML("[Content_Types].xml")
				if err != nil {
					t.Fatal(err)
				}
				for _, e := range ct.Root().ChildElements() {
					if e.SelectAttrValue("PartName", "") == "/xl/workbook.xml" {
						e.CreateAttr("PartName", "/xl/renamed.xml")
					}
				}
				if err := p.SetXML("[Content_Types].xml", ct); err != nil {
					t.Fatal(err)
				}
				if tc.name == "relocated" {
					rels, err := p.XML("_rels/.rels")
					if err != nil {
						t.Fatal(err)
					}
					for _, e := range rels.Root().ChildElements() {
						if strings.HasSuffix(e.SelectAttrValue("Type", ""), "/officeDocument") {
							e.CreateAttr("Target", "xl/renamed.xml")
						}
					}
					if err := p.SetXML("_rels/.rels", rels); err != nil {
						t.Fatal(err)
					}
				}
			}
			data, err := p.Bytes()
			if err != nil {
				t.Fatal(err)
			}
			state, global, err := testsupport.WorksheetStateAndGlobalName(data, "Raw", "Rate")
			if err != nil || state != tc.state || global != tc.global {
				t.Fatalf("%s %v %v; want %s %v", state, global, err, tc.state, tc.global)
			}
		})
	}
}
