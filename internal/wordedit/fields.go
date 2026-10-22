package wordedit

import (
	"errors"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/beevik/etree"
	"github.com/openvibely/openvibely-paperworks/internal/office"
)

var bookmarkRe = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{0,39}$`)

func bookmarkName(v string) error {
	if !bookmarkRe.MatchString(v) {
		return errors.New("bookmark needs a letter followed by up to 39 letters, digits or underscores")
	}
	return nil
}

func addBookmark(d *Doc, p *etree.Element, name string) error {
	if err := bookmarkName(name); err != nil {
		return err
	}
	starts := all(d.MainRoot(), "bookmarkStart")
	maxID := 0
	for _, s := range starts {
		if getW(s, "name") == name {
			return errors.New("bookmark name already exists")
		}
		if n, err := strconv.Atoi(getW(s, "id")); err == nil && n > maxID {
			maxID = n
		}
	}
	id := strconv.Itoa(maxID + 1)
	start, end := W("bookmarkStart"), W("bookmarkEnd")
	setW(start, "id", id)
	setW(start, "name", name)
	setW(end, "id", id)
	pos := 0
	if child(p, "pPr") != nil {
		pos = 1
	}
	insertAt(p, pos, start)
	p.AddChild(end)
	d.Touch(d.Main)
	return nil
}

func addField(p *etree.Element, instr, display string) *etree.Element {
	f := W("fldSimple")
	setW(f, "instr", instr)
	setW(f, "dirty", "true")
	r := W("r")
	r.AddChild(WT("t", display))
	f.AddChild(r)
	p.AddChild(f)
	return f
}

func internalLink(name, text string) *etree.Element {
	link := W("hyperlink")
	setW(link, "anchor", name)
	r := W("r")
	rp := W("rPr")
	st := W("rStyle")
	setW(st, "val", "Hyperlink")
	rp.AddChild(st)
	r.AddChild(rp)
	r.AddChild(WT("t", text))
	link.AddChild(r)
	return link
}

func hasBookmark(d *Doc, name string) bool {
	for _, s := range all(d.MainRoot(), "bookmarkStart") {
		if getW(s, "name") == name {
			return true
		}
	}
	return false
}

// placeContent puts a temporary paragraph's children at a marker or appends it to the body.
func placeContent(d *Doc, p *etree.Element, marker string) error {
	if marker == "" {
		appendBody(d, p)
		d.Touch(d.Main)
		return nil
	}
	_, err := replaceMarker(d, marker, p.ChildElements(), nil)
	return err
}

func reference(d *Doc, spec map[string]any) error {
	name := str(spec, "name")
	if err := bookmarkName(name); err != nil {
		return err
	}
	if !hasBookmark(d, name) {
		return errors.New("unknown bookmark")
	}
	p := W("p")
	text, hasText := spec["text"].(string)
	if truthy(spec["page"]) || truthy(spec["field"]) {
		kind := "REF "
		if truthy(spec["page"]) {
			kind = "PAGEREF "
		}
		addField(p, kind+name+` \h`, text)
	} else {
		if !hasText {
			text = name
		}
		p.AddChild(internalLink(name, text))
	}
	return placeContent(d, p, str(spec, "marker"))
}

var labelRe = regexp.MustCompile(`^[A-Za-z]+$`)

func caption(d *Doc, spec map[string]any) error {
	label := str(spec, "label")
	if !labelRe.MatchString(label) {
		return errors.New("caption label must contain letters only")
	}
	p := W("p")
	pp := pPr(p)
	st := W("pStyle")
	setW(st, "val", styleID(d, "Caption"))
	pp.AddChild(st)
	p.AddChild(runOf(label + " "))
	field := addField(p, "SEQ "+label+` \* ARABIC`, "")
	p.AddChild(runOf(": " + str(spec, "text")))
	switch {
	case spec["after_paragraph"] != nil:
		ref, err := selectParagraph(d, spec["after_paragraph"])
		if err != nil {
			return err
		}
		addNext(ref, p)
	case spec["after_table"] != nil:
		i, ok := intArg(spec["after_table"])
		tables := bodyTables(d)
		if !ok || i < 0 || i >= len(tables) {
			return errors.New("unknown table index")
		}
		addNext(tables[i], p)
	default:
		appendBody(d, p)
	}
	d.Touch(d.Main)
	if b := str(spec, "bookmark"); b != "" {
		if err := addBookmark(d, p, b); err != nil {
			return err
		}
		start, end := child(p, "bookmarkStart"), child(p, "bookmarkEnd")
		remove(start)
		remove(end)
		addPrev(field, start)
		addNext(field, end)
	}
	return nil
}

func runOf(text string) *etree.Element {
	r := W("r")
	setRunText(r, text)
	return r
}

var tocRe = regexp.MustCompile(`^[1-9]-[1-9]$`)

func toc(d *Doc, spec map[string]any) error {
	levels := "1-3"
	if v, ok := spec["levels"].(string); ok {
		levels = v
	}
	if !tocRe.MatchString(levels) || levels[0] > levels[2] {
		return errors.New("TOC levels must be an ascending range from 1 to 9")
	}
	p := W("p")
	for _, kind := range []string{"begin", "instruction", "separate", "display", "end"} {
		r := W("r")
		switch kind {
		case "instruction":
			r.AddChild(WT("instrText", ` TOC \o "`+levels+`" \h \z \u `))
		case "display":
			r.AddChild(WT("t", "Update table of contents"))
		default:
			f := W("fldChar")
			setW(f, "fldCharType", kind)
			r.AddChild(f)
		}
		p.AddChild(r)
	}
	if err := requestFieldRefresh(d); err != nil {
		return err
	}
	return placeContent(d, p, str(spec, "marker"))
}

// headingLevel resolves outline level through the paragraph's style chain.
func headingLevel(d *Doc, p *etree.Element) (int, bool) {
	if pp := child(p, "pPr"); pp != nil {
		if o := child(pp, "outlineLvl"); o != nil {
			n, err := strconv.Atoi(getW(o, "val"))
			return n + 1, err == nil
		}
	}
	styles := d.styles()
	id := paraStyle(p)
	if id == "" {
		id = styles.def
	}
	seen := map[string]bool{}
	for id != "" && !seen[id] {
		seen[id] = true
		s := styles.byID[id]
		if s == nil {
			break
		}
		if pp := child(s, "pPr"); pp != nil {
			if o := child(pp, "outlineLvl"); o != nil {
				n, err := strconv.Atoi(getW(o, "val"))
				return n + 1, err == nil
			}
		}
		name := getW(child(s, "name"), "val")
		if m := regexp.MustCompile(`^(?i:heading) ([1-9])$`).FindStringSubmatch(name); m != nil {
			return int(m[1][0] - '0'), true
		}
		id = getW(child(s, "basedOn"), "val")
	}
	return 0, false
}

func paraStyle(p *etree.Element) string {
	return getW(child(child(p, "pPr"), "pStyle"), "val")
}

type styleTable struct {
	byID   map[string]*etree.Element
	byName map[string]string
	def    string
	part   string
}

func (d *Doc) styles() styleTable {
	t := styleTable{byID: map[string]*etree.Element{}, byName: map[string]string{}}
	part, _ := d.Related(d.Main, "styles")
	if part == "" {
		return t
	}
	t.part = part
	root, err := d.Root(part)
	if err != nil {
		return t
	}
	for _, s := range office.Children(root, nsW, "style") {
		id := getW(s, "styleId")
		t.byID[id] = s
		name := getW(child(s, "name"), "val")
		t.byName[strings.ToLower(name)] = id
		if getW(s, "type") == "paragraph" && office.Truthy(getW(s, "default"), false) {
			t.def = id
		}
	}
	return t
}

// styleID maps a UI style name to its ID, falling back to the name without spaces.
func styleID(d *Doc, name string) string {
	if id, ok := d.styles().byName[strings.ToLower(name)]; ok {
		return id
	}
	return strings.ReplaceAll(name, " ", "")
}

func styleName(d *Doc, p *etree.Element) string {
	t := d.styles()
	id := paraStyle(p)
	if id == "" {
		id = t.def
	}
	if s := t.byID[id]; s != nil {
		return getW(child(s, "name"), "val")
	}
	return id
}

func navigation(d *Doc, spec map[string]any) error {
	maxLevel := 3
	if v, ok := spec["max_level"]; ok {
		n, ok := intArg(v)
		if !ok || n < 1 || n > 9 {
			return errors.New("max_level must be from 1 to 9")
		}
		maxLevel = n
	}
	includeCaptions := true
	if v, ok := spec["include_captions"].(bool); ok {
		includeCaptions = v
	}
	type head struct {
		p     *etree.Element
		level int
	}
	var heads []head
	for _, p := range bodyParagraphs(d) {
		lvl, ok := headingLevel(d, p)
		if ok && lvl <= maxLevel {
			heads = append(heads, head{p, lvl})
		} else if includeCaptions && strings.EqualFold(styleName(d, p), "caption") {
			heads = append(heads, head{p, 1})
		}
	}
	if len(heads) == 0 {
		return errors.New("no headings found")
	}
	prefix := "Contents"
	if v, ok := spec["prefix"].(string); ok {
		prefix = v
	}
	if err := bookmarkName(prefix); err != nil {
		return err
	}
	if len(prefix) > 25 {
		return errors.New("navigation prefix is too long")
	}
	title := "Contents"
	if v, ok := spec["title"].(string); ok {
		title = v
	}
	tp := newPara(title)
	d.Body().InsertChildAt(0, tp)
	if err := addBookmark(d, tp, prefix); err != nil {
		return err
	}
	back := true
	if v, ok := spec["back_links"].(bool); ok {
		back = v
	}
	cursor := tp
	for i, h := range heads {
		name := prefix + "_" + strconv.Itoa(i+1)
		if err := addBookmark(d, h.p, name); err != nil {
			return err
		}
		item := W("p")
		ind := W("ind")
		setW(ind, "left", strconv.Itoa((h.level-1)*288))
		insertInPPr(pPr(item), ind)
		item.AddChild(internalLink(name, paraText(h.p)))
		addNext(cursor, item)
		cursor = item
		if back {
			bp := W("p")
			bp.AddChild(internalLink(prefix, "Back to contents"))
			addNext(h.p, bp)
		}
	}
	bottom := W("p")
	appendBody(d, bottom)
	if err := addBookmark(d, bottom, prefix+"_Bottom"); err != nil {
		return err
	}
	bottom.AddChild(internalLink(prefix, "Top"))
	tp.AddChild(runOf(" "))
	tp.AddChild(internalLink(prefix+"_Bottom", "Bottom"))
	d.Touch(d.Main)
	return nil
}

type fieldRec struct {
	pos        int
	instr      string
	texts      []*etree.Element
	simple     *etree.Element
	begin, sep *etree.Element
	end        *etree.Element
	nested     bool
	kind       string
	target     string
	root       *etree.Element
	part       string
}

func materialize(d *Doc, only []string) error {
	if only == nil {
		only = []string{"SEQ", "REF"}
	}
	want := map[string]bool{}
	for _, k := range only {
		if k != "SEQ" && k != "REF" {
			return errors.New("only must select SEQ and/or REF")
		}
		want[k] = true
	}
	if len(want) == 0 {
		return errors.New("only must select SEQ and/or REF")
	}
	parts := append([]string{d.Main}, without(d.WordParts(), d.Main)...)
	var fields []*fieldRec
	for _, part := range parts {
		root, _ := d.Root(part)
		var stack []*fieldRec
		var records []*fieldRec
		skipped := map[*etree.Element]bool{}
		pos := 0
		var walkErr error
		office.Iter(root, func(e *etree.Element) bool {
			pos++
			if skipped[e] || walkErr != nil {
				return walkErr == nil
			}
			switch {
			case is(e, "fldSimple"):
				if len(stack) > 0 {
					stack[len(stack)-1].nested = true
				}
				nested := len(stack) > 0
				office.Iter(e, func(x *etree.Element) bool {
					if x != e {
						skipped[x] = true
						if is(x, "fldSimple") || is(x, "fldChar") {
							nested = true
						}
					}
					return true
				})
				records = append(records, &fieldRec{pos: pos, instr: getW(e, "instr"), texts: all(e, "t"), simple: e, nested: nested})
			case is(e, "fldChar"):
				switch getW(e, "fldCharType") {
				case "begin":
					if len(stack) > 0 {
						stack[len(stack)-1].nested = true
					}
					stack = append(stack, &fieldRec{pos: pos, nested: len(stack) > 0, begin: e})
				case "separate", "end":
					if len(stack) == 0 {
						walkErr = errors.New("unbalanced complex field")
						return false
					}
					cur := stack[len(stack)-1]
					if getW(e, "fldCharType") == "separate" {
						if cur.sep != nil {
							walkErr = errors.New("repeated field separator")
							return false
						}
						cur.sep = e
					} else {
						cur.end = e
						records = append(records, cur)
						stack = stack[:len(stack)-1]
					}
				}
			case len(stack) > 0 && is(e, "instrText") && stack[len(stack)-1].sep == nil:
				stack[len(stack)-1].instr += e.Text()
			case len(stack) > 0 && is(e, "t") && stack[len(stack)-1].sep != nil:
				stack[len(stack)-1].texts = append(stack[len(stack)-1].texts, e)
			}
			return true
		})
		if walkErr != nil {
			return walkErr
		}
		if len(stack) > 0 {
			return errors.New("unterminated complex field")
		}
		sort.SliceStable(records, func(i, j int) bool { return records[i].pos < records[j].pos })
		for _, r := range records {
			tokens := strings.Fields(r.instr)
			if len(tokens) == 0 || !want[strings.ToUpper(tokens[0])] {
				continue
			}
			if len(tokens) < 2 || r.nested {
				return errors.New("incomplete or nested SEQ/REF field requires an office editor")
			}
			r.kind, r.target, r.root, r.part = strings.ToUpper(tokens[0]), tokens[1], root, part
			rest := strings.Join(tokens[2:], " ")
			if (r.kind == "SEQ" && rest != "" && rest != `\* ARABIC`) || (r.kind == "REF" && rest != "" && rest != `\h`) {
				return errors.New("unsupported " + r.kind + " switches; update in an office application")
			}
			if r.simple == nil && (r.sep == nil || ancestor(r.begin, "p") != ancestor(r.end, "p")) {
				return errors.New("complex SEQ/REF field needs a result within one paragraph")
			}
			fields = append(fields, r)
		}
	}
	setResult := func(f *fieldRec, value string) error {
		if len(f.texts) == 0 {
			t := W("t")
			if f.simple != nil {
				r := W("r")
				r.AddChild(t)
				f.simple.AddChild(r)
			} else {
				if !is(f.sep.Parent(), "r") {
					return errors.New("field separator must be inside a run")
				}
				addNext(f.sep, t)
			}
			f.texts = append(f.texts, t)
		}
		f.texts[0].SetText(value)
		f.texts[0].RemoveAttr("xml:space")
		f.texts[0].CreateAttr("xml:space", "preserve")
		for _, t := range f.texts[1:] {
			t.SetText("")
		}
		d.Touch(f.part)
		return nil
	}
	counters := map[string]int{}
	for _, f := range fields {
		if f.kind == "SEQ" {
			key := f.part + "\x00" + f.target
			counters[key]++
			if err := setResult(f, strconv.Itoa(counters[key])); err != nil {
				return err
			}
		}
	}
	if !want["REF"] {
		return nil
	}
	bookmarks := map[string]string{}
	for _, part := range parts {
		root, _ := d.Root(part)
		type open struct {
			name string
			b    *strings.Builder
		}
		active := map[string]*open{}
		var order []string
		office.Iter(root, func(e *etree.Element) bool {
			switch {
			case is(e, "bookmarkStart"):
				id := getW(e, "id")
				active[id] = &open{getW(e, "name"), &strings.Builder{}}
				order = append(order, id)
			case is(e, "bookmarkEnd"):
				if o := active[getW(e, "id")]; o != nil {
					delete(active, getW(e, "id"))
					if o.name != "" {
						bookmarks[o.name] = o.b.String()
					}
				}
			case is(e, "t"):
				for _, o := range active {
					o.b.WriteString(e.Text())
				}
			}
			return true
		})
		if len(active) > 0 {
			return errors.New("unclosed bookmark requires repair before field evaluation")
		}
	}
	for _, f := range fields {
		if f.kind == "REF" {
			v, ok := bookmarks[f.target]
			if !ok {
				return errors.New("REF target not found")
			}
			if err := setResult(f, v); err != nil {
				return err
			}
		}
	}
	return nil
}

var refInstr = regexp.MustCompile(`(?i)^(REF|PAGEREF)\b`)

func freezeReferences(d *Doc) error {
	for _, part := range d.WordParts() {
		root, _ := d.Root(part)
		changed := false
		for _, f := range all(root, "fldSimple") {
			tokens := strings.Fields(getW(f, "instr"))
			if len(tokens) == 0 || (tokens[0] != "REF" && tokens[0] != "PAGEREF") {
				continue
			}
			if strings.TrimSpace(textOf(f)) == "" {
				return errors.New("reference has no cached result; refresh before freezing")
			}
			for _, c := range f.ChildElements() {
				f.RemoveChild(c)
				addPrev(f, c)
			}
			remove(f)
			changed = true
		}
		for _, p := range all(root, "p") {
			kids := p.ChildElements()
			type span struct {
				start, end int
				cached     []*etree.Element
			}
			var spans []span
			var stack [][2]int
			for i, c := range kids {
				kind := ""
				if is(c, "r") {
					if fc := child(c, "fldChar"); fc != nil {
						kind = getW(fc, "fldCharType")
					}
				}
				switch {
				case kind == "begin":
					stack = append(stack, [2]int{i, -1})
				case kind == "separate" && len(stack) > 0:
					stack[len(stack)-1][1] = i
				case kind == "end" && len(stack) > 0:
					top := stack[len(stack)-1]
					stack = stack[:len(stack)-1]
					stop := top[1]
					if stop < 0 {
						stop = i
					}
					var instr strings.Builder
					for _, x := range kids[top[0]+1 : stop] {
						for _, t := range all(x, "instrText") {
							instr.WriteString(t.Text())
						}
					}
					if !refInstr.MatchString(strings.TrimSpace(instr.String())) {
						continue
					}
					if top[1] < 0 {
						return errors.New("reference has no cached result; refresh before freezing")
					}
					cached := kids[top[1]+1 : i]
					var ct strings.Builder
					for _, x := range cached {
						ct.WriteString(textOf(x))
						if len(all(x, "fldChar")) > 0 {
							return errors.New("nested reference caches require an office editor")
						}
					}
					if strings.TrimSpace(ct.String()) == "" {
						return errors.New("reference has no cached result; refresh before freezing")
					}
					spans = append(spans, span{top[0], i, cached})
				}
			}
			for j := len(spans) - 1; j >= 0; j-- {
				s := spans[j]
				anchor := kids[s.start]
				for _, c := range s.cached {
					addPrev(anchor, c.Copy())
				}
				for _, c := range kids[s.start : s.end+1] {
					p.RemoveChild(c)
				}
				changed = true
			}
		}
		for _, it := range all(root, "instrText") {
			if refInstr.MatchString(strings.TrimSpace(it.Text())) {
				return errors.New("unbalanced or cross-paragraph reference field requires an office editor")
			}
		}
		if changed {
			d.Touch(part)
		}
	}
	return nil
}

func textOf(e *etree.Element) string {
	var b strings.Builder
	for _, t := range all(e, "t") {
		b.WriteString(t.Text())
	}
	return b.String()
}

func without(list []string, s string) []string {
	var out []string
	for _, x := range list {
		if x != s {
			out = append(out, x)
		}
	}
	return out
}

func str(spec map[string]any, k string) string { s, _ := spec[k].(string); return s }

func truthy(v any) bool {
	switch x := v.(type) {
	case bool:
		return x
	case string:
		return x != ""
	case float64:
		return x != 0
	}
	return v != nil
}

// requestFieldRefresh changes settings only; it does not calculate field results.
func requestFieldRefresh(d *Doc) error {
	s, err := d.Settings()
	if err != nil {
		return err
	}
	u := child(s, "updateFields")
	if u == nil {
		u = W("updateFields")
		// CT_Settings places updateFields before these following settings.
		following := map[string]bool{}
		for _, tag := range strings.Fields("hdrShapeDefaults footnotePr endnotePr compat docVars rsids mathPr attachedSchema themeFontLang clrSchemeMapping doNotIncludeSubdocsInStats doNotAutoCompressPictures forceUpgrade captions readModeInkLockDown smartTagType schemaLibrary shapeDefaults doNotEmbedSmartTags decimalSymbol listSeparator") {
			following[tag] = true
		}
		inserted := false
		for _, c := range s.ChildElements() {
			if following[c.Tag] {
				s.InsertChildAt(c.Index(), u)
				inserted = true
				break
			}
		}
		if !inserted {
			s.AddChild(u)
		}
	}
	setW(u, "val", "true")
	return nil
}
