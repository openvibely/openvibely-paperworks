package cli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func choiceFormPDF(flags int) []byte {
	objects := []string{
		"<< /Type /Catalog /Pages 2 0 R /AcroForm << /Fields [4 0 R] /DR << /Font << /Helv 5 0 R >> >> /DA (/Helv 12 Tf 0 g) >> >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 300 300] /Resources << >> /Annots [4 0 R] >>",
		fmt.Sprintf("<< /Type /Annot /Subtype /Widget /FT /Ch /T (choice) /Ff %d /Opt [(Alpha) (Beta)] /Rect [20 20 200 60] /P 3 0 R /DA (/Helv 12 Tf 0 g) >>", flags),
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
	}
	var b bytes.Buffer
	b.WriteString("%PDF-1.7\n")
	offsets := []int{0}
	for i, object := range objects {
		offsets = append(offsets, b.Len())
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", i+1, object)
	}
	start := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f \n", len(offsets))
	for _, offset := range offsets[1:] {
		fmt.Fprintf(&b, "%010d 00000 n \n", offset)
	}
	fmt.Fprintf(&b, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(offsets), start)
	return b.Bytes()
}

func TestFillCLIChoiceValidation(t *testing.T) {
	for _, flatten := range []bool{false, true} {
		for _, value := range []any{"Custom", []string{"Alpha", "Typo"}, "Beta", []string{"Alpha", "Beta"}, "", []string{}} {
			t.Run(fmt.Sprintf("%v/flatten=%v", value, flatten), func(t *testing.T) {
				dir := t.TempDir()
				source := filepath.Join(dir, "source.pdf")
				original := choiceFormPDF(1 << 21)
				if err := os.WriteFile(source, original, 0600); err != nil {
					t.Fatal(err)
				}
				spec := writeSpec(t, dir, "values.json", map[string]any{"choice": value})
				args := []string{"fill", source, "--values", spec}
				if flatten {
					args = append(args, "--flatten")
				}
				invalid := strings.Contains(fmt.Sprint(value), "Custom") || strings.Contains(fmt.Sprint(value), "Typo")
				output := filepath.Join(dir, "strict.pdf")
				strictArgs := append(append([]string{}, args...), "--output", output, "--strict-choices")
				var out, errout bytes.Buffer
				code := Main(strictArgs, &out, &errout)
				if invalid {
					if code == 0 {
						t.Fatal("strict fill accepted invalid value")
					}
					for _, text := range []string{"choice", "Alpha", "Beta"} {
						if !strings.Contains(errout.String(), text) {
							t.Fatal(errout.String())
						}
					}
					if _, err := os.Stat(output); !os.IsNotExist(err) {
						t.Fatal("output exists", err)
					}
				} else if code != 0 {
					t.Fatal(errout.String())
				}
				result := runCLI(t, append(args, "--output", filepath.Join(dir, "permissive.pdf"))...)
				if invalid {
					warnings, ok := result["warnings"].([]any)
					if !ok || len(warnings) != 1 {
						t.Fatal(result)
					}
					warning := warnings[0].(map[string]any)
					if warning["field"] != "choice" || warning["code"] != "unmatched_choice" {
						t.Fatal(warning)
					}
				}
				after, err := os.ReadFile(source)
				if err != nil || !bytes.Equal(original, after) {
					t.Fatal("source changed", err)
				}
			})
		}
	}
	var out, errout bytes.Buffer
	if code := Main([]string{"fill", "--help"}, &out, &errout); code != 0 || !strings.Contains(out.String(), "--strict-choices") {
		t.Fatalf("help: %d %s %s", code, &out, &errout)
	}
}
