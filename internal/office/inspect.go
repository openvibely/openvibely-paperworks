// Package office implements read-only inspection and narrow edits of
// DOCX/XLSX/PPTX packages.
package office

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/beevik/etree"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
)

const (
	NSW   = "http://schemas.openxmlformats.org/wordprocessingml/2006/main"
	NSS   = "http://schemas.openxmlformats.org/spreadsheetml/2006/main"
	NSP   = "http://schemas.openxmlformats.org/presentationml/2006/main"
	NSA   = "http://schemas.openxmlformats.org/drawingml/2006/main"
	NSR   = "http://schemas.openxmlformats.org/officeDocument/2006/relationships"
	NSC   = "http://schemas.openxmlformats.org/drawingml/2006/chart"
	NSWP  = "http://schemas.openxmlformats.org/drawingml/2006/wordprocessingDrawing"
	NSM   = "http://schemas.openxmlformats.org/officeDocument/2006/math"
	NSV   = "urn:schemas-microsoft-com:vml"
	NSW14 = "http://schemas.microsoft.com/office/word/2010/wordml"
	NSW15 = "http://schemas.microsoft.com/office/word/2012/wordml"
	NSX14 = "http://schemas.microsoft.com/office/spreadsheetml/2009/9/main"
)

// Is reports whether e has the given namespace URI and local name.
func Is(e *etree.Element, ns, local string) bool {
	return e != nil && e.Tag == local && e.NamespaceURI() == ns
}

// Iter walks e and its descendants in document order.
func Iter(e *etree.Element, fn func(*etree.Element) bool) {
	if e == nil || !fn(e) {
		return
	}
	for _, c := range e.ChildElements() {
		Iter(c, fn)
	}
}

// All collects descendants (including e) matching ns:local.
func All(e *etree.Element, ns, local string) []*etree.Element {
	var out []*etree.Element
	Iter(e, func(x *etree.Element) bool {
		if Is(x, ns, local) {
			out = append(out, x)
		}
		return true
	})
	return out
}

// WordSections returns current body section properties in document order.
// Historical sectPr elements inside revision snapshots are not sections.
func WordSections(root *etree.Element) []*etree.Element {
	if Is(root, NSW, "document") {
		root = Child(root, NSW, "body")
	}
	if !Is(root, NSW, "body") {
		return nil
	}
	var sections []*etree.Element
	var visit func(*etree.Element)
	visit = func(container *etree.Element) {
		if container == nil {
			return
		}
		for _, block := range container.ChildElements() {
			switch {
			case Is(block, NSW, "sectPr") && container == root:
				sections = append(sections, block)
			case Is(block, NSW, "p"):
				// Read only current paragraph properties, never revision
				// snapshots or separate stories inside drawings/text boxes.
				if section := Child(Child(block, NSW, "pPr"), NSW, "sectPr"); section != nil {
					sections = append(sections, section)
				}
			case Is(block, NSW, "sdt"):
				visit(Child(block, NSW, "sdtContent"))
			case Is(block, NSW, "customXml"), Is(block, NSW, "ins"), Is(block, NSW, "moveTo"):
				visit(block)
			}
		}
	}
	visit(root)
	return sections
}

// Child returns the first direct child matching ns:local.
func Child(e *etree.Element, ns, local string) *etree.Element {
	if e == nil {
		return nil
	}
	for _, c := range e.ChildElements() {
		if Is(c, ns, local) {
			return c
		}
	}
	return nil
}

func Children(e *etree.Element, ns, local string) []*etree.Element {
	var out []*etree.Element
	if e == nil {
		return out
	}
	for _, c := range e.ChildElements() {
		if Is(c, ns, local) {
			out = append(out, c)
		}
	}
	return out
}

// Attr reads an attribute by namespace URI (empty for unqualified).
func Attr(e *etree.Element, ns, local string) (string, bool) {
	if e == nil {
		return "", false
	}
	for _, a := range e.Attr {
		if a.Key == local && a.NamespaceURI() == ns {
			return a.Value, true
		}
	}
	return "", false
}

func AttrV(e *etree.Element, ns, local string) string { v, _ := Attr(e, ns, local); return v }

func Text(e *etree.Element, ns, local string) string {
	var b strings.Builder
	for _, t := range All(e, ns, local) {
		b.WriteString(t.Text())
	}
	return b.String()
}

func attrMap(e *etree.Element) map[string]string {
	m := map[string]string{}
	for _, a := range e.Attr {
		if a.Space == "xmlns" || a.Key == "xmlns" {
			continue
		}
		m[a.Key] = a.Value
	}
	return m
}

// XML inspection follows ElementTree's expanded attribute names. Unqualified
// attributes stay unqualified; a default namespace never applies to attributes.
func reportAttrMap(e *etree.Element) map[string]string {
	m := map[string]string{}
	for _, a := range e.Attr {
		if a.Space == "xmlns" || a.Key == "xmlns" {
			continue
		}
		key := a.Key
		uri := a.NamespaceURI()
		// The xml prefix is implicitly bound, even without an xmlns:xml
		// declaration. etree does not resolve that implicit binding.
		if a.Space == "xml" {
			uri = "http://www.w3.org/XML/1998/namespace"
		}
		if uri != "" {
			key = "{" + uri + "}" + key
		}
		m[key] = a.Value
	}
	return m
}

type Report map[string]any

