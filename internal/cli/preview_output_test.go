package cli

import (
	"bytes"
	"context"
	"errors"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestPreviewExportExistingOutput(t *testing.T) {
	for _, kind := range []string{"empty", "nonempty", "file"} {
		t.Run(kind, func(t *testing.T) {
			root := t.TempDir()
			// Invalid Office input proves rejection precedes conversion or rendering.
			source := filepath.Join(root, "source.xlsx")
			original := []byte("not an Office package")
			if err := os.WriteFile(source, original, 0600); err != nil {
				t.Fatal(err)
			}
			out := filepath.Join(root, "output")
			sentinel := out
			if kind != "file" {
				if err := os.Mkdir(out, 0700); err != nil {
					t.Fatal(err)
				}
				sentinel = filepath.Join(out, "keep")
			}
			if kind != "empty" {
				if err := os.WriteFile(sentinel, []byte("stale"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			_, err := commands["preview-export"].Run(context.Background(), Args{Pos: []string{source}, Flags: map[string]string{"output": out}})
			var usage UsageError
			if !errors.Is(err, os.ErrExist) || !errors.As(err, &usage) || err.Error() != "output must be a new directory" {
				t.Fatalf("expected unchanged usage diagnostic with ErrExist identity, got %v", err)
			}
			var stdout, stderr bytes.Buffer
			if code := Main([]string{"preview-export", source, "--output", out}, &stdout, &stderr); code != ExitUsage || stderr.String() != "paperworks preview-export: output must be a new directory\n\n"+commands["preview-export"].Usage+"\n" {
				t.Fatalf("exit=%d, stderr=%q", code, stderr.String())
			}
			data, err := os.ReadFile(source)
			if err != nil || !bytes.Equal(data, original) {
				t.Fatal("input changed", err)
			}
			if kind != "empty" {
				data, err := os.ReadFile(sentinel)
				if err != nil || string(data) != "stale" {
					t.Fatal("existing output changed", err)
				}
			}
			if kind != "file" {
				entries, err := os.ReadDir(out)
				want := 0
				if kind == "nonempty" {
					want = 1
				}
				if err != nil || len(entries) != want {
					t.Fatal("existing directory changed", entries, err)
				}
			}
			entries, err := os.ReadDir(root)
			if err != nil || len(entries) != 2 {
				t.Fatal("unexpected staging files", entries, err)
			}
		})
	}
}

func TestPreviewExportNestedOutput(t *testing.T) {
	source := "../../testdata/fixtures/libreoffice-unsupported-treemap.pdf"
	original, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "missing", "nested", "output")
	result, err := commands["preview-export"].Run(context.Background(), Args{Pos: []string{source}, Flags: map[string]string{"output": out, "max-pixels": "100"}})
	if err != nil {
		t.Fatal(err)
	}
	pages := result.(map[string]any)["pages"].([]string)
	if len(pages) == 0 {
		t.Fatal("no preview pages")
	}
	for _, page := range pages {
		if filepath.Dir(page) != out {
			t.Fatal("page outside output", page)
		}
		data, err := os.ReadFile(page)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := png.Decode(bytes.NewReader(data)); err != nil {
			t.Fatal("invalid preview", err)
		}
	}
	entries, err := os.ReadDir(out)
	if err != nil || len(entries) != len(pages) {
		t.Fatal("unexpected published files", entries, err)
	}
	entries, err = os.ReadDir(filepath.Dir(out))
	if err != nil || len(entries) != 1 || entries[0].Name() != "output" {
		t.Fatal("staging directory retained", entries, err)
	}
	data, err := os.ReadFile(source)
	if err != nil || !bytes.Equal(data, original) {
		t.Fatal("input changed", err)
	}
}

func TestPreviewExportRenderOptionValidation(t *testing.T) {
	source := "../../testdata/fixtures/libreoffice-unsupported-treemap.pdf"
	usage := commands["preview-export"].Usage
	for _, tc := range []struct {
		name, flag, value, message string
	}{
		{name: "max-pixels", flag: "--max-pixels", value: "10", message: "max_pixels must be between 100 and 10000"},
		{name: "dpi", flag: "--dpi", value: "35", message: "DPI must be between 36 and 600"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			out := filepath.Join(root, "output")
			args := []string{"preview-export", source, "--output", out, tc.flag, tc.value}
			var stdout, stderr bytes.Buffer
			if code := Main(args, &stdout, &stderr); code != ExitUsage {
				t.Fatalf("exit=%d, want %d; stderr=%q", code, ExitUsage, stderr.String())
			}
			wantErr := "paperworks preview-export: " + tc.message + "\n\n" + usage + "\n"
			if stderr.String() != wantErr {
				t.Fatalf("stderr=%q, want %q", stderr.String(), wantErr)
			}
			if stdout.Len() != 0 {
				t.Fatalf("unexpected stdout: %q", stdout.String())
			}
			if _, err := os.Stat(out); !os.IsNotExist(err) {
				t.Fatalf("invalid options created output: %v", err)
			}
			stages, err := filepath.Glob(filepath.Join(root, ".paperworks-preview-*"))
			if err != nil || len(stages) != 0 {
				t.Fatalf("invalid options left staging artifacts: %v, %v", stages, err)
			}
			entries, err := os.ReadDir(root)
			if err != nil || len(entries) != 0 {
				t.Fatalf("invalid options created artifacts: %v, %v", entries, err)
			}
		})
	}

	t.Run("valid max-pixels", func(t *testing.T) {
		root := t.TempDir()
		out := filepath.Join(root, "output")
		var stdout, stderr bytes.Buffer
		args := []string{"preview-export", source, "--output", out, "--max-pixels", "100"}
		if code := Main(args, &stdout, &stderr); code != ExitOK {
			t.Fatalf("exit=%d, want %d; stderr=%q", code, ExitOK, stderr.String())
		}
		entries, err := os.ReadDir(out)
		if err != nil || len(entries) == 0 {
			t.Fatalf("valid options did not publish rendered previews: %v, %v", entries, err)
		}
		stages, err := filepath.Glob(filepath.Join(root, ".paperworks-preview-*"))
		if err != nil || len(stages) != 0 {
			t.Fatalf("valid export left staging artifacts: %v, %v", stages, err)
		}
	})
}

func TestPreviewExportFailureDoesNotPublish(t *testing.T) {
	for _, nested := range []bool{false, true} {
		t.Run(map[bool]string{false: "existing-parent", true: "missing-parents"}[nested], func(t *testing.T) {
			root := t.TempDir()
			source := filepath.Join(root, "invalid.pdf")
			original := []byte("not a PDF")
			if err := os.WriteFile(source, original, 0600); err != nil {
				t.Fatal(err)
			}
			parent := t.TempDir()
			if nested {
				parent = filepath.Join(parent, "missing", "nested")
			}
			out := filepath.Join(parent, "output")
			_, err := commands["preview-export"].Run(context.Background(), Args{Pos: []string{source}, Flags: map[string]string{"output": out}})
			if err == nil {
				t.Fatal("invalid input accepted")
			}
			if _, err := os.Stat(out); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("failed preview published", err)
			}
			entries, err := os.ReadDir(parent)
			if err != nil || len(entries) != 0 {
				t.Fatal("staging files retained", entries, err)
			}
			data, err := os.ReadFile(source)
			if err != nil || !bytes.Equal(data, original) {
				t.Fatal("input changed", err)
			}
		})
	}
}
