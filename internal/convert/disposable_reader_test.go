package convert

import (
	"archive/zip"
	"io"
	"path"
	"testing"

	"github.com/beevik/etree"
)

// Independent counterpart of Presentation(...).slides[0].shapes. Chart
// detection is GraphicFrame.graphicData.uri, not descendant chart discovery.
func referenceDisposableFirstSlideHasChart(t *testing.T, filename string) bool {
	t.Helper()
	z, err := zip.OpenReader(filename)
	if err != nil {
		t.Fatal(err)
	}
	defer z.Close()
	read := func(name string) *etree.Element {
		t.Helper()
		f, err := z.Open(name)
		if err != nil {
			t.Fatal(err)
		}
		b, err := io.ReadAll(f)
		f.Close()
		if err != nil {
			t.Fatal(err)
		}
		d := etree.NewDocument()
		if err := d.ReadFromBytes(b); err != nil {
			t.Fatal(err)
		}
		return d.Root()
	}
	const p = "http://schemas.openxmlformats.org/presentationml/2006/main"
	const a = "http://schemas.openxmlformats.org/drawingml/2006/main"
	const r = "http://schemas.openxmlformats.org/officeDocument/2006/relationships"
	child := func(e *etree.Element, ns, name string) *etree.Element {
		if e != nil {
			for _, c := range e.ChildElements() {
				if c.NamespaceURI() == ns && c.Tag == name {
					return c
				}
			}
		}
		return nil
	}
	resolve := func(owner, id string) string {
		rels := read(path.Join(path.Dir(owner), "_rels", path.Base(owner)+".rels"))
		for _, rel := range rels.ChildElements() {
			if rel.SelectAttrValue("Id", "") == id {
				if rel.SelectAttrValue("TargetMode", "") == "External" {
					t.Fatal("external slide relationship")
				}
				target := rel.SelectAttrValue("Target", "")
				if path.IsAbs(target) {
					return target[1:]
				}
				return path.Clean(path.Join(path.Dir(owner), target))
			}
		}
		t.Fatalf("missing relationship %s", id)
		return ""
	}
	owner := ""
	for _, rel := range read("_rels/.rels").ChildElements() {
		if rel.SelectAttrValue("Type", "") == r+"/officeDocument" {
			owner = path.Clean(rel.SelectAttrValue("Target", ""))
			if path.IsAbs(owner) {
				owner = owner[1:]
			}
		}
	}
	if owner == "" {
		t.Fatal("missing presentation relationship")
	}
	ids := child(read(owner), p, "sldIdLst")
	if ids == nil || len(ids.ChildElements()) == 0 {
		t.Fatal("no slides")
	}
	id := ""
	for _, attr := range ids.ChildElements()[0].Attr {
		if attr.Key == "id" && attr.NamespaceURI() == r {
			id = attr.Value
		}
	}
	slide := read(resolve(owner, id))
	shapes := child(child(slide, p, "cSld"), p, "spTree")
	if shapes == nil {
		t.Fatal("missing shape tree")
	}
	for _, shape := range shapes.ChildElements() {
		if shape.NamespaceURI() != p || shape.Tag != "graphicFrame" {
			continue
		}
		data := child(child(shape, a, "graphic"), a, "graphicData")
		if data != nil && data.SelectAttrValue("uri", "") == "http://schemas.openxmlformats.org/drawingml/2006/chart" {
			return true
		}
	}
	return false
}