// LoadXMLParts parses every XML/rels part, rejecting entity declarations.
func LoadXMLParts(p *opc.Package) (map[string]*etree.Document, error) {
	roots := map[string]*etree.Document{}
	for _, part := range p.Parts {
		if strings.HasSuffix(part.Name, ".xml") || strings.HasSuffix(part.Name, ".rels") {
			doc, err := opc.ParseXML(part.Data)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", part.Name, err)
			}
			roots[part.Name] = doc
		}
	}
	return roots, nil
}

type ExtRel struct {
	Part   string `json:"part"`
	Target string `json:"target"`
	Type   string `json:"type"`
}

// PackageReport inspects an Office package without running an office application.
func PackageReport(file string) (Report, error) {
	p, err := opc.Open(file)
	if err != nil {
		return nil, err
	}
	r, err := PackageReportOf(p)
	if err != nil {
		return nil, err
	}
	r["file"] = file
	return r, nil
}

// PresentationSlideSelection reads one slide without parsing unrelated package
// XML parts. Its value has the same shape as the corresponding PackageReport
// slides entry.
func PresentationSlideSelection(file string, index int) (map[string]any, error) {
	p, err := opc.Open(file)
	if err != nil {
		return nil, err
	}
	presentation, err := p.XML("ppt/presentation.xml")
	if err != nil {
		return nil, err
	}
	presentationRels, err := p.Rels("ppt/presentation.xml")
	if err != nil {
		return nil, err
	}
	part, err := presentationSlidePart(presentation.Root(), presentationRels, index)
	if err != nil {
		return nil, err
	}
	slide, err := p.XML(part)
	if err != nil {
		return nil, err
	}
	if !Is(slide.Root(), NSP, "sld") {
		return nil, fmt.Errorf("%s: expected a presentation slide", part)
	}
	return presentationSlideEntry(part, slide.Root()), nil
}

func presentationSlideIDs(presentation *etree.Element) []string {
	var ids []string
	if list := Child(presentation, NSP, "sldIdLst"); list != nil {
		for _, slide := range Children(list, NSP, "sldId") {
			ids = append(ids, AttrV(slide, NSR, "id"))
		}
	}
	return ids
}

func presentationSlidePart(presentation *etree.Element, rels []opc.Relationship, index int) (string, error) {
	ids := presentationSlideIDs(presentation)
	if index < 0 || index >= len(ids) {
		return "", fmt.Errorf("slide index out of range")
	}
	for _, rel := range rels {
		if rel.ID != ids[index] {
			continue
		}
		if !strings.HasSuffix(rel.Type, "/slide") || rel.TargetMode == "External" {
			break
		}
		return opc.Resolve("ppt/presentation.xml", rel.Target), nil
	}
	return "", fmt.Errorf("ppt/presentation.xml: slide %d has no valid slide relationship", index)
}

func presentationSlideEntry(part string, root *etree.Element) map[string]any {
	return map[string]any{
		"part":       part,
		"attributes": reportAttrMap(root),
		"xml":        xmlString(root),
	}
}

func PackageReportOf(p *opc.Package) (Report, error) {
	roots, err := LoadXMLParts(p)
	if err != nil {
		return nil, err
	}
	report := Report{}
	issues := []string{}
	exts := []ExtRel{}
	if ct := roots["[Content_Types].xml"]; ct != nil {
		defaults, overrides := map[string]string{}, map[string]string{}
		var overrideOrder []string
		for _, e := range ct.Root().ChildElements() {
			// Reference tests expanded tags, excluding unqualified declarations.
			if e.NamespaceURI() == "" {
				continue
			}
			switch e.Tag {
			case "Default":
				defaults[e.SelectAttrValue("Extension", "")] = e.SelectAttrValue("ContentType", "")
			case "Override":
				name := strings.TrimLeft(e.SelectAttrValue("PartName", ""), "/")
				if _, exists := overrides[name]; !exists {
					overrideOrder = append(overrideOrder, name)
				}
				overrides[name] = e.SelectAttrValue("ContentType", "")
			}
		}
		for _, n := range p.Names() {
			if n == "[Content_Types].xml" {
				continue
			}
			// Match Reference name.rsplit('.', 1)[-1], including dotless names.
			ext := n[strings.LastIndex(n, ".")+1:]
			if overrides[n] == "" && defaults[ext] == "" {
				issues = append(issues, "Missing content type: "+n)
			}
		}
		for _, k := range overrideOrder {
			if !p.Has(k) {
				issues = append(issues, "Content type references missing part: "+k)
			}
		}
	}
	// Match Reference's inspection contract: validate content-type declarations
	// when present; a partial package can still be inspected without a manifest.
	parts := map[string]any{}
	for _, part := range p.Parts {
		parts[part.Name] = map[string]any{"bytes": len(part.Data), "sha256": opc.Hash(part.Data)}
	}
	report["package_parts"] = parts
	names := p.SortedNames()
	for _, name := range names {
		if !strings.HasSuffix(name, ".rels") {
			continue
		}
		src := opc.SourceOf(name)
		for _, e := range roots[name].Root().ChildElements() {
			target := e.SelectAttrValue("Target", "")
			if e.SelectAttrValue("TargetMode", "") == "External" {
				exts = append(exts, ExtRel{name, target, e.SelectAttrValue("Type", "")})
				continue
			}
			if strings.SplitN(target, "#", 2)[0] == "" {
				continue
			}
			resolved := opc.Resolve(src, target)
			if !p.Has(resolved) {
				issues = append(issues, name+": missing relationship target "+resolved)
			}
		}
	}
	report["external_relationships"] = exts
	switch {
	case roots["word/document.xml"] != nil:
		wordReport(p, roots, report, &issues)
	case roots["xl/workbook.xml"] != nil:
		if err := workbookReport(p, roots, report, &issues); err != nil {
			return nil, err
		}
	case roots["ppt/presentation.xml"] != nil:
		if err := presentationReport(p, roots, report, &issues); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("expected DOCX, XLSX or PPTX package")
	}
	report["issues"] = issues
	return report, nil
}

