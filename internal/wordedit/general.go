package wordedit

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/beevik/etree"
	"github.com/openvibely/openvibely-paperworks/internal/office"
)

// generalEdit deliberately targets one paragraph/run/table rather than doing
// global redaction. Indices are zero-based, matching inspect and existing edits.
func generalEdit(d *Doc, s map[string]any) error {
	part := str(s, "part")
	if part == "" {
		part = d.Main
	}
	root, err := d.Root(part)
	if err != nil {
		return err
	}
	if part != d.Main && !is(root, "hdr") && !is(root, "ftr") {
		return fmt.Errorf("part must be document, header or footer")
	}
	container := root
	if part == d.Main {
		container = d.Body()
	}
	op := str(s, "operation")
	if op == "batch" {
		edits, ok := s["edits"].([]any)
		if !ok || len(edits) == 0 || len(edits) > 10000 {
			return fmt.Errorf("edits must contain 1..10000 operations")
		}
		for i, v := range edits {
			m, ok := v.(map[string]any)
			if !ok || str(m, "operation") == "batch" {
				return fmt.Errorf("invalid edit %d", i)
			}
			if err := generalEdit(d, m); err != nil {
				return fmt.Errorf("edit %d: %w", i, err)
			}
		}
		return nil
	}
	if op == "section-insert" || op == "header-footer" || op == "header-footer-link" {
		if op == "header-footer-link" && part != d.Main {
			return fmt.Errorf("header-footer-link targets document sections, not a story part")
		}
		return sectionEdit(d, s)
	}
	if strings.HasPrefix(op, "image-") {
		return imageEdit(d, part, root, s)
	}
	if strings.HasPrefix(op, "table-") {
		err = editTable(container, s)
		if err == nil {
			d.Touch(part)
		}
		return err
	}
	if op == "section-format" {
		err = editSection(container, s)
		if err == nil {
			d.Touch(part)
		}
		return err
	}
	paras := office.Children(container, nsW, "p")
	i, ok := intArg(s["paragraph"])
	if !ok || i < 0 || i >= len(paras) {
		return fmt.Errorf("paragraph index out of range")
	}
	p := paras[i]
	switch op {
	case "replace-text":
		err = replaceOrdinary(p, s)
	case "paragraph-insert":
		text, ok := s["text"].(string)
		if !ok {
			return fmt.Errorf("text required")
		}
		new := W("p")
		if prop := child(p, "pPr"); prop != nil {
			pr := prop.Copy()
			new.AddChild(pr)
		}
		r := W("r")
		addRunText(r, text)
		new.AddChild(r)
		if str(s, "position") == "after" {
			p.Parent().InsertChildAt(p.Index()+1, new)
		} else if str(s, "position") == "before" || str(s, "position") == "" {
			p.Parent().InsertChildAt(p.Index(), new)
		} else {
			return fmt.Errorf("position must be before or after")
		}
		for _, section := range all(new, "sectPr") {
			remove(section)
		}
	case "paragraph-delete":
		if len(all(p, "sectPr")) > 0 {
			return fmt.Errorf("paragraph owns a section break; refusing implicit section deletion")
		}
		if len(all(p, "commentRangeStart"))+len(all(p, "commentRangeEnd"))+len(all(p, "bookmarkStart"))+len(all(p, "bookmarkEnd")) > 0 {
			return fmt.Errorf("paragraph contains anchored annotations; remove them explicitly first")
		}
		if !paragraphContainsCompleteFields(p) {
			return fmt.Errorf("paragraph crosses a field boundary; refusing partial field deletion")
		}
		remove(p)
	case "paragraph-format":
		err = formatParagraph(p, s)
	case "run-format":
		runs := office.Children(p, nsW, "r")
		j, ok := intArg(s["run"])
		if !ok || j < 0 || j >= len(runs) {
			return fmt.Errorf("run index out of range")
		}
		err = formatRun(runs[j], s)
	default:
		return fmt.Errorf("unsupported general edit %q", op)
	}
	if err == nil {
		d.Touch(part)
	}
	return err
}
func property(parent *etree.Element, name string) *etree.Element {
	e := child(parent, name)
	if e == nil {
		e = W(name)
		if name == "pPr" || name == "rPr" || name == "tcPr" {
			parent.InsertChildAt(0, e)
		} else {
			insertWordProperty(parent, e)
		}
	}
	return e
}
func valProp(pr *etree.Element, name, value string) {
	p := child(pr, name)
	if p == nil {
		p = W(name)
		insertWordProperty(pr, p)
	}
	setW(p, "val", value)
}
func formatRun(r *etree.Element, s map[string]any) error {
	pr := property(r, "rPr")
	for _, k := range []string{"bold", "italic", "strike"} {
		if v, ok := s[k]; ok {
			b, ok := v.(bool)
			if !ok {
				return fmt.Errorf("%s must be boolean", k)
			}
			name := map[string]string{"bold": "b", "italic": "i", "strike": "strike"}[k]
			value := "0"
			if b {
				value = "1"
			}
			valProp(pr, name, value)
		}
	}
	if v, ok := s["font_size"]; ok {
		n, ok := v.(float64)
		if !ok || n <= 0 || n > 1638 {
			return fmt.Errorf("invalid font_size")
		}
		valProp(pr, "sz", strconv.Itoa(int(n*2)))
		valProp(pr, "szCs", strconv.Itoa(int(n*2)))
	}
	if v := str(s, "font"); v != "" {
		f := property(pr, "rFonts")
		// Theme bindings take precedence over explicit font names.
		for _, k := range []string{"asciiTheme", "hAnsiTheme", "eastAsiaTheme", "cstheme"} {
			removeW(f, k)
		}
		for _, k := range []string{"ascii", "hAnsi", "eastAsia", "cs"} {
			setW(f, k, v)
		}
	}
	if v := str(s, "color"); v != "" {
		if len(v) != 6 {
			return fmt.Errorf("color must be six hex digits")
		}
		if _, err := strconv.ParseUint(v, 16, 24); err != nil {
			return err
		}
		valProp(pr, "color", v)
	}
	if v := str(s, "style"); v != "" {
		valProp(pr, "rStyle", v)
	}
	return nil
}
func formatParagraph(p *etree.Element, s map[string]any) error {
	pr := property(p, "pPr")
	if v := str(s, "style"); v != "" {
		valProp(pr, "pStyle", v)
	}
	if v := str(s, "alignment"); v != "" {
		switch v {
		case "left", "right", "center", "both":
		default:
			return fmt.Errorf("invalid alignment")
		}
		valProp(pr, "jc", v)
	}
	for _, k := range []string{"before", "after", "line"} {
		if v, ok := s[k]; ok {
			n, ok := intArg(v)
			if !ok || n < 0 {
				return fmt.Errorf("%s must be nonnegative twips", k)
			}
			sp := child(pr, "spacing")
			if sp == nil {
				sp = W("spacing")
				insertInPPr(pr, sp)
			}
			setW(sp, k, strconv.Itoa(n))
			if k == "line" {
				// The API accepts twips; Word's default/auto rule instead
				// interprets line as 240ths of a line.
				setW(sp, "lineRule", "exact")
			}
		}
	}
	return nil
}

