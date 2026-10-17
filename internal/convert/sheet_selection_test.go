package convert

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openvibely/openvibely-paperworks/internal/pdf"
	"github.com/xuri/excelize/v2"
)

func TestSelectPreviewSheetsRejectsInvalidMappingAndCancellation(t *testing.T) {
	dir := t.TempDir()
	workbook := filepath.Join(dir, "sheets.xlsx")
	f := excelize.NewFile()
	if _, err := f.NewSheet("Second"); err != nil {
		t.Fatal(err)
	}
	if err := f.SaveAs(workbook); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(workbook)
	if err != nil {
		t.Fatal(err)
	}
	for _, cancel := range []bool{false, true} {
		name := "mismatch.pdf"
		if cancel {
			name = "cancelled.pdf"
		}
		path := filepath.Join(dir, name)
		if err := pdf.Create(context.Background(), pdf.CreateSpec{Pages: []pdf.CreatePage{{}}}, dir, path); err != nil {
			t.Fatal(err)
		}
		ctx, stop := context.WithCancel(context.Background())
		if cancel {
			stop()
		}
		err := SelectPreviewSheets(ctx, workbook, path)
		stop()
		if cancel {
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("cancellation: %v", err)
			}
		} else if err == nil || !strings.Contains(err.Error(), "page count") {
			t.Fatalf("mapping: %v", err)
		}
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("invalid preview remains: %v", err)
		}
		after, err := os.ReadFile(workbook)
		if err != nil || !bytes.Equal(before, after) {
			t.Fatalf("workbook changed: %v", err)
		}
	}
}

// DocumentGapChecks.test_whole_sheet_export_selects_nonadjacent_pages_and_rejects_mismatch.
func TestReferenceSelectPreviewSheetsNonadjacent(t *testing.T) {
	dir := t.TempDir()
	workbook := referenceCallbackInput(t, "selection.xlsx")
	before := contents(t, workbook)
	pdfPath := filepath.Join(dir, "preview.pdf")
	export := func(count int) {
		t.Helper()
		if err := os.WriteFile(pdfPath, referenceCallbackFixture(t, fmt.Sprintf("selection-%d.pdf", count)), 0600); err != nil {
			t.Fatal(err)
		}
	}
	export(4)
	if err := SelectPreviewSheets(context.Background(), workbook, pdfPath); err != nil {
		t.Fatal(err)
	}
	pages, err := pdf.PageText(context.Background(), contents(t, pdfPath))
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) != 2 {
		t.Fatalf("page count %d", len(pages))
	}
	if pages[0] != "" {
		t.Fatalf("empty sheet text = %q", pages[0])
	}
	if !strings.Contains(pages[1], "SHEET_2") {
		t.Fatalf("selected sheet missing: %q", pages[1])
	}
	for _, hidden := range []string{"SHEET_0", "SHEET_3"} {
		if strings.Contains(strings.Join(pages, ""), hidden) {
			t.Fatal("hidden sheet leaked", hidden)
		}
	}
	export(3)
	if err := SelectPreviewSheets(context.Background(), workbook, pdfPath); err == nil || !strings.Contains(err.Error(), "page count") {
		t.Fatalf("mismatch: %v", err)
	}
	if _, err := os.Stat(pdfPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("invalid PDF retained", err)
	}
	assertUnchanged(t, workbook, before)
}

// The remaining render calls in test_workbook_preview_rejects_invalid_selection.
func TestReferenceRenderRangeRequiresSheet(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"source.xlsx", "source.pdf", "source.pptx"} {
		t.Run(name, func(t *testing.T) {
			_, err := RenderFile(context.Background(), filepath.Join(dir, name), filepath.Join(t.TempDir(), "render"), "", "A1:B2", pdf.RenderOptions{})
			if err == nil || !strings.Contains(err.Error(), "requires XLSX and --sheet") {
				t.Fatalf("wrong selection failure: %v", err)
			}
		})
	}
}
