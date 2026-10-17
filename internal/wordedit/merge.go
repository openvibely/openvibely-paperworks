package wordedit

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"

	"github.com/beevik/etree"
	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
)

// merge appends other documents' bodies. The first document keeps its
// sections, headers and footers; referenced media, links, styles and
// numbering definitions are imported with remapped identifiers.
func merge(d *Doc, files []string) error {
	if len(files) == 0 {
		return errors.New("merge needs at least one file")
	}
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			return err
		}
		src, err := Open(raw)
		if err != nil {
			return fmt.Errorf("%s: %w", f, err)
		}
		if err := appendDoc(d, src); err != nil {
			return fmt.Errorf("%s: %w", f, err)
		}
	}
	return nil
}

var importableRel = map[string]bool{"image": true, "hyperlink": true, "chart": true, "oleObject": true, "package": true, "diagramData": true, "diagramLayout": true, "diagramQuickStyle": true, "diagramColors": true, "diagramDrawing": true}

func appendDoc(d, src *Doc) error {
	rels, err := src.Pkg.Rels(src.Main)
	if err != nil {
		return err
	}
	relByID := map[string]opc.Relationship{}
	for _, r := range rels {
		relByID[r.ID] = r
	}
	numMap, err := importNumbering(d, src)
	if err != nil {
		return err
	}
	if err := importStyles(d, src, numMap); err != nil {
		return err
	}
	idMap := map[string]string{}
	copied := map[string]string{}
	remapAttr := func(e *etree.Element) error {
		for i, a := range e.Attr {
			if a.NamespaceURI() != nsR {
				continue
			}
			if nid, ok := idMap[a.Value]; ok {
				e.Attr[i].Value = nid
				continue
			}
			r, ok := relByID[a.Value]
			if !ok {
				return fmt.Errorf("broken relationship %s", a.Value)
			}
			kind := r.Type[strings.LastIndex(r.Type, "/")+1:]
			if !importableRel[kind] {
				return fmt.Errorf("unsupported relationship type %q in merged body", kind)
			}
			var nid string
			if r.TargetMode == "External" {
				nid, err = d.Pkg.AddRel(d.Main, r.Type, r.Target, true)
			} else {
				target, err2 := copyPartTree(d, src, opc.Resolve(src.Main, r.Target), copied)
				if err2 != nil {
					return err2
				}
				rel, _ := relPath(d.Main, target)
				if kind == "image" {
					existing, e := d.Pkg.Rels(d.Main)
					if e != nil {
						return e
					}
					for _, candidate := range existing {
						if candidate.Type == r.Type && candidate.TargetMode != "External" && opc.Resolve(d.Main, candidate.Target) == target {
							nid = candidate.ID
							break
						}
					}
				}
				if nid == "" {
					nid, err = d.Pkg.AddRel(d.Main, r.Type, rel, false)
				}
			}
			if err != nil {
				return err
			}
			idMap[a.Value] = nid
			e.Attr[i].Value = nid
		}
		return nil
	}
	maxDocPr := 0
	office.Iter(d.MainRoot(), func(e *etree.Element) bool {
		if e.Tag == "docPr" {
			if n, err := strconv.Atoi(e.SelectAttrValue("id", "")); err == nil && n > maxDocPr {
				maxDocPr = n
			}
		}
		return true
	})
	restarted := map[string]bool{}
	bmMax := numericIDs([]*etree.Element{d.MainRoot()}, nsW, "id")
	for _, blk := range src.Body().ChildElements() {
		if is(blk, "sectPr") {
			continue
		}
		c := blk.Copy()
		// Bind the copied subtree before namespace-aware relationship/numbering walks.
		appendBody(d, c)
		var walkErr error
		office.Iter(c, func(e *etree.Element) bool {
			if is(e, "sectPr") {
				remove(e)
				return false
			}
			if err := remapAttr(e); err != nil {
				walkErr = err
				return false
			}
			if e.Tag == "docPr" {
				maxDocPr++
				e.CreateAttr("id", strconv.Itoa(maxDocPr))
			}
			if is(e, "numId") {
				if n, ok := numMap[getW(e, "val")]; ok {
					setW(e, "val", n)
				}
			}
			if is(e, "bookmarkStart") || is(e, "bookmarkEnd") {
				if n, err := strconv.Atoi(getW(e, "id")); err == nil {
					setW(e, "id", strconv.Itoa(n+bmMax+1))
				}
			}
			if is(e, "commentRangeStart") || is(e, "commentRangeEnd") || is(e, "commentReference") || is(e, "footnoteReference") || is(e, "endnoteReference") {
				walkErr = errors.New("merging comments or notes is unsupported; remove them or merge in an office editor")
				return false
			}
			return true
		})
		if walkErr != nil {
			return walkErr
		}
		if err := restartMergedNumbering(d, c, restarted, numMap); err != nil {
			return err
		}
	}
	// Composer renumbers picture properties independently of drawing properties.
	for i, e := range office.All(d.Body(), "http://schemas.openxmlformats.org/drawingml/2006/picture", "cNvPr") {
		e.CreateAttr("id", strconv.Itoa(i+1))
	}
	d.Touch(d.Main)
	return nil
}

