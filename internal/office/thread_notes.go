package office

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/beevik/etree"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
)

// legacyComments reads the worksheet's notes without assuming Excel's part names.
func legacyComments(p *opc.Package, owner string) (string, *etree.Document, error) {
	rels, err := p.Rels(owner)
	if err != nil {
		return "", nil, err
	}
	var part string
	for _, r := range rels {
		if r.Type != NSR+"/comments" {
			continue
		}
		if part != "" || r.TargetMode == "External" {
			return "", nil, fmt.Errorf("invalid legacy comments relationship")
		}
		part = opc.Resolve(owner, r.Target)
	}
	if part == "" {
		return "", nil, nil
	}
	d, err := p.XML(part)
	return part, d, err
}

func legacyNoteAt(d *etree.Document, cell string) *etree.Element {
	if d == nil {
		return nil
	}
	for _, e := range Children(Child(d.Root(), NSS, "commentList"), NSS, "comment") {
		if strings.EqualFold(strings.ReplaceAll(e.SelectAttrValue("ref", ""), "$", ""), cell) {
			return e
		}
	}
	return nil
}

// syncThreadNote maintains the compatibility note used by readers without
// threaded-comment support. tc= identifies the pair to Excel and SheetJS.
func syncThreadNote(p *opc.Package, st *sheetThreads, root *etree.Element, persons map[string]*etree.Element) error {
	part, d, err := legacyComments(p, st.owner)
	if err != nil {
		return err
	}
	if d == nil {
		part = uniquePart(p, "xl", "comments")
		d = newRoot("comments", NSS)
		if _, err = p.AddRel(st.owner, NSR+"/comments", opc.Relative(st.owner, part), false); err != nil {
			return err
		}
	}
	authors := EnsureChild(d.Root(), NSS, "authors", []string{"authors", "commentList", "extLst"})
	list := EnsureChild(d.Root(), NSS, "commentList", []string{"authors", "commentList", "extLst"})
	id, cell := root.SelectAttrValue("id", ""), root.SelectAttrValue("ref", "")
	note := legacyNoteAt(d, cell)
	author := "tc=" + id
	aid := -1
	for i, a := range Children(authors, NSS, "author") {
		if a.Text() == author {
			aid = i
			break
		}
	}
	if note != nil {
		n, e := strconv.Atoi(note.SelectAttrValue("authorId", ""))
		aa := Children(authors, NSS, "author")
		if e != nil || n < 0 || n >= len(aa) || aa[n].Text() != author {
			return fmt.Errorf("cell %s already has a legacy-only note or a different thread fallback", cell)
		}
	}
	if aid < 0 {
		aid = len(Children(authors, NSS, "author"))
		NewChild(authors, NSS, "author", nil).SetText(author)
	}
	if note == nil {
		note = NewChild(list, NSS, "comment", nil)
		note.CreateAttr("ref", cell)
	}
	note.CreateAttr("authorId", strconv.Itoa(aid))
	var texts []string
	for _, e := range Children(st.doc.Root(), NSTC, "threadedComment") {
		if e == root || e.SelectAttrValue("parentId", "") == id {
			texts = append(texts, persons[e.SelectAttrValue("personId", "")].SelectAttrValue("displayName", "")+":\n"+Text(e, NSTC, "text"))
		}
	}
	RemoveAll(note, NSS, "text")
	text := NewChild(note, NSS, "text", []string{"extLst"})
	t := NewChild(text, NSS, "t", nil)
	t.CreateAttr("xml:space", "preserve")
	t.SetText(strings.Join(texts, "\n\n"))
	if err = p.SetXML(part, d); err != nil {
		return err
	}
	if err = p.EnsureContentType(part, "application/vnd.openxmlformats-officedocument.spreadsheetml.comments+xml"); err != nil {
		return err
	}
	return ensureThreadNoteShape(p, st.owner, cell)
}

