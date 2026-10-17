package office

import (
	"archive/zip"
	"fmt"
	"math"
	"math/big"
	"path"
	"strings"
	"testing"

	"github.com/beevik/etree"
)

// Independent saved-package acquisition. reference presentation library starts with the package
// officeDocument relationship, then follows ordered sldId references. It does
// not discover slides by filenames or by production PresentationSlides.
func referencePackageXML(t *testing.T, z *zip.ReadCloser, name string) *etree.Element {
	t.Helper()
	d := etree.NewDocument()
	if err := referencePreviewReadXML(d, referencePreviewZIPRead(t, z, name)); err != nil {
		t.Fatal(name, err)
	}
	if d.Root() == nil {
		t.Fatal("empty XML", name)
	}
	return d.Root()
}
func referencePackageTarget(t *testing.T, z *zip.ReadCloser, owner, id, kind string) string {
	t.Helper()
	relfile := "_rels/.rels"
	if owner != "" {
		relfile = path.Join(path.Dir(owner), "_rels", path.Base(owner)+".rels")
	}
	rels := referencePackageXML(t, z, relfile)
	const ns = "http://schemas.openxmlformats.org/package/2006/relationships"
	matches := map[string]*etree.Element{}
	for _, r := range rels.ChildElements() {
		if r.NamespaceURI() == ns && r.Tag == "Relationship" {
			matches[r.SelectAttrValue("Id", "")] = r
		}
	}
	var found *etree.Element
	if id != "" {
		found = matches[id]
	} else {
		for _, r := range matches {
			if r.SelectAttrValue("Type", "") == kind {
				if found != nil {
					t.Fatal("multiple officeDocument relationships")
				}
				found = r
			}
		}
	}
	if found == nil || found.SelectAttrValue("Type", "") != kind {
		t.Fatal("missing or wrong relationship", owner, id, kind)
	}
	if found.SelectAttrValue("TargetMode", "") == "External" {
		t.Fatal("external package relationship", owner, id)
	}
	target := found.SelectAttrValue("Target", "")
	if target == "" {
		t.Fatal("empty relationship target", owner, id)
	}
	// PackURI.from_rel_ref uses POSIX join/abspath, not URI unquoting.
	if strings.HasPrefix(target, "/") {
		return strings.TrimPrefix(path.Clean(target), "/")
	}
	return strings.TrimPrefix(path.Clean("/"+path.Dir(owner)+"/"+target), "/")
}
func referencePresentationRoots(t *testing.T, filename string) []*etree.Element {
	t.Helper()
	z := referencePreviewZIP(t, filename)
	main := referencePackageTarget(t, z, "", "", NSR+"/officeDocument")
	ct := referencePackageXML(t, z, "[Content_Types].xml")
	const ctns = "http://schemas.openxmlformats.org/package/2006/content-types"
	mime := ""
	for _, e := range ct.ChildElements() {
		if e.NamespaceURI() == ctns && e.Tag == "Default" && e.SelectAttrValue("Extension", "") == strings.TrimPrefix(path.Ext(main), ".") {
			mime = e.SelectAttrValue("ContentType", "")
		}
	}
	for _, e := range ct.ChildElements() {
		if e.NamespaceURI() == ctns && e.Tag == "Override" && e.SelectAttrValue("PartName", "") == "/"+main {
			mime = e.SelectAttrValue("ContentType", "")
		}
	}
	if mime != "application/vnd.openxmlformats-officedocument.presentationml.presentation.main+xml" {
		t.Fatal("unsupported presentation content type", mime)
	}
	root := referencePackageXML(t, z, main)
	if root.NamespaceURI() != NSP || root.Tag != "presentation" {
		t.Fatal("not a presentation", main)
	}
	var slides []*etree.Element
	for _, id := range Children(Child(root, NSP, "sldIdLst"), NSP, "sldId") {
		part := referencePackageTarget(t, z, main, AttrV(id, NSR, "id"), NSR+"/slide")
		slide := referencePackageXML(t, z, part)
		if slide.NamespaceURI() != NSP || slide.Tag != "sld" {
			t.Fatal("not a slide", part)
		}
		slides = append(slides, slide)
	}
	return slides
}