// splitRunReplace replaces the w:t text of a plain run with prefix, a run
// carrying insert as w:t/w:tab/w:br, and suffix, all sharing the run's rPr.
func splitRunReplace(t *etree.Element, prefix, insert, suffix string) {
	r := t.Parent()
	parent := r.Parent()
	idx := r.Index()
	mid := W("r")
	setRunText(mid, strings.ReplaceAll(insert, "\r\n", "\n"))
	if rp := child(r, "rPr"); rp != nil {
		mid.InsertChildAt(0, rp.Copy())
	}
	var tail *etree.Element
	if suffix != "" {
		tail = runSlice(r, suffix, false)
	}
	// Keep cached page boundaries on the same side of the replaced text.
	// Prefix and suffix stay in separate runs so later edits can search them.
	var before, after []*etree.Element
	for _, c := range r.ChildElements() {
		if is(c, "lastRenderedPageBreak") {
			if c.Index() < t.Index() {
				before = append(before, c.Copy())
			} else {
				after = append(after, c.Copy())
			}
			r.RemoveChild(c)
		}
	}
	first := mid
	if prefix != "" {
		first = r
	}
	markerIndex := 0
	if rp := child(r, "rPr"); rp != nil {
		markerIndex = 1 // mid carries a copy of rPr as its first child.
		if first == r {
			markerIndex = rp.Index() + 1
		}
	}
	for _, marker := range before {
		first.InsertChildAt(markerIndex, marker)
		markerIndex++
	}
	last := mid
	if tail != nil {
		last = tail
	}
	for _, marker := range after {
		last.AddChild(marker)
	}
	if prefix == "" {
		parent.RemoveChildAt(idx)
	} else {
		t.SetText(prefix)
		t.CreateAttr("xml:space", "preserve")
		idx++
	}
	parent.InsertChildAt(idx, mid)
	if tail != nil {
		parent.InsertChildAt(idx+1, tail)
	}
}