func ensureThreadNoteShape(p *opc.Package, owner, cell string) error {
	sheet, err := p.XML(owner)
	if err != nil {
		return err
	}
	legacy := Child(sheet.Root(), NSS, "legacyDrawing")
	var part, rid string
	rels, err := p.Rels(owner)
	if err != nil {
		return err
	}
	if legacy != nil {
		rid = AttrV(legacy, NSR, "id")
		for _, r := range rels {
			if r.ID == rid && r.Type == NSR+"/vmlDrawing" && r.TargetMode != "External" {
				part = opc.Resolve(owner, r.Target)
			}
		}
		if part == "" {
			return fmt.Errorf("invalid legacyDrawing relationship")
		}
	}
	var d *etree.Document
	if part != "" {
		d, err = p.XML(part)
		if err != nil {
			return err
		}
	} else {
		part = p.UnusedName("xl/drawings/vmlDrawing", ".vml")
		d = etree.NewDocument()
		if err = d.ReadFromString(`<xml xmlns:v="urn:schemas-microsoft-com:vml" xmlns:o="urn:schemas-microsoft-com:office:office" xmlns:x="urn:schemas-microsoft-com:office:excel"><v:shapetype id="_x0000_t202" coordsize="21600,21600" o:spt="202" path="m,l,21600r21600,l21600,xe"><v:stroke joinstyle="miter"/><v:path gradientshapeok="t" o:connecttype="rect"/></v:shapetype></xml>`); err != nil {
			return err
		}
		rid, err = p.AddRel(owner, NSR+"/vmlDrawing", opc.Relative(owner, part), false)
		if err != nil {
			return err
		}
		legacy = EnsureChild(sheet.Root(), NSS, "legacyDrawing", WorksheetOrder)
		legacy.CreateAttr("xmlns:r", NSR)
		legacy.CreateAttr("r:id", rid)
		if err = p.SetXML(owner, sheet); err != nil {
			return err
		}
	}
	const nsExcel = "urn:schemas-microsoft-com:office:excel"
	col, row, _ := SplitCell(cell)
	used := map[string]bool{}
	for _, e := range All(d.Root(), NSV, "shape") {
		used[e.SelectAttrValue("id", "")] = true
		cd := Child(e, nsExcel, "ClientData")
		if cd != nil && cd.SelectAttrValue("ObjectType", "") == "Note" && Text(cd, nsExcel, "Row") == strconv.Itoa(row-1) && Text(cd, nsExcel, "Column") == strconv.Itoa(col-1) {
			return nil
		}
	}
	hasNoteType := false
	for _, e := range All(d.Root(), NSV, "shapetype") {
		hasNoteType = hasNoteType || e.SelectAttrValue("id", "") == "_x0000_t202"
	}
	if !hasNoteType {
		typ := etree.NewDocument()
		if err := typ.ReadFromString(`<v:shapetype xmlns:v="urn:schemas-microsoft-com:vml" xmlns:o="urn:schemas-microsoft-com:office:office" id="_x0000_t202" coordsize="21600,21600" o:spt="202" path="m,l,21600r21600,l21600,xe"><v:stroke joinstyle="miter"/><v:path gradientshapeok="t" o:connecttype="rect"/></v:shapetype>`); err != nil {
			return err
		}
		d.Root().InsertChildAt(0, typ.Root())
	}
	shapeID := 1025
	for used[fmt.Sprintf("_x0000_s%d", shapeID)] {
		shapeID++
	}
	shape := etree.NewDocument()
	err = shape.ReadFromString(fmt.Sprintf(`<v:shape xmlns:v="urn:schemas-microsoft-com:vml" xmlns:o="urn:schemas-microsoft-com:office:office" xmlns:x="urn:schemas-microsoft-com:office:excel" id="_x0000_s%d" type="#_x0000_t202" style="position:absolute;width:144pt;height:79pt;z-index:1;visibility:hidden" fillcolor="#ffffe1" o:insetmode="auto"><v:fill color2="#ffffe1"/><v:shadow on="t" color="black" obscured="t"/><v:path o:connecttype="none"/><v:textbox style="mso-direction-alt:auto"><div style="text-align:left"/></v:textbox><x:ClientData ObjectType="Note"><x:MoveWithCells/><x:SizeWithCells/><x:Anchor>%d, 15, %d, 2, %d, 31, %d, 1</x:Anchor><x:AutoFill>False</x:AutoFill><x:Row>%d</x:Row><x:Column>%d</x:Column></x:ClientData></v:shape>`, shapeID, col-1, row-1, min(col+2, 16383), min(row+3, 1048575), row-1, col-1))
	if err != nil {
		return err
	}
	d.Root().AddChild(shape.Root())
	if err = p.SetXML(part, d); err != nil {
		return err
	}
	return p.EnsureContentType(part, "application/vnd.openxmlformats-officedocument.vmlDrawing")
}
