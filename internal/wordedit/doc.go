// Package wordedit applies narrow, structure-preserving edits to DOCX packages.
package wordedit

import (
	"errors"
	"fmt"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/beevik/etree"
	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
)

const (
	nsW   = office.NSW
	nsM   = office.NSM
	nsR   = office.NSR
	nsV   = office.NSV
	nsO   = "urn:schemas-microsoft-com:office:office"
	nsW14 = office.NSW14
	nsW15 = office.NSW15
	nsXML = "http://www.w3.org/XML/1998/namespace"

	relBase = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/"
	ctBase  = "application/vnd.openxmlformats-officedocument.wordprocessingml."
)

var canonical = map[string]string{"w": nsW, "m": nsM, "r": nsR, "v": nsV, "o": nsO, "w14": nsW14, "w15": nsW15}

// Doc is an opened DOCX whose XML parts are parsed on demand and written back
// only when modified, so untouched parts keep their original bytes.
type Doc struct {
	Pkg       *opc.Package
	Main      string
	docs      map[string]*etree.Document
	dirty     map[string]bool
	now       time.Time
	addedPart bool
}

func Open(raw []byte) (*Doc, error) {
	p, err := opc.Read(raw)
	if err != nil {
		return nil, err
	}
	d := &Doc{Pkg: p, docs: map[string]*etree.Document{}, dirty: map[string]bool{}, now: time.Now().UTC()}
	rels, err := p.Rels("")
	if err != nil {
		return nil, err
	}
	for _, r := range rels {
		if strings.HasSuffix(r.Type, "/officeDocument") {
			d.Main = opc.Resolve("", r.Target)
		}
	}
	if d.Main == "" || !p.Has(d.Main) {
		return nil, errors.New("not a Word document: missing main document part")
	}
	root, err := d.Root(d.Main)
	if err != nil {
		return nil, err
	}
	if !office.Is(root, nsW, "document") || d.Body() == nil {
		return nil, errors.New("not a Word document: main part lacks w:document/w:body")
	}
	return d, nil
}

func (d *Doc) XML(part string) (*etree.Document, error) {
	if x := d.docs[part]; x != nil {
		return x, nil
	}
	x, err := d.Pkg.XML(part)
	if err != nil {
		return nil, err
	}
	d.docs[part] = x
	return x, nil
}

func (d *Doc) Root(part string) (*etree.Element, error) {
	x, err := d.XML(part)
	if err != nil {
		return nil, err
	}
	return x.Root(), nil
}

func (d *Doc) Touch(part string) { d.dirty[part] = true }

func (d *Doc) MainRoot() *etree.Element { r, _ := d.Root(d.Main); return r }

func (d *Doc) Body() *etree.Element { return office.Child(d.MainRoot(), nsW, "body") }

// WordParts lists parsed XML parts with WordprocessingML roots (document,
// headers, footers, notes, comments, settings, styles, numbering…).
func (d *Doc) WordParts() []string {
	var out []string
	for _, name := range d.Pkg.SortedNames() {
		if !strings.HasSuffix(name, ".xml") || strings.HasPrefix(name, "[") || strings.Contains(name, "_rels/") {
			continue
		}
		root, err := d.Root(name)
		if err != nil || root.NamespaceURI() != nsW {
			continue
		}
		out = append(out, name)
	}
	return out
}

// StoryParts are parts holding document text: body, headers and footers.
func (d *Doc) StoryParts(kinds ...string) []string {
	var out []string
	for _, name := range d.WordParts() {
		root, _ := d.Root(name)
		for _, k := range kinds {
			if root.Tag == k {
				out = append(out, name)
			}
		}
	}
	return out
}

