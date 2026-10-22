package office

import (
	"crypto/rand"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/beevik/etree"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
)

const (
	NSTC   = "http://schemas.microsoft.com/office/spreadsheetml/2018/threadedcomments"
	relTR  = "http://schemas.microsoft.com/office/2017/10/relationships/"
	uuidRe = `^\{[0-9A-F-]+\}$`
)

type CommentSpec struct {
	Action   string  `json:"action"`
	Sheet    string  `json:"sheet"`
	Cell     string  `json:"cell"`
	ThreadID string  `json:"thread_id"`
	Author   string  `json:"author"`
	AuthorID *string `json:"author_id"`
	Text     string  `json:"text"`
}

type Thread struct {
	Sheet    string  `json:"sheet"`
	Cell     string  `json:"cell"`
	ID       string  `json:"id"`
	ParentID *string `json:"parent_id"`
	Author   string  `json:"author"`
	AuthorID string  `json:"author_id"`
	Text     string  `json:"text"`
	Resolved bool    `json:"resolved"`
	Date     string  `json:"date"`
}

func NewGUID() string {
	var b [16]byte
	rand.Read(b[:])
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return strings.ToUpper(fmt.Sprintf("{%x-%x-%x-%x-%x}", b[0:4], b[4:6], b[6:8], b[8:10], b[10:]))
}

func newRoot(tag, ns string) *etree.Document {
	d := etree.NewDocument()
	d.CreateProcInst("xml", `version="1.0" encoding="UTF-8" standalone="yes"`)
	r := d.CreateElement(tag)
	r.CreateAttr("xmlns", ns)
	return d
}

func uniquePart(p *opc.Package, folder, stem string) string {
	return p.UnusedName(folder+"/"+stem, ".xml")
}

func singleRel(p *opc.Package, owner, kind string) (string, error) {
	rels, err := p.Rels(owner)
	if err != nil {
		return "", err
	}
	found := ""
	for _, r := range rels {
		if r.Type != relTR+kind {
			continue
		}
		if found != "" {
			return "", errors.New("multiple " + kind + " parts")
		}
		if r.TargetMode == "External" {
			return "", errors.New("external comment relationship")
		}
		found = opc.Resolve(owner, r.Target)
		if !p.Has(found) {
			return "", errors.New("missing comment relationship target")
		}
	}
	return found, nil
}

func insertBeforeExt(root, e *etree.Element) {
	InsertBefore(root, e, []string{"extLst"})
}

type sheetThreads struct {
	owner, part string
	doc         *etree.Document
}

// WorkbookComments lists native threaded comments, or applies add, reply,
// resolve or reopen to p (which the caller saves to a new file).
func WorkbookComments(p *opc.Package, spec CommentSpec) ([]Thread, error) {
	action := spec.Action
	if action == "" {
		action = "list"
	}
	switch action {
	case "list", "add", "reply", "resolve", "reopen":
	default:
		return nil, errors.New("unknown comment action")
	}
	pp, err := singleRel(p, "xl/workbook.xml", "person")
	if err != nil {
		return nil, err
	}
	hadPeople := pp != ""
	var people *etree.Document
	if hadPeople {
		if people, err = p.XML(pp); err != nil {
			return nil, err
		}
	} else {
		pp = uniquePart(p, "xl/persons", "person")
		people = newRoot("personList", NSTC)
	}
	persons := map[string]*etree.Element{}
	for _, e := range Children(people.Root(), NSTC, "person") {
		id := e.SelectAttrValue("id", "")
		if id == "" || persons[id] != nil {
			return nil, errors.New("missing or duplicate person IDs")
		}
		persons[id] = e
	}
	order, _, byName, err := WorkbookSheets(p)
	if err != nil {
		return nil, err
	}
	sheets := map[string]*sheetThreads{}
	all := map[string]bool{}
	for _, name := range order {
		owner := byName[name]
		if owner == "" {
			return nil, errors.New("missing comment relationship target")
		}
		cp, err := singleRel(p, owner, "threadedComment")
		if err != nil {
			return nil, err
		}
		st := &sheetThreads{owner: owner, part: cp}
		if cp != "" {
			if st.doc, err = p.XML(cp); err != nil {
				return nil, err
			}
		} else {
			st.doc = newRoot("ThreadedComments", NSTC)
		}
		ids := map[string]*etree.Element{}
		items := Children(st.doc.Root(), NSTC, "threadedComment")
		for _, e := range items {
			id := e.SelectAttrValue("id", "")
			if id == "" || ids[id] != nil || all[id] {
				return nil, errors.New("duplicate/missing comment IDs")
			}
			ids[id] = e
			all[id] = true
		}
		for _, e := range items {
			if persons[e.SelectAttrValue("personId", "")] == nil {
				return nil, errors.New("unknown comment author")
			}
			if parent := e.SelectAttrValue("parentId", ""); parent != "" {
				pe := ids[parent]
				if pe == nil || pe.SelectAttrValue("parentId", "") != "" || pe.SelectAttrValue("ref", "") != e.SelectAttrValue("ref", "") {
					return nil, errors.New("invalid thread parent")
				}
			}
		}
		sheets[name] = st
	}
	if action != "list" {
		if err := editThread(p, spec, action, sheets, persons, people, pp, hadPeople); err != nil {
			return nil, err
		}
	}
	var out []Thread
	for _, name := range order {
		for _, e := range Children(sheets[name].doc.Root(), NSTC, "threadedComment") {
			t := Thread{Sheet: name, Cell: e.SelectAttrValue("ref", ""), ID: e.SelectAttrValue("id", ""),
				AuthorID: e.SelectAttrValue("personId", ""), Text: Text(e, NSTC, "text"),
				Resolved: Truthy(e.SelectAttrValue("done", "0"), false), Date: e.SelectAttrValue("dT", "")}
			if v := e.SelectAttrValue("parentId", ""); v != "" {
				t.ParentID = &v
			}
			if pe := persons[t.AuthorID]; pe != nil {
				t.Author = pe.SelectAttrValue("displayName", "")
			}
			out = append(out, t)
		}
	}
	return out, nil
}

