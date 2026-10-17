package wordedit

import (
	"errors"
	"strconv"
	"strings"

	"github.com/beevik/etree"
	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
)

func relsName(owner string) string {
	if owner == "" {
		return "_rels/.rels"
	}
	return opc.RelsName(owner)
}

func sourceOf(rels string) string { return opc.SourceOf(rels) }

func resolveTarget(owner, target string) string { return opc.Resolve(owner, target) }

func control(d *Doc, spec map[string]any) error {
	tag := str(spec, "tag")
	if tag == "" {
		return errors.New("content control tag is required")
	}
	parts := d.WordParts()
	roots := d.roots(parts)
	maxID := 0
	for _, root := range roots {
		for _, t := range all(root, "tag") {
			if getW(t, "val") == tag {
				return errors.New("content control tag already exists")
			}
		}
		for _, e := range all(root, "id") {
			if n, err := strconv.Atoi(getW(e, "val")); err == nil && n > maxID {
				maxID = n
			}
		}
	}
	nextID := maxID + 1
	newSdt := func() (*etree.Element, *etree.Element) {
		sdt, props, content := W("sdt"), W("sdtPr"), W("sdtContent")
		id := W("id")
		setW(id, "val", strconv.Itoa(nextID))
		nextID++
		t := W("tag")
		setW(t, "val", tag)
		props.AddChild(id)
		props.AddChild(t)
		props.AddChild(W("text"))
		sdt.AddChild(props)
		sdt.AddChild(content)
		return sdt, content
	}
	marker := str(spec, "marker")
	if marker == "" {
		p, err := selectParagraph(d, spec["paragraph"])
		if err != nil {
			return err
		}
		sdt, content := newSdt()
		addPrev(p, sdt)
		remove(p)
		content.AddChild(p)
		d.Touch(d.Main)
		return nil
	}
	text, hasText := spec["text"].(string)
	_, err := replaceMarker(d, marker, nil, func(runs []*etree.Element) []*etree.Element {
		sdt, content := newSdt()
		if hasText {
			setRunText(runs[0], text)
			runs = runs[:1]
		}
		for _, r := range runs {
			content.AddChild(r)
		}
		return []*etree.Element{sdt}
	})
	return err
}

func controlFill(d *Doc, spec map[string]any) error {
	tag := str(spec, "tag")
	text, ok := spec["text"].(string)
	if !ok {
		return errors.New("control-fill text must be a string")
	}
	type hit struct {
		part string
		sdt  *etree.Element
	}
	var matches []hit
	for _, part := range d.WordParts() {
		root, _ := d.Root(part)
		for _, sdt := range all(root, "sdt") {
			if getW(child(child(sdt, "sdtPr"), "tag"), "val") == tag && child(child(sdt, "sdtPr"), "tag") != nil {
				matches = append(matches, hit{part, sdt})
			}
		}
	}
	if len(matches) == 0 {
		return errors.New("content control tag was not found")
	}
	for _, m := range matches {
		props, content := child(m.sdt, "sdtPr"), child(m.sdt, "sdtContent")
		if child(props, "text") == nil || child(props, "dataBinding") != nil || child(props, "lock") != nil {
			return errors.New("only unbound, unlocked plain-text controls can be filled")
		}
		if content == nil {
			return errors.New("control has no content container")
		}
		kids := content.ChildElements()
		inline := is(m.sdt.Parent(), "p")
		if len(kids) > 0 {
			inline = true
			for _, k := range kids {
				if !is(k, "r") {
					inline = false
				}
			}
		}
		if !inline && (len(kids) > 1 || (len(kids) == 1 && !is(kids[0], "p"))) {
			return errors.New("expected a plain inline or single paragraph control")
		}
		holder := content
		if !inline {
			if len(kids) == 0 {
				content.AddChild(W("p"))
			}
			holder = content.ChildElements()[0]
			for _, k := range holder.ChildElements() {
				if !is(k, "pPr") && !is(k, "r") {
					return errors.New("control contains unsupported structures")
				}
			}
		}
		runs := office.Children(holder, nsW, "r")
		for _, r := range runs {
			for _, c := range r.ChildElements() {
				switch {
				case is(c, "rPr"), is(c, "t"), is(c, "tab"), is(c, "br"), is(c, "cr"):
				default:
					return errors.New("control contains non-text runs")
				}
			}
		}
		if len(runs) > 0 {
			setRunText(runs[0], text)
			for _, r := range runs[1:] {
				holder.RemoveChild(r)
			}
		} else {
			holder.AddChild(runOf(text))
		}
	}
	for _, m := range matches {
		d.Touch(m.part)
	}
	return nil
}

