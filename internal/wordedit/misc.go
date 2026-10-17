package wordedit

import (
	"errors"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/beevik/etree"
	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
)

const (
	nsCP      = "http://schemas.openxmlformats.org/package/2006/metadata/core-properties"
	nsDC      = "http://purl.org/dc/elements/1.1/"
	nsDCTerms = "http://purl.org/dc/terms/"
	nsXSI     = "http://www.w3.org/2001/XMLSchema-instance"
)

var coreFields = map[string][2]string{
	"author": {nsDC, "creator"}, "last_modified_by": {nsCP, "lastModifiedBy"}, "comments": {nsDC, "description"},
	"keywords": {nsCP, "keywords"}, "subject": {nsDC, "subject"}, "category": {nsCP, "category"},
	"identifier": {nsDC, "identifier"}, "language": {nsDC, "language"}, "title": {nsDC, "title"},
	"version": {nsCP, "version"}, "content_status": {nsCP, "contentStatus"},
}

var coreLocal = map[string]string{"creator": nsDC, "lastModifiedBy": nsCP, "description": nsDC, "keywords": nsCP, "identifier": nsDC, "subject": nsDC, "category": nsCP, "language": nsDC, "title": nsDC, "version": nsCP, "contentStatus": nsCP, "created": nsDCTerms, "modified": nsDCTerms}

func corePart(d *Doc) (string, error) {
	rels, err := d.Pkg.Rels("")
	if err != nil {
		return "", err
	}
	for _, r := range rels {
		if strings.HasSuffix(r.Type, "/core-properties") {
			return opc.Resolve("", r.Target), nil
		}
	}
	part := "docProps/core.xml"
	x := etree.NewDocument()
	x.CreateProcInst("xml", `version="1.0" encoding="UTF-8" standalone="yes"`)
	root := x.CreateElement("cp:coreProperties")
	for p, ns := range map[string]string{"cp": nsCP, "dc": nsDC, "dcterms": nsDCTerms, "xsi": nsXSI} {
		root.CreateAttr("xmlns:"+p, ns)
	}
	d.docs[part], d.dirty[part] = x, true
	d.Pkg.Set(part, nil)
	if err := d.Pkg.EnsureContentType(part, "application/vnd.openxmlformats-package.core-properties+xml"); err != nil {
		return "", err
	}
	_, err = d.Pkg.AddRel("", "http://schemas.openxmlformats.org/package/2006/relationships/metadata/core-properties", part, false)
	return part, err
}

// setCore sets core-property elements by local name; empty values clear them.
func setCore(d *Doc, values map[string]string) error {
	part, err := corePart(d)
	if err != nil {
		return err
	}
	root, err := d.Root(part)
	if err != nil {
		return err
	}
	prefix := func(ns string) string {
		for _, a := range root.Attr {
			if a.Space == "xmlns" && a.Value == ns {
				return a.Key
			}
		}
		p := map[string]string{nsCP: "cp", nsDC: "dc", nsDCTerms: "dcterms", nsXSI: "xsi"}[ns]
		root.CreateAttr("xmlns:"+p, ns)
		return p
	}
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, local := range keys {
		ns := coreLocal[local]
		e := office.Child(root, ns, local)
		if e == nil {
			e = root.CreateElement(prefix(ns) + ":" + local)
			if ns == nsDCTerms {
				e.CreateAttr(prefix(nsXSI)+":type", "dcterms:W3CDTF")
			}
		}
		e.SetText(values[local])
	}
	d.Touch(part)
	return nil
}

func metadata(d *Doc, spec map[string]any) error {
	list, ok := spec["clear"].([]any)
	if !ok {
		return errors.New("metadata needs a clear list")
	}
	values := map[string]string{}
	for _, v := range list {
		name, _ := v.(string)
		f, ok := coreFields[name]
		if !ok {
			return fmt.Errorf("unsupported core metadata field: %v", v)
		}
		values[f[1]] = ""
	}
	return setCore(d, values)
}

