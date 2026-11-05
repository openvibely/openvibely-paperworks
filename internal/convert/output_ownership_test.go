package convert

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestConvertExportFailureCleansOwnedOutputAndAllowsRetry(t *testing.T) {
	for _, extension := range []string{"docx", "pptx"} {
		t.Run(extension, func(t *testing.T) {
			var source string
			if extension == "docx" {
				source = referencePackage(t, "source.docx", map[string]string{
					"[Content_Types].xml": `<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"/>`,
					"word/document.xml":   `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body/></w:document>`,
				})
			} else {
				source = referencePackage(t, "source.pptx", map[string]string{
					"[Content_Types].xml":  `<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"/>`,
					"ppt/presentation.xml": `<p:presentation xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main"><p:sldSz cx="9144000" cy="5143500"/></p:presentation>`,
				})
			}
			output := filepath.Join(t.TempDir(), "converted")
			failure := errors.New("export failed")
			calls := 0
			ctx := withDeps(dependencies{export: func(_ context.Context, _, staged, target string) (string, error) {
				calls++
				partial := filepath.Join(staged, "partial."+target)
				if calls == 1 {
					if err := os.WriteFile(partial, []byte("partial export"), 0o600); err != nil {
						t.Fatal(err)
					}
					return partial, failure
				}
				if err := os.WriteFile(partial, []byte("complete export"), 0o600); err != nil {
					t.Fatal(err)
				}
				return partial, nil
			}})

			if _, err := Convert(ctx, source, output, Options{}); !errors.Is(err, failure) {
				t.Fatalf("first conversion error = %v, want %v", err, failure)
			}
			if _, err := os.Lstat(output); !os.IsNotExist(err) {
				t.Fatalf("failed output directory remains: %v", err)
			}

			report, err := Convert(ctx, source, output, Options{})
			if err != nil {
				t.Fatalf("retry failed: %v", err)
			}
			if calls != 2 {
				t.Fatalf("export called %d times, want 2", calls)
			}
			got := report["output"].(string)
			if got != filepath.Join(output, "source.pdf") {
				t.Fatalf("output path = %q", got)
			}
			if data, err := os.ReadFile(got); err != nil || string(data) != "complete export" {
				t.Fatalf("successful export = %q, err %v", data, err)
			}
		})
	}
}

func TestConvertXLSXRefreshFailureCleansPartialOutputAndAllowsRetry(t *testing.T) {
	source := referenceBook(t, "sheet1.xml", "Forecast", `<c r="A1"><v>7</v></c>`, false)
	sourceBytes := contents(t, source)
	output := filepath.Join(t.TempDir(), "refreshed")
	failure := errors.New("refresh failed")
	calls := 0
	ctx := withDeps(dependencies{refresh: func(_ context.Context, _, destination string, _ bool) error {
		calls++
		if calls == 1 {
			if err := os.WriteFile(filepath.Join(filepath.Dir(destination), "partial.xlsx"), []byte("partial refresh"), 0o600); err != nil {
				t.Fatal(err)
			}
			return failure
		}
		return os.WriteFile(destination, sourceBytes, 0o600)
	}})

	if _, err := Convert(ctx, source, output, Options{Refresh: true}); !errors.Is(err, failure) {
		t.Fatalf("first conversion error = %v, want %v", err, failure)
	}
	if _, err := os.Lstat(output); !os.IsNotExist(err) {
		t.Fatalf("failed output directory remains: %v", err)
	}

	report, err := Convert(ctx, source, output, Options{Refresh: true})
	if err != nil {
		t.Fatalf("retry failed: %v", err)
	}
	if calls != 2 {
		t.Fatalf("refresh called %d times, want 2", calls)
	}
	got := report["output"].(string)
	if got != filepath.Join(output, "source.xlsx") {
		t.Fatalf("output path = %q", got)
	}
	if data, err := os.ReadFile(got); err != nil || string(data) != string(sourceBytes) {
		t.Fatalf("successful refresh output mismatch: err %v", err)
	}
}

