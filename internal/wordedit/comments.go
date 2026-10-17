package wordedit

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/beevik/etree"
	"github.com/openvibely/openvibely-paperworks/internal/office"
)

const relCommentsExt = "http://schemas.microsoft.com/office/2011/relationships/commentsExtended"

func (d *Doc) commentsPart(create bool) (string, *etree.Element, error) {
	part, err := d.Related(d.Main, "comments")
	if err != nil {
		return "", nil, err
	}
	if part == "" {
		if !create {
			return "", nil, nil
		}
		part = "word/comments.xml"
		if err := d.AddPart(part, ctBase+"comments+xml", "comments", newRoot("comments")); err != nil {
			return "", nil, err
		}
	}
	root, err := d.Root(part)
	return part, root, err
}

func addComment(d *Doc, text, author string) (int, error) {
	part, root, err := d.commentsPart(true)
	if err != nil {
		return 0, err
	}
	id := -1
	for _, c := range office.Children(root, nsW, "comment") {
		if n, err := strconv.Atoi(getW(c, "id")); err == nil && n > id {
			id = n
		}
	}
	id++
	c := W("comment")
	setW(c, "id", strconv.Itoa(id))
	setW(c, "author", author)
	// reference DOCX library Comments.add_comment defaults initials to an empty string.
	setW(c, "initials", "")
	setW(c, "date", d.timestamp())
	for i, line := range strings.Split(text, "\n") {
		p := W("p")
		st := W("pStyle")
		setW(st, "val", "CommentText")
		pPr(p).AddChild(st)
		if i == 0 {
			r := W("r")
			rp := W("rPr")
			rs := W("rStyle")
			setW(rs, "val", "CommentReference")
			rp.AddChild(rs)
			r.AddChild(rp)
			r.AddChild(W("annotationRef"))
			p.AddChild(r)
		}
		p.AddChild(runOf(line))
		c.AddChild(p)
	}
	root.AddChild(c)
	d.Touch(part)
	return id, nil
}

func comment(d *Doc, spec map[string]any) error {
	needle, contains := spec["contains"].(string)
	if !contains {
		needle = str(spec, "text")
	}
	if needle == "" {
		return errors.New("comment needs a nonempty match")
	}
	author := str(spec, "author")
	if author == "" {
		return errors.New("comment author is required")
	}
	fold := truthy(spec["ignore_case"])
	if fold {
		needle = strings.ToLower(needle)
	}
	var matches []*etree.Element
	for _, p := range all(d.Body(), "p") {
		var b strings.Builder
		office.Iter(p, func(e *etree.Element) bool {
			if is(e, "t") || is(e, "delText") {
				b.WriteString(e.Text())
			}
			return true
		})
		text := b.String()
		if fold {
			text = strings.ToLower(text)
		}
		if (contains && strings.Contains(text, needle)) || (!contains && text == needle) {
			matches = append(matches, p)
		}
	}
	if len(matches) == 0 || (!contains && len(matches) != 1) {
		return errors.New("comment needs one matching nonempty body paragraph")
	}
	if !truthy(spec["all_matches"]) {
		matches = matches[:1]
	}
	for _, p := range matches {
		id, err := addComment(d, str(spec, "comment"), author)
		if err != nil {
			return err
		}
		sid := strconv.Itoa(id)
		start, end, ref := W("commentRangeStart"), W("commentRangeEnd"), W("commentReference")
		for _, e := range []*etree.Element{start, end, ref} {
			setW(e, "id", sid)
		}
		pos := 0
		if child(p, "pPr") != nil {
			pos = 1
		}
		insertAt(p, pos, start)
		p.AddChild(end)
		r := W("r")
		r.AddChild(ref)
		p.AddChild(r)
	}
	d.Touch(d.Main)
	return nil
}