func copyPartTree(d, src *Doc, part string, copied map[string]string) (string, error) {
	if t, ok := copied[part]; ok {
		return t, nil
	}
	data := src.Pkg.Get(part)
	if data == nil {
		return "", fmt.Errorf("missing related part %s", part)
	}
	// Composer shares identical embedded images across appended documents.
	if strings.HasPrefix(src.Pkg.ContentType(part), "image/") {
		for _, candidate := range d.Pkg.Parts {
			if d.Pkg.ContentType(candidate.Name) == src.Pkg.ContentType(part) && bytes.Equal(candidate.Data, data) {
				copied[part] = candidate.Name
				return candidate.Name, nil
			}
		}
	}
	dir, base := path.Split(part)
	ext := path.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	target := part
	for n := 1; d.Pkg.Has(target); n++ {
		target = fmt.Sprintf("%s%s_m%d%s", dir, stem, n, ext)
	}
	copied[part] = target
	d.Pkg.Set(target, data)
	d.addedPart = true
	ct := src.Pkg.ContentType(part)
	if ct != "" {
		if err := mergeContentType(d.Pkg, target, ct); err != nil {
			return "", err
		}
	}
	rels, err := src.Pkg.Rels(part)
	if err != nil {
		return "", err
	}
	for _, r := range rels {
		if r.TargetMode == "External" {
			if _, err := d.Pkg.AddRel(target, r.Type, r.Target, true); err != nil {
				return "", err
			}
			continue
		}
		sub, err := copyPartTree(d, src, opc.Resolve(part, r.Target), copied)
		if err != nil {
			return "", err
		}
		rel, _ := relPathFrom(target, sub)
		if err := addRelWithID(d.Pkg, target, r.ID, r.Type, rel); err != nil {
			return "", err
		}
	}
	return target, nil
}

func relPathFrom(from, to string) (string, error) {
	fd := strings.Split(path.Dir(from), "/")
	td := strings.Split(to, "/")
	i := 0
	for i < len(fd) && i < len(td)-1 && fd[i] == td[i] {
		i++
	}
	return strings.Repeat("../", len(fd)-i) + strings.Join(td[i:], "/"), nil
}

func addRelWithID(p *opc.Package, owner, id, typ, target string) error {
	name := opc.RelsName(owner)
	var x *etree.Document
	if p.Has(name) {
		var err error
		if x, err = p.XML(name); err != nil {
			return err
		}
	} else {
		x = etree.NewDocument()
		x.CreateProcInst("xml", `version="1.0" encoding="UTF-8" standalone="yes"`)
		x.CreateElement("Relationships").CreateAttr("xmlns", opc.NSRel)
	}
	r := x.Root().CreateElement("Relationship")
	r.CreateAttr("Id", id)
	r.CreateAttr("Type", typ)
	r.CreateAttr("Target", target)
	return p.SetXML(name, x)
}

func importStyles(d, src *Doc, numMap map[string]string) error {
	st, sst := d.styles(), src.styles()
	if sst.part == "" {
		return nil
	}
	if st.part == "" {
		return errors.New("first document has no styles part")
	}
	root, _ := d.Root(st.part)
	added := false
	for id, s := range sst.byID {
		if _, ok := st.byID[id]; !ok {
			copy := s.Copy()
			root.AddChild(copy)
			for _, n := range office.All(copy, nsW, "numId") {
				if mapped, ok := numMap[getW(n, "val")]; ok {
					setW(n, "val", mapped)
				}
			}
			added = true
		}
	}
	if added {
		d.Touch(st.part)
	}
	return nil
}