func replaceOrdinary(p *etree.Element, s map[string]any) error {
	old := str(s, "old")
	text, ok := s["new"].(string)
	if old == "" || !ok {
		return fmt.Errorf("nonempty old and string new required")
	}
	occ := 0
	if v, ok := s["occurrence"]; ok {
		var good bool
		occ, good = intArg(v)
		if !good || occ < 0 {
			return fmt.Errorf("invalid occurrence")
		}
	}
	// Only consecutive plain text runs are searchable. Never rewrite field
	// results, hyperlinks, revision markup, drawings, or annotation boundaries.
	var nodes []*etree.Element
	var joined strings.Builder
	seen := 0
	apply := func() bool {
		v := joined.String()
		for offset := 0; offset <= len(v); {
			i := strings.Index(v[offset:], old)
			if i < 0 {
				break
			}
			start := offset + i
			end := start + len(old)
			if seen == occ {
				pos := 0
				inserted := false
				for _, n := range nodes {
					t := n.Text()
					next := pos + len(t)
					if pos < end && next > start {
						lo, hi := max(0, start-pos), min(len(t), end-pos)
						insert := ""
						if !inserted {
							insert = text
							inserted = true
						}
						if hasRunControl(insert) {
							splitRunReplace(n, t[:lo], insert, t[hi:])
						} else {
							n.SetText(t[:lo] + insert + t[hi:])
							n.CreateAttr("xml:space", "preserve")
						}
					}
					pos = next
				}
				return true
			}
			seen++
			offset = end
		}
		return false
	}
	depth := fieldDepthBefore(p)
	for _, r := range p.ChildElements() {
		plain := is(r, "r") && depth == 0
		for _, marker := range all(r, "fldChar") {
			updateFieldDepth(marker, &depth)
			plain = false
		}
		ts := office.Children(r, nsW, "t")
		for _, c := range r.ChildElements() {
			if !is(c, "rPr") && !is(c, "t") && !is(c, "lastRenderedPageBreak") {
				plain = false
			}
		}
		if plain && len(ts) == 1 {
			nodes = append(nodes, ts[0])
			joined.WriteString(ts[0].Text())
		} else {
			if apply() {
				return nil
			}
			nodes = nil
			joined.Reset()
		}
	}
	if apply() {
		return nil
	}
	return fmt.Errorf("requested occurrence not found in ordinary text runs")
}
func editTable(root *etree.Element, s map[string]any) error {
	tables := office.Children(root, nsW, "tbl")
	i, ok := intArg(s["table"])
	if !ok || i < 0 || i >= len(tables) {
		return fmt.Errorf("table index out of range")
	}
	table := tables[i]
	switch str(s, "operation") {
	case "table-column-insert", "table-column-delete", "table-merge", "table-unmerge":
		return tableStructure(table, s)
	}
	rows := office.Children(table, nsW, "tr")
	row, ok := intArg(s["row"])
	if !ok || row < 0 || row >= len(rows) {
		return fmt.Errorf("row index out of range")
	}
	r := rows[row]
	op := str(s, "operation")
	if op == "table-row-insert" {
		if len(all(table, "vMerge")) > 0 {
			return fmt.Errorf("row edits require a table without vertical merges")
		}
		// Build a blank row from formatting only. Cloning the content also
		// duplicates controls, bookmarks, drawings and their identifiers.
		copy := W("tr")
		for _, name := range []string{"tblPrEx", "trPr"} {
			if pr := child(r, name); pr != nil {
				copy.AddChild(pr.Copy())
			}
		}
		for _, c := range all(r, "tc") {
			// Include cells wrapped in controls, but not cells in nested tables.
			owner := c.Parent()
			for owner != nil && !is(owner, "tr") {
				owner = owner.Parent()
			}
			if owner != r {
				continue
			}
			cell := W("tc")
			if pr := child(c, "tcPr"); pr != nil {
				cell.AddChild(pr.Copy())
			}
			paragraphs := office.Children(c, nsW, "p")
			for _, p := range paragraphs {
				blank := W("p")
				if pr := child(p, "pPr"); pr != nil {
					blank.AddChild(pr.Copy())
				}
				cell.AddChild(blank)
			}
			if len(paragraphs) == 0 {
				cell.AddChild(W("p"))
			}
			copy.AddChild(cell)
		}
		r.Parent().InsertChildAt(r.Index(), copy)
		return nil
	}
	if op == "table-row-delete" {
		if len(all(table, "vMerge")) > 0 {
			return fmt.Errorf("row edits require a table without vertical merges")
		}
		if len(rows) == 1 {
			return fmt.Errorf("cannot delete last row")
		}
		remove(r)
		return nil
	}
	cells := office.Children(r, nsW, "tc")
	col, ok := intArg(s["column"])
	if !ok || col < 0 || col >= len(cells) {
		return fmt.Errorf("column index out of range")
	}
	cell := cells[col]
	switch op {
	case "table-cell-text":
		text, ok := s["text"].(string)
		if !ok {
			return fmt.Errorf("text required")
		}
		for _, p := range cell.ChildElements() {
			if !is(p, "tcPr") {
				remove(p)
			}
		}
		p := W("p")
		run := W("r")
		addRunText(run, text)
		p.AddChild(run)
		cell.AddChild(p)
	case "table-cell-format":
		pr := property(cell, "tcPr")
		if fill := str(s, "fill"); fill != "" {
			if len(fill) != 6 {
				return fmt.Errorf("fill must be six hex digits")
			}
			if _, err := strconv.ParseUint(fill, 16, 24); err != nil {
				return err
			}
			setW(property(pr, "shd"), "fill", fill)
		}
		for _, p := range office.Children(cell, nsW, "p") {
			if err := formatParagraph(p, s); err != nil {
				return err
			}
			for _, r := range office.Children(p, nsW, "r") {
				if err := formatRun(r, s); err != nil {
					return err
				}
			}
		}
	default:
		return fmt.Errorf("unsupported table operation %q", op)
	}
	return nil
}
func editSection(root *etree.Element, s map[string]any) error {
	sections := office.WordSections(root)
	i, ok := intArg(s["section"])
	if !ok || i < 0 || i >= len(sections) {
		return fmt.Errorf("section index out of range")
	}
	sec := sections[i]
	for _, k := range []string{"top", "bottom", "left", "right", "header", "footer", "gutter"} {
		if v, ok := s[k]; ok {
			n, ok := intArg(v)
			if !ok || n < 0 {
				return fmt.Errorf("%s must be nonnegative twips", k)
			}
			setW(property(sec, "pgMar"), k, strconv.Itoa(n))
		}
	}
	for _, k := range []string{"width", "height"} {
		if v, ok := s[k]; ok {
			n, ok := intArg(v)
			if !ok || n <= 0 {
				return fmt.Errorf("%s must be positive twips", k)
			}
			attr := "w"
			if k == "height" {
				attr = "h"
			}
			setW(property(sec, "pgSz"), attr, strconv.Itoa(n))
		}
	}
	return nil
}

