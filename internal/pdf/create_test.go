package pdf

import (
	"context"
	"encoding/json"
	"image"
	"os"
	"path/filepath"
	"testing"
)

func TestCreateReportAndInteractiveForm(t *testing.T) {
	data, e := os.ReadFile("../../testdata/workflows/revenue-pdf.json")
	if e != nil {
		t.Fatal(e)
	}
	var spec CreateSpec
	if e = json.Unmarshal(data, &spec); e != nil {
		t.Fatal(e)
	}
	ctx := context.Background()
	dir := t.TempDir()
	src := filepath.Join(dir, "report.pdf")
	if e = Create(ctx, spec, ".", src); e != nil {
		t.Fatal(e)
	}
	report, e := Report(ctx, src)
	if e != nil {
		t.Fatal(e)
	}
	if report.Pages != 1 || len(report.Issues) != 0 || len(report.Widgets) != 2 || *report.Fields["reviewer"] != "Draft" || *report.Fields["approved"] != "/Off" {
		t.Fatalf("report %+v", report)
	}
	raw, e := os.ReadFile(src)
	if e != nil {
		t.Fatal(e)
	}
	// Check meaningful geometry/color: the table header must contain a dark
	// blue rectangle, and text must paint black ink above the table.
	n, e := RenderImages(ctx, raw, RenderOptions{DPI: 72}, func(_ int, img *image.RGBA) error {
		blue, ink := 0, 0
		for y := 0; y < img.Rect.Dy(); y++ {
			for x := 0; x < img.Rect.Dx(); x++ {
				c := img.RGBAAt(x, y)
				if c.R < 45 && c.G >= 35 && c.G < 75 && c.B >= 55 && c.B < 100 {
					blue++
				}
				if y < 160 && c.R < 80 && c.G < 80 && c.B < 80 {
					ink++
				}
			}
		}
		if blue < 1000 || ink < 100 {
			t.Fatalf("missing header or report text: blue=%d ink=%d", blue, ink)
		}
		return nil
	})
	if e != nil || n != 1 {
		t.Fatal(n, e)
	}
	filled := filepath.Join(dir, "filled.pdf")
	if _, e = Fill(ctx, src, filled, map[string]string{"reviewer": "Reviewed", "approved": "/Yes"}, false); e != nil {
		t.Fatal(e)
	}
	got, e := Report(ctx, filled)
	if e != nil {
		t.Fatal(e)
	}
	if *got.Fields["reviewer"] != "Reviewed" || *got.Fields["approved"] != "/Yes" || len(got.Issues) != 0 {
		t.Fatal(got)
	}
	filledData, _ := os.ReadFile(filled)
	a, b := images(t, raw), images(t, filledData)
	if sameImage(a[0], b[0]) {
		t.Fatal("form changes did not affect saved appearance rendering")
	}
	flat := filepath.Join(dir, "flat.pdf")
	if _, e = Fill(ctx, filled, flat, map[string]string{}, true); e != nil {
		t.Fatal(e)
	}
	got, e = Report(ctx, flat)
	if e != nil || got.HasFormTree || len(got.Widgets) != 0 {
		t.Fatal(got, e)
	}
	if e = Create(ctx, spec, ".", src); e == nil {
		t.Fatal("overwrote output")
	}
	after, _ := os.ReadFile(src)
	if string(raw) != string(after) {
		t.Fatal("source changed")
	}
}

func TestRejectUnicodeWithoutWritingOutput(t *testing.T) {
	data, e := os.ReadFile("../../testdata/workflows/revenue-pdf.json")
	if e != nil {
		t.Fatal(e)
	}
	var spec CreateSpec
	if e = json.Unmarshal(data, &spec); e != nil {
		t.Fatal(e)
	}
	spec.Pages[0].Text[0].Value = "Revenue 世界"
	out := filepath.Join(t.TempDir(), "unsupported.pdf")
	if e = Create(context.Background(), spec, ".", out); e == nil {
		t.Fatal("accepted unsupported Unicode")
	}
	if _, e = os.Stat(out); !os.IsNotExist(e) {
		t.Fatal("left output", e)
	}
}
