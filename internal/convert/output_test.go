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
		for _, action := range []string{"convert", "recalculate", "refresh", "render-office", "render-chart", "render-pdf", "compare"} {
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
				case "render-office":
					_, err = RenderFile(ctx, src, out, "", "", pdf.RenderOptions{})
				case "render-chart":
					chart := "../../testdata/fixtures/reference-categorical-histogram.pptx"
					original := contents(t, chart)
					_, err = RenderFile(ctx, chart, out, "", "", pdf.RenderOptions{})
					assertUnchanged(t, chart, original)
				case "render-pdf":
					_, err = RenderFile(ctx, "source.pdf", out, "", "", pdf.RenderOptions{})
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