func insertWordProperty(parent, e *etree.Element) {
	if is(parent, "pPr") {
		insertInPPr(parent, e)
		return
	}
	orders := map[string][]string{
		"rPr":    {"rStyle", "rFonts", "b", "bCs", "i", "iCs", "caps", "smallCaps", "strike", "dstrike", "outline", "shadow", "emboss", "imprint", "noProof", "snapToGrid", "vanish", "webHidden", "color", "spacing", "w", "kern", "position", "sz", "szCs", "highlight", "u", "effect", "bdr", "shd", "fitText", "vertAlign", "rtl", "cs", "em", "lang", "eastAsianLayout", "specVanish", "oMath", "rPrChange"},
		"tcPr":   {"cnfStyle", "tcW", "gridSpan", "hMerge", "vMerge", "tcBorders", "shd", "noWrap", "tcMar", "textDirection", "tcFitText", "vAlign", "hideMark", "headers", "cellIns", "cellDel", "cellMerge", "tcPrChange"},
		"sectPr": {"headerReference", "footerReference", "footnotePr", "endnotePr", "type", "pgSz", "pgMar", "paperSrc", "pgBorders", "lnNumType", "pgNumType", "cols", "formProt", "vAlign", "noEndnote", "titlePg", "textDirection", "bidi", "rtlGutter", "docGrid", "printerSettings", "sectPrChange"},
	}
	order := orders[parent.Tag]
	ranks := map[string]int{}
	for i, n := range order {
		ranks[n] = i
	}
	rank, known := ranks[e.Tag]
	if known {
		for _, c := range parent.ChildElements() {
			if r, ok := ranks[c.Tag]; ok && r > rank {
				parent.InsertChildAt(c.Index(), e)
				return
			}
		}
	}
	parent.AddChild(e)
}