func sortedRoots(roots map[string]*etree.Document, prefix string) []string {
	var out []string
	for n := range roots {
		if strings.HasPrefix(n, prefix) {
			out = append(out, n)
		}
	}
	sort.Strings(out)
	return out
}

func CommentText(c *etree.Element) string {
	text := func(el *etree.Element) string {
		var b strings.Builder
		Iter(el, func(e *etree.Element) bool {
			if e.NamespaceURI() != NSW {
				return true
			}
			switch e.Tag {
			case "t":
				b.WriteString(e.Text())
			case "tab":
				b.WriteString("\t")
			case "br", "cr":
				b.WriteString("\n")
			}
			return true
		})
		return b.String()
	}
	ps := All(c, NSW, "p")
	if len(ps) == 0 {
		return text(c)
	}
	var out []string
	for _, p := range ps {
		out = append(out, text(p))
	}
	return strings.Join(out, "\n")
}

type fieldInfo struct {
	Part        string `json:"part,omitempty"`
	Instruction string `json:"instruction"`
	Incomplete  bool   `json:"incomplete,omitempty"`
}

// FieldInstructions joins complex field codes split across runs.
func FieldInstructions(root *etree.Element) []*fieldInfo {
	var fields []*fieldInfo
	type frame struct {
		f       *fieldInfo
		collect bool
	}
	var stack []*frame
	Iter(root, func(e *etree.Element) bool {
		if e.NamespaceURI() != NSW {
			return true
		}
		switch e.Tag {
		case "fldSimple":
			fields = append(fields, &fieldInfo{Instruction: AttrV(e, NSW, "instr")})
		case "fldChar":
			switch AttrV(e, NSW, "fldCharType") {
			case "begin":
				f := &fieldInfo{Incomplete: true}
				fields = append(fields, f)
				stack = append(stack, &frame{f, true})
			case "separate":
				if len(stack) > 0 {
					stack[len(stack)-1].collect = false
				}
			case "end":
				if len(stack) > 0 {
					stack[len(stack)-1].f.Incomplete = false
					stack = stack[:len(stack)-1]
				}
			}
		case "instrText":
			if len(stack) > 0 && stack[len(stack)-1].collect {
				stack[len(stack)-1].f.Instruction += e.Text()
			} else if len(stack) == 0 {
				fields = append(fields, &fieldInfo{Instruction: e.Text(), Incomplete: true})
			}
		}
		return true
	})
	for _, f := range fields {
		f.Instruction = strings.Join(strings.Fields(f.Instruction), " ")
	}
	return fields
}

var storyRe = regexp.MustCompile(`^word/(header|footer)\d+\.xml$`)
var headingRe = regexp.MustCompile(`(?i)^Heading\s*([1-9])$`)
var urlLabel = regexp.MustCompile(`^https?://`)

func wordReport(p *opc.Package, roots map[string]*etree.Document, report Report, issues *[]string) {
	wordParts := []string{}
	for _, part := range p.Parts {
		if strings.HasPrefix(part.Name, "word/") && roots[part.Name] != nil {
			wordParts = append(wordParts, part.Name)
		}
	}
	ins, del, eq := 0, 0, 0
	for _, n := range wordParts {
		r := roots[n].Root()
		ins += len(All(r, NSW, "ins"))
		del += len(All(r, NSW, "del"))
		eq += len(All(r, NSM, "oMath"))
	}
	report["tracked_insertions"], report["tracked_deletions"], report["equations"] = ins, del, eq
	var comments []*etree.Element
	if c := roots["word/comments.xml"]; c != nil {
		comments = Children(c.Root(), NSW, "comment")
	}
	report["comments"] = len(comments)
	sections, fields, controls, notes := []any{}, []any{}, []any{}, []any{}
	for _, name := range wordParts {
		root := roots[name].Root()
		for _, s := range WordSections(root) {
			m := map[string]any{}
			for _, c := range s.ChildElements() {
				if c.NamespaceURI() == NSW && (c.Tag == "pgSz" || c.Tag == "pgMar" || c.Tag == "cols") {
					m[c.Tag] = reportAttrMap(c)
				}
			}
			sections = append(sections, m)
		}
		for _, f := range FieldInstructions(root) {
			f.Part = name
			fields = append(fields, f)
		}
		for _, sdt := range All(root, NSW, "sdt") {
			pr := Child(sdt, NSW, "sdtPr")
			var tag, alias any
			if t := Child(pr, NSW, "tag"); t != nil {
				tag = AttrV(t, NSW, "val")
			}
			if a := Child(pr, NSW, "alias"); a != nil {
				alias = AttrV(a, NSW, "val")
			}
			controls = append(controls, map[string]any{"part": name, "tag": tag, "alias": alias, "text": Text(sdt, NSW, "t")})
		}
		for _, kind := range []string{"footnote", "endnote"} {
			for _, e := range All(root, NSW, kind) {
				if _, typed := Attr(e, NSW, "type"); typed {
					continue
				}
				notes = append(notes, map[string]any{"part": name, "kind": kind, "id": AttrV(e, NSW, "id"), "text": Text(e, NSW, "t")})
			}
		}
	}
	for _, name := range wordParts {
		root := roots[name].Root()
		Iter(root, func(e *etree.Element) bool {
			if e.Tag == "docPr" && e.SelectAttrValue("descr", "") == "" && e.SelectAttrValue("title", "") == "" {
				*issues = append(*issues, "Image without alternative text: "+e.SelectAttrValue("name", "unnamed"))
			}
			return true
		})
		for _, tbl := range All(root, NSW, "tbl") {
			header := false
			for _, tr := range Children(tbl, NSW, "tr") {
				if Child(Child(tr, NSW, "trPr"), NSW, "tblHeader") != nil {
					header = true
				}
			}
			if !header {
				*issues = append(*issues, "Table without designated header row; review whether appropriate")
			}
		}
	}
	report["sections"], report["fields"], report["controls"], report["notes"] = sections, fields, controls, notes
	details := []map[string]any{}
	for _, c := range comments {
		details = append(details, map[string]any{"id": attrOrNilNS(c, NSW, "id"), "author": attrOrNilNS(c, NSW, "author"), "text": CommentText(c)})
	}
	report["comment_details"] = details
	wordDetails(roots, report, issues, comments, wordParts)
}

