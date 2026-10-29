package pdf

import (
	"bytes"
	"context"
	"image"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func images(t *testing.T, data []byte) []*image.RGBA {
	t.Helper()
	var out []*image.RGBA
	_, err := RenderImages(context.Background(), data, RenderOptions{DPI: 144}, func(_ int, img *image.RGBA) error {
		out = append(out, img)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// radioMarks samples the centres of the three radio buttons at 144 DPI.
func radioMarks(img *image.RGBA) []bool {
	var out []bool
	for i := 0; i < 3; i++ {
		c := img.RGBAAt(100, 180+i*100)
		out = append(out, min(c.R, c.G, c.B) < 60)
	}
	return out
}

func sameImage(a, b *image.RGBA) bool {
	return a.Rect == b.Rect && bytes.Equal(a.Pix, b.Pix)
}

func TestReportInventory(t *testing.T) {
	rep, err := ReportBytes(context.Background(), formPDF())
	if err != nil {
		t.Fatal(err)
	}
	if rep.Pages != 2 || !rep.HasFormTree || len(rep.Widgets) != 6 || len(rep.Issues) != 0 {
		t.Fatalf("report %+v", rep)
	}
	if *rep.Fields["answer"] != "/A" || *rep.Fields["name"] != "Original" || *rep.Fields["approved"] != "/Yes" {
		t.Fatalf("fields %v", rep.Fields)
	}
}

func TestRadioFillPreservesSourceAndSelectsOnlyRequested(t *testing.T) {
	src := writeTemp(t, "source.pdf", formPDF())
	before, _ := os.ReadFile(src)
	for state, want := range map[string][]bool{"A": {true, false, false}, "B": {false, true, false}, "C": {false, false, true}} {
		out := filepath.Join(t.TempDir(), state+".pdf")
		if _, err := Fill(context.Background(), src, out, map[string]string{"answer": "/" + state}, false); err != nil {
			t.Fatal(err)
		}
		rep, err := Report(context.Background(), out)
		if err != nil {
			t.Fatal(err)
		}
		if *rep.Fields["answer"] != "/"+state || len(rep.Issues) > 0 {
			t.Fatalf("%s: %v %v", state, *rep.Fields["answer"], rep.Issues)
		}
		data, _ := os.ReadFile(out)
		if got := radioMarks(images(t, data)[0]); !equalBools(got, want) {
			t.Fatalf("%s marks %v", state, got)
		}
	}
	after, _ := os.ReadFile(src)
	if !bytes.Equal(before, after) {
		t.Fatal("source changed")
	}
}

func equalBools(a, b []bool) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestFillRejectsMalformedInput(t *testing.T) {
	src := writeTemp(t, "malformed.pdf", []byte("not a PDF"))
	out := filepath.Join(t.TempDir(), "filled.pdf")
	if _, err := Fill(context.Background(), src, out, nil, false); err == nil {
		t.Fatal("malformed input unexpectedly filled")
	}
	if _, err := os.Stat(out); err == nil {
		t.Fatal("output published for malformed input")
	}
}

func TestFillRejectsUnknownInvalidAndExistingOutput(t *testing.T) {
	src := writeTemp(t, "source.pdf", formPDF())
	dir := t.TempDir()
	cases := map[string]map[string]string{
		"unknown PDF fields":   {"missing": "x"},
		"invalid button state": {"answer": "/Z"},
	}
	for msg, vals := range cases {
		out := filepath.Join(dir, "x.pdf")
		_, err := Fill(context.Background(), src, out, vals, false)
		if err == nil || !strings.Contains(err.Error(), msg) {
			t.Fatalf("want %q got %v", msg, err)
		}
		if _, err := os.Stat(out); err == nil {
			t.Fatal("output published on failure")
		}
	}
	if _, err := Fill(context.Background(), src, src, nil, false); err == nil {
		t.Fatal("in-place fill allowed")
	}
}

func TestInvalidSavedStateFailsValidation(t *testing.T) {
	objs := formObjs()
	objs[6] = strings.Replace(objs[6], "/AS /A", "/AS /Off", 1)
	bad := rawPDF(objs)
	src := writeTemp(t, "bad.pdf", bad)
	rep, err := Report(context.Background(), src)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Issues) == 0 {
		t.Fatal("expected issues")
	}
	out := filepath.Join(t.TempDir(), "o.pdf")
	if _, err := Fill(context.Background(), src, out, nil, false); err == nil || !strings.Contains(err.Error(), "validation") {
		t.Fatalf("got %v", err)
	}
	if _, err := os.Stat(out); err == nil {
		t.Fatal("output published")
	}
}

func TestFlattenKeepsValuesAndRemovesForm(t *testing.T) {
	src := writeTemp(t, "source.pdf", formPDF())
	out := filepath.Join(t.TempDir(), "flat.pdf")
	if _, err := Fill(context.Background(), src, out, map[string]string{"answer": "/B", "name": "Ada"}, true); err != nil {
		t.Fatal(err)
	}
	rep, err := Report(context.Background(), out)
	if err != nil {
		t.Fatal(err)
	}
	if rep.HasFormTree || len(rep.Widgets) != 0 {
		t.Fatalf("form remains: %+v", rep)
	}
	data, _ := os.ReadFile(out)
	if got := radioMarks(images(t, data)[0]); !equalBools(got, []bool{false, true, false}) {
		t.Fatalf("marks %v", got)
	}
	text, err := PageText(context.Background(), data)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text[1], "Ada") || !strings.Contains(text[1], "Alpha") || strings.Contains(text[1], "Original") {
		t.Fatalf("page 2 text %q", text[1])
	}
}

func TestFlattenWithoutEditsMatchesInteractiveRendering(t *testing.T) {
	src := formPDF()
	path := writeTemp(t, "source.pdf", src)
	out := filepath.Join(t.TempDir(), "flat.pdf")
	if _, err := Fill(context.Background(), path, out, nil, true); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(out)
	orig, flat := images(t, src), images(t, data)
	if !sameImage(orig[0], flat[0]) {
		t.Fatal("page 1 pixels differ after flatten")
	}
}

func TestFlattenMapsResizedAndTransformedAppearances(t *testing.T) {
	cases := []struct{ bbox, m string }{{"[0 0 20 20]", "[1 0 0 1 0 0]"}, {"[0 0 20 20]", "[0 1 -1 0 20 0]"}, {"[0 0 20 20]", "[0.5 0 0 0.5 8 12]"}, {"[5 3 25 23]", "[1 0 0 1 0 0]"}}
	for i, tc := range cases {
		objs := formObjs()
		objs[10] = strings.Replace(objs[10], "/Rect [150 300 170 320]", "/Rect [150 280 210 330]", 1)
		for _, n := range []int{24, 25} {
			objs[n] = strings.Replace(objs[n], "/BBox [0 0 20 20]", "/BBox "+tc.bbox+" /Matrix "+tc.m, 1)
		}
		base := rawPDF(objs)
		path := writeTemp(t, "t.pdf", base)
		out := filepath.Join(t.TempDir(), "flat.pdf")
		if _, err := Fill(context.Background(), path, out, nil, true); err != nil {
			t.Fatalf("%d: %v", i, err)
		}
		data, _ := os.ReadFile(out)
		if !sameImage(images(t, base)[0], images(t, data)[0]) {
			t.Fatalf("%d: pixels differ", i)
		}
	}
}

func TestFlattenInvalidGeometryPublishesNothing(t *testing.T) {
	objs := formObjs()
	objs[24] = strings.Replace(objs[24], "/BBox [0 0 20 20]", "/BBox [0 0 0 20]", 1)
	path := writeTemp(t, "bad.pdf", rawPDF(objs))
	out := filepath.Join(t.TempDir(), "flat.pdf")
	if _, err := Fill(context.Background(), path, out, nil, true); err == nil || !strings.Contains(err.Error(), "geometry") {
		t.Fatalf("got %v", err)
	}
	if _, err := os.Stat(out); err == nil {
		t.Fatal("output published")
	}
}

func TestFlattenIndependentOfPageGraphicsState(t *testing.T) {
	objs := formObjs()
	objs[16] = stream("", "BT /Helv 12 Tf 20 370 Td (Form page 1) Tj ET 2 0 0 2 10 10 cm 0 0 10 10 re W n")
	src := rawPDF(objs)
	path := writeTemp(t, "state.pdf", src)
	out := filepath.Join(t.TempDir(), "flat.pdf")
	if _, err := Fill(context.Background(), path, out, nil, true); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(out)
	if !sameImage(images(t, src)[0], images(t, data)[0]) {
		t.Fatal("graphics state leaked into flattened widgets")
	}
}
