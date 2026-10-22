package wordedit

import (
	"fmt"
	"github.com/beevik/etree"
	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"strconv"
)

func sectionEdit(d *Doc, s map[string]any) error {
	sections := office.WordSections(d.Body())
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
		// sectPr describes the section ending at that element, but its type
		// controls how that section starts. Find the section being split;
		// the requested template section need not be the paragraph's owner.
		var following *etree.Element
		for _, candidate := range sections {
			// Paragraph indices select direct body paragraphs. A later section
			// may end inside a content wrapper, so compare its body ancestor.
			block := candidate
			for block.Parent() != d.Body() {
				block = block.Parent()
			}
			if block.Index() > p.Index() {
				following = candidate
				break
			}
		}
		if following == nil {
			return fmt.Errorf("paragraph has no terminating section properties")
		}
		// Keep the original start of the preceding portion; only the new
		// following portion should use the requested break type.
		insertInPPr(pr, copy)
		if old := child(copy, "type"); old != nil {
			remove(old)
		}
		if start := child(following, "type"); start != nil {
			insertWordProperty(copy, start.Copy())
		}
		valProp(following, "type", kind)
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
	linkOperation := str(s, "operation") == "header-footer-link"
	var content []*etree.Element
	if linkOperation {
		linked, ok := s["linked"].(bool)
		if !ok {
			return fmt.Errorf("linked must be a boolean")
		}
		for _, key := range []string{"text", "rows", "widths_twips"} {
			if _, exists := s[key]; exists {
				return fmt.Errorf("header-footer-link does not accept %s; use header-footer to replace content", key)
			}
		}
		var refs []*etree.Element
		for _, ref := range sec.ChildElements() {
			if is(ref, kind+"Reference") && getW(ref, "type") == typ {
				refs = append(refs, ref)
			}
		}
		if linked {
			// Removing the reference restores inheritance. Keep the old part and
			// relationship: another section may still reference the same story.
			for _, ref := range refs {
				remove(ref)
			}
			if len(refs) > 0 {
				d.Touch(d.Main)
			}
			return nil
		}
		if len(refs) > 0 {
			return nil // Already unlinked; retain its current content.
		}
		// Match python-docx's is_linked_to_previous=false: create an empty
		// definition, rather than silently copying the inherited content.
		p := W("p")
		style := "Header"
		if kind == "footer" {
			style = "Footer"
		}
		valProp(property(p, "pPr"), "pStyle", style)
		content = []*etree.Element{p}
	} else {
		var err error
		content, err = headerFooterContent(s)
		if err != nil {
			return err
		}
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
	for _, el := range content {
		doc.Root().AddChild(el)
	}
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
	if !linkOperation && typ == "first" {
		valProp(sec, "titlePg", "1")
	}
	if !linkOperation && typ == "even" {
		settings, err := d.Settings()
		if err != nil {
			return err
		}
		valProp(settings, "evenAndOddHeaders", "1")
	}
	d.Touch(d.Main)
	return nil
}
