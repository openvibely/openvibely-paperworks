package wordedit

import (
	"errors"
	"strconv"
	"strings"

	"github.com/beevik/etree"
	"github.com/openvibely/openvibely-paperworks/internal/office"
)

var revisionTags = map[string]bool{"ins": true, "del": true, "moveTo": true, "moveFrom": true}
var moveMarkers = map[string]bool{"moveFromRangeStart": true, "moveFromRangeEnd": true, "moveToRangeStart": true, "moveToRangeEnd": true}

func textRevisions(d *Doc, accept, keepTracking bool) error {
	parts := d.WordParts()
	for _, root := range d.roots(parts) {
		var bad error
		office.Iter(root, func(e *etree.Element) bool {
			if e.NamespaceURI() != nsW {
				return true
			}
			if strings.HasSuffix(e.Tag, "PrChange") || e.Tag == "cellIns" || e.Tag == "cellDel" || e.Tag == "cellMerge" {
				bad = errors.New("tracked formatting/table changes require a capable office editor")
				return false
			}
			if revisionTags[e.Tag] {
				parent := e.Parent()
				for parent != nil {
					if is(parent, "hyperlink") {
						parent = parent.Parent()
					} else if is(parent, "sdtContent") && parent.Parent() != nil && is(parent.Parent(), "sdt") {
						parent = parent.Parent().Parent()
					} else {
						break
					}
				}
				if parent == nil || !is(parent, "p") {
					bad = errors.New("only paragraph, hyperlink and inline content-control text revisions are supported")
					return false
				}
			}
			return true
		})
		if bad != nil {
			return bad
		}
	}
	for i, root := range d.roots(parts) {
		var list []*etree.Element
		office.Iter(root, func(e *etree.Element) bool { list = append(list, e); return true })
		changed := false
		for j := len(list) - 1; j >= 0; j-- {
			e := list[j]
			if e.NamespaceURI() != nsW {
				continue
			}
			if moveMarkers[e.Tag] {
				remove(e)
				changed = true
				continue
			}
			if !revisionTags[e.Tag] {
				continue
			}
			changed = true
			parent := e.Parent()
			keep := (e.Tag == "ins" || e.Tag == "moveTo") == accept
			if keep {
				pos := e.Index()
				for _, c := range e.ChildElements() {
					for _, t := range all(c, "delText") {
						t.Tag = "t"
					}
					e.RemoveChild(c)
					parent.InsertChildAt(pos, c)
					pos++
				}
			}
			parent.RemoveChild(e)
		}
		if changed {
			d.Touch(parts[i])
		}
	}
	if !keepTracking {
		s, err := d.Settings()
		if err != nil {
			return err
		}
		for _, e := range office.Children(s, nsW, "trackRevisions") {
			s.RemoveChild(e)
		}
	}
	return nil
}

func (d *Doc) nextRevisionID() int {
	return numericIDs(d.roots(d.WordParts()), nsW, "id") + 1
}

func stamp(e *etree.Element, id int, author, date string) {
	setW(e, "id", strconv.Itoa(id))
	setW(e, "author", author)
	setW(e, "date", date)
}

func redlineInsertion(d *Doc, ident any, text, author string) error {
	if author == "" {
		return errors.New("replacement text and author are required")
	}
	id := ""
	switch v := ident.(type) {
	case string:
		id = v
	case float64:
		id = strconv.Itoa(int(v))
	}
	var matches []*etree.Element
	for _, e := range all(d.MainRoot(), "ins") {
		if getW(e, "id") == id {
			matches = append(matches, e)
		}
	}
	if len(matches) != 1 {
		return errors.New("Insertion ID must identify exactly one revision")
	}
	orig := matches[0]
	kids := orig.ChildElements()
	if p := orig.Parent(); !(is(p, "p") || is(p, "hyperlink")) || len(kids) == 0 {
		return errors.New("only plain text insertions can be replaced")
	}
	for _, r := range kids {
		if !plainRun(r) {
			return errors.New("only plain text insertions can be replaced")
		}
	}
	next := d.nextRevisionID()
	ins := W("ins")
	run := runSlice(kids[0], "", false)
	ins.AddChild(run)
	orig.Tag = "del"
	for _, t := range all(orig, "t") {
		t.Tag = "delText"
	}
	date := d.timestamp()
	stamp(orig, next, author, date)
	stamp(ins, next+1, author, date)
	addNext(orig, ins)
	// Resolve inherited Word namespaces before identifying preserved run properties.
	setRunText(run, text)
	d.Touch(d.Main)
	return nil
}

