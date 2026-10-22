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
