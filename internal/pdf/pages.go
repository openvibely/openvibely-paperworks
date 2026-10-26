package pdf

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// PageCount returns the number of pages in a PDF.
func PageCount(ctx context.Context, data []byte) (int, error) {
	return pageCount(ctx, data, nil)
}

func pageCount(ctx context.Context, data []byte, onPrepare func()) (int, error) {
	c, err := load(ctx, data, onPrepare)
	if err != nil {
		return 0, err
	}
	return c.PageCount, nil
}

func pageSpec(pages []int) []string {
	sel := make([]string, len(pages))
	for i, p := range pages {
		sel[i] = strconv.Itoa(p)
	}
	return sel
}

// PageSelectionError reports a syntactically invalid selection or a page outside the source.
type PageSelectionError struct{ Err error }

func (e PageSelectionError) Error() string { return e.Err.Error() }
func (e PageSelectionError) Unwrap() error { return e.Err }

// ParsePages expands "1,3-5" style selections against a page count, in order.
func ParsePages(spec string, count int) ([]int, error) {
	var out []int
	for _, part := range strings.Split(spec, ",") {
		part = strings.TrimSpace(part)
		lo, hi, isRange := strings.Cut(part, "-")
		a, err := strconv.Atoi(strings.TrimSpace(lo))
		b := a
		if err == nil && isRange {
			b, err = strconv.Atoi(strings.TrimSpace(hi))
		}
		if err != nil || a < 1 || b < a || b > count {
			return nil, fmt.Errorf("invalid page selection %q for %d pages", part, count)
		}
		for p := a; p <= b; p++ {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return nil, errors.New("no pages selected")
	}
	return out, nil
}

func writeNew(path string, data []byte) error {
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

// SelectPages writes the given 1-based pages of data, in order, to dest.
func SelectPages(ctx context.Context, data []byte, pages []int) ([]byte, error) {
	return selectPages(ctx, data, pages, nil)
}

func selectPages(ctx context.Context, data []byte, pages []int, onPrepare func()) ([]byte, error) {
	if len(pages) == 0 {
		return nil, errors.New("no pages selected")
	}
	if onPrepare != nil {
		onPrepare()
	}
	var out bytes.Buffer
	if err := api.Collect(ctx, bytes.NewReader(data), &out, pageSpec(pages), config()); err != nil {
		return nil, fmt.Errorf("select pages: %w", err)
	}
	return out.Bytes(), nil
}

// SelectPagesBySpec validates data once, resolves a page selection against its
// prepared page tree, and returns the selected PDF and its 1-based page list.
func SelectPagesBySpec(ctx context.Context, data []byte, spec string) ([]byte, []int, error) {
	return selectPagesBySpec(ctx, data, spec, nil)
}

func selectPagesBySpec(ctx context.Context, data []byte, spec string, onPrepare func()) ([]byte, []int, error) {
	conf := config()
	conf.Cmd = model.COLLECT
	if onPrepare != nil {
		onPrepare()
	}
	prepared, err := api.ReadValidateAndOptimize(ctx, bytes.NewReader(data), conf, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("read PDF: %w", err)
	}
	pages, err := ParsePages(spec, prepared.PageCount)
	if err != nil {
		return nil, nil, PageSelectionError{Err: err}
	}
	output, err := pdfcpu.ExtractPages(ctx, prepared, pages, false)
	if err != nil {
		return nil, nil, fmt.Errorf("select pages: collect: extract pages: %w", err)
	}
	var out bytes.Buffer
	if err := api.Write(ctx, output, &out, conf); err != nil {
		return nil, nil, fmt.Errorf("select pages: collect: write output: %w", err)
	}
	return out.Bytes(), pages, nil
}

// KeepPages rewrites path in place with only the given 1-based pages.
func KeepPages(ctx context.Context, path string, pages []int) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	out, err := SelectPages(ctx, data, pages)
	if err != nil {
		return err
	}
	return os.WriteFile(path, out, 0o644)
}

// Merge concatenates inputs into a new file.
func Merge(ctx context.Context, inputs []string, dest string) (int, error) {
	if len(inputs) < 2 {
		return 0, errors.New("merge needs at least two inputs")
	}
	conf := config()
	conf.CreateBookmarks = true
	conf.MergeBookmarkMode = model.MergeBookmarkModePreserve
	var out bytes.Buffer
	if err := api.Merge(ctx, "", inputs, &out, conf, false); err != nil {
		return 0, fmt.Errorf("merge: %w", err)
	}
	n, err := PageCount(ctx, out.Bytes())
	if err != nil {
		return 0, err
	}
	return n, writeNew(dest, out.Bytes())
}

// Rotate rotates selected pages (all when nil) by a multiple of 90 degrees.
func Rotate(ctx context.Context, src, dest string, degrees int, pages []int) error {
	if degrees%90 != 0 {
		return errors.New("rotation must be a multiple of 90")
	}
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	var sel []string
	if pages != nil {
		sel = pageSpec(pages)
	}
	var out bytes.Buffer
	if err := api.Rotate(ctx, bytes.NewReader(data), &out, degrees, sel, config()); err != nil {
		return fmt.Errorf("rotate: %w", err)
	}
	return writeNew(dest, out.Bytes())
}

func writeNewWith(path string, write func(io.Writer) error) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	if err := write(f); err != nil {
		f.Close()
		os.Remove(path)
		return err
	}
	return f.Close()
}