func redline(d *Doc, old, new, author string) error {
	if old == "" || author == "" {
		return errors.New("nonempty old text and author, and string new text are required")
	}
	if strings.ContainsAny(old+new, "\n\r\t") {
		return errors.New("use an office editor for paragraph breaks or tabs in replacements")
	}
	type cand struct {
		p     *etree.Element
		runs  []*etree.Element
		start int
	}
	var cands []cand
	var containers []*etree.Element
	for _, p := range all(d.Body(), "p") {
		containers = append(containers, p)
		containers = append(containers, office.Children(p, nsW, "hyperlink")...)
	}
	for _, p := range containers {
		runs := office.Children(p, nsW, "r")
		var b strings.Builder
		for _, r := range runs {
			b.WriteString(runText(r))
		}
		text := b.String()
		if !strings.Contains(text, old) {
			continue
		}
		for _, c := range p.ChildElements() {
			if !is(c, "r") && !is(c, "pPr") {
				return errors.New("matched paragraph has fields, comments, links or revisions; edit it with an office editor")
			}
		}
		for _, r := range runs {
			if !plainRun(r) {
				return errors.New("matched paragraph has non-text runs; refusing to flatten it")
			}
		}
		if strings.Count(text, old) != 1 {
			return errors.New("replacement text must match exactly once")
		}
		cands = append(cands, cand{p, runs, len([]rune(text[:strings.Index(text, old)]))})
	}
	if len(cands) != 1 {
		return errors.New("replacement text must match exactly one plain-text paragraph or hyperlink")
	}
	c := cands[0]
	start, end := c.start, c.start+len([]rune(old))
	next := d.nextRevisionID()
	date := d.timestamp()
	del, ins := W("del"), W("ins")
	stamp(del, next, author, date)
	stamp(ins, next+1, author, date)
	var before, after []*etree.Element
	var styleRun *etree.Element
	offset := 0
	for _, r := range c.runs {
		value := []rune(runText(r))
		finish := offset + len(value)
		switch {
		case finish <= start:
			before = append(before, r.Copy())
		case offset >= end:
			after = append(after, r.Copy())
		default:
			a, b := max(0, start-offset), min(len(value), end-offset)
			if a > 0 {
				before = append(before, runSlice(r, string(value[:a]), false))
			}
			del.AddChild(runSlice(r, string(value[a:b]), true))
			if styleRun == nil {
				styleRun = r
			}
			if b < len(value) {
				after = append(after, runSlice(r, string(value[b:]), false))
			}
		}
		offset = finish
	}
	if new != "" {
		ins.AddChild(runSlice(styleRun, new, false))
	}
	for _, r := range c.runs {
		c.p.RemoveChild(r)
	}
	out := append(before, del)
	if new != "" {
		out = append(out, ins)
	}
	for _, e := range append(out, after...) {
		c.p.AddChild(e)
	}
	d.Touch(d.Main)
	return nil
}

// replaceMarker substitutes one marker (or every occurrence when factory is
// set) found inside contiguous plain runs of body/header/footer paragraphs.
func replaceMarker(d *Doc, marker string, elements []*etree.Element, factory func([]*etree.Element) []*etree.Element) (*etree.Element, error) {
	if marker == "" {
		return nil, errors.New("a nonempty marker is required")
	}
	type match struct {
		part   string
		p      *etree.Element
		runs   []*etree.Element
		starts []int
	}
	var matches []match
	total := 0
	parts, err := d.markerStoryParts()
	if err != nil {
		return nil, err
	}
	for _, part := range parts {
		root, _ := d.Root(part)
		for _, p := range all(root, "p") {
			var groups [][]*etree.Element
			var runs []*etree.Element
			depth := 0
			for _, c := range p.ChildElements() {
				if plainRun(c) && depth == 0 {
					runs = append(runs, c)
					continue
				}
				if len(runs) > 0 {
					groups = append(groups, runs)
					runs = nil
				}
				for _, f := range all(c, "fldChar") {
					switch getW(f, "fldCharType") {
					case "begin":
						depth++
					case "end":
						depth = max(0, depth-1)
					}
				}
			}
			if len(runs) > 0 {
				groups = append(groups, runs)
			}
			for _, g := range groups {
				var b strings.Builder
				for _, r := range g {
					b.WriteString(runText(r))
				}
				text := []rune(b.String())
				m := []rune(marker)
				var starts []int
				for i := 0; i+len(m) <= len(text); {
					if string(text[i:i+len(m)]) == marker {
						starts = append(starts, i)
						i += len(m)
					} else {
						i++
					}
				}
				if len(starts) > 0 {
					matches = append(matches, match{part, p, g, starts})
					total += len(starts)
				}
			}
		}
	}
	if len(matches) == 0 || (factory == nil && total != 1) {
		return nil, errors.New("marker must match exactly once in ordinary text")
	}
	mlen := len([]rune(marker))
	for _, m := range matches {
		type span struct {
			r           *etree.Element
			text        []rune
			left, right int
		}
		var spans []span
		offset := 0
		for _, r := range m.runs {
			t := []rune(runText(r))
			spans = append(spans, span{r, t, offset, offset + len(t)})
			offset += len(t)
		}
		slices := func(s, e int) []*etree.Element {
			var out []*etree.Element
			for _, sp := range spans {
				if sp.left < e && sp.right > s {
					a, b := max(0, s-sp.left), min(len(sp.text), e-sp.left)
					out = append(out, runSlice(sp.r, string(sp.text[a:b]), false))
				}
			}
			return out
		}
		var result []*etree.Element
		cursor := 0
		for _, s := range m.starts {
			result = append(result, slices(cursor, s)...)
			if factory != nil {
				result = append(result, factory(slices(s, s+mlen))...)
			} else {
				result = append(result, elements...)
			}
			cursor = s + mlen
		}
		result = append(result, slices(cursor, offset)...)
		pos := m.runs[0].Index()
		for _, r := range m.runs {
			m.p.RemoveChild(r)
		}
		for i, e := range result {
			if e.Parent() != nil {
				remove(e)
			}
			m.p.InsertChildAt(pos+i, e)
		}
		d.Touch(m.part)
	}
	return matches[0].p, nil
}
