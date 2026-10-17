package office

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"github.com/openvibely/openvibely-paperworks/internal/testsupport"
)

// DocumentGapChecks.test_xlsx_saved_chart_cache_tracks_recalculated_cells_without_restyling.
func TestReferenceSavedChartCacheTracksRecalculation(t *testing.T) {
	source, engine := referenceChartCacheFixture(t, "saved-source"), referenceChartCacheFixture(t, "saved-engine")
	report, e := PackageReport(source)
	if e != nil {
		t.Fatal(e)
	}
	merged, e := PreserveFormulaCaches(source, previewRead(t, engine), report["formulas"].([]Formula))
	if e != nil {
		t.Fatal(e)
	}
	resultPath := filepath.Join(t.TempDir(), "result.xlsx")
	if err := os.WriteFile(resultPath, merged, 0600); err != nil {
		t.Fatal(err)
	}
	if got := referenceCachedScalar(t, resultPath, "xl/worksheets/sheet1.xml", "B1"); got != float64(6) {
		t.Fatalf("B1=%#v, want numeric 6", got)
	}
	p, e := opc.Read(merged)
	if e != nil {
		t.Fatal(e)
	}
	chart := previewXML(t, p, "xl/charts/chart1.xml")
	for _, tc := range []struct{ cache, want string }{{"numCache", "6"}, {"strCache", "Category"}} {
		nodes := All(chart, NSC, tc.cache)
		if len(nodes) == 0 {
			t.Fatal("missing", tc.cache)
		}
		values := All(nodes[0], NSC, "v")
		if len(values) != 1 || values[0].Text() != tc.want {
			t.Errorf("%s values %v, want %s", tc.cache, values, tc.want)
		}
	}
	style := Child(chart, NSC, "style")
	if style == nil || style.SelectAttrValue("val", "") != "12" {
		t.Error("style lost")
	}
	if !bytes.Equal(p.Get("opaque.bin"), []byte("retain exactly")) {
		t.Error("opaque part changed")
	}
}

// DocumentGapChecks.test_relocated_xlsx_chart_refresh_preserves_package_and_reopens.
func TestReferenceRelocatedChartCacheRefresh(t *testing.T) {
	for index, destination := range []string{"xl/charts/sales.xml", "xl/drawings/charts/sales.xml"} {
		t.Run(destination, func(t *testing.T) {
			source := referenceChartCacheFixture(t, fmt.Sprintf("relocated-%d", index))
			p, e := opc.Open(source)
			if e != nil {
				t.Fatal(e)
			}
			before := previewRead(t, source)
			old := previewOpen(t, source)
			if e := RefreshWorkbookChartCaches(p); e != nil {
				t.Fatal(e)
			}
			// Original reader opens the returned ZIP bytes, not the mutable package.
			refreshed, e := p.Bytes()
			if e != nil {
				t.Fatal(e)
			}
			p, e = opc.Read(refreshed)
			if e != nil {
				t.Fatal(e)
			}
			if !reflect.DeepEqual(old.Names(), p.Names()) {
				t.Error("package member order changed")
			}
			for _, n := range old.Names() {
				if n != destination && !bytes.Equal(old.Get(n), p.Get(n)) {
					t.Error("changed part", n)
				}
			}
			root := previewXML(t, p, destination)
			var values []string
			for _, val := range All(root, NSC, "val") {
				for _, ref := range All(val, NSC, "numRef") {
					for _, v := range All(Child(ref, NSC, "numCache"), NSC, "v") {
						values = append(values, v.Text())
					}
				}
			}
			if !reflect.DeepEqual(values, []string{"10", "20"}) {
				t.Error("refreshed values", values)
			}
			style := Child(root, NSC, "style")
			if style == nil || style.SelectAttrValue("val", "") != "12" {
				t.Error("style lost")
			}
			out := filepath.Join(t.TempDir(), "refreshed.xlsx")
			if e := p.SaveNew(out); e != nil {
				t.Fatal(e)
			}
			// Match load_workbook(...).active._charts rather than all chart parts.
			parts, e := testsupport.ActiveWorksheetCharts(previewRead(t, out))
			if e != nil || !reflect.DeepEqual(parts, []string{destination}) {
				t.Error("reopened active worksheet charts", parts, e)
			}
			if got := referenceCachedScalar(t, out, "xl/worksheets/sheet1.xml", "B2"); got != float64(10) {
				t.Fatalf("B2=%#v, want numeric 10", got)
			}
			previewUnchanged(t, source, before)
		})
	}
}
