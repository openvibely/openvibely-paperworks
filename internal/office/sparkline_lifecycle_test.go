package office

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/beevik/etree"
)

// DocumentGapChecks.test_native_sparklines_create_edit_delete_preserve_workbook.
func TestReferenceSparklineLifecycle(t *testing.T) {
	source := referenceOriginalWorkbookFixture(t, "sparkline-fixtures.zip", "lifecycle")
	before := previewRead(t, source)
	edit := func(src, out, raw string) error {
		var spec SparklineSpec
		if err := json.Unmarshal([]byte(raw), &spec); err != nil {
			t.Fatal(err)
		}
		// The Reference wrapper inspects the source before editing and the saved
		// destination afterwards; report errors must not be bypassed.
		if _, err := PackageReport(src); err != nil {
			return err
		}
		if filepath.Ext(src) != ".xlsx" {
			return fmt.Errorf("Sparklines require XLSX")
		}
		if _, err := os.Stat(out); err == nil {
			return os.ErrExist
		} else if !os.IsNotExist(err) {
			return err
		}
		p := previewOpen(t, src)
		if _, err := EditSparklines(p, spec); err != nil {
			return err
		}
		if err := p.SaveNew(out); err != nil {
			return err
		}
		_, err := PackageReport(out)
		return err
	}
	groups := func(file string) []*etree.Element {
		t.Helper()
		z := referencePreviewZIP(t, file)
		d := etree.NewDocument()
		if err := d.ReadFromBytes(referencePreviewZIPRead(t, z, "xl/worksheets/sheet1.xml")); err != nil {
			t.Fatal(err)
		}
		return All(d.Root(), NSX14, "sparklineGroup")
	}
	dir := t.TempDir()
	var createdGroupCounts []int
	var dest string
	for _, kind := range []string{"line", "column", "stacked"} {
		dest = filepath.Join(dir, kind+".xlsx")
		if err := edit(source, dest, `{"sheet":"Data","sparklines":[{"cell":"D1","range":"'Data'!A1:C1"},{"cell":"D2","range":"'Data'!A1:C1"}],"options":{"type":"`+kind+`","high":true,"colorSeries":"112233","minAxisType":"custom","manualMin":0}}`); err != nil {
			t.Fatal(err)
		}
		gs := groups(dest)
		createdGroupCounts = append(createdGroupCounts, len(gs))
		if len(gs) == 0 {
			t.Fatalf("groups: %d", len(gs))
		}
		g := gs[0]
		if g.SelectAttrValue("type", "") != kind {
			t.Fatal("wrong type")
		}
		c := Child(g, NSX14, "colorSeries")
		if c == nil || c.SelectAttrValue("rgb", "") != "FF112233" {
			t.Fatal("series color")
		}
		if g.SelectAttrValue("high", "") != "1" {
			t.Fatal("high flag")
		}
		sparks := Child(g, NSX14, "sparklines")
		if sparks == nil || len(sparks.ChildElements()) != 2 {
			t.Fatal("spark count")
		}
		old, fresh := referencePreviewZIP(t, source), referencePreviewZIP(t, dest)
		for _, member := range old.File {
			if member.Name != "xl/worksheets/sheet1.xml" && !bytes.Equal(referencePreviewZIPRead(t, old, member.Name), referencePreviewZIPRead(t, fresh, member.Name)) {
				t.Fatal("changed part", member.Name)
			}
		}
	}
	changed := filepath.Join(dir, "edited.xlsx")
	if err := edit(dest, changed, `{"sheet":"Data","sparklines":[{"cell":"D1","range":"'Data'!A1:B1"}],"options":{"type":"line","markers":true}}`); err != nil {
		t.Fatal(err)
	}
	gs := groups(changed)
	if len(gs) != 2 {
		t.Fatalf("edited group count %d", len(gs))
	}
	found := false
	for _, g := range gs {
		for _, n := range All(g, NSX14Main, "f") {
			found = found || n.Text() == "'Data'!A1:B1"
		}
	}
	if !found {
		t.Fatal("edited source range missing")
	}
	deleted := filepath.Join(dir, "deleted.xlsx")
	if err := edit(changed, deleted, `{"sheet":"Data","action":"delete","sparklines":[{"cell":"D1"}]}`); err != nil {
		t.Fatal(err)
	}
	var cells []string
	for _, g := range groups(deleted) {
		for _, n := range All(g, NSX14Main, "sqref") {
			cells = append(cells, n.Text())
		}
	}
	if !reflect.DeepEqual(cells, []string{"D2"}) {
		t.Fatal(cells)
	}
	cleared := filepath.Join(dir, "cleared.xlsx")
	if err := edit(deleted, cleared, `{"sheet":"Data","action":"clear"}`); err != nil {
		t.Fatal(err)
	}
	if len(groups(cleared)) != 0 {
		t.Fatal("groups not cleared")
	}
	previewUnchanged(t, source, before)
	bad := filepath.Join(dir, "bad-spark.xlsx")
	if err := edit(source, bad, `{"sheet":"Data","sparklines":[{"cell":"D1","range":"'[external]Data'!A1:C1"}]}`); err == nil || !strings.Contains(err.Error(), "source") {
		t.Fatal("external source error", err)
	}
	if _, err := os.Stat(bad); !os.IsNotExist(err) {
		t.Fatal("bad output exists", err)
	}
	// Retain the former supplemental group-count checks after the original contract.
	if !reflect.DeepEqual(createdGroupCounts, []int{1, 1, 1}) {
		t.Fatal("created group counts", createdGroupCounts)
	}

}
