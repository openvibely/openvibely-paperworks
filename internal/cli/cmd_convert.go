package cli

import (
	"context"
	"errors"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/openvibely/openvibely-paperworks/internal/convert"
	"github.com/openvibely/openvibely-paperworks/internal/lo"
	"github.com/openvibely/openvibely-paperworks/internal/pdf"
)

func renderOptions(a Args) (pdf.RenderOptions, error) {
	var o pdf.RenderOptions
	if v, ok, err := a.Float("dpi"); err != nil {
		return o, err
	} else if ok {
		if v < 36 || v > 600 || math.IsNaN(v) {
			return o, errors.New("DPI must be between 36 and 600")
		}
		o.DPI = v
		o.DPISet = true
	}
	n, err := a.Int("max-pixels", 1600)
	if err != nil {
		return o, err
	}
	o.MaxPixels = n
	o.MaxPixelsSet = true
	return o, o.Validate()
}

// officeErr maps a missing LibreOffice to the dependency exit code.
func officeErr(err error) error {
	if errors.Is(err, lo.ErrMissing) {
		return DependencyError{err.Error()}
	}
	return err
}

func init() {
	register(&Command{
		Name:      "render",
		ValueOpts: []string{"output", "dpi", "max-pixels", "sheet", "range"},
		Summary:   "Render PDF/DOCX/XLSX/PPTX pages to PNG; reuse output directories and overwrite matching pages (Office files via LibreOffice).",
		Usage:     "paperworks render INPUT --output NEW_DIR [--dpi N | --max-pixels N] [--sheet NAME [--range A1:D20]]",
		Run: func(ctx context.Context, a Args) (any, error) {
			in, err := input(a, 1)
			if err != nil {
				return nil, err
			}
			if err := a.Require("output"); err != nil {
				return nil, err
			}
			opt, err := renderOptions(a)
			if err != nil {
				return nil, UsageError{err.Error()}
			}
			if err := existing(in[0]); err != nil {
				return nil, err
			}
			pages, err := convert.RenderFile(ctx, in[0], a.Path("output"), a.Str("sheet"), a.Str("range"), opt)
			if err != nil {
				return nil, officeErr(err)
			}
			return map[string]any{"pages": pages, "visually_inspected": false}, nil
		},
	})
	register(&Command{
		Name:      "convert",
		ValueOpts: []string{"output", "sheet", "range"},
		Summary:   "Export DOCX/XLSX/PPTX to PDF with LibreOffice in a new directory (spreadsheets are recalculated first).",
		Usage:     "paperworks convert INPUT --output NEW_DIR [--sheet NAME [--range A1:D20]]",
		Run: func(ctx context.Context, a Args) (any, error) {
			in, err := input(a, 1)
			if err != nil {
				return nil, err
			}
			if err := existing(in[0]); err != nil {
				return nil, err
			}
			if err := a.Require("output"); err != nil {
				return nil, err
			}
			res, err := convert.Convert(ctx, in[0], a.Path("output"), convert.Options{Sheet: a.Str("sheet"), CellRange: a.Str("range")})
			return res, officeErr(err)
		},
	})
	register(&Command{
		Name:      "recalculate",
		ValueOpts: []string{"output"},
		Summary:   "Recalculate XLSX formulas with LibreOffice into a new directory; only cached results change.",
		Usage:     "paperworks recalculate INPUT.xlsx --output NEW_DIR",
		Run: func(ctx context.Context, a Args) (any, error) {
			in, err := input(a, 1)
			if err != nil {
				return nil, err
			}
			if ext(in[0]) != ".xlsx" {
				return nil, usagef("recalculate requires an .xlsx file")
			}
			if err := existing(in[0]); err != nil {
				return nil, err
			}
			if err := a.Require("output"); err != nil {
				return nil, err
			}
			res, err := convert.Convert(ctx, in[0], a.Path("output"), convert.Options{Recalculate: true})
			return res, officeErr(err)
		},
	})
	register(&Command{
		Name:      "refresh",
		ValueOpts: []string{"output"},
		Summary:   "Refresh DOCX fields/TOC or XLSX formulas via LibreOffice into a new directory (re-saved by LibreOffice).",
		Usage:     "paperworks refresh INPUT.docx|INPUT.xlsx --output NEW_DIR",
		Run: func(ctx context.Context, a Args) (any, error) {
			in, err := input(a, 1)
			if err != nil {
				return nil, err
			}
			if e := ext(in[0]); e != ".docx" && e != ".xlsx" {
				return nil, usagef("refresh supports .docx and .xlsx")
			}
			if err := existing(in[0]); err != nil {
				return nil, err
			}
			if err := a.Require("output"); err != nil {
				return nil, err
			}
			res, err := convert.Convert(ctx, in[0], a.Path("output"), convert.Options{Refresh: true})
			return res, officeErr(err)
		},
	})
	register(&Command{
		Name:      "compare",
		ValueOpts: []string{"against", "output", "dpi", "max-pixels"},
		Summary:   "Render two documents and write per-page pixel diffs (plus a text diff for DOCX pairs).",
		Usage:     "paperworks compare BEFORE --against AFTER --output NEW_DIR [--dpi N | --max-pixels N]",
		BoolOpts:  nil,
		Run: func(ctx context.Context, a Args) (any, error) {
			in, err := input(a, 1)
			if err != nil {
				return nil, err
			}
			if err := a.Require("against", "output"); err != nil {
				return nil, err
			}
			for _, p := range []string{in[0], a.Path("against")} {
				if err := existing(p); err != nil {
					return nil, err
				}
			}
			opt, err := renderOptions(a)
			if err != nil {
				return nil, UsageError{err.Error()}
			}
			res, err := convert.Compare(ctx, in[0], a.Path("against"), a.Path("output"), opt)
			return res, officeErr(err)
		},
	})
	register(&Command{
		Name:      "version",
		ValueOpts: []string{},
		Summary:   "Print the paperworks version.",
		Usage:     "paperworks version",
		Run: func(ctx context.Context, a Args) (any, error) {
			return map[string]any{"version": Version, "os": runtime.GOOS, "arch": runtime.GOARCH, "go": runtime.Version()}, nil
		},
	})
	register(&Command{
		Name:      "capabilities",
		ValueOpts: []string{},
		Summary:   "Report built-in engines and optional external dependencies (LibreOffice).",
		Usage:     "paperworks capabilities",
		Run: func(ctx context.Context, a Args) (any, error) {
			office := map[string]any{"available": false}
			if exe, err := lo.Find(); err == nil {
				office = map[string]any{"available": true, "path": exe, "version": lo.Version(ctx)}
			}
			names := make([]string, 0, len(commands))
			for n := range commands {
				names = append(names, n)
			}
			return map[string]any{
				"version":              Version,
				"pdf_engine":           "PDFium (WebAssembly, embedded)",
				"replacement_complete": false,
				"equation_engine":      map[string]any{"implementation": "go-tex-maintained", "rasterizer": "x/image SFNT/vector", "embedded_fonts": true, "reference_parity_complete": false},
				"libreoffice":          office,
				"requires_libreoffice": []string{
					"render (DOCX/XLSX/PPTX)", "convert", "recalculate", "refresh", "compare (Office inputs)",
				},
				"commands": sortStrings(names),
			}, nil
		},
	})
}

