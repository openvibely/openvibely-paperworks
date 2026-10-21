package wordedit

import (
	"fmt"
	"github.com/beevik/etree"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"strconv"
)

func sectionEdit(d *Doc, s map[string]any) error {
	sections := all(d.Body(), "sectPr")
	i, ok := intArg(s["section"])
	if !ok || i < 0 || i >= len(sections) {
		return fmt.Errorf("section index out of range")
	}
	sec := sections[i]
	if str(s, "operation") == "section-insert" {
		p, err := selectParagraph(d, s["paragraph"])
		if err != nil {
			return err
		}
		pr := property(p, "pPr")
		if child(pr, "sectPr") != nil {
			return fmt.Errorf("paragraph already has a section break")
		}
		copy := sec.Copy()
		kind := str(s, "type")
		if kind == "" {
			kind = "nextPage"
		}
		switch kind {
		case "nextPage", "continuous", "evenPage", "oddPage":
		default:
			return fmt.Errorf("invalid section break type")
		}
		insertInPPr(pr, copy)
		valProp(copy, "type", kind)
		d.Touch(d.Main)
		return nil
	}
	kind := str(s, "kind")
	if kind != "header" && kind != "footer" {
		return fmt.Errorf("kind must be header or footer")
	}
	typ := str(s, "type")
	if typ == "" {
		typ = "default"
	}
	if typ != "default" && typ != "first" && typ != "even" {
		return fmt.Errorf("type must be default, first or even")
	}
	text, ok := s["text"].(string)
	if !ok {
		return fmt.Errorf("text required")
	}
	name := ""
	for n := 1; ; n++ {
		name = "word/" + kind + "Edit" + strconv.Itoa(n) + ".xml"
		if !d.Pkg.Has(name) {
			break
		}
	}
	tag := "hdr"
	if kind == "footer" {
		tag = "ftr"
	}
	doc := newRoot(tag)
	p := W("p")
	r := W("r")
	r.AddChild(WT("t", text))
	p.AddChild(r)
	doc.Root().AddChild(p)
	if err := d.Pkg.SetXML(name, doc); err != nil {
		return err
	}
	if err := d.Pkg.EnsureContentType(name, ctBase+kind+"+xml"); err != nil {
		return err
	}
	rid, err := d.Pkg.AddRel(d.Main, relBase+kind, name[len("word/"):], false)
	if err != nil {
		return err
	}
	// Give the selected section its own part instead of mutating a header/footer
	// shared by other sections.
	for _, ref := range sec.ChildElements() {
		if is(ref, kind+"Reference") && getW(ref, "type") == typ {
			remove(ref)
		}
	}
	ref := etree.NewElement("w:" + kind + "Reference")
	setW(ref, "type", typ)
	ref.CreateAttr("r:id", rid)
	ref.CreateAttr("xmlns:r", opc.NSOffR)
	insertWordProperty(sec, ref)
	if typ == "first" {
		valProp(sec, "titlePg", "1")
	}
	if typ == "even" {
		settings, err := d.Settings()
		if err != nil {
			return err
		}
		valProp(settings, "evenAndOddHeaders", "1")
	}
	d.Touch(d.Main)
	return nil
}
