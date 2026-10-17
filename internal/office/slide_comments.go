package office

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/beevik/etree"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
)

const nsModern = "http://schemas.microsoft.com/office/powerpoint/2018/8/main"
const nsPC = "http://schemas.microsoft.com/office/powerpoint/2013/main/command"
const nsOAC = "http://schemas.microsoft.com/office/drawing/2013/main/command"
const nsReaction = "http://schemas.microsoft.com/office/powerpoint/2022/03/main"
const relModern = "http://schemas.microsoft.com/office/2018/10/relationships/"

type SlideCommentSpec struct {
	Action     string  `json:"action"`
	Slide      int     `json:"slide"`
	Author     string  `json:"author"`
	AuthorID   *string `json:"author_id"`
	Initials   string  `json:"initials"`
	Text       string  `json:"text"`
	ThreadID   string  `json:"thread_id"`
	ReplyID    string  `json:"reply_id"`
	Reaction   string  `json:"reaction"`
	ShapeID    any     `json:"shape_id"`
	Match      *string `json:"text_match"`
	Occurrence int     `json:"occurrence"`
	Position   []int64 `json:"position_emu"`
}
type SlideComment struct {
	Slide     int               `json:"slide"`
	ID        string            `json:"id"`
	ParentID  *string           `json:"parent_id"`
	Author    string            `json:"author"`
	AuthorID  string            `json:"author_id"`
	Text      string            `json:"text"`
	Status    string            `json:"status"`
	Created   string            `json:"created"`
	Anchors   []string          `json:"anchor_xml"`
	Reactions []CommentReaction `json:"reactions"`
}
type CommentReaction struct {
	Type    string   `json:"type"`
	Authors []string `json:"authors"`
}
type commentEntry struct {
	slide       int
	owner, part string
	doc         *etree.Document
	root        *etree.Element
}

