package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/openvibely/openvibely-paperworks/internal/convert"
	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"image/png"
	"os"
	"path/filepath"
)

func init() {
	register(&Command{Name: "slide-preview", Summary: "Render PPTX slides with paired native layout JSON reports.", Usage: "paperworks slide-preview INPUT.pptx --output NEW_DIR [--dpi N | --max-pixels N]", Run: func(ctx context.Context, a Args) (any, error) {
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
		if _, err = os.Lstat(out); !os.IsNotExist(err) {
			return nil, usagef("output must be a new directory")
		}
		pkg, err := opc.Open(in[0])
		if err != nil {
			return nil, err
		}
		reports, err := office.PresentationLayouts(pkg)
		if err != nil {
			return nil, err
		}
		if err = os.MkdirAll(filepath.Dir(out), 0755); err != nil {
			return nil, err
		}
		stage, err := os.MkdirTemp(filepath.Dir(out), ".paperworks-slide-preview-")
		if err != nil {
			return nil, err
		}
		defer os.RemoveAll(stage)
		// Render the same source snapshot used for metadata, even if the input changes.
		snapshot := filepath.Join(stage, "source.pptx")
		if err = pkg.SaveNew(snapshot); err != nil {
			return nil, err
		}
		pages, err := convert.RenderFile(ctx, snapshot, filepath.Join(stage, "rendered"), "", "", opt)
		if err != nil {
			return nil, officeErr(err)
		}
		return publishSlidePreview(ctx, stage, out, pages, reports)
	}})
}
func publishSlidePreview(ctx context.Context, stage, out string, pages []string, reports []map[string]any) (any, error) {
	if len(pages) != len(reports) {
		return nil, fmt.Errorf("rendered page count %d differs from slide count %d; cannot pair layouts", len(pages), len(reports))
	}
	payload := filepath.Join(stage, "paired")
	if err := os.Mkdir(payload, 0755); err != nil {
		return nil, err
	}
	pairs := []map[string]string{}
	for i, page := range pages {
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
	if _, err := os.Lstat(out); !os.IsNotExist(err) {
		return nil, usagef("output must be a new directory")
	}
	if err := os.Rename(payload, out); err != nil {
		return nil, err
	}
	return map[string]any{"slides": pairs, "visually_inspected": false}, nil
}
