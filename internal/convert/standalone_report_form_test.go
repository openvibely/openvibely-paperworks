package convert

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/openvibely/openvibely-paperworks/internal/pdf"
)

// Runtime 603-619: original report/form content, named styles, repeating header,
// aligned amounts and both labeled controls. Uses the captured original package.
func TestReferenceStandaloneRuntimeReportForm(t *testing.T) {
	referenceRuntimeOffice(t)
	dir := t.TempDir()
	must := func(e error) {
		t.Helper()
		if e != nil {
			t.Fatal(e)
		}
	}
	source := referenceRuntimeOriginalPackageFixture(t, "runtime-word-fixtures.zip", "report-form", ".docx")
	for _, c := range []struct{ tag, value string }{{"REVIEWER", "Fixture reviewer"}, {"DECISION", "Request the unobscured record"}} {
		source = referenceRuntimeWordEdit(t, source, filepath.Join(dir, "form-"+c.tag+".docx"), map[string]any{"operation": "control", "tag": c.tag, "marker": "[[" + c.tag + "]]", "text": c.value})
	}
	images, e := RenderFile(context.Background(), source, filepath.Join(dir, "render"), "", "", pdf.RenderOptions{})
	must(e)
	if len(images) == 0 {
		t.Error("report/form rendered no pages")
	}
}