var commentCellRe = regexp.MustCompile(`^\$?[A-Za-z]{1,3}\$?[1-9][0-9]*$`)

func editThread(p *opc.Package, spec CommentSpec, action string, sheets map[string]*sheetThreads, persons map[string]*etree.Element, people *etree.Document, pp string, hadPeople bool) error {
	st := sheets[spec.Sheet]
	if st == nil {
		return errors.New("unknown comment worksheet")
	}
	root := st.doc.Root()
	if st.part == "" {
		if action != "add" {
			return errors.New("thread does not exist")
		}
		st.part = uniquePart(p, "xl/threadedComments", "threadedComment")
		rel, err := relPath(st.owner, st.part)
		if err != nil {
			return err
		}
		if _, err := p.AddRel(st.owner, relTR+"threadedComment", rel, false); err != nil {
			return err
		}
	}
	var parent *etree.Element
	if action != "add" {
		for _, e := range Children(root, NSTC, "threadedComment") {
			if e.SelectAttrValue("id", "") == spec.ThreadID {
				parent = e
			}
		}
		if parent == nil || parent.SelectAttrValue("parentId", "") != "" {
			return errors.New("select a root thread ID")
		}
	}
	if action == "resolve" || action == "reopen" {
		parent.CreateAttr("done", map[bool]string{true: "1", false: "0"}[action == "resolve"])
	} else {
		if strings.TrimSpace(spec.Author) == "" {
			return errors.New("a visible author is required")
		}
		if strings.TrimSpace(spec.Text) == "" || len([]rune(spec.Text)) > 32767 {
			return errors.New("comment needs 1–32767 characters")
		}
		cell := spec.Cell
		if action != "add" {
			cell = parent.SelectAttrValue("ref", "")
		}
		if !commentCellRe.MatchString(cell) {
			return errors.New("comment requires one cell")
		}
		cell = strings.ToUpper(strings.ReplaceAll(cell, "$", ""))
		col, row, _ := SplitCell(cell)
		if row > 1048576 || col > 16384 {
			return errors.New("cell exceeds Excel limits")
		}
		if action == "add" {
			for _, e := range Children(root, NSTC, "threadedComment") {
				if e.SelectAttrValue("ref", "") == cell && e.SelectAttrValue("parentId", "") == "" {
					return errors.New("cell already has a thread; reply to its ID")
				}
			}
		}
		if parent != nil && Truthy(parent.SelectAttrValue("done", "0"), false) {
			return errors.New("Reopen the thread before replying")
		}
		pid := ""
		if spec.AuthorID != nil {
			pid = *spec.AuthorID
			if persons[pid] == nil || persons[pid].SelectAttrValue("displayName", "") != spec.Author {
				return errors.New("author ID/name mismatch")
			}
		} else {
			var matches []string
			for k, v := range persons {
				if v.SelectAttrValue("displayName", "") == spec.Author {
					matches = append(matches, k)
				}
			}
			if len(matches) > 1 {
				return errors.New("ambiguous author; supply author_id")
			}
			if len(matches) == 1 {
				pid = matches[0]
			} else {
				pid = NewGUID()
				person := etree.NewElement(qual(people.Root().Space, "person"))
				for _, kv := range [][2]string{{"displayName", spec.Author}, {"id", pid}, {"userId", pid}, {"providerId", "None"}} {
					person.CreateAttr(kv[0], kv[1])
				}
				insertBeforeExt(people.Root(), person)
				persons[pid] = person
				if err := p.SetXML(pp, people); err != nil {
					return err
				}
				if !hadPeople {
					rel, err := relPath("xl/workbook.xml", pp)
					if err != nil {
						return err
					}
					if _, err := p.AddRel("xl/workbook.xml", relTR+"person", rel, false); err != nil {
						return err
					}
				}
			}
		}
		e := etree.NewElement(qual(root.Space, "threadedComment"))
		e.CreateAttr("ref", cell)
		e.CreateAttr("dT", time.Now().UTC().Format("2006-01-02T15:04:05.000Z"))
		e.CreateAttr("personId", pid)
		e.CreateAttr("id", NewGUID())
		if parent != nil {
			e.CreateAttr("parentId", parent.SelectAttrValue("id", ""))
		}
		e.CreateElement(qual(root.Space, "text")).SetText(spec.Text)
		insertBeforeExt(root, e)
	}
	if err := p.SetXML(st.part, st.doc); err != nil {
		return err
	}
	for _, pc := range [][2]string{{pp, "person"}, {st.part, "threadedcomments"}} {
		if !p.Has(pc[0]) {
			continue
		}
		if err := p.EnsureContentType(pc[0], "application/vnd.ms-excel."+pc[1]+"+xml"); err != nil {
			return err
		}
	}
	return nil
}