func googleTitle(d *Doc) error {
	t := d.styles()
	titles := map[string]bool{}
	for id, s := range t.byID {
		if getW(child(s, "name"), "val") == "Title" {
			titles[id] = true
			for _, b := range all(s, "pBdr") {
				remove(b)
				d.Touch(t.part)
			}
		}
	}
	for _, p := range bodyParagraphs(d) {
		if titles[paraStyle(p)] {
			for _, b := range all(p, "pBdr") {
				remove(b)
				d.Touch(d.Main)
			}
		}
	}
	return nil
}

func accessibility(d *Doc, spec map[string]any) error {
	alt := map[string]string{}
	if m, ok := spec["image_alt"].(map[string]any); ok {
		for k, v := range m {
			s, ok := v.(string)
			if !ok {
				return errors.New("image descriptions must be strings")
			}
			alt[k] = s
		}
	}
	found := map[string]bool{}
	for _, part := range d.WordParts() {
		root, _ := d.Root(part)
		office.Iter(root, func(e *etree.Element) bool {
			if e.Tag == "docPr" {
				name := e.SelectAttrValue("name", "")
				if v, ok := alt[name]; ok {
					e.RemoveAttr("descr")
					e.CreateAttr("descr", v)
					found[name] = true
					d.Touch(part)
				}
			}
			return true
		})
	}
	var missing []string
	for k := range alt {
		if !found[k] {
			missing = append(missing, k)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return fmt.Errorf("unknown image names: %s", strings.Join(missing, ", "))
	}
	tables := bodyTables(d)
	if list, ok := spec["header_tables"].([]any); ok {
		for _, v := range list {
			i, ok := intArg(v)
			if !ok || i < 0 || i >= len(tables) {
				return errors.New("unknown table index")
			}
			tr := child(tables[i], "tr")
			if tr == nil {
				return errors.New("table has no rows")
			}
			trPr := child(tr, "trPr")
			if trPr == nil {
				trPr = W("trPr")
				pos := 0
				if child(tr, "tblPrEx") != nil {
					pos = 1
				}
				insertAt(tr, pos, trPr)
			}
			if child(trPr, "tblHeader") == nil {
				trPr.AddChild(W("tblHeader"))
			}
			d.Touch(d.Main)
		}
	}
	return nil
}

func normalize(d *Doc, spec map[string]any) error {
	list, _ := spec["styles"].([]any)
	t := d.styles()
	selected := map[string]bool{}
	for _, v := range list {
		name, _ := v.(string)
		if _, ok := t.byName[strings.ToLower(name)]; !ok {
			return errors.New("select existing paragraph styles to normalize")
		}
		selected[strings.ToLower(name)] = true
	}
	if len(selected) == 0 {
		return errors.New("select existing paragraph styles to normalize")
	}
	var spacing []float64
	if hs, ok := spec["heading_spacing"].([]any); ok && len(hs) > 0 {
		if len(hs) != 2 {
			return errors.New("heading spacing must be two point values from 0 to 144")
		}
		for _, v := range hs {
			f, ok := v.(float64)
			if !ok || f < 0 || f > 144 {
				return errors.New("heading spacing must be two point values from 0 to 144")
			}
			spacing = append(spacing, f)
		}
	}
	for _, p := range all(d.Body(), "p") {
		if !selected[strings.ToLower(styleName(d, p))] {
			continue
		}
		for _, r := range office.Children(p, nsW, "r") {
			if rp := child(r, "rPr"); rp != nil {
				for _, c := range rp.ChildElements() {
					if !is(c, "rStyle") {
						rp.RemoveChild(c)
					}
				}
			}
		}
		if truthy(spec["clear_paragraph_overrides"]) {
			if pp := child(p, "pPr"); pp != nil {
				for _, c := range pp.ChildElements() {
					if !is(c, "pStyle") && !is(c, "numPr") && !is(c, "sectPr") {
						pp.RemoveChild(c)
					}
				}
			}
		}
		if spacing != nil {
			if _, ok := headingLevel(d, p); ok {
				pp := pPr(p)
				sp := child(pp, "spacing")
				if sp == nil {
					sp = W("spacing")
					insertInPPr(pp, sp)
				}
				setW(sp, "before", strconv.Itoa(int(math.Round(spacing[0]*20))))
				setW(sp, "after", strconv.Itoa(int(math.Round(spacing[1]*20))))
			}
		}
	}
	d.Touch(d.Main)
	return nil
}

func tableGeometry(d *Doc, spec map[string]any) error {
	tables := bodyTables(d)
	i, ok := intArg(spec["table"])
	if !ok || i < 0 || i >= len(tables) {
		return errors.New("unknown table index")
	}
	tbl := tables[i]
	raw, _ := spec["widths_twips"].([]any)
	grid := child(tbl, "tblGrid")
	cols := office.Children(grid, nsW, "gridCol")
	var widths []int
	for _, v := range raw {
		n, ok := intArg(v)
		if !ok || n <= 0 {
			return errors.New("supply a positive twip width for each grid column")
		}
		widths = append(widths, n)
	}
	if len(widths) != len(cols) || len(widths) == 0 {
		return errors.New("supply a positive twip width for each grid column")
	}
	indent := 0
	if v, has := spec["indent_twips"]; has {
		n, ok := intArg(v)
		if !ok || n < 0 {
			return errors.New("table indent must be a nonnegative integer in twips")
		}
		indent = n
	}
	total := 0
	for _, w := range widths {
		total += w
	}
	var sect *etree.Element
	for _, sib := range tbl.Parent().ChildElements()[index(tbl)+1:] {
		if found := all(sib, "sectPr"); len(found) > 0 {
			sect = found[0]
			break
		}
	}
	if sect == nil {
		return errors.New("cannot determine table page section")
	}
	pgSz, pgMar := child(sect, "pgSz"), child(sect, "pgMar")
	pw, e1 := strconv.Atoi(getW(pgSz, "w"))
	left, e2 := strconv.Atoi(getW(pgMar, "left"))
	right, e3 := strconv.Atoi(getW(pgMar, "right"))
	if pgSz == nil || pgMar == nil || e1 != nil || e2 != nil || e3 != nil {
		return errors.New("section needs explicit page size and margins before table geometry")
	}
	gutter, _ := strconv.Atoi(getW(pgMar, "gutter"))
	available := pw - left - right - gutter
	if c := child(sect, "cols"); c != nil {
		var explicit []int
		for _, col := range office.Children(c, nsW, "col") {
			if n, err := strconv.Atoi(getW(col, "w")); err == nil {
				explicit = append(explicit, n)
			}
		}
		count := 1
		if v := getW(c, "num"); v != "" {
			count, _ = strconv.Atoi(v)
		}
		if len(explicit) > 0 {
			available = explicit[0]
			for _, n := range explicit {
				available = min(available, n)
			}
		} else if count > 1 {
			space := 720
			if v := getW(c, "space"); v != "" {
				space, _ = strconv.Atoi(v)
			}
			available = (available - space*(count-1)) / count
		}
	}
	if total+indent > available {
		return errors.New("table width and indent exceed the usable section column width")
	}
	tp := child(tbl, "tblPr")
	if tp == nil {
		tp = W("tblPr")
		tbl.InsertChildAt(0, tp)
	}
	tblOrder := []string{"tblStyle", "tblpPr", "tblOverlap", "bidiVisual", "tblStyleRowBandSize", "tblStyleColBandSize", "tblW", "jc", "tblCellSpacing", "tblInd", "tblBorders", "shd", "tblLayout", "tblCellMar", "tblLook", "tblCaption", "tblDescription"}
	// reference DOCX library sets autofit before the script appends missing width/indent.
	// Existing width/indent nodes retain their original positions.
	layout := office.EnsureChild(tp, nsW, "tblLayout", tblOrder)
	setW(layout, "type", "fixed")
	for _, kv := range []struct {
		tag string
		val int
	}{{"tblW", total}, {"tblInd", indent}} {
		e := child(tp, kv.tag)
		if e == nil {
			e = W(kv.tag)
			tp.AddChild(e)
		}
		setW(e, "type", "dxa")
		setW(e, "w", strconv.Itoa(kv.val))
	}
	for j, c := range cols {
		setW(c, "w", strconv.Itoa(widths[j]))
	}
	for _, tr := range office.Children(tbl, nsW, "tr") {
		col := 0
		if gb := child(child(tr, "trPr"), "gridBefore"); gb != nil {
			col, _ = strconv.Atoi(getW(gb, "val"))
		}
		for _, tc := range office.Children(tr, nsW, "tc") {
			tcPr := child(tc, "tcPr")
			if tcPr == nil {
				tcPr = W("tcPr")
				tc.InsertChildAt(0, tcPr)
			}
			span := 1
			if gs := child(tcPr, "gridSpan"); gs != nil {
				span, _ = strconv.Atoi(getW(gs, "val"))
			}
			if col+span > len(widths) {
				return errors.New("cell span exceeds grid")
			}
			sum := 0
			for _, w := range widths[col : col+span] {
				sum += w
			}
			e := child(tcPr, "tcW")
			if e == nil {
				e = W("tcW")
				tcPr.AddChild(e)
			}
			setW(e, "type", "dxa")
			setW(e, "w", strconv.Itoa(sum))
			col += span
		}
	}
	d.Touch(d.Main)
	return nil
}

func watermark(d *Doc, spec map[string]any) error {
	text := str(spec, "text")
	if text == "" {
		return errors.New("watermark text is required")
	}
	used := map[string]bool{}
	for _, part := range d.WordParts() {
		root, _ := d.Root(part)
		office.Iter(root, func(e *etree.Element) bool {
			if v := e.SelectAttrValue("id", ""); v != "" {
				used[v] = true
			}
			return true
		})
	}
	var headers []string
	seen := map[string]bool{}
	inherited := map[string]string{}
	sects := all(d.Body(), "sectPr")
	if len(sects) == 0 {
		return errors.New("document has no section properties")
	}
	// Accessing each reference DOCX library header resolves its inherited definition or
	// creates one, even when first/even page display is disabled.
	for _, sect := range sects {
		for _, kind := range []string{"default", "first", "even"} {
			target := inherited[kind]
			for _, ref := range office.Children(sect, nsW, "headerReference") {
				if office.AttrV(ref, nsW, "type") != kind {
					continue
				}
				var err error
				target, err = office.RelTarget(d.Pkg, d.Main, office.AttrV(ref, nsR, "id"))
				if err != nil {
					return err
				}
			}
			if target == "" {
				target = "word/header1.xml"
				for n := 2; d.Pkg.Has(target); n++ {
					target = fmt.Sprintf("word/header%d.xml", n)
				}
				x := newRoot("hdr")
				x.Root().CreateAttr("xmlns:r", nsR)
				x.Root().CreateAttr("xmlns:mc", "http://schemas.openxmlformats.org/markup-compatibility/2006")
				x.Root().CreateAttr("xmlns:w14", nsW14)
				x.Root().CreateAttr("xmlns:wp14", "http://schemas.microsoft.com/office/word/2010/wordprocessingDrawing")
				x.Root().CreateAttr("mc:Ignorable", "w14 wp14")
				paragraph := W("p")
				style := W("pStyle")
				setW(style, "val", "Header")
				pPr(paragraph).AddChild(style)
				x.Root().AddChild(paragraph)
				if err := d.AddPart(target, ctBase+"header+xml", "header", x); err != nil {
					return err
				}
				rels, err := d.Pkg.Rels(d.Main)
				if err != nil {
					return err
				}
				ref := W("headerReference")
				setW(ref, "type", kind)
				ref.CreateAttr("r:id", rels[len(rels)-1].ID)
				index := 0
				for i, token := range sect.Child {
					if e, ok := token.(*etree.Element); ok && is(e, "headerReference") {
						index = i + 1
					}
				}
				sect.InsertChildAt(index, ref)
				d.Touch(d.Main)
			}
			inherited[kind] = target
			if !seen[target] {
				seen[target] = true
				headers = append(headers, target)
			}
		}
	}
	serial := 0
	for _, part := range headers {
		root, err := d.Root(part)
		if err != nil {
			return err
		}
		var shapeID, typeID string
		for {
			serial++
			shapeID, typeID = fmt.Sprintf("DocumentWatermark%d", serial), fmt.Sprintf("_x0000_t136_%d", serial)
			if !used[shapeID] && !used[typeID] {
				break
			}
		}
		used[shapeID], used[typeID] = true, true
		pict := W("pict")
		st := pict.CreateElement("v:shapetype")
		st.CreateAttr("id", typeID)
		st.CreateAttr("coordsize", "21600,21600")
		st.CreateAttr("o:spt", "136")
		st.CreateAttr("path", "m0,10800l21600,10800e")
		st.CreateElement("v:path").CreateAttr("textpathok", "t")
		tp := st.CreateElement("v:textpath")
		tp.CreateAttr("on", "t")
		tp.CreateAttr("fitshape", "t")
		sh := pict.CreateElement("v:shape")
		sh.CreateAttr("id", shapeID)
		sh.CreateAttr("type", "#"+typeID)
		sh.CreateAttr("coordsize", "21600,21600")
		sh.CreateAttr("style", "position:absolute;width:360pt;height:72pt;rotation:315;z-index:-251654144;mso-position-horizontal:center;mso-position-horizontal-relative:margin;mso-position-vertical:center;mso-position-vertical-relative:margin")
		sh.CreateAttr("fillcolor", "#d0d0d0")
		sh.CreateAttr("stroked", "f")
		sh.CreateElement("v:path").CreateAttr("textpathok", "t")
		txt := sh.CreateElement("v:textpath")
		txt.CreateAttr("on", "t")
		txt.CreateAttr("string", text)
		txt.CreateAttr("style", "font-family:Arial;font-size:48pt")
		p := child(root, "p")
		if p == nil {
			p = W("p")
			root.AddChild(p)
		}
		r := W("r")
		r.AddChild(pict)
		p.AddChild(r)
		d.Touch(part)
	}
	return nil
}

func watermarkRemove(d *Doc, spec map[string]any) error {
	contains := str(spec, "contains")
	if contains == "" {
		return errors.New("select nonempty watermark text")
	}
	removed := 0
	for _, part := range d.StoryParts("hdr") {
		root, _ := d.Root(part)
		types := map[string]bool{}
		for _, sh := range office.All(root, nsV, "shape") {
			for _, tp := range office.All(sh, nsV, "textpath") {
				if strings.Contains(tp.SelectAttrValue("string", ""), contains) {
					types[strings.TrimPrefix(sh.SelectAttrValue("type", ""), "#")] = true
					remove(sh)
					removed++
					d.Touch(part)
					break
				}
			}
		}
		usedTypes := map[string]bool{}
		for _, sh := range office.All(root, nsV, "shape") {
			usedTypes[strings.TrimPrefix(sh.SelectAttrValue("type", ""), "#")] = true
		}
		for _, def := range office.All(root, nsV, "shapetype") {
			id := def.SelectAttrValue("id", "")
			if types[id] && !usedTypes[id] {
				remove(def)
			}
		}
	}
	if removed == 0 {
		return errors.New("no matching VML header watermark found")
	}
	return nil
}

var redactAttrs = map[string]bool{"descr": true, "title": true, "author": true, "initials": true}

func redact(d *Doc, spec map[string]any) ([]string, error) {
	var exprs []*regexp.Regexp
	matched := map[string]bool{}
	if list, ok := spec["values"].([]any); ok {
		for _, v := range list {
			s, ok := v.(string)
			if !ok || s == "" {
				return nil, errors.New("redaction requires nonempty exact strings")
			}
			exprs = append(exprs, regexp.MustCompile(regexp.QuoteMeta(s)))
			matched[s] = true
		}
	}
	if list, ok := spec["patterns"].([]any); ok {
		for _, v := range list {
			s, _ := v.(string)
			re, err := regexp.Compile(s)
			if err != nil {
				return nil, fmt.Errorf("invalid redaction pattern %q: %w", s, err)
			}
			exprs = append(exprs, re)
		}
	}
	if len(exprs) == 0 {
		return nil, errors.New("supply exact values or regex patterns")
	}
	replacement := "[REDACTED]"
	if v, ok := spec["replacement"]; ok {
		s, ok := v.(string)
		if !ok {
			return nil, errors.New("replacement must be a string")
		}
		replacement = s
	}
	preserve := truthy(spec["preserve_length"])
	if preserve && replacement == "" {
		return nil, errors.New("a nonempty replacement is required for fixed-length masking")
	}
	var rangeErr error
	ranges := func(text string) [][2]int {
		var spans [][2]int
		for _, re := range exprs {
			for _, m := range re.FindAllStringIndex(text, -1) {
				if m[0] == m[1] {
					rangeErr = errors.New("redaction patterns must not match empty text")
					continue
				}
				matched[text[m[0]:m[1]]] = true
				spans = append(spans, [2]int{m[0], m[1]})
			}
		}
		sort.Slice(spans, func(i, j int) bool {
			return spans[i][0] < spans[j][0] || (spans[i][0] == spans[j][0] && spans[i][1] < spans[j][1])
		})
		var merged [][2]int
		for _, s := range spans {
			if n := len(merged); n > 0 && s[0] < merged[n-1][1] {
				merged[n-1][1] = max(merged[n-1][1], s[1])
			} else {
				merged = append(merged, s)
			}
		}
		return merged
	}
	mask := func(size int) string {
		if !preserve {
			return replacement
		}
		r := []rune(replacement)
		out := make([]rune, size)
		for i := range out {
			out[i] = r[i%len(r)]
		}
		return string(out)
	}
	replaceNodes := func(texts []*etree.Element) bool {
		var b strings.Builder
		for _, t := range texts {
			b.WriteString(t.Text())
		}
		combined := b.String()
		rs := ranges(combined)
		for k := len(rs) - 1; k >= 0; k-- {
			start, end := rs[k][0], rs[k][1]
			offset, inserted := 0, false
			for _, t := range texts {
				text := t.Text()
				finish := offset + len(text)
				if offset < end && finish > start {
					a, bb := max(0, start-offset), min(len(text), end-offset)
					sub := ""
					if preserve {
						sub = mask(len([]rune(text[a:bb])))
					} else if !inserted {
						sub = replacement
					}
					t.SetText(text[:a] + sub + text[bb:])
					inserted = true
				}
				offset = finish
			}
		}
		return len(rs) > 0
	}
	for _, name := range d.Pkg.SortedNames() {
		if strings.Contains(name, "/embeddings/") {
			return nil, errors.New("embedded files need separate redaction before publication")
		}
		ct := d.Pkg.ContentType(name)
		if !(strings.HasSuffix(ct, "+xml") || ct == "application/xml") || strings.HasSuffix(name, ".rels") {
			continue
		}
		root, err := d.Root(name)
		if err != nil {
			return nil, err
		}
		changed := false
		processed := map[*etree.Element]bool{}
		for _, p := range all(root, "p") {
			var texts []*etree.Element
			office.Iter(p, func(e *etree.Element) bool {
				if is(e, "t") || is(e, "delText") {
					texts = append(texts, e)
				}
				return true
			})
			if replaceNodes(texts) {
				changed = true
			}
			for _, t := range texts {
				processed[t] = true
			}
		}
		office.Iter(root, func(e *etree.Element) bool {
			if !processed[e] && len(e.ChildElements()) == 0 && e.Text() != "" {
				if replaceNodes([]*etree.Element{e}) {
					changed = true
				}
			}
			for i, a := range e.Attr {
				if redactAttrs[a.Key] && a.Space != "xmlns" {
					text := a.Value
					rs := ranges(text)
					for k := len(rs) - 1; k >= 0; k-- {
						text = text[:rs[k][0]] + mask(len([]rune(text[rs[k][0]:rs[k][1]]))) + text[rs[k][1]:]
					}
					if text != a.Value {
						e.Attr[i].Value = text
						changed = true
					}
				}
			}
			return true
		})
		if changed {
			d.Touch(name)
		}
	}
	if rangeErr != nil {
		return nil, rangeErr
	}
	for _, owner := range append([]string{""}, d.Pkg.SortedNames()...) {
		rels, _ := d.Pkg.Rels(owner)
		for _, r := range rels {
			if r.TargetMode == "External" && len(ranges(r.Target)) > 0 {
				return nil, errors.New("sensitive value occurs in an external relationship; remove the link first")
			}
		}
	}
	var values []string
	for v := range matched {
		if strings.Contains(replacement, v) {
			return nil, errors.New("redaction replacement still contains a requested value")
		}
		values = append(values, v)
	}
	sort.Strings(values)
	return values, nil
}
