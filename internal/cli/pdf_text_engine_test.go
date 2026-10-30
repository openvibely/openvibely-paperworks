package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPDFTextExplicitEngineFailureDoesNotPublish(t *testing.T) {
	original := "../../testdata/reference-mirror/runtime-font-render.pdf"
	data, err := os.ReadFile(original)
	if err != nil {
		t.Fatal(err)
	}
	// Core Helvetica is supported now. Use an unknown font name of the same
	// byte length so the original xref offsets remain valid in this new fixture.
	if !bytes.Contains(data, []byte("/Helvetica")) {
		t.Fatal("missing source font")
	}
	data = bytes.ReplaceAll(data, []byte("/Helvetica"), []byte("/OtherFont"))
	src := filepath.Join(t.TempDir(), "unsupported-font.pdf")
	if err := os.WriteFile(src, data, 0600); err != nil {
		t.Fatal(err)
	}
	for _, engine := range []string{"pypdf", "invalid"} {
		t.Run(engine, func(t *testing.T) {
			output := filepath.Join(t.TempDir(), "text.txt")
			var out, errs bytes.Buffer
			code := Main([]string{"pdf-text", src, "--engine", engine, "--output", output}, &out, &errs)
			if code == 0 {
				t.Fatalf("unsupported engine/input succeeded: %s", out.String())
			}
			if _, err := os.Stat(output); !os.IsNotExist(err) {
				t.Fatalf("failure published output: %v", err)
			}
			want := "unsupported Type1 font"
			if engine == "invalid" {
				want = "engine must be"
			}
			if !strings.Contains(errs.String(), want) {
				t.Fatalf("missing rejection: %s", errs.String())
			}
		})
	}
	// The same unsupported Type1 document remains readable by the explicit
	// PDFium engine and the existing default; no error-triggered fallback occurs.
	var out, errs bytes.Buffer
	if code := Main([]string{"pdf-text", src, "--engine", "pdfium"}, &out, &errs); code != 0 || !strings.Contains(out.String(), "Heading") {
		t.Fatalf("PDFium %d: %s %s", code, out.String(), errs.String())
	}
}

func TestPDFTextPageSelectionAndOutput(t *testing.T) {
	src := writePDFTextFixture(t, false)
	cases := []struct {
		name       string
		selector   string
		wantText   []string
		wantNumber []int
	}{
		{name: "single page", selector: "2", wantText: []string{"Page two"}, wantNumber: []int{2}},
		{name: "noncontiguous and range order", selector: "3,1-2", wantText: []string{"Page three", "Page one", "Page two"}, wantNumber: []int{3, 1, 2}},
		{name: "omitted selector", wantText: []string{"Page one", "Page two", "Page three"}, wantNumber: []int{1, 2, 3}},
	}
	for _, engine := range []string{"pdfium", "pypdf"} {
		for _, tc := range cases {
			t.Run(engine+"/"+tc.name, func(t *testing.T) {
				args := []string{"pdf-text", src, "--engine", engine}
				if tc.selector != "" {
					args = append(args, "--pages", tc.selector)
				}
				jsonResult, stdout := runPDFTextCommand(t, args)
				gotText := jsonStrings(jsonResult["pages"])
				gotNumbers := jsonNumbers(jsonResult["page_numbers"])
				if len(gotText) != len(tc.wantText) || len(gotNumbers) != len(tc.wantNumber) {
					t.Fatalf("JSON result pages=%v page_numbers=%v", gotText, gotNumbers)
				}
				for i := range tc.wantText {
					if !strings.Contains(gotText[i], tc.wantText[i]) || gotNumbers[i] != tc.wantNumber[i] {
						t.Fatalf("JSON page %d = %q from source page %d; want %q from %d (stdout %s)", i, gotText[i], gotNumbers[i], tc.wantText[i], tc.wantNumber[i], stdout)
					}
				}

				output := filepath.Join(t.TempDir(), "selected.txt")
				outputArgs := append(append([]string(nil), args...), "--output", output)
				outputResult, _ := runPDFTextCommand(t, outputArgs)
				if got := jsonNumbers(outputResult["page_numbers"]); len(got) != len(tc.wantNumber) {
					t.Fatalf("output result page_numbers = %v", got)
				} else {
					for i := range got {
						if got[i] != tc.wantNumber[i] {
							t.Fatalf("output page_numbers = %v, want %v", got, tc.wantNumber)
						}
					}
				}
				file, err := os.ReadFile(output)
				if err != nil {
					t.Fatal(err)
				}
				if want := strings.Join(gotText, "\f"); string(file) != want {
					t.Fatalf("output text = %q, want form-feed-separated JSON page text %q", string(file), want)
				}
			})
		}
	}
}

