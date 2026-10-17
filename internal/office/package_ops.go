package office

import (
	"fmt"
	"path/filepath"
	"sort"

	"github.com/beevik/etree"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
)

// RepairNotes restores a missing presentation notes-master list without
// re-serializing any other part.
func RepairNotes(source, destination string) (map[string]any, error) {
	if filepath.Clean(source) == filepath.Clean(destination) {
		return nil, fmt.Errorf("output must be a new PPTX file")
	}
	p, err := opc.Open(source)
	if err != nil {
		return nil, err
	}
	if _, err := PackageReportOf(p); err != nil {
		return nil, err
	}
	if err := RepairNotesPackage(p); err != nil {
		return nil, err
	}
	if err := p.SaveNew(destination); err != nil {
		return nil, err
	}
	r, err := PackageReport(destination)
	if err != nil {
		return nil, err
	}
	return map[string]any{"output": destination, "issues": r["issues"]}, nil
}

func RepairNotesPackage(p *opc.Package) error {
	rels, err := p.Rels("ppt/presentation.xml")
	if err != nil {
		return err
	}
	var masters []string
	for _, r := range rels {
		if len(r.Type) >= 12 && r.Type[len(r.Type)-12:] == "/notesMaster" && r.TargetMode != "External" {
			masters = append(masters, r.ID)
		}
	}
	if len(masters) > 1 {
		return fmt.Errorf("ambiguous notes masters require manual repair")
	}
	doc, err := p.XML("ppt/presentation.xml")
	if err != nil {
		return err
	}
	root := doc.Root()
	// minidom repair_notes searches descendants, even for malformed nesting.
	// Do not silently add a second list when an existing nested list is present.
	lists := All(root, NSP, "notesMasterIdLst")
	if len(lists) > 0 {
		var refs []string
		for _, e := range All(lists[0], NSP, "notesMasterId") {
			refs = append(refs, AttrV(e, NSR, "id"))
		}
		if len(lists) != 1 || fmt.Sprint(refs) != fmt.Sprint(masters) {
			return fmt.Errorf("existing notes-master list is inconsistent; refusing to replace it")
		}
		return nil
	}
	if len(masters) == 0 {
		return nil
	}
	prefix := root.Space
	group := etree.NewElement(qual(prefix, "notesMasterIdLst"))
	entry := group.CreateElement(qual(prefix, "notesMasterId"))
	entry.CreateAttr("xmlns:r", NSR)
	entry.CreateAttr("r:id", masters[0])
	idx := len(root.Child)
	for _, c := range root.ChildElements() {
		if c.Tag != "sldMasterIdLst" {
			idx = c.Index()
			break
		}
	}
	root.InsertChildAt(idx, group)
	raw, err := formulaCacheXML(doc)
	if err != nil {
		return err
	}
	p.Set("ppt/presentation.xml", raw)
	return nil
}

func qual(prefix, local string) string {
	if prefix == "" {
		return local
	}
	return prefix + ":" + local
}

// nsPrefix finds the prefix bound to ns in scope at e.
func nsPrefix(e *etree.Element, ns string) string {
	for x := e; x != nil; x = x.Parent() {
		for _, a := range x.Attr {
			if a.Space == "xmlns" && a.Value == ns {
				return a.Key
			}
		}
	}
	return ""
}

// VerifyPreservation rejects any package part change not explicitly allowed.
func VerifyPreservation(before, after string, allowed []string) (map[string]any, error) {
	a, err := PackageReport(before)
	if err != nil {
		return nil, err
	}
	b, err := PackageReport(after)
	if err != nil {
		return nil, err
	}
	first := a["package_parts"].(map[string]any)
	second := b["package_parts"].(map[string]any)
	allow := map[string]bool{}
	for _, n := range allowed {
		allow[n] = true
	}
	var changed, unexpected []string
	keys := map[string]bool{}
	for k := range first {
		keys[k] = true
	}
	for k := range second {
		keys[k] = true
	}
	preserved := 0
	for k := range keys {
		x, okx := first[k].(map[string]any)
		y, oky := second[k].(map[string]any)
		if okx && oky && x["sha256"] == y["sha256"] {
			preserved++
			continue
		}
		changed = append(changed, k)
		if !allow[k] {
			unexpected = append(unexpected, k)
		}
	}
	sort.Strings(changed)
	sort.Strings(unexpected)
	if len(unexpected) > 0 {
		return nil, fmt.Errorf("unexpected package changes: %v", unexpected)
	}
	if changed == nil {
		changed = []string{}
	}
	return map[string]any{"changed_parts": changed, "preserved_parts": preserved, "preservation_passed": true}, nil
}
