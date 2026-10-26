package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/openvibely/openvibely-paperworks/internal/convert"
	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
)

func init() {
	register(&Command{Name: "slide-preview", ValueOpts: []string{"output", "slides", "dpi", "max-pixels"}, Summary: "Render PPTX slides with paired native layout JSON reports.", Usage: "paperworks slide-preview INPUT.pptx --output DIR [--slides 1,3] [--dpi N | --max-pixels N]", Run: func(ctx context.Context, a Args) (any, error) {
		return runSlidePreview(ctx, a, convert.RenderFile)
	}})
}

func runSlidePreview(ctx context.Context, a Args, render previewFileRenderer) (any, error) {
	in, err := input(a, 1)
	if err != nil {
		return nil, err
	}
	if ext(in[0]) != ".pptx" {
		return nil, usagef("slide-preview requires PPTX")
	}
	if err = a.Require("output"); err != nil {
		return nil, err
	}
	opt, err := renderOptions(a)
	if err != nil {
		return nil, err
	}
	out := a.Path("output")
	pkg, err := opc.Open(in[0])
	if err != nil {
		return nil, err
	}
	reports, err := office.PresentationLayouts(pkg)
	if err != nil {
		return nil, err
	}
	selected, err := previewSelection(a.Str("slides"), len(reports))
	if err != nil {
		return nil, err
	}
	stage, cleanup, err := createPreviewStage(out, ".paperworks-slide-preview-")
	if err != nil {
		return nil, err
	}
	defer cleanup()
	// Render the same source snapshot used for metadata, even if the input changes.
	snapshot := filepath.Join(stage, "source.pptx")
	if err = pkg.SaveNew(snapshot); err != nil {
		return nil, err
	}
	pages, err := render(ctx, snapshot, filepath.Join(stage, "rendered"), "", "", opt)
	if err != nil {
		return nil, officeErr(err)
	}
	return publishSelectedSlidePreview(ctx, stage, out, pages, reports, selected)
}
func publishSlidePreview(ctx context.Context, stage, out string, pages []string, reports []map[string]any) (any, error) {
	return publishSelectedSlidePreview(ctx, stage, out, pages, reports, nil)
}
func publishSelectedSlidePreview(ctx context.Context, stage, out string, pages []string, reports []map[string]any, selected map[int]bool) (any, error) {
	if len(pages) != len(reports) {
		return nil, fmt.Errorf("rendered page count %d differs from slide count %d; cannot pair layouts", len(pages), len(reports))
	}
	release, err := lockSlidePreview(out)
	if err != nil {
		return nil, err
	}
	defer release()
	payload := filepath.Join(stage, "paired")
	if err := os.Mkdir(payload, 0755); err != nil {
		return nil, err
	}
	existing := false
	if info, err := os.Lstat(out); err == nil {
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return nil, usagef("output must be a directory")
		}
		existing = true
		entries, err := os.ReadDir(out)
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			info, err := entry.Info()
			if err != nil {
				return nil, err
			}
			if !info.Mode().IsRegular() {
				return nil, usagef("preview directory must contain only regular files")
			}
			if selected == nil && previewPairName.MatchString(entry.Name()) {
				continue
			}
			data, err := os.ReadFile(filepath.Join(out, entry.Name()))
			if err != nil {
				return nil, err
			}
			if err := os.WriteFile(filepath.Join(payload, entry.Name()), data, info.Mode().Perm()); err != nil {
				return nil, err
			}
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	pairs := []map[string]string{}
	for i, page := range pages {
		if selected != nil && !selected[i+1] {
			continue
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		f, err := os.Open(page)
		if err != nil {
			return nil, err
		}
		size, err := png.DecodeConfig(f)
		f.Close()
		if err != nil {
			return nil, err
		}
		stem := fmt.Sprintf("slide-%d", i+1)
		imagePath := stem + ".png"
		jsonPath := stem + ".layout.json"
		reports[i]["preview"] = map[string]any{"file": imagePath, "width": size.Width, "height": size.Height}
		b, err := json.MarshalIndent(reports[i], "", "  ")
		if err != nil {
			return nil, err
		}
		if err = os.WriteFile(filepath.Join(payload, jsonPath), b, 0644); err != nil {
			return nil, err
		}
		if err = os.Rename(page, filepath.Join(payload, imagePath)); err != nil {
			return nil, err
		}
		pairs = append(pairs, map[string]string{"image": filepath.Join(out, imagePath), "layout": filepath.Join(out, jsonPath)})
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	backup := ""
	if existing {
		var err error
		backup, err = os.MkdirTemp(filepath.Dir(out), ".previous-preview-")
		if err != nil {
			return nil, err
		}
		if err = os.Remove(backup); err != nil {
			return nil, err
		}
		if err = os.Rename(out, backup); err != nil {
			return nil, err
		}
	} else if _, err := os.Lstat(out); !os.IsNotExist(err) {
		return nil, usagef("output appeared during rendering")
	}
	if err := os.Rename(payload, out); err != nil {
		if existing {
			if restore := os.Rename(backup, out); restore != nil {
				return nil, fmt.Errorf("publish: %v; restore: %v; previous files retained at %s", err, restore, backup)
			}
		}
		return nil, err
	}
	if existing {
		_ = os.RemoveAll(backup)
	}
	return map[string]any{"slides": pairs, "visually_inspected": false}, nil
}

var previewPairName = regexp.MustCompile(`^slide-[1-9][0-9]*\.(png|layout\.json)$`)

func previewSelection(value string, count int) (map[int]bool, error) {
	if value == "" {
		return nil, nil
	}
	result := map[int]bool{}
	for _, token := range strings.Split(value, ",") {
		n, err := strconv.Atoi(strings.TrimSpace(token))
		if err != nil || n < 1 || n > count {
			return nil, usagef("--slides requires comma-separated slide numbers from 1 to %d", count)
		}
		result[n] = true
	}
	return result, nil
}