func TestPDFTextMalformedOrOutOfRangeSelectionDoesNotPublish(t *testing.T) {
	src := writePDFTextFixture(t, false)
	cases := []struct {
		name string
		args []string
	}{
		{name: "malformed", args: []string{"--pages", "2,,3"}},
		{name: "out of range", args: []string{"--pages", "4"}},
		{name: "empty separate value", args: []string{"--pages", ""}},
		{name: "empty equals value", args: []string{"--pages="}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			output := filepath.Join(t.TempDir(), "must-not-exist.txt")
			args := append([]string{"pdf-text", src, "--engine", "pdfium"}, tc.args...)
			args = append(args, "--output", output)
			var stdout, stderr bytes.Buffer
			code := Main(args, &stdout, &stderr)
			if code == 0 {
				t.Fatalf("selection %v succeeded: %s", tc.args, stdout.String())
			}
			if !strings.Contains(stderr.String(), "invalid page selection") {
				t.Fatalf("selection error is unclear: %s", stderr.String())
			}
			if _, err := os.Stat(output); !os.IsNotExist(err) {
				t.Fatalf("invalid selection wrote output: %v", err)
			}
		})
	}
}

func TestPDFTextSelectedPypdfSkipsUnsupportedUnselectedPages(t *testing.T) {
	src := writePDFTextFixture(t, true)
	result, stdout := runPDFTextCommand(t, []string{"pdf-text", src, "--engine", "pypdf", "--pages", "3,2"})
	if got, want := jsonStrings(result["pages"]), []string{"Page three", "Page two"}; len(got) != len(want) || !strings.Contains(got[0], want[0]) || !strings.Contains(got[1], want[1]) {
		t.Fatalf("selected supported pages = %v (stdout %s), want %v", got, stdout, want)
	}
	if got, want := jsonNumbers(result["page_numbers"]), []int{3, 2}; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("source page numbers = %v, want %v", got, want)
	}

	output := filepath.Join(t.TempDir(), "unsupported-selected.txt")
	var out, errs bytes.Buffer
	code := Main([]string{"pdf-text", src, "--engine", "pypdf", "--pages", "1", "--output", output}, &out, &errs)
	if code == 0 || !strings.Contains(errs.String(), "pypdf text page 1: font F: unsupported Type1 font") {
		t.Fatalf("selected unsupported page error: code=%d stdout=%s stderr=%s", code, out.String(), errs.String())
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Fatalf("unsupported selected page wrote output: %v", err)
	}
}

func runPDFTextCommand(t *testing.T, args []string) (map[string]any, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	if code := Main(args, &stdout, &stderr); code != 0 {
		t.Fatalf("pdf-text command failed (%d): %s; stdout: %s", code, stderr.String(), stdout.String())
	}
	var result map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode result %q: %v", stdout.String(), err)
	}
	return result, stdout.String()
}

func jsonStrings(value any) []string {
	items, _ := value.([]any)
	out := make([]string, len(items))
	for i, item := range items {
		out[i], _ = item.(string)
	}
	return out
}

func jsonNumbers(value any) []int {
	items, _ := value.([]any)
	out := make([]int, len(items))
	for i, item := range items {
		n, _ := item.(float64)
		out[i] = int(n)
	}
	return out
}

func writePDFTextFixture(t *testing.T, unsupportedFirstPage bool) string {
	t.Helper()
	fontOne := "/Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding"
	if unsupportedFirstPage {
		fontOne = "/Type /Font /Subtype /Type1 /BaseFont /OtherFont /Encoding /WinAnsiEncoding"
	}
	objects := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R 4 0 R 5 0 R] /Count 3 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 100] /Resources << /Font << /F 6 0 R >> >> /Contents 8 0 R >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 100] /Resources << /Font << /F 7 0 R >> >> /Contents 9 0 R >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 100] /Resources << /Font << /F 7 0 R >> >> /Contents 10 0 R >>",
		"<< " + fontOne + " >>",
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>",
		pdfTextStream("Page one"),
		pdfTextStream("Page two"),
		pdfTextStream("Page three"),
	}
	data := rawPDFText(objects)
	path := filepath.Join(t.TempDir(), "pages.pdf")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func pdfTextStream(text string) string {
	body := "BT /F 12 Tf 10 50 Td (" + text + ") Tj ET"
	return fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(body), body)
}

func rawPDFText(objects []string) []byte {
	var b bytes.Buffer
	b.WriteString("%PDF-1.7\n%\xe2\xe3\xcf\xd3\n")
	offsets := make([]int, len(objects))
	for i, object := range objects {
		offsets[i] = b.Len()
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", i+1, object)
	}
	xref := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f \n", len(objects)+1)
	for _, offset := range offsets {
		fmt.Fprintf(&b, "%010d 00000 n \n", offset)
	}
	fmt.Fprintf(&b, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objects)+1, xref)
	return b.Bytes()
}
