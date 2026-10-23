package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"github.com/xuri/excelize/v2"
)

func makeInspectSelectWorkbook(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "inspect.xlsx")
	f := excelize.NewFile()
	if err := f.SetSheetName("Sheet1", "Data"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.NewSheet("Other"); err != nil {
		t.Fatal(err)
	}
	for cell, value := range map[string]string{
		"A1": "Needle alpha",
		"C1": "needle beta",
		"A2": "needle gamma",
	} {
		if err := f.SetCellValue("Data", cell, value); err != nil {
			t.Fatal(err)
		}
	}
	if err := f.SetCellFormula("Data", "B1", "1+1"); err != nil {
		t.Fatal(err)
	}
	if err := f.SetCellFormula("Other", "A1", `="OutsideNeedle"`); err != nil {
		t.Fatal(err)
	}
	if err := f.SaveAs(path); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

func makeInspectSelectPresentation(t *testing.T, malformedSlide bool) string {
	t.Helper()
	pkg, err := opc.Open("../../testdata/reference-mirror/literal-chart.pptx")
	if err != nil {
		t.Fatal(err)
	}
	pkg.Set("custom/ignored.xml", []byte("<broken"))
	if malformedSlide {
		pkg.Set("ppt/slides/slide1.xml", []byte("<broken"))
	}
	path := filepath.Join(t.TempDir(), "focused.pptx")
	if err := pkg.SaveNew(path); err != nil {
		t.Fatal(err)
	}
	return path
}

func runInspectSelectError(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := Main(args, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func TestInspectSelectSheetRangeReadsOnlyRequestedCells(t *testing.T) {
	input := makeInspectSelectWorkbook(t)

	// An unrelated XML package part makes full package inspection fail. The
	// focused read must still open the workbook and return the selected cells.
	pkg, err := opc.Open(input)
	if err != nil {
		t.Fatal(err)
	}
	pkg.Set("custom/ignored.xml", []byte("<broken"))
	focused := filepath.Join(t.TempDir(), "focused.xlsx")
	if err := pkg.SaveNew(focused); err != nil {
		t.Fatal(err)
	}
	if _, err := office.PackageReport(focused); err == nil {
		t.Fatal("fixture should fail full package inspection")
	}

	result := runCLI(t, "inspect-select", focused, "--sheet", "Data", "--range", "A1:B1")
	if result["sheet"] != "Data" || result["cache_warning"] != "Values may be stale until recalculated." {
		t.Fatal(result)
	}
	cells, ok := result["cells"].([]any)
	if !ok || len(cells) != 2 {
		t.Fatal(result)
	}
	first := cells[0].(map[string]any)
	if first["cell"] != "A1" || first["value"] != "Needle alpha" || first["formula"] != "" {
		t.Fatal(first)
	}
	second := cells[1].(map[string]any)
	if second["cell"] != "B1" || second["formula"] != "1+1" {
		t.Fatal(second)
	}
	if _, ok := result["formulas"]; ok {
		t.Fatalf("focused result contains full-report formulas: %v", result)
	}
}

func TestInspectSelectRangeSearchAndTruncation(t *testing.T) {
	input := makeInspectSelectWorkbook(t)
	result := runCLI(t, "inspect-select", input, "--sheet", "Data", "--range", "A1:C2", "--search", "needle", "--max-items", "1")
	matches, ok := result["matches"].([]any)
	if !ok || len(matches) != 1 || result["truncated"] != true {
		t.Fatal(result)
	}
	match := matches[0].(map[string]any)
	if match["path"] != "/cells/0/value" || match["value"] != "Needle alpha" {
		t.Fatal(match)
	}
}

func TestInspectSelectRequiresSheetAndRangeAndXLSX(t *testing.T) {
	input := makeInspectSelectWorkbook(t)
	textFile := filepath.Join(t.TempDir(), "input.csv")
	if err := os.WriteFile(textFile, []byte("data"), 0600); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		args []string
		want string
	}{
		{name: "missing range", args: []string{"inspect-select", input, "--sheet", "Data"}, want: "sheet and range must be supplied together"},
		{name: "missing sheet", args: []string{"inspect-select", input, "--range", "A1"}, want: "sheet and range must be supplied together"},
		{name: "non-XLSX", args: []string{"inspect-select", textFile, "--sheet", "Data", "--range", "A1"}, want: "sheet/range selection requires XLSX"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, stdout, stderr := runInspectSelectError(t, tc.args...)
			if code != ExitUsage || !strings.Contains(stderr, tc.want) {
				t.Fatalf("exit=%d stdout=%q stderr=%q, want %q", code, stdout, stderr, tc.want)
			}
		})
	}
}

func TestInspectSelectRejectsInvalidAndOversizedRanges(t *testing.T) {
	input := makeInspectSelectWorkbook(t)
	cases := []struct {
		rangeValue string
		want       string
	}{
		{rangeValue: "A1:not-a-cell", want: "invalid range"},
		{rangeValue: "B2:A1", want: "range must not be reversed"},
		{rangeValue: "A1:CW100", want: "range must contain 1..10000 cells"},
	}
	for _, tc := range cases {
		t.Run(tc.rangeValue, func(t *testing.T) {
			code, stdout, stderr := runInspectSelectError(t, "inspect-select", input, "--sheet", "Data", "--range", tc.rangeValue)
			if code != ExitUsage || !strings.Contains(stderr, tc.want) {
				t.Fatalf("exit=%d stdout=%q stderr=%q, want %q", code, stdout, stderr, tc.want)
			}
		})
	}
}

func TestInspectSelectPointerAndFullReportSearch(t *testing.T) {
	input := makeInspectSelectWorkbook(t)
	pointer := runCLI(t, "inspect-select", input, "--path", "/formulas/0")
	if pointer["formula"] != "1+1" {
		t.Fatalf("pointer selection = %v", pointer)
	}

	result := runCLI(t, "inspect-select", input, "--search", "OutsideNeedle")
	matches, ok := result["matches"].([]any)
	if !ok || len(matches) == 0 {
		t.Fatal(result)
	}
	for _, item := range matches {
		match := item.(map[string]any)
		if strings.Contains(strings.ToLower(match["path"].(string)), "/formulas/") && strings.Contains(strings.ToLower(match["value"].(string)), "outsideneedle") {
			return
		}
	}
	t.Fatal(result)
}

func TestInspectSelectPPTXSlideIgnoresMalformedUnrelatedXML(t *testing.T) {
	input := makeInspectSelectPresentation(t, false)
	if _, err := office.PackageReport(input); err == nil {
		t.Fatal("full inspection should reject the malformed unrelated XML part")
	}

	selection := runCLI(t, "inspect-select", input, "--path", "/slides/0")
	if selection["part"] != "ppt/slides/slide1.xml" {
		t.Fatalf("selected part = %v", selection["part"])
	}
	slideXML, ok := selection["xml"].(string)
	if !ok || !strings.Contains(slideXML, "<") || !strings.Contains(slideXML, "sld") {
		t.Fatalf("selected slide XML = %v", selection["xml"])
	}
}

func TestInspectSelectPPTXSlideReportsMalformedRequiredXML(t *testing.T) {
	input := makeInspectSelectPresentation(t, true)
	code, stdout, stderr := runInspectSelectError(t, "inspect-select", input, "--path", "/slides/0")
	if code == 0 || !strings.Contains(stderr, "ppt/slides/slide1.xml") {
		t.Fatalf("exit=%d stdout=%q stderr=%q; want selected slide part error", code, stdout, stderr)
	}
}

func TestSelectJSONPointerEscaping(t *testing.T) {
	segments, err := decodeJSONPointer("/a~1b/~0key")
	if err != nil {
		t.Fatal(err)
	}
	value, err := selectJSONPointer(map[string]any{"a/b": map[string]any{"~key": "selected"}}, segments)
	if err != nil || value != "selected" {
		t.Fatalf("selection = %v, error = %v", value, err)
	}
}
