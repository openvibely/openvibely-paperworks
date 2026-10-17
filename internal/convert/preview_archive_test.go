package convert

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/openvibely/openvibely-paperworks/internal/lo"
	"github.com/openvibely/openvibely-paperworks/internal/pdf"
)

// These original reference fixtures contain a stored root relationship part.
// Rewriting it with a ZIP data descriptor prevented LibreOffice from opening
// the disposable preview even though Go and Reference ZIP readers accepted it.
func TestLibreOfficeImportedChartPreviewArchives(t *testing.T) {
	if _, err := lo.Find(); err != nil {
		t.Skip("LibreOffice unavailable")
	}
	for _, name := range []string{"reference-treemap.pptx", "reference-doughnut.pptx"} {
		t.Run(name, func(t *testing.T) {
			source := filepath.Join("../../testdata/fixtures", name)
			before, err := os.ReadFile(source)
			if err != nil {
				t.Fatal(err)
			}
			pages, err := RenderFile(context.Background(), source, filepath.Join(t.TempDir(), "render"), "", "", pdf.RenderOptions{DPI: 72})
			if err != nil {
				t.Fatal(err)
			}
			if len(pages) != 1 {
				t.Fatalf("pages: %v", pages)
			}
			after, err := os.ReadFile(source)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatal("source changed", err)
			}
		})
	}
}
