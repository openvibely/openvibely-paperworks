package pdf

import (
	"context"
	"errors"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// test_document_libraries.py: both portable-renderer resolution cases.
func TestReferenceStandaloneLibrariesResolution(t *testing.T) {
	data, err := os.ReadFile("../../testdata/reference-mirror/library-dimensions.pdf")
	if err != nil {
		t.Fatal(err)
	}
	src := writeTemp(t, "dimensions.pdf", data)
	for _, c := range []struct {
		name          string
		options       RenderOptions
		width, height int
	}{{"dpi-144", RenderOptions{DPI: 144}, 864, 432}, {"max-pixels-1000", RenderOptions{MaxPixels: 1000}, 1000, 500}} {
		t.Run(c.name, func(t *testing.T) {
			pages, e := RenderPages(context.Background(), src, t.TempDir(), c.options)
			if e != nil || len(pages) != 1 {
				t.Fatalf("render: %v %v", pages, e)
			}
			f, e := os.Open(pages[0])
			if e != nil {
				t.Fatal(e)
			}
			defer f.Close()
			im, e := png.DecodeConfig(f)
			if e != nil {
				t.Fatal(e)
			}
			if im.Width != c.width || im.Height != c.height {
				t.Errorf("dimensions: %dx%d want %dx%d", im.Width, im.Height, c.width, c.height)
			}
		})
	}
}

func TestReferenceStandaloneLibrariesPDFForms(t *testing.T) {
	ctx := context.Background()
	report := func(t *testing.T, data []byte) *FormReport {
		t.Helper()
		r, e := ReportBytes(ctx, data)
		if e != nil {
			t.Fatal(e)
		}
		return r
	}
	t.Run("saved-appearance", func(t *testing.T) {
		data, err := os.ReadFile("../../testdata/reference-mirror/library-filled.pdf")
		if err != nil {
			t.Fatal(err)
		}
		r := report(t, data)
		if r.Fields["account"] == nil || *r.Fields["account"] != "Robot" {
			t.Error(r.Fields)
		}
		if len(r.Widgets) != 1 || r.Widgets[0].Value == nil || *r.Widgets[0].Value != "Robot" {
			t.Error(r.Widgets)
		}
		if len(r.Issues) != 0 {
			t.Error(r.Issues)
		}
		data, err = os.ReadFile("../../testdata/reference-mirror/library-bad-appearance.pdf")
		if err != nil {
			t.Fatal(err)
		}
		r = report(t, data)
		found := false
		for _, issue := range r.Issues {
			found = found || strings.Contains(issue, "appearance")
		}
		if !found {
			t.Error("missing appearance issue", r.Issues)
		}
	})
	t.Run("already-flattened", func(t *testing.T) {
		data, err := os.ReadFile("../../testdata/reference-mirror/library-flattened.pdf")
		if err != nil {
			t.Fatal(err)
		}
		r := report(t, data)
		if len(r.Widgets) != 0 || r.HasFormTree {
			t.Error(r)
		}
	})
	data, err := os.ReadFile("../../testdata/reference-mirror/library-form.pdf")
	if err != nil {
		t.Fatal(err)
	}
	src := writeTemp(t, "form.pdf", data)
	for _, flat := range []bool{false, true} {
		t.Run(fmt.Sprintf("helper-flat-%t", flat), func(t *testing.T) {
			out := filepath.Join(t.TempDir(), "filled.pdf")
			if _, e := Fill(ctx, src, out, map[string]string{"account": "Ada"}, flat); e != nil {
				t.Fatal(e)
			}
			data, e := os.ReadFile(out)
			if e != nil {
				t.Fatal(e)
			}
			r := report(t, data)
			if flat {
				if len(r.Widgets) != 0 {
					t.Error(r.Widgets)
				}
				text, e := PageText(ctx, data)
				if e != nil || len(text) != 1 || !strings.Contains(text[0], "Ada") {
					t.Errorf("text %v %v", text, e)
				}
			} else if r.Fields["account"] == nil || *r.Fields["account"] != "Ada" {
				t.Error(r.Fields)
			}
		})
	}
	t.Run("orphan-repair", func(t *testing.T) {
		data, err := os.ReadFile("../../testdata/reference-mirror/library-form-orphan.pdf")
		if err != nil {
			t.Fatal(err)
		}
		before := append([]byte(nil), data...)
		orphan := writeTemp(t, "orphan.pdf", data)
		out := filepath.Join(t.TempDir(), "repaired.pdf")
		if _, e := Fill(ctx, orphan, out, map[string]string{"account": "Recovered"}, false); e != nil {
			t.Fatal(e)
		}
		r, e := Report(ctx, out)
		if e != nil {
			t.Fatal(e)
		}
		if r.Fields["account"] == nil || *r.Fields["account"] != "Recovered" {
			t.Error(r.Fields)
		}
		referenceFormUnchanged(t, orphan, before)
	})
	for _, kind := range []string{"unknown", "duplicate", "checkbox", "invalid-checkbox"} {
		t.Run(kind, func(t *testing.T) {
			values := map[string]string{"missing": "Ada"}
			var data []byte
			var err error
			switch kind {
			case "unknown":
				data, err = os.ReadFile("../../testdata/reference-mirror/library-form.pdf")
			case "duplicate":
				data, err = os.ReadFile("../../testdata/reference-mirror/library-form-duplicate.pdf")
				values = map[string]string{"account": "Ada"}
			default:
				data, err = os.ReadFile("../../testdata/reference-mirror/library-checkbox.pdf")
				values = map[string]string{"approved": "/Yes"}
				if kind == "invalid-checkbox" {
					values["approved"] = "/NotAState"
				}

			}
			if err != nil {
				t.Fatal(err)
			}
			before := append([]byte(nil), data...)
			input := writeTemp(t, "input.pdf", data)
			out := filepath.Join(t.TempDir(), "output.pdf")
			_, e := Fill(ctx, input, out, values, false)
			if kind == "checkbox" {
				if e != nil {
					t.Fatal(e)
				}
				r, e := Report(ctx, out)
				if e != nil {
					t.Fatal(e)
				}
				if r.Fields["approved"] == nil || *r.Fields["approved"] != "/Yes" {
					t.Error(r.Fields)
				}
			} else {
				if e == nil {
					t.Error("invalid field accepted")
				}
				if _, e := os.Stat(out); !errors.Is(e, os.ErrNotExist) {
					t.Errorf("published invalid output: %v", e)
				}
			}
			referenceFormUnchanged(t, input, before)
		})
	}
}

func TestReferenceStandaloneLibrariesRenderedFonts(t *testing.T) {
	// Exact original ReportLab canvas, including its default page and font resources.
	data, err := os.ReadFile("../../testdata/reference-mirror/runtime-font-render.pdf")
	if err != nil {
		t.Fatal(err)
	}

	t.Run("family-and-pages", func(t *testing.T) {
		got, e := VerifyFonts(context.Background(), data, []string{"Helvetica"})
		if e != nil || !reflect.DeepEqual(got, []FontUse{{Family: "Helvetica", Pages: []int{1}}}) {
			t.Errorf("fonts: %v %v", got, e)
		}
	})
	t.Run("wrong-family", func(t *testing.T) {
		_, e := VerifyFonts(context.Background(), data, []string{"Arial"})
		if e == nil || !strings.Contains(e.Error(), "Unapproved rendered fonts") {
			t.Errorf("wrong font error: %v", e)
		}
	})
	for i, policy := range [][]string{{}, nil, {""}} {
		t.Run(fmt.Sprintf("empty-policy-%d", i), func(t *testing.T) {
			if _, e := VerifyFonts(context.Background(), data, policy); e == nil {
				t.Error("empty font policy accepted")
			}
		})
	}
}
