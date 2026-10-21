package wordedit

import (
	"fmt"
	"os"
	"strconv"

	"github.com/beevik/etree"
	"github.com/openvibely/openvibely-paperworks/internal/office"
)

// cloneComponent copies an inclusive range of top-level body blocks, preserving
// related media through the same importer used by document merging.
func cloneComponent(d *Doc, source string, spec map[string]any) error {
	raw, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	src, err := Open(raw)
	if err != nil {
		return err
	}
	blocks := func(doc *Doc) []*etree.Element {
		var out []*etree.Element
		for _, b := range doc.Body().ChildElements() {
			if !is(b, "sectPr") {
				out = append(out, b)
			}
		}
		return out
	}
	sourceBlocks, destBlocks := blocks(src), blocks(d)
	start, ok := intArg(spec["start"])
	if !ok {
		return fmt.Errorf("clone-component requires integer start")
	}
	end, ok := intArg(spec["end"])
	if !ok || start < 0 || end < start || end >= len(sourceBlocks) {
		return fmt.Errorf("clone-component requires a valid inclusive start/end block range")
	}
	before := len(destBlocks)
	if v, exists := spec["before"]; exists {
		before, ok = intArg(v)
		if !ok || before < 0 || before > len(destBlocks) {
			return fmt.Errorf("before block index out of range")
		}
	}
	selected := map[*etree.Element]bool{}
	for _, b := range sourceBlocks[start : end+1] {
		selected[b] = true
	}
	for _, b := range sourceBlocks {
		if !selected[b] {
			src.Body().RemoveChild(b)
		}
	}
	// Components containing references to other document stories need a separate
	// import policy. Fail rather than silently removing those dependencies.
	var invalid error
	starts, ends := map[string]bool{}, map[string]bool{}
	office.Iter(src.Body(), func(e *etree.Element) bool {
		if is(e, "sectPr") && e.Parent() != src.Body() {
			invalid = fmt.Errorf("component contains a section break")
		}
		for _, tag := range []string{"commentRangeStart", "commentRangeEnd", "commentReference", "footnoteReference", "endnoteReference", "fldChar", "fldSimple", "instrText", "ins", "del", "moveFrom", "moveTo", "pPrChange", "rPrChange", "tblPrChange", "trPrChange", "tcPrChange", "sectPrChange", "tblGridChange", "dataBinding", "altChunk"} {
			if is(e, tag) {
				invalid = fmt.Errorf("component contains unsupported %s dependency", tag)
			}
		}
		if is(e, "bookmarkStart") {
			id := getW(e, "id")
			if starts[id] {
				invalid = fmt.Errorf("duplicate bookmark start")
			}
			starts[id] = true
		}
		if is(e, "bookmarkEnd") {
			id := getW(e, "id")
			if ends[id] {
				invalid = fmt.Errorf("duplicate bookmark end")
			}
			ends[id] = true
		}
		return true
	})
	if invalid != nil {
		return invalid
	}
	if len(starts) != len(ends) {
		return fmt.Errorf("component crosses a bookmark boundary")
	}
	for id := range starts {
		if !ends[id] {
			return fmt.Errorf("component crosses a bookmark boundary")
		}
	}
	usedNames := map[string]bool{}
	for _, b := range office.All(d.MainRoot(), nsW, "bookmarkStart") {
		usedNames[getW(b, "name")] = true
	}
	names := map[string]string{}
	next := 1
	for _, b := range office.All(src.Body(), nsW, "bookmarkStart") {
		name := getW(b, "name")
		if _, ok := names[name]; ok {
			return fmt.Errorf("duplicate component bookmark name")
		}
		newName := name
		if usedNames[newName] {
			for {
				newName = "PaperworksClone" + strconv.Itoa(next)
				next++
				if !usedNames[newName] {
					break
				}
			}
		}
		names[name] = newName
		usedNames[newName] = true
		setW(b, "name", newName)
	}
	for _, link := range office.All(src.Body(), nsW, "hyperlink") {
		if name := getW(link, "anchor"); name != "" {
			if renamed, ok := names[name]; ok {
				setW(link, "anchor", renamed)
			} else if !usedNames[name] {
				return fmt.Errorf("component hyperlink references absent bookmark %s", name)
			}
		}
	}
	// Renaming conflicting style IDs keeps the destination's existing content
	// intact, while the imported component retains its own style definitions.
	dstStyles, srcStyles := d.styles(), src.styles()
	styleMap := map[string]string{}
	usedStyles := map[string]bool{}
	for id := range dstStyles.byID {
		usedStyles[id] = true
	}
	for id := range srcStyles.byID {
		usedStyles[id] = true
	}
	for id, s := range srcStyles.byID {
		// A numbering ID is local to its document. Even identical style XML
		// can refer to a different list definition; import numbered styles
		// independently and let importNumbering remap their dependencies.
		numbered := false
		for _, n := range office.All(s, nsW, "numId") {
			if id := getW(n, "val"); id != "" && id != "0" {
				numbered = true
			}
		}
		if other := dstStyles.byID[id]; other != nil && (!sameCloneElement(s, other) || numbered) {
			for n := 1; ; n++ {
				candidate := id + "_clone" + strconv.Itoa(n)
				if !usedStyles[candidate] {
					styleMap[id] = candidate
					usedStyles[candidate] = true
					break
				}
			}
		}
	}
	// If a style depends on a renamed style, it too must be imported, even if its
	// own definition originally matched the destination.
	for changed := true; changed; {
		changed = false
		for id, s := range srcStyles.byID {
			if styleMap[id] != "" || dstStyles.byID[id] == nil {
				continue
			}
			for _, e := range s.ChildElements() {
				if (is(e, "basedOn") || is(e, "next") || is(e, "link")) && styleMap[getW(e, "val")] != "" {
					for n := 1; ; n++ {
						candidate := id + "_clone" + strconv.Itoa(n)
						if !usedStyles[candidate] {
							styleMap[id] = candidate
							usedStyles[candidate] = true
							break
						}
					}
					changed = true
					break
				}
			}
		}
	}
	// Paragraphs without an explicit style still inherit the source default.
	// Pin it when it differs from the destination default or is renamed.
	if srcStyles.def != "" && (srcStyles.def != dstStyles.def || styleMap[srcStyles.def] != "") {
		for _, paragraph := range office.All(src.Body(), nsW, "p") {
			pr := child(paragraph, "pPr")
			if pr == nil {
				pr = etree.NewElement("w:pPr")
				paragraph.InsertChildAt(0, pr)
			}
			if child(pr, "pStyle") == nil {
				st := etree.NewElement("w:pStyle")
				setW(st, "val", srcStyles.def)
				pr.InsertChildAt(0, st)
			}
		}
	}
	for id, s := range srcStyles.byID {
		if mapped := styleMap[id]; mapped != "" {
			setW(s, "styleId", mapped)
		}
		// Imported defaults must never become defaults for untouched content.
		// Shared styles are skipped by importStyles, leaving their destination
		// definitions intact. Cloned paragraphs have explicit styles above.
		removeW(s, "default")
	}
	remapStyles := func(root *etree.Element) {
		office.Iter(root, func(e *etree.Element) bool {
			for _, tag := range []string{"pStyle", "rStyle", "tblStyle", "basedOn", "next", "link", "styleLink", "numStyleLink"} {
				if is(e, tag) {
					if mapped := styleMap[getW(e, "val")]; mapped != "" {
						setW(e, "val", mapped)
					}
				}
			}
			return true
		})
	}
	remapStyles(src.Body())
	if srcStyles.part != "" {
		r, err := src.Root(srcStyles.part)
		if err != nil {
			return err
		}
		remapStyles(r)
	}
	if part, err := src.Related(src.Main, "numbering"); err != nil {
		return err
	} else if part != "" {
		r, err := src.Root(part)
		if err != nil {
			return err
		}
		remapStyles(r)
	}
	maxID := 0
	for _, pr := range office.All(d.MainRoot(), nsW, "sdtPr") {
		if id := child(pr, "id"); id != nil {
			if n, err := strconv.Atoi(getW(id, "val")); err == nil && n > maxID {
				maxID = n
			}
		}
	}
	for _, pr := range office.All(src.Body(), nsW, "sdtPr") {
		if id := child(pr, "id"); id != nil {
			maxID++
			setW(id, "val", strconv.Itoa(maxID))
		}
	}
	// Namespace bindings inherited from the template must travel with the block.
	for _, b := range blocks(src) {
		for _, a := range src.MainRoot().Attr {
			if a.Space == "xmlns" || a.Key == "xmlns" {
				if b.SelectAttr(a.FullKey()) == nil {
					b.CreateAttr(a.FullKey(), a.Value)
				}
			}
		}
	}
	original := map[*etree.Element]bool{}
	for _, b := range d.Body().ChildElements() {
		original[b] = true
	}
	if err := appendDoc(d, src); err != nil {
		return err
	}
	if before < len(destBlocks) {
		for _, b := range d.Body().ChildElements() {
			if !original[b] {
				d.Body().RemoveChild(b)
				d.Body().InsertChildAt(destBlocks[before].Index(), b)
			}
		}
	}
	d.Touch(d.Main)
	return nil
}
func sameCloneElement(a, b *etree.Element) bool {
	x, y := etree.NewDocument(), etree.NewDocument()
	x.SetRoot(a.Copy())
	y.SetRoot(b.Copy())
	sx, _ := x.WriteToString()
	sy, _ := y.WriteToString()
	return sx == sy
}