func commentEdit(d *Doc, spec map[string]any) error {
	part, root, err := d.commentsPart(false)
	if err != nil {
		return err
	}
	id, ok := intArg(spec["id"])
	if s, isStr := spec["id"].(string); isStr {
		id, err = strconv.Atoi(s)
		ok = err == nil
	}
	var c *etree.Element
	if root != nil && ok {
		for _, x := range office.Children(root, nsW, "comment") {
			if getW(x, "id") == strconv.Itoa(id) {
				c = x
			}
		}
	}
	if c == nil {
		return errors.New("unknown comment ID")
	}
	_, hasText := spec["text"]
	_, hasAppend := spec["append"]
	_, hasResolved := spec["resolved"]
	if !hasText && !hasAppend && !hasResolved {
		return errors.New("supply text, append or resolved")
	}
	paras := office.Children(c, nsW, "p")
	if len(paras) == 0 {
		return errors.New("comment has no paragraphs")
	}
	prevLast := paras[len(paras)-1]
	prevID := office.AttrV(prevLast, nsW14, "paraId")
	if hasText {
		first := paras[0]
		for _, r := range first.ChildElements() {
			if !is(r, "pPr") {
				first.RemoveChild(r)
			}
		}
		first.AddChild(runOf(str(spec, "text")))
		for _, p := range paras[1:] {
			c.RemoveChild(p)
		}
	}
	if hasAppend {
		for _, line := range strings.Split(str(spec, "append"), "\n") {
			p := newPara(line)
			if st := child(child(paras[0], "pPr"), "pStyle"); st != nil {
				pPr(p).AddChild(st.Copy())
			}
			c.AddChild(p)
		}
	}
	paras = office.Children(c, nsW, "p")
	last := paras[len(paras)-1]
	if prevID != "" && last != prevLast {
		removeNS(prevLast, nsW14, "paraId")
		last.CreateAttr("w14:paraId", prevID)
	}
	if hasResolved {
		resolved, ok := spec["resolved"].(bool)
		if !ok {
			return errors.New("resolved must be a boolean")
		}
		flag := map[bool]string{true: "1", false: "0"}[resolved]
		if _, has := office.Attr(c, nsW, "done"); has {
			setW(c, "done", flag)
		}
		ident := office.AttrV(last, nsW14, "paraId")
		if ident == "" {
			used := map[string]bool{}
			office.Iter(root, func(e *etree.Element) bool {
				used[office.AttrV(e, nsW14, "paraId")] = true
				return true
			})
			n := 1
			for used[fmt.Sprintf("%08X", n)] {
				n++
			}
			ident = fmt.Sprintf("%08X", n)
			last.CreateAttr("w14:paraId", ident)
		}
		extPart, err := d.Related(d.Main, "commentsExtended")
		if err != nil {
			return err
		}
		if extPart == "" {
			extPart = "word/commentsExtended.xml"
			x := etree.NewDocument()
			x.CreateProcInst("xml", `version="1.0" encoding="UTF-8" standalone="yes"`)
			r := x.CreateElement("w15:commentsEx")
			r.CreateAttr("xmlns:w15", nsW15)
			if err := d.AddPart(extPart, "application/vnd.ms-word.commentsExtended+xml", relCommentsExt, x); err != nil {
				return err
			}
		}
		ext, err := d.Root(extPart)
		if err != nil {
			return err
		}
		var entry *etree.Element
		for _, e := range ext.ChildElements() {
			if office.AttrV(e, nsW15, "paraId") == ident {
				entry = e
			}
		}
		if entry == nil {
			entry = ext.CreateElement("w15:commentEx")
			entry.CreateAttr("w15:paraId", ident)
		}
		removeNS(entry, nsW15, "done")
		entry.CreateAttr("w15:done", flag)
		d.Touch(extPart)
	}
	d.Touch(part)
	return nil
}

func removeNS(e *etree.Element, ns, key string) {
	for i, a := range e.Attr {
		if a.Key == key && a.NamespaceURI() == ns {
			e.Attr = append(e.Attr[:i], e.Attr[i+1:]...)
			return
		}
	}
}

func commentsRemove(d *Doc) error {
	for _, part := range d.WordParts() {
		root, _ := d.Root(part)
		changed := false
		for _, tag := range []string{"commentRangeStart", "commentRangeEnd", "commentReference"} {
			for _, e := range all(root, tag) {
				// Reference removes the marker itself, retaining its containing run.
				remove(e)
				changed = true
			}
		}
		if changed {
			d.Touch(part)
		}
	}
	for _, owner := range append([]string{""}, d.Pkg.SortedNames()...) {
		rels, err := d.Pkg.Rels(owner)
		if err != nil || len(rels) == 0 {
			continue
		}
		for _, r := range rels {
			last := r.Type[strings.LastIndex(r.Type, "/")+1:]
			if strings.HasPrefix(last, "comments") {
				if err := d.dropRel(owner, r.ID); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// dropRel removes a relationship and its target part when nothing else references it.
func (d *Doc) dropRel(owner, id string) error {
	name := relsName(owner)
	x, err := d.XML(name)
	if err != nil {
		return err
	}
	var target string
	for _, e := range x.Root().ChildElements() {
		if e.SelectAttrValue("Id", "") == id {
			if e.SelectAttrValue("TargetMode", "") != "External" {
				target = resolveTarget(owner, e.SelectAttrValue("Target", ""))
			}
			x.Root().RemoveChild(e)
		}
	}
	d.Touch(name)
	if target == "" || d.referenced(target) {
		return nil
	}
	d.Pkg.Remove(target)
	delete(d.docs, target)
	delete(d.dirty, target)
	if d.Pkg.Has(relsName(target)) {
		d.Pkg.Remove(relsName(target))
	}
	ct, err := d.XML("[Content_Types].xml")
	if err != nil {
		return err
	}
	for _, o := range ct.Root().SelectElements("Override") {
		if o.SelectAttrValue("PartName", "") == "/"+target {
			ct.Root().RemoveChild(o)
		}
	}
	d.Touch("[Content_Types].xml")
	return nil
}

func (d *Doc) referenced(target string) bool {
	for _, n := range d.Pkg.Names() {
		if !strings.HasSuffix(n, ".rels") {
			continue
		}
		x, err := d.XML(n)
		if err != nil {
			continue
		}
		owner := sourceOf(n)
		for _, e := range x.Root().ChildElements() {
			if e.SelectAttrValue("TargetMode", "") != "External" && resolveTarget(owner, e.SelectAttrValue("Target", "")) == target {
				return true
			}
		}
	}
	return false
}
