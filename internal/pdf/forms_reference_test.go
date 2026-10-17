package pdf

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Ports test_flatten_refreshes_stale_and_missing_text_appearances from the
// original test_pdf_forms.py. The fixture mirrors its fields and two pages;
// the refreshed interactive oracle here uses Go, not the Reference library.
func TestReferenceFlattenRefreshesStaleAndMissingTextAppearances(t *testing.T) {
	for _, kind := range []string{"stale", "missing"} {
		t.Run(kind, func(t *testing.T) {
			objs := formObjs()
			objs[12] = strings.Replace(objs[12], "/V (Original)", "/V (Current account)", 1)
			if kind == "missing" {
				objs[13] = strings.Replace(objs[13], "/AP << /N 27 0 R >>", "", 1)
			}
			before := rawPDF(objs)
			source := writeTemp(t, "source.pdf", before)
			dir := t.TempDir()
			refreshed := filepath.Join(dir, "refreshed.pdf")
			flat := filepath.Join(dir, "flat.pdf")
			ctx := context.Background()
			// Repaint every saved value, as the Reference reference oracle does.
			if _, err := Fill(ctx, source, refreshed, map[string]string{"answer": "/A", "approved": "/Yes", "name": "Current account", "choice": "Alpha"}, false); err != nil {
				t.Fatal(err)
			}
			if _, err := Fill(ctx, source, flat, nil, true); err != nil {
				t.Fatal(err)
			}
			report, err := Report(ctx, flat)
			if err != nil {
				t.Fatal(err)
			}
			if report.HasFormTree || len(report.Widgets) != 0 || len(report.Issues) != 0 {
				t.Fatalf("form remains: %+v", report)
			}
			data, err := os.ReadFile(flat)
			if err != nil {
				t.Fatal(err)
			}
			text, err := PageText(ctx, data)
			if err != nil {
				t.Fatal(err)
			}
			if len(text) != 2 || !strings.Contains(text[1], "Current account") || strings.Contains(text[1], "Original") || !strings.Contains(text[1], "Alpha") {
				t.Fatalf("text: %q", text)
			}
			oracle, err := os.ReadFile(refreshed)
			if err != nil {
				t.Fatal(err)
			}
			expected, actual := images(t, oracle), images(t, data)
			if len(expected) != 2 || len(actual) != 2 {
				t.Fatal("page count changed")
			}
			for i := range expected {
				if !sameImage(expected[i], actual[i]) {
					t.Fatalf("page %d appearance changed", i+1)
				}
			}
			after, err := os.ReadFile(source)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatal("source changed", err)
			}
		})
	}
}

// Maps test_flatten_without_edits_preserves_values_and_existing_resources.
func TestReferenceFlattenPreservesExistingXObject(t *testing.T) {
	objs := formObjs()
	objs[2] = strings.Replace(objs[2], "/Resources << /Font", "/Resources << /XObject << /Fm_widget_0 19 0 R >> /Font", 1)
	before := rawPDF(objs)
	source := writeTemp(t, "source.pdf", before)
	ctx := context.Background()
	out := filepath.Join(t.TempDir(), "flat.pdf")
	refreshed := filepath.Join(t.TempDir(), "refreshed.pdf")
	if _, err := Fill(ctx, source, refreshed, map[string]string{"answer": "/A", "approved": "/Yes", "name": "Original", "choice": "Alpha"}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := Fill(ctx, source, out, nil, true); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	c, err := Load(ctx, data)
	if err != nil {
		t.Fatal(err)
	}
	page, _, _, err := c.PageDict(ctx, 1, false)
	if err != nil {
		t.Fatal(err)
	}
	resources := dictOf(c.XRefTable, page["Resources"]).d
	xobjects := dictOf(c.XRefTable, resources["XObject"]).d
	retained, _, err := c.DereferenceStreamDict(xobjects["Fm_widget_0"])
	if err != nil || retained == nil {
		t.Fatal("lost existing XObject", err)
	}
	if err := retained.Decode(); err != nil {
		t.Fatal(err)
	}
	if string(retained.Content) != radioOff+" "+radioOn {
		t.Fatalf("replaced existing XObject: %q", retained.Content)
	}
	rep, err := ReportBytes(ctx, data)
	if err != nil || rep.HasFormTree || len(rep.Widgets) != 0 {
		t.Fatal(rep, err)
	}
	oracle, err := os.ReadFile(refreshed)
	if err != nil {
		t.Fatal(err)
	}
	expected, actual := images(t, oracle), images(t, data)
	if len(expected) != 2 || len(actual) != 2 {
		t.Fatal("page count changed")
	}
	for i := range expected {
		if !sameImage(expected[i], actual[i]) {
			t.Fatalf("page %d changed", i+1)
		}
	}
	after, err := os.ReadFile(source)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("source changed", err)
	}
}
