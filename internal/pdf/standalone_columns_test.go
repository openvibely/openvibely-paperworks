package pdf

import (
	"context"
	"image/png"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/klippa-app/go-pdfium/requests"
)

// Original runtime lines 243-259: same page, positions, column text, crop
// rectangles and scan built from the rendered source, rather than an empty PDF.
func TestReferenceStandaloneRuntimeColumnsAndScan(t *testing.T) {
	ctx := context.Background()
	// Exact ReportLab input from unchanged original statements243-255. A raw
	// substitute used different text operators and omitted WinAnsiEncoding.
	data, err := os.ReadFile("../../testdata/reference-mirror/runtime-two-column.pdf")
	if err != nil {
		t.Fatal(err)
	}
	// Release the single PDFium worker before starting a render.
	func() {
		inst, done, err := engine(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer done()
		doc, err := inst.OpenDocument(&requests.OpenDocument{File: &data})
		if err != nil {
			t.Fatal(err)
		}
		defer inst.FPDF_CloseDocument(&requests.FPDF_CloseDocument{Document: doc.Document})
		text, err := inst.FPDFText_LoadPage(&requests.FPDFText_LoadPage{Page: requests.Page{ByIndex: &requests.PageByIndex{Document: doc.Document, Index: 0}}})
		if err != nil {
			t.Fatal(err)
		}
		defer inst.FPDFText_ClosePage(&requests.FPDFText_ClosePage{TextPage: text.TextPage})
		for _, c := range []struct {
			name        string
			left, right float64
			want        []string
		}{
			{"revenue", 20, 280, []string{"Revenue", "Q1: $120", "Q2: $180"}},
			{"costs", 310, 580, []string{"Costs", "Q1: $40", "Q2: $50"}},
		} {
			t.Run(c.name, func(t *testing.T) {
				// pdfplumber crops use top-origin coordinates; PDFium uses bottom-origin.
				got, err := inst.FPDFText_GetBoundedText(&requests.FPDFText_GetBoundedText{TextPage: text.TextPage, Left: c.left, Right: c.right, Top: 330, Bottom: 230})
				if err != nil {
					t.Fatal(err)
				}
				lines := strings.Split(strings.TrimSpace(strings.ReplaceAll(got.Text, "\r\n", "\n")), "\n")
				if !reflect.DeepEqual(lines, c.want) {
					t.Errorf("crop lines = %q, want %q", lines, c.want)
				}
			})
		}
	}()
	dir := t.TempDir()
	source := filepath.Join(dir, "two-column.pdf")
	if err := os.WriteFile(source, data, 0600); err != nil {
		t.Fatal(err)
	}
	paths, err := RenderPages(ctx, source, t.TempDir(), RenderOptions{})
	if err != nil || len(paths) == 0 {
		t.Fatalf("source render %v: %v", paths, err)
	}
	f, err := os.Open(paths[0])
	if err != nil {
		t.Fatal(err)
	}
	im, err := png.Decode(f)
	f.Close()
	if err != nil {
		t.Fatal(err)
	}
	scan := referenceScanPDF(t, im)
	t.Run("scan-no-text", func(t *testing.T) {
		pages, err := PageText(ctx, scan)
		if err != nil || len(pages) == 0 {
			t.Fatalf("scan text %v: %v", pages, err)
		}
		if strings.TrimSpace(pages[0]) != "" {
			t.Errorf("scan has text: %q", pages[0])
		}
	})
	t.Run("scan-renders", func(t *testing.T) {
		src := filepath.Join(t.TempDir(), "scanned.pdf")
		if err := os.WriteFile(src, scan, 0600); err != nil {
			t.Fatal(err)
		}
		pages, err := RenderPages(ctx, src, t.TempDir(), RenderOptions{})
		if err != nil || len(pages) == 0 {
			t.Errorf("scan render %v: %v", pages, err)
		}
	})
}