// Openpyxl Bool treats the strings false/f/0 as false and otherwise applies
// Reference truthiness. Missing attributes use the field's declared default.
func referenceWorksheetBool(e *etree.Element, name string, defaultValue bool) bool {
	if e == nil {
		return defaultValue
	}
	a := e.SelectAttr(name)
	if a == nil {
		return defaultValue
	}
	return a.Value != "" && a.Value != "false" && a.Value != "f" && a.Value != "0"
}
func referenceWorksheetParts(t *testing.T, filename string) map[string]string {
	t.Helper()
	z := referencePreviewZIP(t, filename)
	root := referencePackageXML(t, z, "xl/workbook.xml")
	result := map[string]string{}
	for _, s := range Children(Child(root, NSS, "sheets"), NSS, "sheet") {
		result[s.SelectAttrValue("name", "")] = referencePackageTarget(t, z, "xl/workbook.xml", AttrV(s, NSR, "id"), NSR+"/worksheet")
	}
	return result
}
func TestReferencePackageReaderRelocatedSlideOrder(t *testing.T) {
	const ns = "http://schemas.openxmlformats.org/package/2006/relationships"
	file := referenceCheckPackageNamed(t, "relocated.pptx", []referenceCheckPart{
		{"[Content_Types].xml", `<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.slide+xml"/><Override PartName="/deck/main.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.presentation.main+xml"/></Types>`},
		{"_rels/.rels", `<Relationships xmlns="` + ns + `"><Relationship Id="office" Type="` + NSR + `/officeDocument" Target="deck/main.xml"/></Relationships>`},
		{"deck/main.xml", `<p:presentation xmlns:p="` + NSP + `" xmlns:r="` + NSR + `"><p:sldIdLst><p:sldId id="256" r:id="two"/><p:sldId id="257" r:id="one"/></p:sldIdLst></p:presentation>`},
		{"deck/_rels/main.xml.rels", `<Relationships xmlns="` + ns + `"><Relationship Id="one" Type="` + NSR + `/slide" Target="../pages/one.xml"/><Relationship Id="two" Type="` + NSR + `/slide" Target="/pages/two.xml"/></Relationships>`},
		{"pages/one.xml", `<p:sld xmlns:p="` + NSP + `" show="1"/>`}, {"pages/two.xml", `<p:sld xmlns:p="` + NSP + `" show="0"/>`},
	})
	roots := referencePresentationRoots(t, file)
	if len(roots) != 2 || roots[0].SelectAttrValue("show", "") != "0" || roots[1].SelectAttrValue("show", "") != "1" {
		t.Fatal("wrong slide order", roots)
	}
	for _, v := range []string{"1", "true", "0", "false", "f", "", "False"} {
		e := etree.NewElement("printOptions")
		e.CreateAttr("headings", v)
		want := v == "1" || v == "true" || v == "False"
		if referenceWorksheetBool(e, "headings", false) != want {
			t.Fatal(fmt.Sprintf("boolean %q", v))
		}
	}
}

func referencePreviewAlmostFivePlaces(t *testing.T, first, second float64) {
	t.Helper()
	if first == second {
		return
	}
	exact := new(big.Rat).SetFloat64(math.Abs(first - second))
	if exact == nil || exact.Cmp(big.NewRat(1, 200000)) > 0 {
		t.Fatalf("%v != %v within 5 decimal places", first, second)
	}
}

// Follow the same first-slide/notes relationship chain as reference presentation library, then
// read the BODY placeholder rather than searching arbitrary notes-part bytes.
func referenceFirstSlideNotesText(t *testing.T, filename string) string {
	t.Helper()
	z := referencePreviewZIP(t, filename)
	main := referencePackageTarget(t, z, "", "", NSR+"/officeDocument")
	root := referencePackageXML(t, z, main)
	ids := Children(Child(root, NSP, "sldIdLst"), NSP, "sldId")
	if len(ids) == 0 {
		t.Fatal("presentation has no first slide")
	}
	slide := referencePackageTarget(t, z, main, AttrV(ids[0], NSR, "id"), NSR+"/slide")
	part := referencePackageTarget(t, z, slide, "", NSR+"/notesSlide")
	notes := referencePackageXML(t, z, part)
	tree := Child(Child(notes, NSP, "cSld"), NSP, "spTree")
	labels := referenceHistogramShapeTexts(t, notes)
	for i, shape := range Children(tree, NSP, "sp") {
		ph := Child(Child(Child(shape, NSP, "nvSpPr"), NSP, "nvPr"), NSP, "ph")
		if ph != nil && ph.SelectAttrValue("type", "obj") == "body" {
			return labels[i]
		}
	}
	t.Fatal("notes slide has no body placeholder")
	return ""
}
