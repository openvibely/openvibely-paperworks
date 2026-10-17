package cli

import (
	"bytes"
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