// A deleted paragraph may contain complete fields, but not a start, end or
// separator belonging to a field that extends into another paragraph.
func paragraphContainsCompleteFields(p *etree.Element) bool {
	depth := 0
	for _, marker := range all(p, "fldChar") {
		switch getW(marker, "fldCharType") {
		case "begin":
			depth++
		case "end":
			if depth == 0 {
				return false
			}
			depth--
		case "separate":
			if depth == 0 {
				return false
			}
		}
	}
	return depth == 0
}

func updateFieldDepth(e *etree.Element, depth *int) {
	switch getW(e, "fldCharType") {
	case "begin":
		*depth++
	case "end":
		if *depth > 0 {
			*depth--
		}
	}
}

// Complex fields can span paragraphs. Account for field markers before the
// selected paragraph, not just those inside it.
func fieldDepthBefore(p *etree.Element) int {
	root := p
	for root.Parent() != nil {
		root = root.Parent()
	}
	depth := 0
	var visit func(*etree.Element) bool
	visit = func(e *etree.Element) bool {
		if e == p {
			return true
		}
		if is(e, "fldChar") {
			updateFieldDepth(e, &depth)
		}
		for _, c := range e.ChildElements() {
			if visit(c) {
				return true
			}
		}
		return false
	}
	visit(root)
	return depth
}
