package cli

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/openvibely/openvibely-paperworks/internal/pdf"
)

func TestRenamePreviewExclusivePreservesDestinationInstalledAtPublishBoundary(t *testing.T) {
	for _, tc := range []struct {
		name     string
		sentinel bool
	}{
		{name: "empty destination"},
		{name: "destination with sentinel", sentinel: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			stage := filepath.Join(root, "stage")
			out := filepath.Join(root, "out")
			if err := os.Mkdir(stage, 0700); err != nil {
				t.Fatal(err)
			}
			stagedFile := filepath.Join(stage, "page.png")
			if err := os.WriteFile(stagedFile, []byte("staged output"), 0600); err != nil {
				t.Fatal(err)
			}

			var original os.FileInfo
			var setupErr error
			err := renamePreviewExclusiveAtBoundary(stage, out, func() {
				if err := os.Mkdir(out, 0700); err != nil {
					setupErr = err
					return
				}
				if tc.sentinel {
					if err := os.WriteFile(filepath.Join(out, "sentinel"), []byte("keep me"), 0600); err != nil {
						setupErr = err
						return
					}
				}
				original, setupErr = os.Lstat(out)
			})
			if setupErr != nil {
				t.Fatal(setupErr)
			}
			if err == nil {
				t.Fatal("publication unexpectedly replaced a destination installed at the publish boundary")
			}

			after, err := os.Lstat(out)
			if err != nil {
				t.Fatalf("concurrent destination disappeared: %v", err)
			}
			if !os.SameFile(original, after) {
				t.Fatal("concurrent destination was replaced")
			}
			entries, err := os.ReadDir(out)
			if err != nil {
				t.Fatal(err)
			}
			if tc.sentinel {
				if len(entries) != 1 || entries[0].Name() != "sentinel" {
					t.Fatalf("destination contents changed: %v", entries)
				}
				data, err := os.ReadFile(filepath.Join(out, "sentinel"))
				if err != nil || string(data) != "keep me" {
					t.Fatalf("sentinel changed: %q, %v", data, err)
				}
			} else if len(entries) != 0 {
				t.Fatalf("empty destination contents changed: %v", entries)
			}
			if data, err := os.ReadFile(stagedFile); err != nil || string(data) != "staged output" {
				t.Fatalf("staged file changed after rejected publication: %q, %v", data, err)
			}
		})
	}
}

func TestPreviewExportPreservesDestinationCreatedDuringRendering(t *testing.T) {
	for _, tc := range []struct {
		name     string
		sentinel bool
	}{
		{name: "empty destination"},
		{name: "destination with sentinel", sentinel: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			inputPath := filepath.Join(root, "input.pdf")
			if err := os.WriteFile(inputPath, []byte("renderer input"), 0600); err != nil {
				t.Fatal(err)
			}
			out := filepath.Join(root, "out")
			var original os.FileInfo
			render := previewFileRenderer(func(context.Context, string, string, string, string, pdf.RenderOptions) ([]string, error) {
				if err := os.Mkdir(out, 0700); err != nil {
					return nil, err
				}
				if tc.sentinel {
					if err := os.WriteFile(filepath.Join(out, "sentinel"), []byte("keep me"), 0600); err != nil {
						return nil, err
					}
				}
				var err error
				original, err = os.Lstat(out)
				return nil, err
			})

			_, err := runPreviewExport(context.Background(), Args{
				Pos:   []string{inputPath},
				Flags: map[string]string{"output": out},
			}, render)
			if err == nil {
				t.Fatal("publication unexpectedly replaced a destination created during rendering")
			}

			after, err := os.Lstat(out)
			if err != nil {
				t.Fatalf("concurrent destination disappeared: %v", err)
			}
			if !os.SameFile(original, after) {
				t.Fatal("concurrent destination was replaced")
			}
			entries, err := os.ReadDir(out)
			if err != nil {
				t.Fatal(err)
			}
			if tc.sentinel {
				if len(entries) != 1 || entries[0].Name() != "sentinel" {
					t.Fatalf("destination contents changed: %v", entries)
				}
				data, err := os.ReadFile(filepath.Join(out, "sentinel"))
				if err != nil || string(data) != "keep me" {
					t.Fatalf("sentinel changed: %q, %v", data, err)
				}
			} else if len(entries) != 0 {
				t.Fatalf("empty destination contents changed: %v", entries)
			}

			stages, err := filepath.Glob(filepath.Join(root, ".paperworks-preview-*"))
			if err != nil || len(stages) != 0 {
				t.Fatalf("failed publication leaked staging: %v (glob error: %v)", stages, err)
			}
		})
	}
}