func TestConvertXLSXRecalculationValidationFailureCleansOutput(t *testing.T) {
	source := referenceBook(t, "sheet1.xml", "Forecast", `<c r="B2"><f>A2*2</f><v>4</v></c>`, false)
	converted := referenceBook(t, "sheet3.xml", "Forecast", `<c r="B2"><v>4</v></c>`, false)
	output := filepath.Join(t.TempDir(), "recalculated")
	ctx := withDeps(dependencies{refresh: func(_ context.Context, _, destination string, _ bool) error {
		if err := os.WriteFile(filepath.Join(filepath.Dir(destination), "partial.xlsx"), []byte("partial refresh"), 0o600); err != nil {
			return err
		}
		return os.WriteFile(destination, contents(t, converted), 0o600)
	}})

	if _, err := Convert(ctx, source, output, Options{Recalculate: true}); err == nil {
		t.Fatal("expected recalculation validation failure")
	}
	if _, err := os.Lstat(output); !os.IsNotExist(err) {
		t.Fatalf("failed output directory remains: %v", err)
	}
}

func TestRemoveOwnedOutputDirPreservesReplacementPath(t *testing.T) {
	root := t.TempDir()
	output := filepath.Join(root, "output")
	replacement := filepath.Join(root, "replacement")
	identity, err := createOutputDir(output)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(replacement, 0o755); err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(replacement, "sentinel")
	want := []byte("unrelated data")
	if err := os.WriteFile(sentinel, want, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(output); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(replacement, output); err != nil {
		t.Fatal(err)
	}
	if err := removeOwnedOutputDir(output, identity); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(sentinel); err != nil || string(got) != string(want) {
		t.Fatalf("replacement path changed: got %q, err %v", got, err)
	}
}

func TestConvertPreservesPreExistingOutputDirectory(t *testing.T) {
	source := referenceBook(t, "sheet1.xml", "Forecast", `<c r="A1"><v>7</v></c>`, false)
	output := filepath.Join(t.TempDir(), "existing")
	sentinel := filepath.Join(output, "nested", "sentinel.bin")
	want := []byte{0, 1, 2, 0xff, '\n'}
	if err := os.MkdirAll(filepath.Dir(sentinel), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sentinel, want, 0o600); err != nil {
		t.Fatal(err)
	}
	ctx := withDeps(dependencies{refresh: func(context.Context, string, string, bool) error {
		t.Fatal("conversion ran for an existing output path")
		return nil
	}})

	if _, err := Convert(ctx, source, output, Options{Refresh: true}); !errors.Is(err, os.ErrExist) {
		t.Fatalf("conversion error = %v, want os.ErrExist", err)
	}
	if got, err := os.ReadFile(sentinel); err != nil || string(got) != string(want) {
		t.Fatalf("pre-existing sentinel changed: got %v, err %v", got, err)
	}
}

func TestConvertSuccessRetainsOutputDirectoryAndGeneratedFile(t *testing.T) {
	source := referencePackage(t, "source.docx", map[string]string{
		"[Content_Types].xml": `<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"/>`,
		"word/document.xml":   `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body/></w:document>`,
	})
	output := filepath.Join(t.TempDir(), "converted")
	ctx := withDeps(dependencies{export: func(_ context.Context, _, staged, target string) (string, error) {
		file := filepath.Join(staged, "result."+target)
		return file, os.WriteFile(file, []byte("pdf bytes"), 0o600)
	}})

	report, err := Convert(ctx, source, output, Options{})
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(output)
	if err != nil || !info.IsDir() {
		t.Fatalf("successful output directory missing: info %v, err %v", info, err)
	}
	got := report["output"].(string)
	if data, err := os.ReadFile(got); err != nil || string(data) != "pdf bytes" {
		t.Fatalf("generated output = %q, err %v", data, err)
	}
}