// markerStoryParts follows the package relationship graph in depth-first order.
// Marker factories allocate IDs as they visit parts; alphabetical ZIP names do
// not preserve the order used by reference DOCX library's package.parts iterator.
func (d *Doc) markerStoryParts() ([]string, error) {
	var parts []string
	seen := map[string]bool{}
	var visit func(string) error
	visit = func(owner string) error {
		rels, err := d.Pkg.Rels(owner)
		if err != nil {
			return err
		}
		for _, rel := range rels {
			if rel.TargetMode == "External" {
				continue
			}
			name := opc.Resolve(owner, rel.Target)
			if seen[name] {
				continue
			}
			seen[name] = true
			if !d.Pkg.Has(name) {
				return fmt.Errorf("missing relationship target %s", name)
			}
			contentType := d.Pkg.ContentType(name)
			storyType := contentType == ctBase+"document.main+xml" || contentType == ctBase+"header+xml" || contentType == ctBase+"footer+xml"
			if storyType && rel.Type != relBase+"image" {
				root, err := d.Root(name)
				if err != nil {
					return err
				}
				if root.NamespaceURI() == nsW && (root.Tag == "document" || root.Tag == "hdr" || root.Tag == "ftr") {
					parts = append(parts, name)
				}
			}
			if err := visit(name); err != nil {
				return err
			}
		}
		return nil
	}
	if err := visit(""); err != nil {
		return nil, err
	}
	return parts, nil
}

// Related returns the single part related from owner with the given type suffix.
func (d *Doc) Related(owner, kind string) (string, error) {
	rels, err := d.Pkg.Rels(owner)
	if err != nil {
		return "", err
	}
	found := ""
	for _, r := range rels {
		if r.Type == relBase+kind || strings.HasSuffix(r.Type, "/"+kind) {
			if r.TargetMode == "External" {
				return "", fmt.Errorf("external %s relationship is unsupported", kind)
			}
			if found != "" {
				return "", fmt.Errorf("ambiguous %s relationships", kind)
			}
			found = opc.Resolve(owner, r.Target)
		}
	}
	return found, nil
}

// Settings returns the settings root, creating the part when missing.
func (d *Doc) Settings() (*etree.Element, error) {
	part, err := d.Related(d.Main, "settings")
	if err != nil {
		return nil, err
	}
	if part == "" {
		part = "word/settings.xml"
		if err := d.AddPart(part, ctBase+"settings+xml", "settings", newRoot("settings")); err != nil {
			return nil, err
		}
	}
	d.Touch(part)
	return d.Root(part)
}

func newRoot(local string) *etree.Document {
	x := etree.NewDocument()
	x.CreateProcInst("xml", `version="1.0" encoding="UTF-8" standalone="yes"`)
	r := x.CreateElement("w:" + local)
	r.CreateAttr("xmlns:w", nsW)
	return x
}

// AddPart registers a new XML part related from the main document.
func (d *Doc) AddPart(part, contentType, relKind string, x *etree.Document) error {
	if d.Pkg.Has(part) {
		return fmt.Errorf("part already exists: %s", part)
	}
	d.docs[part] = x
	d.dirty[part] = true
	d.Pkg.Set(part, []byte{})
	d.addedPart = true
	if err := d.Pkg.EnsureContentType(part, contentType); err != nil {
		return err
	}
	typ := relBase + relKind
	if strings.HasPrefix(relKind, "http") {
		typ = relKind
	}
	target, _ := relPath(d.Main, part)
	_, err := d.Pkg.AddRel(d.Main, typ, target, false)
	return err
}

func relPath(from, to string) (string, error) {
	dir := path.Dir(from)
	if strings.HasPrefix(to, dir+"/") {
		return strings.TrimPrefix(to, dir+"/"), nil
	}
	return "/" + to, nil
}

// Bytes serializes modified parts, declares canonical prefixes used by new
// markup, and reparses the whole package before returning it.
func (d *Doc) Bytes() ([]byte, error) {
	names := make([]string, 0, len(d.dirty))
	for n := range d.dirty {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		x := d.docs[n]
		if x == nil {
			continue
		}
		declare(x.Root())
		if err := d.Pkg.SetXML(n, x); err != nil {
			return nil, err
		}
	}
	if d.addedPart {
		if err := d.orderAddedParts(); err != nil {
			return nil, err
		}
	}
	data, err := d.Pkg.Bytes()
	if err != nil {
		return nil, err
	}
	if _, err := Open(data); err != nil {
		return nil, fmt.Errorf("edited package failed to reopen: %w", err)
	}
	return data, nil
}