// Split writes one PDF per page range into a new directory. Each element of
// ranges is a list of pages for one output file.
func splitSourceContext(ctx context.Context, data []byte, template *model.Context, conf *model.Configuration) (*model.Context, error) {
	source, err := api.ReadContext(ctx, bytes.NewReader(data), conf.Clone())
	if err != nil {
		return nil, err
	}
	source.PageCount = template.PageCount
	if template.Form != nil {
		source.Form = template.Form.Clone().(types.Dict)
	}
	if err := pdfcpu.CacheFormFonts(source); err != nil {
		return nil, fmt.Errorf("cache form fonts: %w", err)
	}

	destinations := &model.Node{D: types.NewDict()}
	if err := template.Names["Dests"].Process(ctx, template.XRefTable, func(_ *model.XRefTable, key string, value *types.Object) error {
		if value == nil || *value == nil {
			destinations.AppendToNames(key, nil)
			return nil
		}
		destinations.AppendToNames(key, (*value).Clone())
		return nil
	}); err != nil {
		return nil, fmt.Errorf("clone named destinations: %w", err)
	}
	source.Names = map[string]*model.Node{"Dests": destinations}
	return source, nil
}

func Split(ctx context.Context, src, outDir string, ranges [][]int) ([]string, error) {
	return split(ctx, src, outDir, ranges, nil)
}

func split(ctx context.Context, src, outDir string, ranges [][]int, afterOutput func()) ([]string, error) {
	return splitWithOutputWriter(ctx, src, outDir, ranges, afterOutput, writeNewWith)
}

func splitWithOutputWriter(ctx context.Context, src, outDir string, ranges [][]int, afterOutput func(), writeOutput func(string, func(io.Writer) error) error) ([]string, error) {
	data, err := os.ReadFile(src)
	if err != nil {
		return nil, err
	}
	if len(ranges) == 0 {
		return nil, nil
	}
	// SelectPages rejects an empty selection before parsing the PDF. Preserve
	// that error ordering for the first output group.
	if len(ranges[0]) == 0 {
		return nil, errors.New("select pages: no pages selected")
	}

	conf := config()
	conf.Cmd = model.COLLECT
	prepared, err := api.ReadValidateAndOptimize(ctx, bytes.NewReader(data), conf, nil)
	if err != nil {
		return nil, fmt.Errorf("select pages: collect: %w", err)
	}
	data = nil

	// ExtractPages mutates pdfcpu's cached Dests name tree while migrating
	// named destinations. Other source state is read-only during extraction, so
	// only PDFs with that cache need an isolated context for each output.
	isolateSource := len(ranges) > 1 && prepared.Names["Dests"] != nil
	var preparedData []byte
	var preparedTemplate *model.Context
	pageCount := prepared.PageCount
	if isolateSource {
		preparedTemplate = prepared
		var source bytes.Buffer
		if err := api.WriteContext(ctx, prepared, &source); err != nil {
			return nil, fmt.Errorf("select pages: prepare source for extraction: %w", err)
		}
		preparedData = source.Bytes()
		prepared = nil
	}

	stem := strings.TrimSuffix(filepath.Base(src), filepath.Ext(src))
	var files []string
	for i, pages := range ranges {
		if len(pages) == 0 {
			return files, errors.New("select pages: no pages selected")
		}
		selected, err := api.PagesForPageCollection(pageCount, pageSpec(pages))
		if err != nil {
			return files, fmt.Errorf("select pages: collect: parse page selection: %w", err)
		}

		source := prepared
		if isolateSource {
			source, err = splitSourceContext(ctx, preparedData, preparedTemplate, conf)
			if err != nil {
				return files, fmt.Errorf("select pages: read prepared source: %w", err)
			}
		}
		output, err := pdfcpu.ExtractPages(ctx, source, selected, false)
		if err != nil {
			return files, fmt.Errorf("select pages: collect: extract pages: %w", err)
		}

		name := filepath.Join(outDir, fmt.Sprintf("%s-%03d.pdf", stem, i+1))
		if err := writeOutput(name, func(w io.Writer) error {
			if err := api.Write(ctx, output, w, conf); err != nil {
				return fmt.Errorf("select pages: collect: write output: %w", err)
			}
			return nil
		}); err != nil {
			return files, err
		}
		files = append(files, name)
		if afterOutput != nil {
			afterOutput()
		}
	}
	return files, nil
}

// Watermark stamps text onto selected pages (all when nil). desc uses
// pdfcpu's watermark description syntax, e.g. "rot:45, op:0.2, scale:0.6 abs".
func Watermark(ctx context.Context, src, dest, text, desc string, pages []int) error {
	if strings.TrimSpace(text) == "" {
		return errors.New("watermark text is empty")
	}
	if desc == "" {
		desc = "font:Helvetica, points:48, rot:45, op:0.25, fillc:#808080, scale:0.6 rel"
	}
	wm, err := api.TextWatermark(ctx, text, desc, true, false, types.POINTS, config())
	if err != nil {
		return fmt.Errorf("watermark: %w", err)
	}
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	var sel []string
	if pages != nil {
		sel = pageSpec(pages)
	}
	var out bytes.Buffer
	if err := api.AddWatermarks(ctx, bytes.NewReader(data), &out, sel, wm, config()); err != nil {
		return fmt.Errorf("watermark: %w", err)
	}
	return writeNew(dest, out.Bytes())
}
