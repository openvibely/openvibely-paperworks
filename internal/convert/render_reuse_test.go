package convert

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/openvibely/openvibely-paperworks/internal/pdf"
)

func TestRenderXLSXReuse(t *testing.T) {
	src := referenceBook(t, "sheet1.xml", "Data", `<c r="A1"><v>1</v></c>`, false)
	out := filepath.Join(t.TempDir(), "render")
	calls := 0
	ctx := withDeps(dependencies{refresh: func(_ context.Context, _, dst string, _ bool) error {
		calls++
		// Match LibreOffice's exclusive publication behavior.
		f, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			return err
		}
		if _, err := f.Write(contents(t, "../../testdata/fixtures/libreoffice-unsupported-treemap.pdf")); err != nil {
			f.Close()
			return err
		}
		return f.Close()
	}})
	for i := 0; i < 2; i++ {
		if _, err := RenderFile(ctx, src, out, "Data", "A1:A1", pdf.RenderOptions{DPI: 36}); err != nil {
			t.Fatal(err)
		}
	}
	if calls != 2 {
		t.Fatal(calls)
	}
}

func TestRenderXLSXReuseWithLibreOffice(t *testing.T) {
	referenceRuntimeOffice(t)
	src := referenceRuntimeOriginalWorkbookFixture(t, "runtime-workflow-fixtures.zip", "view-settings")
	out := filepath.Join(t.TempDir(), "render")
	for i := 0; i < 2; i++ {
		pages, err := RenderFile(context.Background(), src, out, "", "", pdf.RenderOptions{DPI: 36})
		if err != nil {
			t.Fatalf("render %d: %v", i+1, err)
		}
		if len(pages) != 4 {
			t.Fatalf("got %d pages", len(pages))
		}
	}
}

func TestRenderReplacesLinksWithoutChangingTargets(t *testing.T) {
	for _, hard := range []bool{false, true} {
		name := "symlink"
		if hard {
			name = "hardlink"
		}
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			src := filepath.Join(dir, "input.pdf")
			original := contents(t, "../../testdata/fixtures/libreoffice-unsupported-treemap.pdf")
			if err := os.WriteFile(src, original, 0600); err != nil {
				t.Fatal(err)
			}
			out := filepath.Join(dir, "render")
			if err := os.Mkdir(out, 0700); err != nil {
				t.Fatal(err)
			}
			page := filepath.Join(out, "page-0001.png")
			link := os.Symlink
			if hard {
				link = os.Link
			}
			if err := link(src, page); err != nil {
				t.Fatal(err)
			}
			pages, err := RenderFile(context.Background(), src, out, "", "", pdf.RenderOptions{DPI: 36})
			if err != nil {
				t.Fatal(err)
			}
			if len(pages) == 0 {
				t.Fatal("no pages")
			}
			assertUnchanged(t, src, original)
			if _, err := loadPNG(page); err != nil {
				t.Fatal(err)
			}
			// Office intermediate PDFs use the same replacement semantics.
			dest := filepath.Join(out, "intermediate.pdf")
			if err := link(src, dest); err != nil {
				t.Fatal(err)
			}
			replacement := filepath.Join(dir, "replacement.pdf")
			if err := os.WriteFile(replacement, []byte("replacement"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := publishRenderPDF(replacement, dest); err != nil {
				t.Fatal(err)
			}
			assertUnchanged(t, src, original)
			if string(contents(t, dest)) != "replacement" {
				t.Fatal("destination not replaced")
			}
		})
	}
}
