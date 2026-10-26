package cli

import (
	"context"
	"os"
	"path/filepath"

	"github.com/openvibely/openvibely-paperworks/internal/convert"
	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"github.com/openvibely/openvibely-paperworks/internal/pdf"
)

// oneInOut validates INPUT plus a new --output file.
func oneInOut(a Args) (string, string, error) {
	in, err := input(a, 1)
	if err != nil {
		return "", "", err
	}
	if err := existing(in[0]); err != nil {
		return "", "", err
	}
	out := a.Path("output")
	if err := newFile(out, in[0]); err != nil {
		return "", "", err
	}
	return in[0], out, nil
}

func init() {
	register(&Command{
		Name:      "verify-slides",
		ValueOpts: []string{"requirements"},
		Summary:   "Check a PPTX against explicit requirements (slide count/size, native tables/charts, fonts, table sums, chart caches).",
		Usage:     "paperworks verify-slides INPUT.pptx --requirements SPEC.json",
		Run: func(ctx context.Context, a Args) (any, error) {
			in, err := input(a, 1)
			if err != nil {
				return nil, err
			}
			if ext(in[0]) != ".pptx" {
				return nil, usagef("verify-slides requires a .pptx file")
			}
			var req office.SlideRequirements
			if err := a.JSON("requirements", &req); err != nil {
				return nil, err
			}
			report, err := office.PackageReport(in[0])
			if err != nil {
				return nil, err
			}
			p, err := opc.Open(in[0])
			if err != nil {
				return nil, err
			}
			res, err := office.VerifyPresentation(p, report, req)
			if err != nil {
				return nil, VerifyError{Msg: err.Error()}
			}
			var rendered any
			if req.RenderFonts {
				dir, err := os.MkdirTemp("", "document-fonts-")
				if err != nil {
					return nil, err
				}
				defer os.RemoveAll(dir)
				conv, err := convert.Convert(ctx, in[0], filepath.Join(dir, "render"), convert.Options{})
				if err != nil {
					return nil, officeErr(err)
				}
				data, err := os.ReadFile(conv["output"].(string))
				if err != nil {
					return nil, err
				}
				fonts, err := pdf.VerifyFonts(ctx, data, req.Fonts)
				if err != nil {
					return nil, VerifyError{Msg: err.Error()}
				}
				rendered = fonts
			}
			return map[string]any{
				"slides": res.Slides, "table_owners": nonNil(res.TableOwners), "chart_owners": nonNil(res.ChartOwners),
				"explicit_fonts": nonNil(res.ExplicitFonts), "font_definitions": res.FontDefinitions, "chart_data": nonNil(res.ChartData),
				"requirements_passed": true, "rendered_fonts": rendered,
				"heading_checks": nonNil(res.Headings), "layout_review_required": nonNil(res.LayoutReview),
				"limits": "Visual fit, original workbook preservation and native PowerPoint behavior require separate checks. Without render_fonts, inherited/installed fonts have not been verified.",
			}, nil
		},
	})
	register(&Command{
		Name:      "table-exchange",
		ValueOpts: []string{"output", "sheet", "range", "table-index"},
		Summary:   "Convert one table: XLSX range → new DOCX table, or DOCX table → CSV.",
		Usage:     "paperworks table-exchange SOURCE.xlsx --sheet NAME --range A1:D12 --output NEW.docx | SOURCE.docx [--table-index N] --output NEW.csv",
		Run: func(ctx context.Context, a Args) (any, error) {
			in, out, err := oneInOut(a)
			if err != nil {
				return nil, err
			}
			idx, err := a.Int("table-index", 0)
			if err != nil {
				return nil, err
			}
			return convert.TableExchange(in, out, a.Str("sheet"), a.Str("range"), idx)
		},
	})
	register(&Command{
		Name:      "workbook-inputs",
		ValueOpts: []string{"requirements", "output"},
		Summary:   "Patch literal XLSX input cells on a new copy, keeping formulas, caches and extensions.",
		Usage:     `paperworks workbook-inputs INPUT.xlsx --requirements SPEC.json --output NEW.xlsx   (SPEC: {"sheet":"Data","cells":{"B2":120}})`,
		Run: func(ctx context.Context, a Args) (any, error) {
			in, out, err := oneInOut(a)
			if err != nil {
				return nil, err
			}
			var spec convert.ChartInputSpec
			if err := a.JSON("requirements", &spec); err != nil {
				return nil, err
			}
			return convert.WorkbookInputs(in, out, spec.Sheet, spec.Cells)
		},
	})
	register(&Command{
		Name:      "chart-inputs",
		ValueOpts: []string{"requirements", "output"},
		Summary:   "Edit a chart's embedded-workbook inputs, recalculate with LibreOffice and refresh dependent chart caches.",
		Usage:     `paperworks chart-inputs INPUT.pptx --requirements SPEC.json --output NEW.pptx   (SPEC: {"chart":1,"sheet":"Sheet1","cells":{"B2":150}})`,
		Run: func(ctx context.Context, a Args) (any, error) {
			in, out, err := oneInOut(a)
			if err != nil {
				return nil, err
			}
			var spec convert.ChartInputSpec
			if err := a.JSON("requirements", &spec); err != nil {
				return nil, err
			}
			res, err := convert.ChartInputs(ctx, in, out, spec)
			return res, officeErr(err)
		},
	})
	register(&Command{
		Name:      "materialize-charts",
		ValueOpts: []string{"output"},
		Summary:   "Snapshot literal chart series into new embedded workbooks (never replaces existing lineage).",
		Usage:     "paperworks materialize-charts INPUT.pptx --output NEW.pptx",
		Run: func(ctx context.Context, a Args) (any, error) {
			in, out, err := oneInOut(a)
			if err != nil {
				return nil, err
			}
			return convert.MaterializeCharts(in, out)
		},
	})
	register(&Command{
		Name:      "verify-charts",
		ValueOpts: []string{"target-application"},
		Summary:   "Compare every chart cache with its saved embedded workbook values.",
		Usage:     "paperworks verify-charts INPUT [--target-application powerpoint]",
		Run: func(ctx context.Context, a Args) (any, error) {
			in, err := input(a, 1)
			if err != nil {
				return nil, err
			}
			p, err := opc.Open(in[0])
			if err != nil {
				return nil, err
			}
			res, err := office.VerifyChartData(p, a.Str("target-application"))
			if err != nil {
				return nil, VerifyError{Msg: err.Error()}
			}
			return map[string]any{"charts": nonNil(res)}, nil
		},
	})
	register(&Command{
		Name:      "sparklines",
		ValueOpts: []string{"requirements", "output"},
		Summary:   "Set, delete or clear native XLSX sparklines on one worksheet.",
		Usage:     `paperworks sparklines INPUT.xlsx --requirements SPEC.json --output NEW.xlsx   (SPEC: {"sheet":"Data","action":"set","sparklines":[{"cell":"F2","range":"Data!B2:E2"}],"options":{"type":"line"}})`,
		Run: func(ctx context.Context, a Args) (any, error) {
			in, out, err := oneInOut(a)
			if err != nil {
				return nil, err
			}
			if ext(in) != ".xlsx" {
				return nil, usagef("sparklines require XLSX")
			}
			var spec office.SparklineSpec
			if err := a.JSON("requirements", &spec); err != nil {
				return nil, err
			}
			if _, err := office.PackageReport(in); err != nil {
				return nil, err
			}
			p, err := opc.Open(in)
			if err != nil {
				return nil, err
			}
			targets, err := office.EditSparklines(p, spec)
			if err != nil {
				return nil, err
			}
			if err := p.SaveNew(out); err != nil {
				return nil, err
			}
			report, err := office.PackageReport(out)
			if err != nil {
				return nil, err
			}
			action := spec.Action
			if action == "" {
				action = "set"
			}
			features, _ := report["native_features"].(map[string][]any)
			return map[string]any{"output": out, "action": action, "sheet": spec.Sheet, "targets": nonNil(targets), "native_features": nonNil(features["sparklineGroup"])}, nil
		},
	})
}

func nonNil[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}
