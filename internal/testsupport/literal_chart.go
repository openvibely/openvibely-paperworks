package testsupport

import (
	"os"
	"path/filepath"
	"testing"
)

// LiteralChartFixture is captured after the original chart_deck replace callback.
// Restore its same-path Store write in the caller's own temporary directory.
func LiteralChartFixture(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile("../../testdata/reference-mirror/literal-chart.pptx")
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "literal.pptx")
	if err = os.WriteFile(out, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if err = RewriteStoredZIP(out, func(*OrderedZIP) error { return nil }); err != nil {
		t.Fatal(err)
	}
	return out
}
