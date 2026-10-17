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
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// PageCount returns the number of pages in a PDF.
func PageCount(ctx context.Context, data []byte) (int, error) {
	c, err := Load(ctx, data)
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
	if len(pages) == 0 {
		return nil, errors.New("no pages selected")
	}
	var out bytes.Buffer
	if err := api.Collect(ctx, bytes.NewReader(data), &out, pageSpec(pages), config()); err != nil {
		return nil, fmt.Errorf("select pages: %w", err)
	}
	return out.Bytes(), nil
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
	readers := make([]io.ReadSeeker, len(inputs))
	for i, in := range inputs {
		data, err := os.ReadFile(in)
		if err != nil {
			return 0, err
		}
		readers[i] = bytes.NewReader(data)
	}
	var out bytes.Buffer
	if err := api.MergeRaw(ctx, readers, &out, false, config()); err != nil {
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

// Split writes one PDF per page range into a new directory. Each element of
// ranges is a list of pages for one output file.
func Split(ctx context.Context, src, outDir string, ranges [][]int) ([]string, error) {
	data, err := os.ReadFile(src)
	if err != nil {
		return nil, err
	}
	stem := strings.TrimSuffix(filepath.Base(src), filepath.Ext(src))
	var files []string
	for i, pages := range ranges {
		out, err := SelectPages(ctx, data, pages)
		if err != nil {
			return files, err
		}
		name := filepath.Join(outDir, fmt.Sprintf("%s-%03d.pdf", stem, i+1))
		if err := writeNew(name, out); err != nil {
			return files, err
		}
		files = append(files, name)
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