func declare(root *etree.Element) {
	need := map[string]bool{}
	office.Iter(root, func(e *etree.Element) bool {
		if e.Space != "" && e.NamespaceURI() == "" {
			need[e.Space] = true
		}
		for _, a := range e.Attr {
			if a.Space != "" && a.Space != "xmlns" && a.Space != "xml" && a.NamespaceURI() == "" {
				need[a.Space] = true
			}
		}
		return true
	})
	for p := range need {
		if uri, ok := canonical[p]; ok {
			root.CreateAttr("xmlns:"+p, uri)
		}
	}
}

// --- element helpers -------------------------------------------------------

func W(local string) *etree.Element { return etree.NewElement("w:" + local) }

func WT(local, text string) *etree.Element {
	e := W(local)
	e.SetText(text)
	e.CreateAttr("xml:space", "preserve")
	return e
}

func setW(e *etree.Element, key, val string) {
	for i, a := range e.Attr {
		if a.Key == key && a.NamespaceURI() == nsW {
			e.Attr[i].Value = val
			return
		}
	}
	e.CreateAttr("w:"+key, val)
}

func getW(e *etree.Element, key string) string { return office.AttrV(e, nsW, key) }

func removeW(e *etree.Element, key string) {
	for i, a := range e.Attr {
		if a.Key == key && a.NamespaceURI() == nsW {
			e.Attr = append(e.Attr[:i], e.Attr[i+1:]...)
			return
		}
	}
}

func is(e *etree.Element, local string) bool { return office.Is(e, nsW, local) }

func child(e *etree.Element, local string) *etree.Element { return office.Child(e, nsW, local) }

func all(e *etree.Element, local string) []*etree.Element { return office.All(e, nsW, local) }

func remove(e *etree.Element) {
	if p := e.Parent(); p != nil {
		p.RemoveChild(e)
	}
}

func index(e *etree.Element) int {
	p := e.Parent()
	for i, c := range p.ChildElements() {
		if c == e {
			return i
		}
	}
	return -1
}

// insertAt inserts e before the i-th child element of parent.
func insertAt(parent *etree.Element, i int, e *etree.Element) {
	kids := parent.ChildElements()
	if i >= len(kids) {
		parent.AddChild(e)
		return
	}
	parent.InsertChildAt(kids[i].Index(), e)
}

func addNext(ref, e *etree.Element) { ref.Parent().InsertChildAt(ref.Index()+1, e) }

func addPrev(ref, e *etree.Element) { ref.Parent().InsertChildAt(ref.Index(), e) }

func ancestor(e *etree.Element, local string) *etree.Element {
	for p := e.Parent(); p != nil; p = p.Parent() {
		if is(p, local) {
			return p
		}
	}
	return nil
}

func runText(r *etree.Element) string {
	var b strings.Builder
	for _, t := range office.Children(r, nsW, "t") {
		b.WriteString(t.Text())
	}
	return b.String()
}

func plainRun(r *etree.Element) bool {
	if !is(r, "r") {
		return false
	}
	for _, c := range r.ChildElements() {
		if !is(c, "rPr") && !is(c, "t") {
			return false
		}
	}
	return true
}

// runSlice copies run formatting onto a new run holding text.
func runSlice(r *etree.Element, text string, deleted bool) *etree.Element {
	out := W("r")
	if rp := child(r, "rPr"); rp != nil {
		out.AddChild(rp.Copy())
	}
	if deleted {
		out.AddChild(WT("delText", text))
	} else {
		out.AddChild(WT("t", text))
	}
	return out
}

func hasRunControl(text string) bool {
	return strings.ContainsAny(text, "\t\n\r")
}

// addRunText keeps the historical single preserved w:t for plain text and
// converts tabs and line breaks to w:tab/w:br otherwise.
func addRunText(r *etree.Element, text string) {
	if hasRunControl(text) {
		setRunText(r, strings.ReplaceAll(text, "\r\n", "\n"))
		return
	}
	r.AddChild(WT("t", text))
}

