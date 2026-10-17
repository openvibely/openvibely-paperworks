package office

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openvibely/openvibely-paperworks/internal/opc"
)

// Pinned minidom repair searches descendants, while package_report checks the
// schema's direct list/entry path. Repair must not silently normalize malformed
// nesting or hide the issue subsequently reported by the inspector.
func TestNotesRepairNestedReferences(t *testing.T) {
	for _, mode := range []string{"nested-valid", "nested-wrong", "nested-entry", "unlinked-master", "external-master"} {
		t.Run(mode, func(t *testing.T) {
			p, err := opc.Open(referenceNotesFixture(t))
			if err != nil {
				t.Fatal(err)
			}
			rels, err := p.XML("ppt/_rels/presentation.xml.rels")
			if err != nil {
				t.Fatal(err)
			}
			var id string
			for _, r := range rels.Root().ChildElements() {
				if strings.HasSuffix(r.SelectAttrValue("Type", ""), "/notesMaster") {
					id = r.SelectAttrValue("Id", "")
					if mode == "unlinked-master" {
						rels.Root().RemoveChild(r)
					}
					if mode == "external-master" {
						r.CreateAttr("TargetMode", "External")
						r.CreateAttr("Target", "https://example.invalid/master")
					}
				}
			}
			if id == "" {
				t.Fatal("fixture has no master relationship")
			}
			if err = p.SetXML("ppt/_rels/presentation.xml.rels", rels); err != nil {
				t.Fatal(err)
			}
			d, err := p.XML("ppt/presentation.xml")
			if err != nil {
				t.Fatal(err)
			}
			for _, l := range Children(d.Root(), NSP, "notesMasterIdLst") {
				d.Root().RemoveChild(l)
			}
			if strings.HasPrefix(mode, "nested") {
				parent := d.Root()
				if mode != "nested-entry" {
					parent = parent.CreateElement("probe")
				}
				group := parent.CreateElement("p:notesMasterIdLst")
				if mode == "nested-entry" {
					group = group.CreateElement("probe")
				}
				entry := group.CreateElement("p:notesMasterId")
				if mode == "nested-wrong" {
					id = "absent"
				}
				entry.CreateAttr("r:id", id)
			}
			if err = p.SetXML("ppt/presentation.xml", d); err != nil {
				t.Fatal(err)
			}
			src := filepath.Join(t.TempDir(), "input.pptx")
			if err = p.SaveNew(src); err != nil {
				t.Fatal(err)
			}
			dst := filepath.Join(t.TempDir(), "output.pptx")
			r, err := RepairNotes(src, dst)
			if mode == "nested-wrong" {
				if err == nil {
					t.Fatal("inconsistent nested list accepted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			saved, err := opc.Open(dst)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(p.Get("ppt/presentation.xml"), saved.Get("ppt/presentation.xml")) {
				t.Fatal("existing or absent list rewritten")
			}
			issues := r["issues"].([]string)
			wantIssue := strings.HasPrefix(mode, "nested")
			found := false
			for _, s := range issues {
				if strings.Contains(s, "Invalid or missing presentation notes-master") {
					found = true
				}
				if strings.Contains(s, "Notes slides exist without") {
					t.Fatal("master part exists", s)
				}
			}
			if found != wantIssue {
				t.Fatalf("issues=%v want invalid list=%v", issues, wantIssue)
			}
		})
	}
}
