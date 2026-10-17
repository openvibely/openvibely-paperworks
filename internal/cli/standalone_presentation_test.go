package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/openvibely/openvibely-paperworks/internal/testsupport"
)

// Original three-row standalone table, with the same dimensions and values.
func referenceStandaloneTable(t *testing.T, values []string, font bool) string {
	t.Helper()
	name := ""
	if font {
		name = "font-contract"
	} else {
		switch strings.Join(values, "|") {
		case "10|20|30":
			name = "sums"
		case "10|20|31":
			name = "bad-sum"
		case "33.3%|66.7%|100.0%":
			name = "percent-sum"
		case "33.3%|66.7%|99.0%":
			name = "bad-percent"
		default:
			t.Fatal("unknown original table values", values)
		}
	}
	return testsupport.OriginalPackageFixture(t, "library-report-fixtures.zip", name, ".pptx")
}

func TestReferenceStandaloneLibrariesSlideRequirements(t *testing.T) {
	requirements := func() map[string]any {
		return map[string]any{"slides": 1, "native_tables": []int{1}, "size_emu": []int{9144000, 6858000}, "table_sums": []any{map[string]any{"slide": 1, "table": 1, "inputs": [][]int{{0, 0}, {1, 0}}, "total": []int{2, 0}}}}
	}
	for _, c := range []struct {
		name   string
		values []string
		bad    string
	}{{"integers", []string{"10", "20", "30"}, ""}, {"wrong-count", []string{"10", "20", "30"}, "slides"}, {"missing-chart", []string{"10", "20", "30"}, "native_charts"}, {"wrong-total", []string{"10", "20", "31"}, "sum"}, {"percentages", []string{"33.3%", "66.7%", "100.0%"}, ""}, {"wrong-percent", []string{"33.3%", "66.7%", "99.0%"}, "sum"}, {"no-total-contract", []string{"33.3%", "66.7%", "99.0%"}, "none"}} {
		t.Run(c.name, func(t *testing.T) {
			src := referenceStandaloneTable(t, c.values, false)
			req := requirements()
			if c.bad == "slides" {
				req["slides"] = 2
			}
			if c.bad == "native_charts" {
				req["native_charts"] = []int{1}
			}
			if c.bad == "none" {
				req = map[string]any{"native_tables": []int{1}}
			}
			spec := writeSpec(t, t.TempDir(), "requirements.json", req)
			if c.bad == "" || c.bad == "none" {
				r := runCLI(t, "verify-slides", src, "--requirements", spec)
				if r["requirements_passed"] != true {
					t.Error(r)
				}
			} else {
				var out, err bytes.Buffer
				if Main([]string{"verify-slides", src, "--requirements", spec}, &out, &err) != 4 {
					t.Error("invalid slide requirements accepted", out.String())
				}
			}
		})
	}
	t.Run("paragraph-font", func(t *testing.T) {
		src := referenceStandaloneTable(t, []string{"33.3%", "66.7%", "99.0%"}, true)
		spec := writeSpec(t, t.TempDir(), "bad-font.json", map[string]any{"fonts": []string{"Arial"}})
		var out, err bytes.Buffer
		if Main([]string{"verify-slides", src, "--requirements", spec}, &out, &err) != 4 || !strings.Contains(err.String(), "Courier New") {
			t.Errorf("paragraph font rejection: %s %s", out.String(), err.String())
		}
		spec = writeSpec(t, t.TempDir(), "good-font.json", map[string]any{"fonts": []string{"Courier New"}})
		r := runCLI(t, "verify-slides", src, "--requirements", spec)
		found := false
		for _, item := range r["font_definitions"].([]any) {
			found = found || strings.HasPrefix(item.(map[string]any)["part"].(string), "ppt/theme/")
		}
		if !found {
			t.Error("theme font definitions absent", r)
		}
	})
}

func TestReferenceStandaloneLibrariesChartCache(t *testing.T) {
	dir := t.TempDir()
	src := testsupport.OriginalPackageFixture(t, "library-report-fixtures.zip", "chart", ".pptx")
	requirements := writeSpec(t, dir, "requirements.json", map[string]any{"native_charts": []int{1}})
	t.Run("verified-data", func(t *testing.T) {
		r := runCLI(t, "verify-slides", src, "--requirements", requirements)
		if len(r["chart_data"].([]any)) == 0 {
			t.Error(r)
		}
	})
	// Original bad-cache input preserves ElementTree mutation/serialization.
	bad := testsupport.OriginalPackageFixture(t, "library-report-fixtures.zip", "bad-cache", ".pptx")
	t.Run("stale-cache", func(t *testing.T) {
		empty := writeSpec(t, t.TempDir(), "empty.json", map[string]any{})
		var out, err bytes.Buffer
		if Main([]string{"verify-slides", bad, "--requirements", empty}, &out, &err) != 4 || !strings.Contains(err.String(), "chart cache differs") {
			t.Errorf("cache error: %s %s", out.String(), err.String())
		}
	})
}