// importNumbering copies abstract and concrete numbering definitions used by
// the source and returns the numId remapping.
func importNumbering(d, src *Doc) (map[string]string, error) {
	out := map[string]string{}
	used := map[string]bool{}
	for _, n := range office.All(src.Body(), nsW, "numId") {
		used[getW(n, "val")] = true
	}
	// Numbering in an already-present style is shared, rather than imported.
	dstStyles, srcStyles := d.styles(), src.styles()
	for id, st := range srcStyles.byID {
		if dstStyles.byID[id] == nil {
			for _, n := range office.All(st, nsW, "numId") {
				used[getW(n, "val")] = true
			}
		}
	}
	if len(used) == 0 {
		return out, nil
	}
	spart, err := src.Related(src.Main, "numbering")
	if err != nil || spart == "" {
		return out, err
	}
	sroot, err := src.Root(spart)
	if err != nil {
		return nil, err
	}
	part, err := d.Related(d.Main, "numbering")
	if err != nil {
		return nil, err
	}
	if part == "" {
		part = "word/numbering.xml"
		if err := d.AddPart(part, ctBase+"numbering+xml", "numbering", newRoot("numbering")); err != nil {
			return nil, err
		}
	}
	root, err := d.Root(part)
	if err != nil {
		return nil, err
	}
	maxAbs, maxNum := -1, 0
	for _, a := range office.Children(root, nsW, "abstractNum") {
		if n, err := strconv.Atoi(getW(a, "abstractNumId")); err == nil && n > maxAbs {
			maxAbs = n
		}
	}
	for _, n := range office.Children(root, nsW, "num") {
		if v, err := strconv.Atoi(getW(n, "numId")); err == nil && v > maxNum {
			maxNum = v
		}
	}
	neededAbs := map[string]bool{}
	for _, n := range office.Children(sroot, nsW, "num") {
		if used[getW(n, "numId")] {
			neededAbs[getW(child(n, "abstractNumId"), "val")] = true
		}
	}
	absMap := map[string]string{}
	firstNum := office.Child(root, nsW, "num")
	for _, a := range office.Children(sroot, nsW, "abstractNum") {
		if !neededAbs[getW(a, "abstractNumId")] {
			continue
		}
		maxAbs++
		c := a.Copy()
		absMap[getW(a, "abstractNumId")] = strconv.Itoa(maxAbs)
		setW(c, "abstractNumId", strconv.Itoa(maxAbs))
		for _, nsid := range office.Children(c, nsW, "nsid") {
			c.RemoveChild(nsid)
		}
		if firstNum != nil {
			addPrev(firstNum, c)
		} else {
			root.AddChild(c)
		}
	}
	for _, n := range office.Children(sroot, nsW, "num") {
		if !used[getW(n, "numId")] {
			continue
		}
		maxNum++
		c := n.Copy()
		out[getW(n, "numId")] = strconv.Itoa(maxNum)
		setW(c, "numId", strconv.Itoa(maxNum))
		root.AddChild(c)
		if an := child(c, "abstractNumId"); an != nil {
			setW(an, "val", absMap[getW(an, "val")])
		}
	}
	d.Touch(part)
	return out, nil
}

// stylePack copies styles/theme/fontTable/numbering from a template package.
func stylePack(d *Doc, template string) error {
	raw, err := os.ReadFile(template)
	if err != nil {
		return err
	}
	pack, err := opc.Read(raw)
	if err != nil {
		return err
	}
	for _, it := range []struct{ path, kind string }{{"word/styles.xml", "styles"}, {"word/theme/theme1.xml", "theme"}, {"word/fontTable.xml", "fontTable"}, {"word/numbering.xml", "numbering"}} {
		if !pack.Has(it.path) {
			continue
		}
		if prs, _ := pack.Rels(it.path); len(prs) > 0 {
			return errors.New("style pack has linked resources; import those explicitly before applying")
		}
		ct := pack.ContentType(it.path)
		if ct == "" {
			return errors.New("style part lacks a declared content type")
		}
		target, err := d.Related(d.Main, it.kind)
		if err != nil {
			return err
		}
		if target == "" {
			target = it.path
			d.Pkg.Set(target, nil)
			rel, _ := relPath(d.Main, target)
			if _, err := d.Pkg.AddRel(d.Main, relBase+it.kind, rel, false); err != nil {
				return err
			}
		}
		if strings.HasPrefix(target, "../") {
			return errors.New("invalid style target")
		}
		d.Pkg.Set(target, pack.Get(it.path))
		delete(d.docs, target)
		if err := d.Pkg.EnsureContentType(target, ct); err != nil {
			return err
		}
	}
	return nil
}

