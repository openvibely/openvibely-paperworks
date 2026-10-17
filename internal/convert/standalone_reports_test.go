package convert

import (
	"bytes"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/openvibely/openvibely-paperworks/internal/office"
)

// test_document_libraries.py:84,86: use the original table and formula,
// including the deliberately absent table-header designation.
func TestReferenceStandaloneLibrariesPackageReports(t *testing.T) {
	t.Run("word-header", func(t *testing.T) {
		src := referenceRuntimeOriginalPackageFixture(t, "library-report-fixtures.zip", "doc", ".docx")
		r, e := office.PackageReport(src)
		if e != nil {
			t.Fatal(e)
		}
		found := false
		for _, s := range r["issues"].([]string) {
			found = found || strings.Contains(s, "header")
		}
		if !found {
			t.Errorf("missing header issue: %v", r["issues"])
		}
	})
	t.Run("formula", func(t *testing.T) {
		src := referenceRuntimeOriginalPackageFixture(t, "library-report-fixtures.zip", "book", ".xlsx")
		r, e := office.PackageReport(src)
		if e != nil {
			t.Fatal(e)
		}
		formulas := r["formulas"].([]office.Formula)
		if len(formulas) == 0 || (formulas[0].Formula == nil || *formulas[0].Formula != "A1*3") {
			t.Errorf("formulas: %v", formulas)
		}
	})
}

// test_document_libraries.py:234–243. Match reference DOCX library's shared image part
// explicitly, then inspect both placements and the full watermark lifecycle.
func TestReferenceStandaloneLibrariesImageWatermarks(t *testing.T) {
	dir := t.TempDir()
	src := referenceRuntimeOriginalPackageFixture(t, "runtime-word-fixtures.zip", "library-image-audit", ".docx")
	t.Run("image-report", func(t *testing.T) {
		r, e := office.PackageReport(src)
		if e != nil {
			t.Fatal(e)
		}
		images := r["images"].([]any)
		if len(images) != 2 {
			t.Fatalf("images: %v", images)
		}
		var placements []string
		for _, entry := range images {
			m := entry.(map[string]any)
			placements = append(placements, m["placement"].(string))
			if !reflect.DeepEqual(m["targets"], []string{"media/image1.png"}) {
				t.Errorf("targets: %v", m["targets"])
			}
		}
		if !reflect.DeepEqual(placements, []string{"inline", "anchor"}) {
			t.Error(placements)
		}
		if !strings.Contains(images[1].(map[string]any)["position"].(map[string]string)["positionH"], "914400") {
			t.Error("missing horizontal position")
		}
	})
	wm1 := referenceRuntimeWordEdit(t, src, filepath.Join(dir, "wm1.docx"), map[string]any{"operation": "watermark", "text": "DRAFT"})
	wm2 := referenceRuntimeWordEdit(t, wm1, filepath.Join(dir, "wm2.docx"), map[string]any{"operation": "watermark", "text": "PRIVATE"})
	original := contents(t, wm2) // Reference snapshots before the first report.
	texts := func(t *testing.T, path string) []string {
		t.Helper()
		r, e := office.PackageReport(path)
		if e != nil {
			t.Fatal(e)
		}
		var result []string
		for _, entry := range r["watermarks"].([]any) {
			result = append(result, entry.(map[string]any)["text"].(string))
		}
		return result
	}
	t.Run("combined-and-read-only", func(t *testing.T) {
		got := texts(t, wm2)
		sort.Strings(got)
		if !reflect.DeepEqual(got, []string{"DRAFT", "DRAFT", "DRAFT", "PRIVATE", "PRIVATE", "PRIVATE"}) {
			t.Error(got)
		}
	})
	t.Run("read-only", func(t *testing.T) {
		_ = texts(t, wm2)
		if !bytes.Equal(original, contents(t, wm2)) {
			t.Error("report modified source")
		}
	})
	t.Run("remove-draft", func(t *testing.T) {
		wm3 := referenceRuntimeWordEdit(t, wm2, filepath.Join(dir, "wm3.docx"), map[string]any{"operation": "watermark-remove", "contains": "DRAFT"})
		if got := texts(t, wm3); !reflect.DeepEqual(got, []string{"PRIVATE", "PRIVATE", "PRIVATE"}) {
			t.Error(got)
		}
	})
}

// Original empty textbox starts exactly at the right slide edge and extends an inch beyond it.
func TestReferenceStandaloneLibrariesSlideBounds(t *testing.T) {
	src := referenceRuntimeOriginalPackageFixture(t, "library-report-fixtures.zip", "deck", ".pptx")
	r, e := office.PackageReport(src)
	if e != nil {
		t.Fatal(e)
	}
	if len(r["issues"].([]string)) == 0 {
		t.Error("offslide textbox had no issues")
	}
}
