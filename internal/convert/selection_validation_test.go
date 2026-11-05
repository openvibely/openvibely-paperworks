package convert

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openvibely/openvibely-paperworks/internal/pdf"
)

func TestSelectionValidationIsSharedAndPrecedesRenderConversion(t *testing.T) {
	for _, tc := range []struct {
		name      string
		ext       string
		cellRange string
	}{
		{name: "DOCX with range", ext: ".docx", cellRange: "A1:B2"},
		{name: "XLSX range without sheet", ext: ".xlsx", cellRange: "A1:B2"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := filepath.Join(t.TempDir(), "source"+tc.ext)
			output := filepath.Join(t.TempDir(), "render")
			convertCalled := false
			ctx := withDeps(dependencies{convert: func(context.Context, string, string, Options) (map[string]any, error) {
				convertCalled = true
				return nil, nil
			}})

			_, renderErr := RenderFile(ctx, source, output, "", tc.cellRange, pdf.RenderOptions{})
			if renderErr == nil || renderErr.Error() != "preview selection requires XLSX and --sheet" {
				t.Fatalf("RenderFile selection error = %v", renderErr)
			}
			if convertCalled {
				t.Fatal("RenderFile invoked conversion for an invalid selection")
			}

			_, convertErr := Convert(context.Background(), source, filepath.Join(t.TempDir(), "converted"), Options{CellRange: tc.cellRange})
			if convertErr == nil || convertErr.Error() != renderErr.Error() {
				t.Fatalf("Convert selection error = %v, RenderFile error = %v", convertErr, renderErr)
			}
		})
	}
}

func TestConvertSelectionCannotRecalculateOrRefresh(t *testing.T) {
	for _, tc := range []struct {
		name string
		opts Options
	}{
		{name: "recalculate", opts: Options{Recalculate: true, Sheet: "Data", CellRange: "A1:B2"}},
		{name: "refresh", opts: Options{Refresh: true, Sheet: "Data", CellRange: "A1:B2"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Convert(context.Background(), "source.xlsx", filepath.Join(t.TempDir(), "output"), tc.opts)
			if err == nil || !strings.Contains(err.Error(), "cannot be combined with recalculation or refresh") {
				t.Fatalf("expected Convert-specific selection restriction, got %v", err)
			}
		})
	}
}