func nsElement(tag, ns string) *etree.Element {
	e := etree.NewElement(tag)
	e.CreateAttr("xmlns", ns)
	return e
}
func nsChild(parent *etree.Element, tag, ns string) *etree.Element {
	e := nsElement(tag, ns)
	parent.AddChild(e)
	return e
}
func modernPart(p *opc.Package, owner, kind string) (string, error) {
	rels, e := p.Rels(owner)
	if e != nil {
		return "", e
	}
	found := ""
	for _, r := range rels {
		if r.Type != relModern+kind {
			continue
		}
		if found != "" || r.TargetMode == "External" {
			return "", fmt.Errorf("ambiguous/external modern %s relationship", kind)
		}
		found = opc.Resolve(owner, r.Target)
		if !p.Has(found) {
			return "", fmt.Errorf("missing modern %s part", kind)
		}
	}
	return found, nil
}
func commentItems(root *etree.Element) []*etree.Element {
	return append([]*etree.Element{root}, Children(Child(root, nsModern, "replyLst"), nsModern, "reply")...)
}
func SlideComments(p *opc.Package, s SlideCommentSpec) ([]SlideComment, error) {
	action := s.Action
	if action == "" {
		action = "list"
	}
	switch action {
	case "list", "add", "reply", "resolve", "reopen", "react":
	default:
		return nil, fmt.Errorf("unknown presentation comment action")
	}
	ap, e := modernPart(p, "ppt/presentation.xml", "authors")
	if e != nil {
		return nil, e
	}
	hadAuthors := ap != ""
	var authors *etree.Document
	if hadAuthors {
		authors, e = p.XML(ap)
		if e != nil {
			return nil, e
		}
		if !Is(authors.Root(), nsModern, "authorLst") {
			return nil, fmt.Errorf("unsupported author list")
		}
	} else {
		ap = uniquePart(p, "ppt", "authors")
		authors = newRoot("authorLst", nsModern)
	}
	people := map[string]*etree.Element{}
	for _, a := range Children(authors.Root(), nsModern, "author") {
		id := a.SelectAttrValue("id", "")
		if id == "" || people[id] != nil {
			return nil, fmt.Errorf("invalid author IDs")
		}
		people[id] = a
	}
	person := func() (string, error) {
		if strings.TrimSpace(s.Author) == "" {
			return "", fmt.Errorf("a visible author is required")
		}
		if s.AuthorID != nil {
			a := people[*s.AuthorID]
			if a == nil || a.SelectAttrValue("name", "") != s.Author {
				return "", fmt.Errorf("author ID/name mismatch")
			}
			return *s.AuthorID, nil
		}
		id := ""
		for key, a := range people {
			if a.SelectAttrValue("name", "") == s.Author {
				if id != "" {
					return "", fmt.Errorf("ambiguous author; specify author_id")
				}
				id = key
			}
		}
		if id != "" {
			return id, nil
		}
		id = NewGUID()
		a := nsElement("author", nsModern)
		a.CreateAttr("id", id)
		a.CreateAttr("name", s.Author)
		a.CreateAttr("initials", s.Initials)
		insertBeforeExt(authors.Root(), a)
		people[id] = a
		if !hadAuthors {
			if _, e := p.AddRel("ppt/presentation.xml", relModern+"authors", strings.TrimPrefix(ap, "ppt/"), false); e != nil {
				return "", e
			}
			hadAuthors = true
		}
		if e := p.EnsureContentType(ap, "application/vnd.ms-powerpoint.authors+xml"); e != nil {
			return "", e
		}
		if e := p.SetXML(ap, authors); e != nil {
			return "", e
		}
		return id, nil
	}
	slides, e := PresentationSlides(p)
	if e != nil {
		return nil, e
	}
	var entries []commentEntry
	ids := map[string]bool{}
	for i, owner := range slides {
		part, e := modernPart(p, owner, "comments")
		if e != nil {
			return nil, e
		}
		if part == "" {
			continue
		}
		d, e := p.XML(part)
		if e != nil {
			return nil, e
		}
		if !Is(d.Root(), nsModern, "cmLst") {
			return nil, fmt.Errorf("unsupported comment format")
		}
		for _, cm := range Children(d.Root(), nsModern, "cm") {
			for _, v := range commentItems(cm) {
				id := v.SelectAttrValue("id", "")
				if id == "" || ids[id] || people[v.SelectAttrValue("authorId", "")] == nil {
					return nil, fmt.Errorf("invalid comment ID/author")
				}
				ids[id] = true
			}
			entries = append(entries, commentEntry{i + 1, owner, part, d, cm})
		}
	}
	if action == "list" {
		result := []SlideComment{}
		for _, entry := range entries {
			cm := entry.root
			for _, v := range commentItems(cm) {
				id := v.SelectAttrValue("id", "")
				aid := v.SelectAttrValue("authorId", "")
				item := SlideComment{Slide: entry.slide, ID: id, AuthorID: aid, Author: people[aid].SelectAttrValue("name", ""), Status: v.SelectAttrValue("status", "active"), Created: v.SelectAttrValue("created", ""), Anchors: []string{}, Reactions: []CommentReaction{}}
				if v != cm {
					pid := cm.SelectAttrValue("id", "")
					item.ParentID = &pid
				}
				var paras []string
				for _, par := range Children(Child(v, nsModern, "txBody"), NSA, "p") {
					paras = append(paras, commentParagraphText(par))
				}
				item.Text = strings.Join(paras, "\n")
				for _, a := range cm.ChildElements() {
					if strings.HasSuffix(a.Tag, "MkLst") {
						d := etree.NewDocument()
						d.SetRoot(commentAnchorCopy(a))
						text, _ := d.WriteToString()
						item.Anchors = append(item.Anchors, text)
					}
				}
				for _, rx := range All(v, nsReaction, "rxn") {
					r := CommentReaction{Type: rx.SelectAttrValue("type", ""), Authors: []string{}}
					for _, a := range rx.ChildElements() {
						r.Authors = append(r.Authors, a.SelectAttrValue("authorId", ""))
					}
					item.Reactions = append(item.Reactions, r)
				}
				result = append(result, item)
			}
		}
		return result, nil
	}
	now := func() string { return time.Now().UTC().Format("2006-01-02T15:04:05.000Z") }
	var entry commentEntry
	if action == "add" {
		if s.Slide < 1 || s.Slide > len(slides) {
			return nil, fmt.Errorf("select a valid one-based slide")
		}
		owner := slides[s.Slide-1]
		cp, e := modernPart(p, owner, "comments")
		if e != nil {
			return nil, e
		}
		var d *etree.Document
		if cp == "" {
			cp = uniquePart(p, "ppt/comments", "modernComment")
			d = newRoot("cmLst", nsModern)
			target, err := relPath(owner, cp)
			if err != nil {
				return nil, err
			}
			if _, e = p.AddRel(owner, relModern+"comments", target, false); e != nil {
				return nil, e
			}
			if e = p.EnsureContentType(cp, "application/vnd.ms-powerpoint.comments+xml"); e != nil {
				return nil, e
			}
		} else {
			d, e = p.XML(cp)
			if e != nil {
				return nil, e
			}
		}
		aid, e := person()
		if e != nil {
			return nil, e
		}
		cm := nsElement("cm", nsModern)
		cm.CreateAttr("id", NewGUID())
		cm.CreateAttr("authorId", aid)
		cm.CreateAttr("status", "active")
		cm.CreateAttr("created", now())
		insertBeforeExt(d.Root(), cm)
		sr, e := p.XML(owner)
		if e != nil {
			return nil, e
		}
		const p14 = "http://schemas.microsoft.com/office/powerpoint/2010/main"
		creation := All(sr.Root(), p14, "creationId")
		var cid string
		if len(creation) > 0 {
			cid = creation[0].SelectAttrValue("val", "")
		} else {
			cs := Child(sr.Root(), NSP, "cSld")
			if cs == nil {
				return nil, fmt.Errorf("missing slide content")
			}
			exts := Child(cs, NSP, "extLst")
			if exts == nil {
				exts = nsChild(cs, "extLst", NSP)
			}
			ext := nsChild(exts, "ext", NSP)
			ext.CreateAttr("uri", "{BB962C8B-B14F-4D97-AF65-F5344CB8AC3E}")
			var b [4]byte
			if _, e = rand.Read(b[:]); e != nil {
				return nil, e
			}
			cid = strconv.FormatUint(uint64(binary.BigEndian.Uint32(b[:])), 10)
			nsChild(ext, "creationId", p14).CreateAttr("val", cid)
		}
		if s.Match != nil && s.ShapeID == nil {
			return nil, fmt.Errorf("text anchor needs shape_id")
		}
		anchor := nsChild(cm, "sldMkLst", nsPC)
		if s.ShapeID != nil {
			anchor.Tag = "deMkLst"
			anchor.CreateAttr("xmlns", nsOAC)
			if s.Match != nil {
				anchor.Tag = "txMkLst"
			}
		}
		nsChild(anchor, "docMk", nsPC)
		pres, e := p.XML("ppt/presentation.xml")
		if e != nil {
			return nil, e
		}
		sids := Children(Child(pres.Root(), NSP, "sldIdLst"), NSP, "sldId")
		if len(sids) != len(slides) {
			return nil, fmt.Errorf("slide list mismatch")
		}
		sm := nsChild(anchor, "sldMk", nsPC)
		sm.CreateAttr("cId", cid)
		sm.CreateAttr("sldId", sids[s.Slide-1].SelectAttrValue("id", ""))
		if s.ShapeID != nil {
			shapeID := ""
			switch v := s.ShapeID.(type) {
			case string:
				shapeID = v
			case float64:
				shapeID = strconv.FormatFloat(v, 'f', -1, 64)
			case int:
				shapeID = strconv.Itoa(v)
			default:
				return nil, fmt.Errorf("invalid shape_id")
			}
			var props []*etree.Element
			for _, v := range All(sr.Root(), NSP, "cNvPr") {
				if v.SelectAttrValue("id", "") == shapeID {
					props = append(props, v)
				}
			}
			if len(props) != 1 {
				return nil, fmt.Errorf("unknown or ambiguous shape_id")
			}
			prop := props[0]
			shape := prop.Parent().Parent()
			monikers := map[string]string{"sp": "spMk", "grpSp": "grpSpMk", "graphicFrame": "graphicFrameMk", "pic": "picMk", "cxnSp": "cxnSpMk"}
			moniker := monikers[shape.Tag]
			if moniker == "" || shape.NamespaceURI() != NSP {
				return nil, fmt.Errorf("unsupported comment target")
			}
			const a16 = "http://schemas.microsoft.com/office/drawing/2014/main"
			creations := All(prop, a16, "creationId")
			scid := ""
			if len(creations) > 0 {
				scid = creations[0].SelectAttrValue("id", "")
			} else {
				exts := Child(prop, NSA, "extLst")
				if exts == nil {
					exts = nsChild(prop, "extLst", NSA)
				}
				ext := nsChild(exts, "ext", NSA)
				ext.CreateAttr("uri", "{FF2B5EF4-FFF2-40B4-BE49-F238E27FC236}")
				scid = NewGUID()
				nsChild(ext, "creationId", a16).CreateAttr("id", scid)
			}
			mk := nsChild(anchor, moniker, nsOAC)
			mk.CreateAttr("id", shapeID)
			mk.CreateAttr("creationId", scid)
			if s.Match != nil {
				if *s.Match == "" || s.Occurrence < 0 {
					return nil, fmt.Errorf("nonempty text_match and nonnegative occurrence required")
				}
				paras := Children(Child(shape, NSP, "txBody"), NSA, "p")
				if len(paras) == 0 {
					return nil, fmt.Errorf("comment target has no shape text")
				}
				var lines []string
				for _, par := range paras {
					var text strings.Builder
					Iter(par, func(v *etree.Element) bool {
						if Is(v, NSA, "t") {
							text.WriteString(v.Text())
						} else if Is(v, NSA, "br") {
							text.WriteByte('\n')
						}
						return true
					})
					lines = append(lines, text.String())
				}
				text := strings.Join(lines, "\n")
				start := -1
				offset := 0
				for i := 0; i <= s.Occurrence; i++ {
					idx := strings.Index(text[offset:], *s.Match)
					if idx < 0 {
						return nil, fmt.Errorf("text match occurrence not found")
					}
					start = offset + idx
					offset = start + len(*s.Match)
				}
				mk := nsChild(anchor, "txMk", nsOAC)
				mk.CreateAttr("cp", strconv.Itoa(len(utf16.Encode([]rune(text[:start])))))
				mk.CreateAttr("len", strconv.Itoa(len(utf16.Encode([]rune(*s.Match)))))
			}
		}
		pos := s.Position
		if pos == nil {
			pos = []int64{0, 0}
		}
		if len(pos) != 2 || pos[0] < 0 || pos[1] < 0 {
			return nil, fmt.Errorf("position_emu needs two nonnegative integers")
		}
		at := nsChild(cm, "pos", nsModern)
		at.CreateAttr("x", strconv.FormatInt(pos[0], 10))
		at.CreateAttr("y", strconv.FormatInt(pos[1], 10))
		if e = p.SetXML(owner, sr); e != nil {
			return nil, e
		}
		entry = commentEntry{s.Slide, owner, cp, d, cm}
	} else {
		found := false
		for _, v := range entries {
			if v.root.SelectAttrValue("id", "") == s.ThreadID {
				entry = v
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("unknown root thread_id")
		}
	}
	cm := entry.root
	switch action {
	case "add", "reply":
		if strings.TrimSpace(s.Text) == "" || utf8.RuneCountInString(s.Text) > 32767 {
			return nil, fmt.Errorf("comment needs 1..32767 characters")
		}
		target := cm
		if action == "reply" {
			if cm.SelectAttrValue("status", "") == "resolved" {
				return nil, fmt.Errorf("Reopen thread before replying")
			}
			aid, e := person()
			if e != nil {
				return nil, e
			}
			replies := Child(cm, nsModern, "replyLst")
			if replies == nil {
				replies = nsElement("replyLst", nsModern)
				InsertBefore(cm, replies, []string{"txBody", "extLst"})
			}
			target = nsChild(replies, "reply", nsModern)
			target.CreateAttr("id", NewGUID())
			target.CreateAttr("authorId", aid)
			target.CreateAttr("status", "active")
			target.CreateAttr("created", now())
		}
		body := nsChild(target, "txBody", nsModern)
		nsChild(body, "bodyPr", NSA)
		nsChild(body, "lstStyle", NSA)
		for _, line := range strings.Split(s.Text, "\n") {
			nsChild(nsChild(nsChild(body, "p", NSA), "r", NSA), "t", NSA).SetText(line)
		}
	case "resolve", "reopen":
		status := "active"
		if action == "resolve" {
			status = "resolved"
		}
		for _, item := range commentItems(cm) {
			item.CreateAttr("status", status)
		}
	case "react":
		target := cm
		if s.ReplyID != "" {
			target = nil
			for _, v := range Children(Child(cm, nsModern, "replyLst"), nsModern, "reply") {
				if v.SelectAttrValue("id", "") == s.ReplyID {
					target = v
				}
			}
			if target == nil {
				return nil, fmt.Errorf("unknown reply_id")
			}
		}
		if s.Reaction == "" || utf8.RuneCountInString(s.Reaction) > 32 {
			return nil, fmt.Errorf("short reaction required")
		}
		aid, e := person()
		if e != nil {
			return nil, e
		}
		exts := Child(target, nsModern, "extLst")
		if exts == nil {
			exts = nsChild(target, "extLst", nsModern)
		}
		const uri = "{57CB4572-C831-44C2-8A1C-0ADB6CCDFE69}"
		var ext *etree.Element
		for _, v := range exts.ChildElements() {
			if v.SelectAttrValue("uri", "") == uri {
				ext = v
			}
		}
		if ext == nil {
			ext = nsChild(exts, "ext", NSP)
			ext.CreateAttr("uri", uri)
		}
		reactions := Child(ext, nsReaction, "reactions")
		if reactions == nil {
			reactions = nsChild(ext, "reactions", nsReaction)
		}
		var rxn *etree.Element
		for _, v := range Children(reactions, nsReaction, "rxn") {
			if v.SelectAttrValue("type", "") == s.Reaction {
				rxn = v
			}
		}
		if rxn == nil {
			rxn = nsChild(reactions, "rxn", nsReaction)
			rxn.CreateAttr("type", s.Reaction)
		}
		var prev *etree.Element
		for _, v := range rxn.ChildElements() {
			if v.SelectAttrValue("authorId", "") == aid {
				prev = v
			}
		}
		if prev == nil {
			v := nsChild(rxn, "instance", nsReaction)
			v.CreateAttr("authorId", aid)
			v.CreateAttr("time", now())
		} else {
			rxn.RemoveChild(prev)
			if len(rxn.ChildElements()) == 0 {
				reactions.RemoveChild(rxn)
			}
		}
	}
	if e = p.SetXML(entry.part, entry.doc); e != nil {
		return nil, e
	}
	return SlideComments(p, SlideCommentSpec{})
}

// commentAnchorCopy retains the in-scope namespace declarations that lxml
// includes when reporting a detached subtree. Declarations precede ordinary
// attributes; neither the source XML nor comment identities are modified.
func commentAnchorCopy(source *etree.Element) *etree.Element {
	out := source.Copy()
	seen := map[string]bool{}
	isNS := func(a etree.Attr) bool { return a.Space == "xmlns" || (a.Space == "" && a.Key == "xmlns") }
	for _, a := range out.Attr {
		if isNS(a) {
			seen[a.FullKey()] = true
		}
	}
	for ancestor := source.Parent(); ancestor != nil; ancestor = ancestor.Parent() {
		for _, a := range ancestor.Attr {
			if isNS(a) && !seen[a.FullKey()] {
				out.CreateAttr(a.FullKey(), a.Value)
				seen[a.FullKey()] = true
			}
		}
	}
	var order func(*etree.Element)
	order = func(e *etree.Element) {
		attrs := append([]etree.Attr(nil), e.Attr...)
		e.Attr = nil
		for _, a := range attrs {
			if isNS(a) {
				e.CreateAttr(a.FullKey(), a.Value)
			}
		}
		for _, a := range attrs {
			if !isNS(a) {
				e.CreateAttr(a.FullKey(), a.Value)
			}
		}
		for _, child := range e.ChildElements() {
			order(child)
		}
	}
	order(out)
	return out
}

// commentParagraphText follows lxml paragraph.itertext() in the pinned comment
// report. Character data includes element tails but excludes comments/PIs;
// empty DrawingML break elements do not synthesize a text-frame soft break.
func commentParagraphText(par *etree.Element) string {
	var b strings.Builder
	var visit func(*etree.Element)
	visit = func(e *etree.Element) {
		for _, token := range e.Child {
			switch v := token.(type) {
			case *etree.CharData:
				b.WriteString(v.Data)
			case *etree.Element:
				visit(v)
			}
		}
	}
	visit(par)
	return b.String()
}
