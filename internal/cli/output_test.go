package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestPDFSplitOutputDirectory(t *testing.T) {
	source := "../../testdata/fixtures/libreoffice-unsupported-treemap.pdf"
	original, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"missing-output", "empty", "nonempty", "nested"} {
		t.Run(mode, func(t *testing.T) {
			flags := map[string]string{}
			out := t.TempDir()
			if mode != "missing-output" {
				flags["output"] = out
			}
			if mode == "nonempty" {
				if err := os.WriteFile(filepath.Join(out, "keep"), []byte("stale"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if mode == "nested" {
				out = filepath.Join(out, "missing", "nested")
				flags["output"] = out
			}
			_, err := commands["pdf-split"].Run(context.Background(), Args{Pos: []string{source}, Flags: flags})
			switch mode {
			case "missing-output":
				var usage UsageError
				if !errors.As(err, &usage) || err.Error() != "--output must name a new directory" {
					t.Fatalf("unexpected usage error: %v", err)
				}
			case "nested":
				if err != nil {
					t.Fatal(err)
				}
				entries, err := os.ReadDir(out)
				if err != nil || len(entries) == 0 {
					t.Fatalf("missing split files: %v", err)
				}
			default:
				if !errors.Is(err, os.ErrExist) {
					t.Fatalf("expected ErrExist, got %v", err)
				}
				entries, err := os.ReadDir(out)
				if err != nil {
					t.Fatal(err)
				}
				if mode == "empty" && len(entries) != 0 {
					t.Fatal("output changed")
				}
				if mode == "nonempty" {
					data, err := os.ReadFile(filepath.Join(out, "keep"))
					if err != nil || string(data) != "stale" || len(entries) != 1 {
						t.Fatal("output changed", err)
					}
				}
			}
			data, err := os.ReadFile(source)
			if err != nil || !bytes.Equal(original, data) {
				t.Fatal("input changed", err)
			}
		})
	}
}
