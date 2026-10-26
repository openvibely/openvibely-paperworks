package cli

import (
	"context"
	"encoding/json"
	"errors"
	"image"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openvibely/openvibely-paperworks/internal/pdf"
	"github.com/openvibely/openvibely-paperworks/internal/pptx"
)

func TestPreviewCommandsCleanStageAfterRenderErrors(t *testing.T) {
	for _, command := range []string{"preview-export", "slide-preview"} {
		for _, outcome := range []string{"render-error", "cancelled"} {
			t.Run(command+"/"+outcome, func(t *testing.T) {
				root := t.TempDir()
				inputPath := filepath.Join(root, "input.pdf")
				if command == "slide-preview" {
					inputPath = filepath.Join(root, "input.pptx")
					if err := pptx.Create(context.Background(), pptx.Spec{Slides: []pptx.Slide{{Elements: []pptx.Element{{Type: "text", Text: "preview", X: 1, Y: 2, W: 3, H: 1}}}}}, root, inputPath); err != nil {
						t.Fatal(err)
					}
				} else if err := os.WriteFile(inputPath, []byte("renderer input"), 0600); err != nil {
					t.Fatal(err)
				}
				out := filepath.Join(root, "nested", "out")
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				forcedErr := errors.New("forced render failure")
				render := previewFileRenderer(func(ctx context.Context, source, renderDir, sheet, cellRange string, _ pdf.RenderOptions) ([]string, error) {
					stage := filepath.Dir(renderDir)
					if !strings.HasPrefix(filepath.Base(stage), map[string]string{"preview-export": ".paperworks-preview-", "slide-preview": ".paperworks-slide-preview-"}[command]) {
						t.Fatalf("unexpected stage directory %q", stage)
					}
					if _, err := os.Stat(stage); err != nil {
						t.Fatalf("stage missing during render: %v", err)
					}
					if command == "slide-preview" {
						if _, err := os.Stat(source); err != nil {
							t.Fatalf("source snapshot missing during render: %v", err)
						}
					}
					if outcome == "cancelled" {
						cancel()
						return nil, ctx.Err()
					}
					return nil, forcedErr
				})
				args := Args{Pos: []string{inputPath}, Flags: map[string]string{"output": out}}
				var err error
				if command == "preview-export" {
					_, err = runPreviewExport(ctx, args, render)
				} else {
					_, err = runSlidePreview(ctx, args, render)
				}
				if outcome == "cancelled" {
					if !errors.Is(err, context.Canceled) {
						t.Fatalf("expected cancellation, got %v", err)
					}
				} else if !errors.Is(err, forcedErr) {
					t.Fatalf("expected forced render failure, got %v", err)
				}
				stages, globErr := filepath.Glob(filepath.Join(root, "nested", ".paperworks-*"))
				if globErr != nil || len(stages) != 0 {
					t.Fatalf("staging leaked: %v (glob error: %v)", stages, globErr)
				}
				if _, statErr := os.Lstat(out); !os.IsNotExist(statErr) {
					t.Fatalf("failed command published output: %v", statErr)
				}
			})
		}
	}
}

func TestPreviewCommandsKeepSuccessfulPublication(t *testing.T) {
	for _, command := range []string{"preview-export", "slide-preview"} {
		t.Run(command, func(t *testing.T) {
			root := t.TempDir()
			inputPath := filepath.Join(root, "input.pdf")
			if command == "slide-preview" {
				inputPath = filepath.Join(root, "input.pptx")
				if err := pptx.Create(context.Background(), pptx.Spec{Slides: []pptx.Slide{{Elements: []pptx.Element{{Type: "text", Text: "preview", X: 1, Y: 2, W: 3, H: 1}}}}}, root, inputPath); err != nil {
					t.Fatal(err)
				}
			} else if err := os.WriteFile(inputPath, []byte("renderer input"), 0600); err != nil {
				t.Fatal(err)
			}
			out := filepath.Join(root, "nested", "out")
			render := previewFileRenderer(func(_ context.Context, _, renderDir, _, _ string, _ pdf.RenderOptions) ([]string, error) {
				if err := os.MkdirAll(renderDir, 0700); err != nil {
					return nil, err
				}
				page := filepath.Join(renderDir, "renderer-page.png")
				if err := writePreview(page, image.NewRGBA(image.Rect(0, 0, 8, 6)), "png"); err != nil {
					return nil, err
				}
				return []string{page}, nil
			})
			args := Args{Pos: []string{inputPath}, Flags: map[string]string{"output": out}}
			var result any
			var err error
			if command == "preview-export" {
				result, err = runPreviewExport(context.Background(), args, render)
			} else {
				result, err = runSlidePreview(context.Background(), args, render)
			}
			if err != nil {
				t.Fatal(err)
			}
			if command == "preview-export" {
				pages := result.(map[string]any)["pages"].([]string)
				if len(pages) != 1 || filepath.Base(pages[0]) != "page-0001.png" {
					t.Fatalf("unexpected export result: %#v", result)
				}
				info, err := os.Stat(pages[0])
				if err != nil || info.Mode().Perm() != 0600 {
					t.Fatalf("exported page missing or permissions changed: %v", err)
				}
			} else {
				slides := result.(map[string]any)["slides"].([]map[string]string)
				if len(slides) != 1 || filepath.Base(slides[0]["image"]) != "slide-1.png" || filepath.Base(slides[0]["layout"]) != "slide-1.layout.json" {
					t.Fatalf("unexpected slide result: %#v", result)
				}
				if _, err := os.Stat(slides[0]["image"]); err != nil {
					t.Fatal(err)
				}
				data, err := os.ReadFile(slides[0]["layout"])
				if err != nil {
					t.Fatal(err)
				}
				var layout map[string]any
				if err := json.Unmarshal(data, &layout); err != nil || layout["preview"].(map[string]any)["file"] != "slide-1.png" {
					t.Fatalf("unexpected layout publication: %v %v", layout, err)
				}
			}
			stages, err := filepath.Glob(filepath.Join(root, "nested", ".paperworks-*"))
			if err != nil || len(stages) != 0 {
				t.Fatalf("successful publication left staging: %v (glob error: %v)", stages, err)
			}
		})
	}
}

func TestCreatePreviewStageFailuresPreserveOutput(t *testing.T) {
	root := t.TempDir()
	out := filepath.Join(root, "out")
	if err := os.Mkdir(out, 0755); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(out, "keep.txt")
	if err := os.WriteFile(marker, []byte("keep"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := createPreviewStage(out, "invalid/prefix-"); err == nil {
		t.Fatal("expected temporary directory allocation failure")
	}
	if data, err := os.ReadFile(marker); err != nil || string(data) != "keep" {
		t.Fatalf("existing output changed: %q, %v", data, err)
	}

	blocked := filepath.Join(root, "blocked")
	if err := os.WriteFile(blocked, []byte("parent"), 0600); err != nil {
		t.Fatal(err)
	}
	missingOut := filepath.Join(blocked, "nested", "out")
	if _, _, err := createPreviewStage(missingOut, ".paperworks-preview-"); err == nil {
		t.Fatal("expected parent creation failure")
	}
	if data, err := os.ReadFile(blocked); err != nil || string(data) != "parent" {
		t.Fatalf("parent path changed: %q, %v", data, err)
	}
	if _, err := os.Stat(missingOut); err == nil {
		t.Fatal("failed stage setup created final output")
	}
}