func sortStrings(s []string) []string {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
	return s
}

func pagesFlag(ctx context.Context, a Args, path string) ([]int, error) {
	spec := a.Str("pages")
	if spec == "" {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	n, err := pdf.PageCount(ctx, data)
	if err != nil {
		return nil, err
	}
	p, err := pdf.ParsePages(spec, n)
	if err != nil {
		return nil, UsageError{err.Error()}
	}
	return p, nil
}

func init() {
	register(&Command{
		Name:      "fill",
		ValueOpts: []string{"values", "output"},
		Summary:   "Fill PDF AcroForm fields into a new PDF (optionally flattened).",
		Usage:     "paperworks fill INPUT.pdf --values values.json --output NEW.pdf [--flatten] [--strict-choices]\n  --strict-choices: Reject unmatched choice export values before writing (editable combos allow custom text).\n  Default: preserve unmatched values and return structured warnings. Empty strings/arrays clear selections.\n  values.json: {\"Field name\": \"value\", \"Checkbox\": \"/Yes\", \"Multi-select list\": [\"Alpha\", \"Beta\"]}",
		BoolOpts:  []string{"flatten", "strict-choices"},
		Run: func(ctx context.Context, a Args) (any, error) {
			in, err := input(a, 1)
			if err != nil {
				return nil, err
			}
			if err := existing(in[0]); err != nil {
				return nil, err
			}
			if ext(in[0]) != ".pdf" {
				return nil, usagef("fill requires a PDF")
			}
			if err := a.Require("values", "output"); err != nil {
				return nil, err
			}
			var values map[string]any
			if err := a.JSON("values", &values); err != nil {
				return nil, err
			}
			out := a.Path("output")
			if err := newFile(out, in[0]); err != nil {
				return nil, err
			}
			return pdf.FillValues(ctx, in[0], out, values, a.Bools["flatten"], pdf.FillOptions{StrictChoices: a.Bools["strict-choices"]})
		},
	})
	register(&Command{
		Name:      "pdf-merge",
		ValueOpts: []string{"output"},
		Summary:   "Concatenate PDFs into a new PDF.",
		Usage:     "paperworks pdf-merge A.pdf B.pdf [...] --output NEW.pdf",
		Run: func(ctx context.Context, a Args) (any, error) {
			if len(a.Pos) < 2 {
				return nil, usagef("pdf-merge needs at least two input PDFs")
			}
			var ins []string
			for _, p := range a.Pos {
				p = absPath(p)
				if err := existing(p); err != nil {
					return nil, err
				}
				ins = append(ins, p)
			}
			out := a.Path("output")
			if err := newFile(out, ""); err != nil {
				return nil, err
			}
			n, err := pdf.Merge(ctx, ins, out)
			if err != nil {
				return nil, err
			}
			return map[string]any{"output": out, "pages": n, "inputs": ins}, nil
		},
	})
	register(&Command{
		Name:      "pdf-select",
		ValueOpts: []string{"pages", "output"},
		Summary:   "Write selected/reordered pages of a PDF to a new PDF.",
		Usage:     "paperworks pdf-select INPUT.pdf --pages 3,1-2 --output NEW.pdf",
		Run: func(ctx context.Context, a Args) (any, error) {
			in, err := input(a, 1)
			if err != nil {
				return nil, err
			}
			if err := a.Require("pages"); err != nil {
				return nil, err
			}
			out := a.Path("output")
			if err := newFile(out, in[0]); err != nil {
				return nil, err
			}
			data, err := os.ReadFile(in[0])
			if err != nil {
				return nil, err
			}
			res, pages, err := pdf.SelectPagesBySpec(ctx, data, a.Str("pages"))
			if err != nil {
				var selectionErr pdf.PageSelectionError
				if errors.As(err, &selectionErr) {
					return nil, UsageError{selectionErr.Error()}
				}
				return nil, err
			}
			return map[string]any{"output": out, "pages": pages}, writeExclusive(out, res)
		},
	})
	register(&Command{
		Name:      "pdf-split",
		ValueOpts: []string{"ranges", "output"},
		Summary:   "Split a PDF into one file per page, or per --ranges group, in a new directory.",
		Usage:     "paperworks pdf-split INPUT.pdf --output NEW_DIR [--ranges \"1-2;3;4-6\"]",
		Run: func(ctx context.Context, a Args) (any, error) {
			in, err := input(a, 1)
			if err != nil {
				return nil, err
			}
			data, err := os.ReadFile(in[0])
			if err != nil {
				return nil, err
			}
			n, err := pdf.PageCount(ctx, data)
			if err != nil {
				return nil, err
			}
			var groups [][]int
			if r := a.Str("ranges"); r != "" {
				for _, g := range strings.Split(r, ";") {
					p, err := pdf.ParsePages(g, n)
					if err != nil {
						return nil, UsageError{err.Error()}
					}
					groups = append(groups, p)
				}
			} else {
				for i := 1; i <= n; i++ {
					groups = append(groups, []int{i})
				}
			}
			out := a.Path("output")
			if err := newDir(out); err != nil {
				return nil, err
			}
			files, err := pdf.Split(ctx, in[0], out, groups)
			return map[string]any{"files": files}, err
		},
	})
	register(&Command{
		Name:      "pdf-rotate",
		ValueOpts: []string{"degrees", "pages", "output"},
		Summary:   "Rotate PDF pages by a multiple of 90 degrees into a new PDF.",
		Usage:     "paperworks pdf-rotate INPUT.pdf --degrees 90 [--pages 1,3] --output NEW.pdf",
		Run: func(ctx context.Context, a Args) (any, error) {
			in, err := input(a, 1)
			if err != nil {
				return nil, err
			}
			deg, err := a.Int("degrees", 90)
			if err != nil {
				return nil, err
			}
			out := a.Path("output")
			if err := newFile(out, in[0]); err != nil {
				return nil, err
			}
			pages, err := pagesFlag(ctx, a, in[0])
			if err != nil {
				return nil, err
			}
			return map[string]any{"output": out, "degrees": deg}, pdf.Rotate(ctx, in[0], out, deg, pages)
		},
	})
	register(&Command{
		Name:      "pdf-watermark",
		ValueOpts: []string{"text", "style", "pages", "output"},
		Summary:   "Stamp a text watermark onto PDF pages into a new PDF.",
		Usage:     "paperworks pdf-watermark INPUT.pdf --text DRAFT [--style \"rot:45, op:0.25, scale:0.6 rel\"] [--pages 1-2] --output NEW.pdf",
		Run: func(ctx context.Context, a Args) (any, error) {
			in, err := input(a, 1)
			if err != nil {
				return nil, err
			}
			if err := a.Require("text"); err != nil {
				return nil, err
			}
			out := a.Path("output")
			if err := newFile(out, in[0]); err != nil {
				return nil, err
			}
			pages, err := pagesFlag(ctx, a, in[0])
			if err != nil {
				return nil, err
			}
			return map[string]any{"output": out}, pdf.Watermark(ctx, in[0], out, a.Str("text"), a.Str("style"), pages)
		},
	})
	register(&Command{
		Name:      "pdf-text",
		ValueOpts: []string{"engine", "output"},
		Summary:   "Extract page text; pypdf mode supports a bounded subset and rejects unsupported pages.",
		Usage:     "paperworks pdf-text INPUT.pdf [--engine pdfium|pypdf] [--output NEW.txt]",
		Run: func(ctx context.Context, a Args) (any, error) {
			in, err := input(a, 1)
			if err != nil {
				return nil, err
			}
			data, err := os.ReadFile(in[0])
			if err != nil {
				return nil, err
			}
			var pages []string
			switch a.Str("engine") {
			case "", "pdfium":
				pages, err = pdf.PageText(ctx, data)
			case "pypdf":
				pages, err = pdf.PypdfPageText(ctx, data)
			default:
				return nil, UsageError{"engine must be pdfium or pypdf"}
			}
			if err != nil {
				return nil, err
			}
			if out := a.Path("output"); out != "" {
				if err := newFile(out, in[0]); err != nil {
					return nil, err
				}
				return map[string]any{"output": out, "pages": len(pages)}, writeExclusive(out, []byte(strings.Join(pages, "\f")))
			}
			return map[string]any{"pages": pages}, nil
		},
	})
}

func writeExclusive(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		os.Remove(path)
		return err
	}
	return f.Close()
}
