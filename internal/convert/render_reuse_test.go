package convert

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
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

func TestRenderFileRerenderReplacesPageImageSet(t *testing.T) {
	dir := t.TempDir()
	shortPDF := filepath.Join(dir, "short.pdf")
	longPDF := filepath.Join(dir, "long.pdf")
	writeRenderPageCountPDF(t, shortPDF, 4)
	writeRenderPageCountPDF(t, longPDF, 10)

	for _, office := range []bool{false, true} {
		name := "pdf"
		if office {
			name = "office"
		}
		t.Run(name, func(t *testing.T) {
			out := filepath.Join(t.TempDir(), "render")
			if err := os.MkdirAll(out, 0755); err != nil {
				t.Fatal(err)
			}
			unrelated := filepath.Join(out, "notes.txt")
			if err := os.WriteFile(unrelated, []byte("keep this file"), 0600); err != nil {
				t.Fatal(err)
			}
			noncanonicalPageName := filepath.Join(out, "page-00001.png")
			if err := os.WriteFile(noncanonicalPageName, []byte("keep this too"), 0600); err != nil {
				t.Fatal(err)
			}
			input := shortPDF
			ctx := context.Background()
			if office {
				input = filepath.Join(dir, "source.docx")
				call := 0
				ctx = withDeps(dependencies{convert: func(context.Context, string, string, Options) (map[string]any, error) {
					call++
					if call == 2 {
						return map[string]any{"output": longPDF}, nil
					}
					return map[string]any{"output": shortPDF}, nil
				}})
			}

			for _, count := range []int{4, 10, 4} {
				if !office {
					input = shortPDF
					if count == 10 {
						input = longPDF
					}
				}
				pages, err := RenderFile(ctx, input, out, "", "", pdf.RenderOptions{DPI: 36})
				if err != nil {
					t.Fatalf("render %d pages: %v", count, err)
				}
				assertRenderPageSet(t, out, pages, count)
				if got := string(contents(t, unrelated)); got != "keep this file" {
					t.Fatalf("unrelated file changed: %q", got)
				}
				if got := string(contents(t, noncanonicalPageName)); got != "keep this too" {
					t.Fatalf("noncanonical page-like file changed: %q", got)
				}
			}
		})
	}
}

func TestRenderFileFailedRerenderPreservesPageImages(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source.pdf")
	writeRenderPageCountPDF(t, source, 6)
	out := filepath.Join(dir, "render")
	pages, err := RenderFile(context.Background(), source, out, "", "", pdf.RenderOptions{DPI: 36})
	if err != nil {
		t.Fatal(err)
	}
	before := make([][]byte, len(pages))
	for i, path := range pages {
		before[i] = contents(t, path)
	}
	if err := os.WriteFile(source, []byte("not a PDF"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := RenderFile(context.Background(), source, out, "", "", pdf.RenderOptions{DPI: 36}); err == nil {
		t.Fatal("invalid rerender unexpectedly succeeded")
	}
	assertRenderPageSet(t, out, pages, len(pages))
	for i, path := range pages {
		if got := contents(t, path); string(got) != string(before[i]) {
			t.Fatalf("page image %q changed after failed rerender", path)
		}
	}
}

func writeRenderPageCountPDF(t *testing.T, path string, count int) {
	t.Helper()
	if err := pdf.Create(context.Background(), pdf.CreateSpec{Pages: make([]pdf.CreatePage, count)}, filepath.Dir(path), path); err != nil {
		t.Fatal(err)
	}
}

func assertRenderPageSet(t *testing.T, out string, pages []string, count int) {
	t.Helper()
	if len(pages) != count {
		t.Fatalf("got %d returned paths, want %d", len(pages), count)
	}
	want := make([]string, count)
	for i := range want {
		want[i] = fmt.Sprintf("page-%04d.png", i+1)
		if filepath.Base(pages[i]) != want[i] {
			t.Fatalf("returned path %d = %q, want %q", i, filepath.Base(pages[i]), want[i])
		}
	}
	entries, err := os.ReadDir(out)
	if err != nil {
		t.Fatal(err)
	}
	got := make([]string, 0, count)
	for _, entry := range entries {
		if isPageImageNameForTest(entry.Name()) {
			got = append(got, entry.Name())
		}
	}
	sort.Strings(got)
	if len(got) != len(want) {
		t.Fatalf("directory has %d page images, want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("directory page images = %v, want %v", got, want)
		}
	}
}

func isPageImageNameForTest(name string) bool {
	if len(name) < len("page-0001.png") || name[:5] != "page-" || filepath.Ext(name) != ".png" {
		return false
	}
	number := name[5 : len(name)-4]
	if len(number) < 4 {
		return false
	}
	page, err := strconv.Atoi(number)
	return err == nil && page > 0 && fmt.Sprintf("page-%04d.png", page) == name
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