// setRunText mirrors reference DOCX library Run.text: tabs and newlines become w:tab/w:br.
func setRunText(r *etree.Element, text string) {
	for _, c := range r.ChildElements() {
		if !is(c, "rPr") {
			r.RemoveChild(c)
		}
	}
	var buf strings.Builder
	flush := func() {
		if buf.Len() > 0 {
			text := buf.String()
			e := W("t")
			e.SetText(text)
			// reference DOCX library CT_R.add_t preserves space only at a text fragment edge.
			if strings.TrimFunc(text, func(r rune) bool { return unicode.IsSpace(r) || r >= 0x1c && r <= 0x1f }) != text {
				e.CreateAttr("xml:space", "preserve")
			}
			r.AddChild(e)
			buf.Reset()
		}
	}
	for _, ch := range text {
		switch ch {
		case '\t':
			flush()
			r.AddChild(W("tab"))
		case '\n', '\r':
			flush()
			r.AddChild(W("br"))
		default:
			buf.WriteRune(ch)
		}
	}
	flush()
}

func paraText(p *etree.Element) string {
	var b strings.Builder
	office.Iter(p, func(e *etree.Element) bool {
		if is(e, "t") {
			b.WriteString(e.Text())
		}
		return true
	})
	return b.String()
}

func newPara(text string) *etree.Element {
	p := W("p")
	if text != "" {
		r := W("r")
		setRunText(r, text)
		p.AddChild(r)
	}
	return p
}

func pPr(p *etree.Element) *etree.Element {
	if pp := child(p, "pPr"); pp != nil {
		return pp
	}
	pp := W("pPr")
	p.InsertChildAt(0, pp)
	return pp
}

// insertInPPr places a property element in schema order inside w:pPr.
func insertInPPr(pp, e *etree.Element) {
	order := []string{"pStyle", "keepNext", "keepLines", "pageBreakBefore", "framePr", "widowControl", "numPr", "suppressLineNumbers", "pBdr", "shd", "tabs", "suppressAutoHyphens", "kinsoku", "wordWrap", "overflowPunct", "topLinePunct", "autoSpaceDE", "autoSpaceDN", "bidi", "adjustRightInd", "snapToGrid", "spacing", "ind", "contextualSpacing", "mirrorIndents", "suppressOverlap", "jc", "textDirection", "textAlignment", "textboxTightWrap", "outlineLvl", "divId", "cnfStyle", "rPr", "sectPr", "pPrChange"}
	pos := map[string]int{}
	for i, n := range order {
		pos[n] = i
	}
	mine := pos[e.Tag]
	for _, c := range pp.ChildElements() {
		if p, ok := pos[c.Tag]; ok && p > mine {
			addPrev(c, e)
			return
		}
	}
	pp.AddChild(e)
}

func bodyParagraphs(d *Doc) []*etree.Element { return office.Children(d.Body(), nsW, "p") }

func bodyTables(d *Doc) []*etree.Element { return office.Children(d.Body(), nsW, "tbl") }

func selectParagraph(d *Doc, v any) (*etree.Element, error) {
	i, ok := intArg(v)
	ps := bodyParagraphs(d)
	if !ok || i < 0 || i >= len(ps) {
		return nil, errors.New("unknown body paragraph index")
	}
	return ps[i], nil
}

func intArg(v any) (int, bool) {
	f, ok := v.(float64)
	if !ok || f != float64(int(f)) {
		return 0, false
	}
	return int(f), true
}

// appendBody adds a block before the body's final sectPr.
func appendBody(d *Doc, e *etree.Element) {
	body := d.Body()
	kids := body.ChildElements()
	if n := len(kids); n > 0 && is(kids[n-1], "sectPr") {
		addPrev(kids[n-1], e)
		return
	}
	body.AddChild(e)
}

func numericIDs(roots []*etree.Element, attrNS, key string) int {
	max := 0
	for _, root := range roots {
		office.Iter(root, func(e *etree.Element) bool {
			if v, ok := office.Attr(e, attrNS, key); ok {
				if n, err := strconv.Atoi(v); err == nil && n > max {
					max = n
				}
			}
			return true
		})
	}
	return max
}

func (d *Doc) roots(parts []string) []*etree.Element {
	var out []*etree.Element
	for _, p := range parts {
		if r, err := d.Root(p); err == nil {
			out = append(out, r)
		}
	}
	return out
}

func (d *Doc) timestamp() string { return d.now.Format("2006-01-02T15:04:05Z") }
