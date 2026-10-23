package convert

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/openvibely/openvibely-paperworks/internal/pdf"
)

func TestOutputDirectoryExisting(t *testing.T) {
	src := referenceBook(t, "sheet1.xml", "Data", `<c r="A1"><v>1</v></c>`, false)
	before := contents(t, src)

	for _, nonempty := range []bool{false, true} {
		for _, action := range []string{"convert", "recalculate", "refresh", "compare"} {
			t.Run(action+map[bool]string{false: "/empty", true: "/nonempty"}[nonempty], func(t *testing.T) {
				ctx := withDeps(dependencies{
					refresh: func(context.Context, string, string, bool) error { t.Fatal("LibreOffice refresh invoked"); return nil },
					export: func(context.Context, string, string, string) (string, error) {
						t.Fatal("LibreOffice export invoked")
						return "", nil
					},
					render: func(context.Context, string, string, pdf.RenderOptions) ([]string, error) {
						t.Fatal("renderer invoked")
						return nil, nil
					},
				})
				out := t.TempDir()
				sentinel := filepath.Join(out, "keep")
				if nonempty {
					if err := os.WriteFile(sentinel, []byte("stale"), 0600); err != nil {
						t.Fatal(err)
					}
				}
				var err error
				switch action {
				case "compare":
					_, err = Compare(ctx, "before.pdf", "after.pdf", out, pdf.RenderOptions{})
				default:
					_, err = Convert(ctx, src, out, Options{Recalculate: action == "recalculate", Refresh: action == "refresh"})
				}
				if !errors.Is(err, os.ErrExist) {
					t.Fatalf("expected ErrExist, got %v", err)
				}
				assertUnchanged(t, src, before)
				entries, err := os.ReadDir(out)
				if err != nil {
					t.Fatal(err)
				}
				if nonempty {
					if len(entries) != 1 || string(contents(t, sentinel)) != "stale" {
						t.Fatal("existing output changed")
					}
				} else if len(entries) != 0 {
					t.Fatal("empty output changed")
				}
			})
		}
	}
}

func TestPDFOutputNestedDirectories(t *testing.T) {
	for _, compare := range []bool{false, true} {
		t.Run(map[bool]string{false: "render", true: "compare"}[compare], func(t *testing.T) {
			out := filepath.Join(t.TempDir(), "missing", "nested", "output")
			var rendered []string
			ctx := withDeps(dependencies{render: func(_ context.Context, in, dst string, _ pdf.RenderOptions) ([]string, error) {
				st, err := os.Stat(dst)
				if err != nil || !st.IsDir() {
					t.Fatalf("render directory missing: %s: %v", dst, err)
				}
				rendered = append(rendered, in, dst)
				return nil, nil
			}})
			var err error
			want := []string{"before.pdf", out}
			if compare {
				_, err = Compare(ctx, "before.pdf", "after.pdf", out, pdf.RenderOptions{})
				want = []string{"before.pdf", filepath.Join(out, "before"), "after.pdf", filepath.Join(out, "after")}
			} else {
				_, err = RenderFile(ctx, "before.pdf", out, "", "", pdf.RenderOptions{})
			}
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(rendered, want) {
				t.Fatalf("render calls %v, want %v", rendered, want)
			}
		})
	}
}

func TestConvertNestedOutputDirectory(t *testing.T) {
	for _, action := range []string{"convert", "recalculate", "refresh"} {
		t.Run(action, func(t *testing.T) {
			src := referenceBook(t, "sheet1.xml", "Data", `<c r="A1"><v>1</v></c>`, false)
			original := contents(t, src)
			out := filepath.Join(t.TempDir(), "missing", "nested", "output")
			calls := 0
			ctx := withDeps(dependencies{refresh: func(_ context.Context, _, dst string, _ bool) error {
				calls++
				return os.WriteFile(dst, original, 0600)
			}})
			result, err := Convert(ctx, src, out, Options{Recalculate: action == "recalculate", Refresh: action == "refresh"})
			if err != nil {
				t.Fatal(err)
			}
			if calls != 1 {
				t.Fatalf("refresh calls = %d", calls)
			}
			if filepath.Dir(result["output"].(string)) != out {
				t.Fatal(result)
			}
			if len(contents(t, result["output"].(string))) == 0 {
				t.Fatal("empty result")
			}
			assertUnchanged(t, src, original)
		})
	}
}

// A failed Office export leaves the output directory behind. Retrying must
// reach conversion again, and later renders must overwrite their page images.
func TestRenderReuseAfterFailedExport(t *testing.T) {
	src := "../../testdata/regression/word-comment-anchor-baseline/source.docx"
	out := filepath.Join(t.TempDir(), "render")
	failed := errors.New("export failed")
	calls := 0
	ctx := withDeps(dependencies{export: func(_ context.Context, _, dir, _ string) (string, error) {
		calls++
		if calls == 1 {
			return "", failed
		}
		dst := filepath.Join(dir, "converted.pdf")
		return dst, os.WriteFile(dst, contents(t, "../../testdata/fixtures/libreoffice-unsupported-treemap.pdf"), 0600)
	}})
	if _, err := RenderFile(ctx, src, out, "", "", pdf.RenderOptions{DPI: 36}); !errors.Is(err, failed) {
		t.Fatal(err)
	}
	sentinel := filepath.Join(out, "notes.txt")
	if err := os.WriteFile(sentinel, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := RenderFile(ctx, src, out, "", "", pdf.RenderOptions{}); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatal(calls)
	}
	input := "../../testdata/fixtures/libreoffice-unsupported-treemap.pdf"
	for i := 0; i < 2; i++ {
		pages, err := RenderFile(context.Background(), input, out, "", "", pdf.RenderOptions{DPI: 36})
		if err != nil {
			t.Fatal(err)
		}
		if len(pages) == 0 {
			t.Fatal("no pages")
		}
		if i == 0 {
			if err := os.WriteFile(pages[0], []byte("stale"), 0600); err != nil {
				t.Fatal(err)
			}
		} else if string(contents(t, pages[0])) == "stale" {
			t.Fatal("page not replaced")
		}
	}
	if string(contents(t, sentinel)) != "keep" {
		t.Fatal("unrelated file changed")
	}
}