func wordDetails(roots map[string]*etree.Document, report Report, issues *[]string, comments []*etree.Element, wordParts []string) {
	styles := map[string]*etree.Element{}
	if s := roots["word/styles.xml"]; s != nil {
		for _, st := range Children(s.Root(), NSW, "style") {
			styles[AttrV(st, NSW, "styleId")] = st
		}
	}
	images, watermarks, direct, stories, styleWarnings := []any{}, []any{}, []any{}, []any{}, []any{}
	fontUsage := map[string]int{}
	headings := []any{}
	noteIDs := map[string]map[string]bool{}
	for _, kind := range []string{"footnote", "endnote"} {
		noteIDs[kind] = map[string]bool{}
		counts := map[string]int{}
		if d := roots["word/"+kind+"s.xml"]; d != nil {
			for _, e := range Children(d.Root(), NSW, kind) {
				id := AttrV(e, NSW, "id")
				counts[id]++
				if _, typed := Attr(e, NSW, "type"); !typed {
					noteIDs[kind][id] = true
				}
			}
		}
		for _, id := range sortedKeys(counts) {
			if counts[id] > 1 {
				*issues = append(*issues, "Duplicate "+kind+" ID "+id)
			}
		}
	}
	refs := map[string]map[string]bool{"footnote": {}, "endnote": {}}
	for _, name := range wordParts {
		root := roots[name].Root()
		lastLevel := 0
		story := name == "word/document.xml" || storyRe.MatchString(name) || name == "word/footnotes.xml" || name == "word/endnotes.xml" || name == "word/comments.xml"
		if story {
			entries := []any{}
			Iter(root, func(e *etree.Element) bool {
				if e.NamespaceURI() != NSW {
					return true
				}
				switch e.Tag {
				case "t":
					entries = append(entries, map[string]string{"kind": "text", "text": e.Text()})
				case "delText":
					entries = append(entries, map[string]string{"kind": "deleted", "text": e.Text()})
				case "instrText":
					entries = append(entries, map[string]string{"kind": "field", "text": e.Text()})
				case "fldSimple":
					entries = append(entries, map[string]string{"kind": "field", "text": AttrV(e, NSW, "instr")})
				}
				return true
			})
			stories = append(stories, map[string]any{"part": name, "content": entries})
		}
		for number, p := range All(root, NSW, "p") {
			text := Text(p, NSW, "t")
			props := Child(p, NSW, "pPr")
			outline := Child(props, NSW, "outlineLvl")
			styleID := ""
			if sr := Child(props, NSW, "pStyle"); sr != nil {
				styleID = AttrV(sr, NSW, "val")
			}
			cur, seen := styleID, map[string]bool{}
			for outline == nil && styles[cur] != nil && !seen[cur] {
				seen[cur] = true
				st := styles[cur]
				outline = Child(Child(st, NSW, "pPr"), NSW, "outlineLvl")
				cur = AttrV(Child(st, NSW, "basedOn"), NSW, "val")
			}
			level := 0
			if outline != nil {
				level, _ = strconv.Atoi(AttrV(outline, NSW, "val"))
				level++
			} else if m := headingRe.FindStringSubmatch(styleID); m != nil {
				level, _ = strconv.Atoi(m[1])
			}
			if level >= 1 && level <= 9 {
				headings = append(headings, map[string]any{"part": name, "paragraph": number, "style": nilIfEmpty(styleID), "level": level, "text": text})
				if level > lastLevel+1 {
					*issues = append(*issues, fmt.Sprintf("%s: heading level jumps from %d to %d: %s", name, lastLevel, level, text))
				}
				lastLevel = level
			}
			var formats []map[string]string
			for _, r := range Children(p, NSW, "r") {
				for _, rpr := range Children(r, NSW, "rPr") {
					for _, e := range rpr.ChildElements() {
						if e.Tag == "rStyle" {
							continue
						}
						m := reportAttrMap(e)
						m["property"] = e.Tag
						formats = append(formats, m)
					}
				}
			}
			if len(formats) > 0 {
				direct = append(direct, map[string]any{"part": name, "paragraph": number, "style": nilIfEmpty(styleID), "text": text, "properties": formats})
			}
			if story {
				var overrides []string
				if props != nil {
					for _, e := range props.ChildElements() {
						if e.Tag == "spacing" || e.Tag == "ind" || e.Tag == "jc" {
							overrides = append(overrides, e.Tag)
						}
					}
				}
				if len(overrides) > 0 {
					styleWarnings = append(styleWarnings, map[string]any{"part": name, "paragraph": number, "reason": "paragraph overrides", "properties": overrides})
				}
				t := strings.TrimSpace(text)
				if level == 0 && t != "" && len([]rune(t)) <= 100 && !strings.HasSuffix(strings.TrimRight(text, " \t"), ".") && !strings.HasSuffix(t, ";") && !strings.HasSuffix(t, ":") {
					bold := false
					for _, r := range Children(p, NSW, "r") {
						if b := Child(Child(r, NSW, "rPr"), NSW, "b"); b != nil {
							v, ok := Attr(b, NSW, "val")
							if !ok || (v != "0" && v != "false" && v != "off") {
								bold = true
							}
						}
					}
					if bold {
						styleWarnings = append(styleWarnings, map[string]any{"part": name, "paragraph": number, "reason": "possible heading without outline level", "text": text})
					}
				}
				for _, r := range Children(p, NSW, "r") {
					if f := Child(Child(r, NSW, "rPr"), NSW, "rFonts"); f != nil {
						seenF := map[string]bool{}
						for _, a := range f.Attr {
							if !seenF[a.Value] {
								seenF[a.Value] = true
								fontUsage[a.Value]++
							}
						}
					}
				}
			}
			for _, link := range All(p, NSW, "hyperlink") {
				label := strings.TrimSpace(Text(link, NSW, "t"))
				l := strings.ToLower(label)
				if label == "" || l == "here" || l == "click here" || l == "link" || l == "read more" || urlLabel.MatchString(label) {
					*issues = append(*issues, fmt.Sprintf("%s: hyperlink needs a descriptive label: %s", name, label))
				}
			}
		}
		for kind := range noteIDs {
			for _, ref := range All(root, NSW, kind+"Reference") {
				id := AttrV(ref, NSW, "id")
				refs[kind][id] = true
				if !noteIDs[kind][id] {
					*issues = append(*issues, fmt.Sprintf("%s: missing %s %s", name, kind, id))
				}
			}
		}
		relsDoc := roots[opc.RelsName(name)]
		rels := map[string]string{}
		if relsDoc != nil {
			for _, e := range relsDoc.Root().ChildElements() {
				rels[e.SelectAttrValue("Id", "")] = e.SelectAttrValue("Target", "")
			}
		}
		for _, kind := range []string{"inline", "anchor"} {
			for _, d := range All(root, NSWP, kind) {
				props := Child(d, NSWP, "docPr")
				size := Child(d, NSWP, "extent")
				var targets []string
				for _, b := range All(d, NSA, "blip") {
					targets = append(targets, rels[AttrV(b, NSR, "embed")])
				}
				pos := map[string]string{}
				for _, ax := range []string{"positionH", "positionV"} {
					if e := Child(d, NSWP, ax); e != nil {
						pos[ax] = xmlString(e)
					}
				}
				ext := map[string]string{}
				if size != nil {
					ext = reportAttrMap(size)
				}
				images = append(images, map[string]any{"part": name, "placement": kind, "name": attrOrNil(props, "name"), "description": attrOrNil(props, "descr"), "extent": ext, "targets": targets, "position": pos})
			}
		}
		for _, shape := range All(root, NSV, "shape") {
			for _, tp := range All(shape, NSV, "textpath") {
				watermarks = append(watermarks, map[string]any{"part": name, "id": attrOrNil(shape, "id"), "text": attrOrNil(tp, "string"), "style": attrOrNil(shape, "style")})
			}
		}
	}
	for _, kind := range []string{"footnote", "endnote"} {
		for _, id := range sortedKeys(noteIDs[kind]) {
			if !refs[kind][id] {
				*issues = append(*issues, "Unreferenced "+kind+" "+id)
			}
		}
	}
	details := report["comment_details"].([]map[string]any)
	var ext *etree.Element
	if d := roots["word/commentsExtended.xml"]; d != nil {
		ext = d.Root()
	}
	for i, c := range comments {
		det := details[i]
		det["date"] = attrOrNilNS(c, NSW, "date")
		det["initials"] = attrOrNilNS(c, NSW, "initials")
		ps := All(c, NSW, "p")
		paraID := ""
		if len(ps) > 0 {
			paraID = AttrV(ps[len(ps)-1], NSW14, "paraId")
		}
		resolved, found := false, false
		if ext != nil && paraID != "" {
			for _, e := range ext.ChildElements() {
				if AttrV(e, NSW15, "paraId") == paraID {
					v := AttrV(e, NSW15, "done")
					resolved, found = v == "1" || v == "true", true
					break
				}
			}
		}
		if !found {
			v := AttrV(c, NSW, "done")
			resolved = v == "1" || v == "true"
		}
		det["resolved"] = resolved
		anchored := []any{}
		for _, name := range wordParts {
			if name != "word/document.xml" && !storyRe.MatchString(name) {
				continue
			}
			active := false
			var b strings.Builder
			Iter(roots[name].Root(), func(e *etree.Element) bool {
				if e.NamespaceURI() != NSW {
					return true
				}
				switch {
				case e.Tag == "commentRangeStart" && AttrV(e, NSW, "id") == det["id"]:
					active = true
				case e.Tag == "commentRangeEnd" && AttrV(e, NSW, "id") == det["id"]:
					active = false
				case active && (e.Tag == "t" || e.Tag == "delText"):
					b.WriteString(e.Text())
				}
				return true
			})
			if b.Len() > 0 {
				anchored = append(anchored, map[string]string{"part": name, "text": b.String()})
			}
		}
		det["anchored_text"] = anchored
	}
	sectionLinks := []any{}
	docrels := map[string]string{}
	if d := roots["word/_rels/document.xml.rels"]; d != nil {
		for _, e := range d.Root().ChildElements() {
			docrels[e.SelectAttrValue("Id", "")] = e.SelectAttrValue("Target", "")
		}
	}
	inherited := map[string]string{}
	if doc := roots["word/document.xml"]; doc != nil {
		for i, s := range WordSections(doc.Root()) {
			declared := map[string]string{}
			for _, kind := range []string{"header", "footer"} {
				for _, ref := range Children(s, NSW, kind+"Reference") {
					t := AttrV(ref, NSW, "type")
					if t == "" {
						t = "default"
					}
					declared[kind+":"+t] = docrels[AttrV(ref, NSR, "id")]
				}
			}
			links := map[string]any{}
			for _, kind := range []string{"header", "footer"} {
				for _, v := range []string{"default", "first", "even"} {
					k := kind + ":" + v
					target, isDecl := declared[k]
					inh, isInh := inherited[k]
					var tv any
					if isDecl {
						tv = target
					} else if isInh {
						tv = inh
					}
					links[k] = map[string]any{"target": tv, "inherited": !isDecl && isInh}
				}
			}
			for k, v := range declared {
				inherited[k] = v
			}
			sectionLinks = append(sectionLinks, map[string]any{"section": i, "links": links})
		}
	}
	sectionDetails := []any{}
	for _, name := range wordParts {
		for _, s := range WordSections(roots[name].Root()) {
			props := []any{}
			for _, e := range s.ChildElements() {
				props = append(props, map[string]any{e.Tag: reportAttrMap(e)})
			}
			sectionDetails = append(sectionDetails, map[string]any{"part": name, "properties": props})
		}
	}
	report["images"], report["watermarks"], report["direct_formatting"] = images, watermarks, direct
	report["stories"], report["style_warnings"], report["font_usage"] = stories, styleWarnings, fontUsage
	report["headings"], report["section_links"], report["section_details"] = headings, sectionLinks, sectionDetails
}

func nilIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func attrOrNil(e *etree.Element, key string) any {
	if e == nil {
		return nil
	}
	if a := e.SelectAttr(key); a != nil {
		return a.Value
	}
	return nil
}

func attrOrNilNS(e *etree.Element, ns, key string) any {
	if v, ok := Attr(e, ns, key); ok {
		return v
	}
	return nil
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// WorkbookSheets maps worksheet part names to sheet names in workbook order.
func WorkbookSheets(p *opc.Package) (order []string, partToName map[string]string, nameToPart map[string]string, err error) {
	wb, err := p.XML("xl/workbook.xml")
	if err != nil {
		return nil, nil, nil, err
	}
	rels, err := p.Rels("xl/workbook.xml")
	if err != nil {
		return nil, nil, nil, err
	}
	targets := map[string]string{}
	for _, r := range rels {
		if r.TargetMode != "External" {
			targets[r.ID] = opc.Resolve("xl/workbook.xml", r.Target)
		}
	}
	partToName, nameToPart = map[string]string{}, map[string]string{}
	for _, s := range All(wb.Root(), NSS, "sheet") {
		name := s.SelectAttrValue("name", "")
		order = append(order, name)
		if t := targets[AttrV(s, NSR, "id")]; t != "" {
			partToName[t] = name
			nameToPart[name] = t
		}
	}
	return order, partToName, nameToPart, nil
}

type Formula struct {
	Part       string            `json:"part"`
	Sheet      string            `json:"sheet"`
	Cell       string            `json:"cell"`
	Formula    *string           `json:"formula"`
	Attributes map[string]string `json:"attributes"`
	Cached     *string           `json:"cached"`
	Type       string            `json:"-"`
}

func workbookReport(p *opc.Package, roots map[string]*etree.Document, report Report, issues *[]string) error {
	order, partToName, _, err := WorkbookSheets(p)
	if err != nil {
		return err
	}
	if order == nil {
		order = []string{}
	}
	report["sheets"] = order
	calc := map[string]string{}
	if c := Child(roots["xl/workbook.xml"].Root(), NSS, "calcPr"); c != nil {
		calc = reportAttrMap(c)
	}
	report["calculation_settings"] = calc
	formulas, errs := []Formula{}, []any{}
	for _, name := range sortedRoots(roots, "xl/worksheets/") {
		if strings.HasSuffix(name, ".rels") {
			continue
		}
		sheet := partToName[name]
		if sheet == "" {
			sheet = name
		}
		for _, c := range All(roots[name].Root(), NSS, "c") {
			f, v := Child(c, NSS, "f"), Child(c, NSS, "v")
			ref := c.SelectAttrValue("r", "?")
			if f != nil {
				fm := Formula{Part: name, Sheet: sheet, Cell: ref, Attributes: reportAttrMap(f), Type: c.SelectAttrValue("t", "")}
				if f.Text() != "" || len(f.Child) > 0 {
					t := f.Text()
					fm.Formula = &t
				}
				if v != nil && v.Text() != "" {
					t := v.Text()
					fm.Cached = &t
				}
				formulas = append(formulas, fm)
				if v == nil || (v.Text() == "" && c.SelectAttrValue("t", "") != "str") {
					*issues = append(*issues, name+":"+ref+" has no formula cache")
				}
			}
			if c.SelectAttrValue("t", "") == "e" {
				var ev any
				if v != nil {
					ev = v.Text()
				}
				errs = append(errs, map[string]any{"part": name, "cell": ref, "error": ev})
			}
		}
	}
	report["formulas"] = formulas
	report["errors"] = errs
	report["cache_warning"] = "Existing cached values do not prove recent recalculation."
	features := map[string][]any{}
	for _, part := range sortedRoots(roots, "xl/") {
		Iter(roots[part].Root(), func(e *etree.Element) bool {
			if Is(e, NSX14, "sparklineGroup") {
				features["sparklineGroup"] = append(features["sparklineGroup"], map[string]any{"part": part, "xml": xmlString(e)})
			}
			if e.NamespaceURI() == NSS {
				switch e.Tag {
				case "definedName", "table", "pivotTableDefinition", "dataValidation", "conditionalFormatting", "autoFilter", "sheetProtection":
					features[e.Tag] = append(features[e.Tag], map[string]any{"part": part, "attributes": reportAttrMap(e), "xml": xmlString(e)})
				}
			}
			return true
		})
	}
	report["native_features"] = features
	return nil
}

func presentationReport(p *opc.Package, roots map[string]*etree.Document, report Report, issues *[]string) error {
	pres := roots["ppt/presentation.xml"].Root()
	masters := map[string]bool{}
	if d := roots["ppt/_rels/presentation.xml.rels"]; d != nil {
		for _, e := range d.Root().ChildElements() {
			if strings.HasSuffix(e.SelectAttrValue("Type", ""), "/notesMaster") && e.SelectAttrValue("TargetMode", "") != "External" {
				masters[e.SelectAttrValue("Id", "")] = true
			}
		}
	}
	var refs []string
	for _, l := range Children(pres, NSP, "notesMasterIdLst") {
		for _, e := range Children(l, NSP, "notesMasterId") {
			refs = append(refs, AttrV(e, NSR, "id"))
		}
	}
	seen := map[string]bool{}
	ok := len(refs) == len(masters)
	for _, r := range refs {
		if seen[r] || !masters[r] {
			ok = false
		}
		seen[r] = true
	}
	if !ok {
		*issues = append(*issues, "Invalid or missing presentation notes-master reference; run repair-notes on a new output file")
	}
	hasNotes, hasNotesMaster := false, false
	for n := range roots {
		if strings.HasPrefix(n, "ppt/notesMasters/") && strings.HasSuffix(n, ".xml") {
			hasNotesMaster = true
		}
		if strings.HasPrefix(n, "ppt/notesSlides/notesSlide") && strings.HasSuffix(n, ".xml") {
			hasNotes = true
		}
	}
	if hasNotes && !hasNotesMaster {
		*issues = append(*issues, "Notes slides exist without a notes master part")
	}
	size := Child(pres, NSP, "sldSz")
	if size == nil {
		return fmt.Errorf("presentation has no slide size")
	}
	width, err := coordinateInteger(size.SelectAttrValue("cx", ""))
	if err != nil {
		return err
	}
	height, err := coordinateInteger(size.SelectAttrValue("cy", ""))
	if err != nil {
		return err
	}
	n := 0
	if l := Child(pres, NSP, "sldIdLst"); l != nil {
		n = len(l.ChildElements())
	}
	var slideRels []opc.Relationship
	if relDoc := roots["ppt/_rels/presentation.xml.rels"]; relDoc != nil {
		for _, rel := range relDoc.Root().ChildElements() {
			slideRels = append(slideRels, opc.Relationship{
				ID: rel.SelectAttrValue("Id", ""), Type: rel.SelectAttrValue("Type", ""),
				Target: rel.SelectAttrValue("Target", ""), TargetMode: rel.SelectAttrValue("TargetMode", ""),
			})
		}
	}
	slideIDs := presentationSlideIDs(pres)
	slides := make([]any, 0, len(slideIDs))
	for index := range slideIDs {
		part, err := presentationSlidePart(pres, slideRels, index)
		if err != nil {
			return err
		}
		doc := roots[part]
		if doc == nil {
			doc, err = p.XML(part)
			if err != nil {
				return err
			}
		}
		if !Is(doc.Root(), NSP, "sld") {
			return fmt.Errorf("%s: expected a presentation slide", part)
		}
		slides = append(slides, presentationSlideEntry(part, doc.Root()))
	}
	report["slides"] = slides
	report["slide_count"] = n
	fonts := []any{}
	// Preserve archive order, as the reference report does for font definitions.
	for _, name := range p.Names() {
		if !strings.HasPrefix(name, "ppt/") || roots[name] == nil {
			continue
		}
		Iter(roots[name].Root(), func(e *etree.Element) bool {
			if e.NamespaceURI() == NSA && (e.Tag == "latin" || e.Tag == "ea" || e.Tag == "cs" || e.Tag == "font") {
				if tf := e.SelectAttrValue("typeface", ""); tf != "" {
					fonts = append(fonts, map[string]string{"part": name, "script": e.Tag, "typeface": tf})
				}
			}
			return true
		})
	}
	report["font_definitions"] = fonts
	for _, name := range sortedRoots(roots, "ppt/slides/slide") {
		if !strings.HasSuffix(name, ".xml") {
			continue
		}
		messages, err := slideBoundsIssuesChecked(roots[name].Root(), width, height)
		if err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		for _, msg := range messages {
			*issues = append(*issues, name+": "+msg)
		}
	}
	report["geometry_warning"] = "Explicit shape/group transforms and rotation checked; inherited geometry, text overflow, shadows and overlaps require visual inspection."
	return nil
}

type pt struct{ x, y float64 }

func rotateShape(points []pt, xfrm *etree.Element, left, top, w, h float64) []pt {
	rot, _ := strconv.ParseFloat(xfrm.SelectAttrValue("rot", "0"), 64)
	ang := rot / 60000 * math.Pi / 180
	cs, sn := math.Cos(ang), math.Sin(ang)
	fx, fy := 1.0, 1.0
	if v := xfrm.SelectAttrValue("flipH", ""); v == "1" || v == "true" {
		fx = -1
	}
	if v := xfrm.SelectAttrValue("flipV", ""); v == "1" || v == "true" {
		fy = -1
	}
	cx, cy := left+w/2, top+h/2
	out := make([]pt, len(points))
	for i, p := range points {
		dx, dy := (p.x-cx)*fx, (p.y-cy)*fy
		out[i] = pt{cx + dx*cs - dy*sn, cy + dx*sn + dy*cs}
	}
	return out
}

func num(e *etree.Element, k string) float64 {
	v, _ := strconv.ParseFloat(e.SelectAttrValue(k, "0"), 64)
	return v
}

// SlideBoundsIssues applies explicit transforms (including nested groups) and
// reports objects whose corners leave the slide.
func SlideBoundsIssues(root *etree.Element, width, height float64) []string {
	out, err := slideBoundsIssuesChecked(root, width, height)
	if err != nil {
		return []string{err.Error()}
	}
	return out
}

func slideBoundsIssuesChecked(root *etree.Element, width, height float64) ([]string, error) {
	var acquisitionErr error
	num := func(e *etree.Element, key string) float64 {
		v, err := coordinateInteger(e.SelectAttrValue(key, ""))
		if err != nil && acquisitionErr == nil {
			acquisitionErr = err
		}
		return v
	}
	rotateShape := func(points []pt, x *etree.Element, l, t, w, h float64) []pt {
		rotation, err := coordinateInteger(x.SelectAttrValue("rot", "0"))
		if err != nil && acquisitionErr == nil {
			acquisitionErr = err
		}
		copy := x.Copy()
		copy.RemoveAttr("rot")
		copy.CreateAttr("rot", strconv.FormatFloat(rotation, 'g', -1, 64))
		return rotateShape(points, copy, l, t, w, h)
	}
	var out []string
	Iter(root, func(x *etree.Element) bool {
		if x.Tag != "xfrm" || (x.NamespaceURI() != NSA && x.NamespaceURI() != NSP) {
			return true
		}
		if Child(x, NSA, "chOff") != nil {
			return true
		}
		off, ext := Child(x, NSA, "off"), Child(x, NSA, "ext")
		if off == nil || ext == nil {
			return true
		}
		l, t, w, h := num(off, "x"), num(off, "y"), num(ext, "cx"), num(ext, "cy")
		corners := rotateShape([]pt{{l, t}, {l + w, t}, {l + w, t + h}, {l, t + h}}, x, l, t, w, h)
		for a := x.Parent(); a != nil; a = a.Parent() {
			if !Is(a, NSP, "grpSp") {
				continue
			}
			tr := Child(Child(a, NSP, "grpSpPr"), NSA, "xfrm")
			if tr == nil {
				corners = nil
				break
			}
			go_, ge, co, ce := Child(tr, NSA, "off"), Child(tr, NSA, "ext"), Child(tr, NSA, "chOff"), Child(tr, NSA, "chExt")
			if go_ == nil || ge == nil || co == nil || ce == nil {
				corners = nil
				break
			}
			gx, gy, gw, gh := num(go_, "x"), num(go_, "y"), num(ge, "cx"), num(ge, "cy")
			cw, ch := num(ce, "cx"), num(ce, "cy")
			if cw <= 0 || ch <= 0 {
				corners = nil
				break
			}
			ox, oy := num(co, "x"), num(co, "y")
			for i, c := range corners {
				corners[i] = pt{gx + (c.x-ox)*gw/cw, gy + (c.y-oy)*gh/ch}
			}
			corners = rotateShape(corners, tr, gx, gy, gw, gh)
		}
		for _, c := range corners {
			if c.x < -1 || c.y < -1 || c.x > width+1 || c.y > height+1 {
				out = append(out, "object exceeds slide bounds")
				break
			}
		}
		return true
	})
	return out, acquisitionErr
}