func protection(d *Doc, spec map[string]any) error {
	mode := str(spec, "mode")
	switch mode {
	case "readOnly", "comments", "forms", "trackedChanges", "none":
	default:
		return errors.New("unsupported editing restriction")
	}
	s, err := d.Settings()
	if err != nil {
		return err
	}
	for _, e := range office.Children(s, nsW, "documentProtection") {
		if getW(e, "hash") != "" || getW(e, "hashValue") != "" {
			return errors.New("password-protected editing restrictions require the authorized editor")
		}
		s.RemoveChild(e)
	}
	if mode != "none" {
		e := W("documentProtection")
		setW(e, "edit", mode)
		setW(e, "enforcement", "1")
		// Match the original editor's settings.append: unrelated settings retain
		// their order and the newly added restriction is the last child.
		s.AddChild(e)
	}
	return nil
}

func note(d *Doc, spec map[string]any) error {
	kind := "footnote"
	if v, ok := spec["kind"].(string); ok {
		kind = v
	}
	if kind != "footnote" && kind != "endnote" {
		return errors.New("note kind must be footnote or endnote")
	}
	marker := str(spec, "marker")
	var para *etree.Element
	if marker == "" {
		var err error
		if para, err = selectParagraph(d, spec["paragraph"]); err != nil {
			return err
		}
	}
	part, err := d.Related(d.Main, kind+"s")
	if err != nil {
		return err
	}
	if part == "" {
		part = "word/" + kind + "s.xml"
		x := newRoot(kind + "s")
		for _, sep := range []struct {
			id   string
			kind string
		}{{"-1", "separator"}, {"0", "continuationSeparator"}} {
			n := W(kind)
			setW(n, "type", sep.kind)
			setW(n, "id", sep.id)
			p, r := W("p"), W("r")
			r.AddChild(W(sep.kind))
			p.AddChild(r)
			n.AddChild(p)
			x.Root().AddChild(n)
		}
		if err := d.AddPart(part, ctBase+kind+"s+xml", kind+"s", x); err != nil {
			return err
		}
	}
	root, err := d.Root(part)
	if err != nil {
		return err
	}
	maxID := 0
	for _, n := range root.ChildElements() {
		if v, err := strconv.Atoi(getW(n, "id")); err == nil && v > maxID {
			maxID = v
		}
	}
	id := strconv.Itoa(maxID + 1)
	n := W(kind)
	setW(n, "id", id)
	p, r := W("p"), W("r")
	r.AddChild(W(kind + "Ref"))
	p.AddChild(r)
	p.AddChild(runOf(" " + str(spec, "text")))
	n.AddChild(p)
	root.AddChild(n)
	d.Touch(part)
	ref := W(kind + "Reference")
	setW(ref, "id", id)
	run := W("r")
	run.AddChild(ref)
	if para != nil {
		para.AddChild(run)
		d.Touch(d.Main)
		return nil
	}
	_, err = replaceMarker(d, marker, []*etree.Element{run}, nil)
	return err
}

func privacy(d *Doc) error {
	if err := setCore(d, map[string]string{"creator": "", "lastModifiedBy": "", "description": "", "keywords": "", "identifier": "", "created": "2000-01-01T00:00:00Z", "modified": "2000-01-01T00:00:00Z"}); err != nil {
		return err
	}
	for _, part := range d.WordParts() {
		root, _ := d.Root(part)
		changed := false
		var list []*etree.Element
		office.Iter(root, func(e *etree.Element) bool { list = append(list, e); return true })
		for _, e := range list {
			if is(e, "rsids") {
				remove(e)
				changed = true
				continue
			}
			kept := e.Attr[:0]
			for _, a := range e.Attr {
				if a.NamespaceURI() == nsW && (strings.HasPrefix(a.Key, "rsid") || a.Key == "author" || a.Key == "date" || a.Key == "initials") {
					changed = true
					continue
				}
				kept = append(kept, a)
			}
			e.Attr = kept
		}
		if changed {
			d.Touch(part)
		}
	}
	for _, owner := range append([]string{""}, d.Pkg.SortedNames()...) {
		rels, err := d.Pkg.Rels(owner)
		if err != nil {
			continue
		}
		for _, r := range rels {
			if strings.HasSuffix(r.Type, "/custom-properties") {
				if err := d.dropRel(owner, r.ID); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