// restartMergedNumbering follows Composer.restart_first_numbering: the first
// non-heading, non-bullet paragraph using a style starts a new list at one.
// A subsequent paragraph inheriting that style continues the original style;
// explicit references to the restarted numbering follow the updated mapping.
func restartMergedNumbering(d *Doc, block *etree.Element, restarted map[string]bool, numMap map[string]string) error {
	styles := office.All(block, nsW, "pStyle")
	if len(styles) == 0 {
		return nil
	}
	id := getW(styles[0], "val")
	if restarted[id] {
		return nil
	}
	style := d.styles().byID[id]
	if style == nil || len(office.All(style, nsW, "outlineLvl")) > 0 {
		return nil
	}
	refs := office.All(block, nsW, "numId")
	if len(refs) == 0 {
		refs = office.All(style, nsW, "numId")
	}
	if len(refs) == 0 {
		return nil
	}
	old := getW(refs[0], "val")
	part, err := d.Related(d.Main, "numbering")
	if err != nil || part == "" {
		return err
	}
	root, err := d.Root(part)
	if err != nil {
		return err
	}
	var num *etree.Element
	next := 1
	nums := office.Children(root, nsW, "num")
	for _, n := range nums {
		v := getW(n, "numId")
		if v == old {
			num = n
		}
		if i, e := strconv.Atoi(v); e == nil && i >= next {
			next = i + 1
		}
	}
	if num == nil {
		return nil
	}
	absID := getW(child(num, "abstractNumId"), "val")
	for _, a := range office.Children(root, nsW, "abstractNum") {
		if getW(a, "abstractNumId") == absID {
			for _, lvl := range office.Children(a, nsW, "lvl") {
				if getW(lvl, "ilvl") == "0" && getW(child(lvl, "numFmt"), "val") == "bullet" {
					return nil
				}
			}
		}
	}
	copied := num.Copy()
	setW(copied, "numId", strconv.Itoa(next))
	override := copied.CreateElement("w:lvlOverride")
	setW(override, "ilvl", "0")
	setW(override.CreateElement("w:startOverride"), "val", "1")
	// The pinned implementation inserts at the last num's index (before it).
	addPrev(nums[len(nums)-1], copied)
	props := styles[0].Parent()
	np := child(props, "numPr")
	if np == nil {
		np = props.CreateElement("w:numPr")
		setW(np.CreateElement("w:ilvl"), "val", "0")
		setW(np.CreateElement("w:numId"), "val", strconv.Itoa(next))
	} else {
		n := child(np, "numId")
		if n == nil {
			n = np.CreateElement("w:numId")
		}
		prior := getW(n, "val")
		setW(n, "val", strconv.Itoa(next))
		for k, v := range numMap {
			if v == prior {
				numMap[k] = strconv.Itoa(next)
			}
		}
	}
	restarted[id] = true
	d.Touch(part)
	return nil
}

// reference DOCX library registers standard image MIME types as extension defaults and
// emits defaults before overrides, sorted by extension. Do not replace a
// conflicting default, since that would change unrelated existing parts.
func mergeContentType(p *opc.Package, part, ct string) error {
	ext := strings.TrimPrefix(path.Ext(part), ".")
	standard := map[string]string{"png": "image/png", "jpeg": "image/jpeg", "jpg": "image/jpeg", "gif": "image/gif", "bmp": "image/bmp", "tiff": "image/tiff"}
	if standard[ext] != ct {
		return p.EnsureContentType(part, ct)
	}
	d, err := p.XML("[Content_Types].xml")
	if err != nil {
		return err
	}
	root := d.Root()
	for _, n := range root.ChildElements() {
		if n.Tag == "Default" && strings.EqualFold(n.SelectAttrValue("Extension", ""), ext) {
			if n.SelectAttrValue("ContentType", "") == ct {
				return nil
			}
			return p.EnsureContentType(part, ct)
		}
	}
	n := etree.NewElement("Default")
	n.CreateAttr("Extension", ext)
	n.CreateAttr("ContentType", ct)
	root.AddChild(n)
	defaults := []*etree.Element{}
	for _, n := range root.ChildElements() {
		if n.Tag == "Default" {
			defaults = append(defaults, n)
		}
	}
	sort.SliceStable(defaults, func(i, j int) bool {
		return defaults[i].SelectAttrValue("Extension", "") < defaults[j].SelectAttrValue("Extension", "")
	})
	for _, n := range defaults {
		root.RemoveChild(n)
	}
	for i, n := range defaults {
		root.InsertChildAt(i, n)
	}
	return p.SetXML("[Content_Types].xml", d)
}
