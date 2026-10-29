package pdf

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

type failAfterBytesWriter struct {
	io.Writer
	remaining int
	err       error
}

func (w *failAfterBytesWriter) Write(p []byte) (int, error) {
	if w.remaining == 0 {
		return 0, w.err
	}
	if len(p) > w.remaining {
		limit := w.remaining
		n, err := w.Writer.Write(p[:limit])
		w.remaining -= n
		if err != nil {
			return n, err
		}
		if n != limit {
			return n, io.ErrShortWrite
		}
		return n, w.err
	}
	n, err := w.Writer.Write(p)
	w.remaining -= n
	return n, err
}

func splitTextFixture(pageCount int) []byte {
	objs := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"",
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>",
	}
	pageRefs := make([]string, pageCount)
	for i := range pageRefs {
		pageRefs[i] = fmt.Sprintf("%d 0 R", i+4)
	}
	objs[1] = fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", strings.Join(pageRefs, " "), pageCount)
	for i := range pageRefs {
		objs = append(objs, fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] /Resources << /Font << /F1 3 0 R >> >> /Contents %d 0 R >>", pageCount+4+i))
	}
	for i := 0; i < pageCount; i++ {
		content := fmt.Sprintf("BT /F1 12 Tf 20 100 Td (Page %03d) Tj ET", i+1)
		objs = append(objs, stream("", content))
	}
	return rawPDF(objs)
}

func splitImageFixture(pageCount int) []byte {
	const side = 32
	img := image.NewRGBA(image.Rect(0, 0, side, side))
	for y := 0; y < side; y++ {
		for x := 0; x < side; x++ {
			img.Set(x, y, color.RGBA{uint8(x * 7), uint8(y * 7), uint8((x + y) * 3), 255})
		}
	}
	var jpegData bytes.Buffer
	if err := jpeg.Encode(&jpegData, img, &jpeg.Options{Quality: 80}); err != nil {
		panic(err)
	}
	objs := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"",
		stream(fmt.Sprintf("/Type /XObject /Subtype /Image /Width %d /Height %d /ColorSpace /DeviceRGB /BitsPerComponent 8 /Filter /DCTDecode", side, side), jpegData.String()),
	}
	pageRefs := make([]string, pageCount)
	for i := range pageRefs {
		pageRefs[i] = fmt.Sprintf("%d 0 R", i+4)
	}
	objs[1] = fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", strings.Join(pageRefs, " "), pageCount)
	for i := range pageRefs {
		objs = append(objs, fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] /Resources << /XObject << /Im0 3 0 R >> >> /Contents %d 0 R >>", pageCount+4+i))
	}
	for i := 0; i < pageCount; i++ {
		objs = append(objs, stream("", "q 160 0 0 160 20 20 cm /Im0 Do Q"))
	}
	return rawPDF(objs)
}

func splitDestinationFixture() []byte {
	return rawPDF([]string{
		"<< /Type /Catalog /Pages 2 0 R /Names << /Dests 5 0 R >> >>",
		"<< /Type /Pages /Kids [3 0 R 4 0 R] /Count 2 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] >>",
		"<< /Names [(first) [3 0 R /Fit] (second) [4 0 R /Fit]] >>",
	})
}

func splitFormDestinationFixture() []byte {
	objs := formObjs()
	objs[0] = strings.TrimSuffix(objs[0], " >>") + " /Names << /Dests 29 0 R >> >>"
	objs = append(objs, "<< /Names [(first) [3 0 R /Fit] (second) [4 0 R /Fit]] >>")
	return rawPDF(objs)
}

func outputPDFs(t *testing.T, files []string) [][]byte {
	t.Helper()
	out := make([][]byte, len(files))
	for i, path := range files {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		out[i] = data
	}
	return out
}

func requireTextPages(t *testing.T, got []byte, expected ...string) {
	t.Helper()
	texts, err := PageText(context.Background(), got)
	if err != nil {
		t.Fatal(err)
	}
	if len(texts) != len(expected) {
		t.Fatalf("page text count = %d, want %d: %#v", len(texts), len(expected), texts)
	}
	for i, want := range expected {
		if !strings.Contains(texts[i], want) {
			t.Errorf("page %d text %q does not contain %q", i+1, texts[i], want)
		}
	}
}

func TestSplitKeepsPageOrderDuplicatesAndSharedResources(t *testing.T) {
	src := writeTemp(t, "shared.pdf", splitTextFixture(5))
	outDir := t.TempDir()
	ranges := [][]int{{3, 1, 3}, {2, 3}, {2, 5}, {5, 2, 5}, {3}}
	files, err := Split(context.Background(), src, outDir, ranges)
	if err != nil {
		t.Fatal(err)
	}
	wantNames := []string{
		filepath.Join(outDir, "shared-001.pdf"),
		filepath.Join(outDir, "shared-002.pdf"),
		filepath.Join(outDir, "shared-003.pdf"),
		filepath.Join(outDir, "shared-004.pdf"),
		filepath.Join(outDir, "shared-005.pdf"),
	}
	if !reflect.DeepEqual(files, wantNames) {
		t.Fatalf("files = %#v, want %#v", files, wantNames)
	}
	for i, expected := range [][]string{{"Page 003", "Page 001", "Page 003"}, {"Page 002", "Page 003"}, {"Page 002", "Page 005"}, {"Page 005", "Page 002", "Page 005"}, {"Page 003"}} {
		requireTextPages(t, outputPDFs(t, []string{files[i]})[0], expected...)
	}
}

func TestSplitSinglePageSourceAndDefaultSinglePageGroups(t *testing.T) {
	t.Run("one-page source", func(t *testing.T) {
		src := writeTemp(t, "one.pdf", splitTextFixture(1))
		files, err := Split(context.Background(), src, t.TempDir(), [][]int{{1}})
		if err != nil {
			t.Fatal(err)
		}
		requireTextPages(t, outputPDFs(t, files)[0], "Page 001")
	})

	t.Run("one output per page", func(t *testing.T) {
		src := writeTemp(t, "default.pdf", splitTextFixture(3))
		files, err := Split(context.Background(), src, t.TempDir(), [][]int{{1}, {2}, {3}})
		if err != nil {
			t.Fatal(err)
		}
		if len(files) != 3 {
			t.Fatalf("got %d output files, want 3", len(files))
		}
		for i, file := range files {
			requireTextPages(t, outputPDFs(t, []string{file})[0], fmt.Sprintf("Page %03d", i+1))
		}
	})
}

func TestSplitFormAndAnnotationEquivalence(t *testing.T) {
	input := splitFormDestinationFixture()
	src := writeTemp(t, "forms.pdf", input)
	ranges := [][]int{{2}, {1}, {1, 2, 1}}
	files, err := Split(context.Background(), src, t.TempDir(), ranges)
	if err != nil {
		t.Fatal(err)
	}
	for i, pages := range ranges {
		want, err := SelectPages(context.Background(), input, pages)
		if err != nil {
			t.Fatal(err)
		}
		got := outputPDFs(t, []string{files[i]})[0]
		gotText, err := PageText(context.Background(), got)
		if err != nil {
			t.Fatal(err)
		}
		wantText, err := PageText(context.Background(), want)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(gotText, wantText) {
			t.Errorf("group %d text = %#v, want %#v", i+1, gotText, wantText)
		}
		gotReport, err := ReportBytes(context.Background(), got)
		if err != nil {
			t.Fatal(err)
		}
		wantReport, err := ReportBytes(context.Background(), want)
		if err != nil {
			t.Fatal(err)
		}
		widgetSummary := func(report *FormReport) [][4]any {
			out := make([][4]any, len(report.Widgets))
			for j, widget := range report.Widgets {
				out[j] = [4]any{widget.Page, widget.Name, widget.Value, widget.NormalAppearance}
			}
			return out
		}
		if !reflect.DeepEqual(gotReport.Fields, wantReport.Fields) || !reflect.DeepEqual(widgetSummary(gotReport), widgetSummary(wantReport)) || !reflect.DeepEqual(gotReport.Issues, wantReport.Issues) {
			t.Errorf("group %d form/annotation report differs: got fields=%v widgets=%v issues=%v, want fields=%v widgets=%v issues=%v", i+1, gotReport.Fields, widgetSummary(gotReport), gotReport.Issues, wantReport.Fields, widgetSummary(wantReport), wantReport.Issues)
		}
	}
}

func TestSplitRenderedPagesMatchCollect(t *testing.T) {
	input := splitImageFixture(3)
	src := writeTemp(t, "images.pdf", input)
	ranges := [][]int{{3, 1, 3}, {2}}
	files, err := Split(context.Background(), src, t.TempDir(), ranges)
	if err != nil {
		t.Fatal(err)
	}
	for i, pages := range ranges {
		baseline, err := SelectPages(context.Background(), input, pages)
		if err != nil {
			t.Fatal(err)
		}
		basePath := writeTemp(t, fmt.Sprintf("baseline-%d.pdf", i), baseline)
		baseImages, err := RenderPages(context.Background(), basePath, t.TempDir(), RenderOptions{DPI: 72})
		if err != nil {
			t.Fatal(err)
		}
		gotImages, err := RenderPages(context.Background(), files[i], t.TempDir(), RenderOptions{DPI: 72})
		if err != nil {
			t.Fatal(err)
		}
		if len(baseImages) != len(gotImages) {
			t.Fatalf("group %d rendered %d images, want %d", i+1, len(gotImages), len(baseImages))
		}
		for j := range baseImages {
			basePNG, err := os.ReadFile(baseImages[j])
			if err != nil {
				t.Fatal(err)
			}
			gotPNG, err := os.ReadFile(gotImages[j])
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(basePNG, gotPNG) {
				t.Errorf("group %d page %d rendering differs", i+1, j+1)
			}
		}
	}
}

func TestSplitDestinationStateIsIsolatedBetweenGroups(t *testing.T) {
	src := writeTemp(t, "destinations.pdf", splitDestinationFixture())
	files, err := Split(context.Background(), src, t.TempDir(), [][]int{{1}, {2}})
	if err != nil {
		t.Fatal(err)
	}
	for i, key := range []string{"first", "second"} {
		data, err := os.ReadFile(files[i])
		if err != nil {
			t.Fatal(err)
		}
		c, err := Load(context.Background(), data)
		if err != nil {
			t.Fatalf("load group %d: %v", i+1, err)
		}
		node := c.Names["Dests"]
		if node == nil {
			t.Fatalf("group %d lost destination name tree", i+1)
		}
		if _, found, err := node.Value(context.Background(), key); err != nil || !found {
			t.Errorf("group %d destination %q found=%v err=%v", i+1, key, found, err)
		}
	}
}

func TestSplitErrorsCancellationAndPartialResults(t *testing.T) {
	t.Run("negative selection in first group is rejected without output", func(t *testing.T) {
		src := writeTemp(t, "input.pdf", splitTextFixture(2))
		outDir := t.TempDir()
		files, err := Split(context.Background(), src, outDir, [][]int{{-1}, {1}})
		if err == nil || len(files) != 0 {
			t.Fatalf("files=%v err=%v, want rejection before any output", files, err)
		}
		entries, err := os.ReadDir(outDir)
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 0 {
			t.Fatalf("output directory contains files after invalid selection: %v", entries)
		}
	})

	t.Run("invalid selection after earlier output", func(t *testing.T) {
		src := writeTemp(t, "input.pdf", splitTextFixture(2))
		for _, tc := range []struct {
			name    string
			invalid int
		}{{"too large", 3}, {"zero", 0}} {
			t.Run(tc.name, func(t *testing.T) {
				files, err := Split(context.Background(), src, t.TempDir(), [][]int{{1}, {tc.invalid}})
				if err == nil || len(files) != 1 {
					t.Fatalf("files=%v err=%v, want first output and selection error", files, err)
				}
				if _, statErr := os.Stat(files[0]); statErr != nil {
					t.Fatalf("first output missing after later selection error: %v", statErr)
				}
			})
		}
	})

	t.Run("empty selection", func(t *testing.T) {
		src := writeTemp(t, "input.pdf", []byte("malformed"))
		_, err := Split(context.Background(), src, t.TempDir(), [][]int{{}})
		if err == nil || !strings.Contains(err.Error(), "no pages selected") {
			t.Fatalf("err=%v, want no pages selected", err)
		}
	})

	t.Run("malformed input", func(t *testing.T) {
		src := writeTemp(t, "input.pdf", []byte("malformed"))
		_, err := Split(context.Background(), src, t.TempDir(), [][]int{{1}})
		if err == nil {
			t.Fatal("expected malformed input error")
		}
	})

	t.Run("preexisting output is exclusive", func(t *testing.T) {
		src := writeTemp(t, "input.pdf", splitTextFixture(2))
		outDir := t.TempDir()
		path := filepath.Join(outDir, "input-001.pdf")
		if err := os.WriteFile(path, []byte("keep"), 0o644); err != nil {
			t.Fatal(err)
		}
		files, err := Split(context.Background(), src, outDir, [][]int{{1}, {2}})
		if err == nil || len(files) != 0 {
			t.Fatalf("files=%v err=%v, want exclusive-create error", files, err)
		}
		got, err := os.ReadFile(path)
		if err != nil || string(got) != "keep" {
			t.Fatalf("preexisting output changed: %q err=%v", got, err)
		}
	})

	t.Run("output creation failure after earlier output", func(t *testing.T) {
		src := writeTemp(t, "input.pdf", splitTextFixture(2))
		outDir := t.TempDir()
		second := filepath.Join(outDir, "input-002.pdf")
		if err := os.WriteFile(second, []byte("block"), 0o644); err != nil {
			t.Fatal(err)
		}
		files, err := Split(context.Background(), src, outDir, [][]int{{1}, {2}})
		if err == nil || len(files) != 1 {
			t.Fatalf("files=%v err=%v, want first result and second output creation failure", files, err)
		}
		if _, statErr := os.Stat(files[0]); statErr != nil {
			t.Fatalf("earlier output missing: %v", statErr)
		}
	})

	t.Run("write failure after earlier output", func(t *testing.T) {
		src := writeTemp(t, "input.pdf", splitTextFixture(2))
		outDir := t.TempDir()
		second := filepath.Join(outDir, "input-002.pdf")
		injectedErr := errors.New("injected output write failure")
		files, err := splitWithOutputWriter(context.Background(), src, outDir, [][]int{{1}, {2}}, nil, func(path string, write func(io.Writer) error) error {
			if path != second {
				return writeNewWith(path, write)
			}
			return writeNewWith(path, func(w io.Writer) error {
				return write(&failAfterBytesWriter{Writer: w, remaining: 1, err: injectedErr})
			})
		})
		if !errors.Is(err, injectedErr) || len(files) != 1 {
			t.Fatalf("files=%v err=%v, want first result and injected write failure", files, err)
		}
		if _, statErr := os.Stat(files[0]); statErr != nil {
			t.Fatalf("earlier output missing: %v", statErr)
		}
		if _, statErr := os.Stat(second); !os.IsNotExist(statErr) {
			t.Fatalf("partial second output remains after write failure: stat err=%v", statErr)
		}
	})

	t.Run("cancellation", func(t *testing.T) {
		src := writeTemp(t, "input.pdf", splitTextFixture(2))
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		files, err := Split(ctx, src, t.TempDir(), [][]int{{1}, {2}})
		if err == nil || len(files) != 0 {
			t.Fatalf("files=%v err=%v, want cancellation", files, err)
		}
	})

	t.Run("cancellation retains completed outputs", func(t *testing.T) {
		src := writeTemp(t, "input.pdf", splitTextFixture(2))
		outDir := t.TempDir()
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		files, err := split(ctx, src, outDir, [][]int{{1}, {2}}, cancel)
		if err == nil || len(files) != 1 {
			t.Fatalf("files=%v err=%v, want one completed result and cancellation", files, err)
		}
		if _, statErr := os.Stat(files[0]); statErr != nil {
			t.Fatalf("completed output missing after cancellation: %v", statErr)
		}
	})
}

func TestSplitBenchmarkFixturesAreDeterministic(t *testing.T) {
	first := splitTextFixture(50)
	second := splitTextFixture(50)
	if !bytes.Equal(first, second) {
		t.Fatal("text fixture is not deterministic")
	}
	a, b := sha256.Sum256(first), sha256.Sum256(second)
	if a != b {
		t.Fatal("fixture hashes differ")
	}
}

func splitLegacyBenchmark(ctx context.Context, src, outDir string, ranges [][]int) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	stem := strings.TrimSuffix(filepath.Base(src), filepath.Ext(src))
	for i, pages := range ranges {
		out, err := SelectPages(ctx, data, pages)
		if err != nil {
			return err
		}
		name := filepath.Join(outDir, fmt.Sprintf("%s-%03d.pdf", stem, i+1))
		if err := writeNew(name, out); err != nil {
			return err
		}
	}
	return nil
}

func BenchmarkSplit(b *testing.B) {
	fixtures := []struct {
		kind  string
		count int
		data  []byte
	}{
		{kind: "text", count: 50, data: splitTextFixture(50)},
		{kind: "text", count: 100, data: splitTextFixture(100)},
		{kind: "text", count: 200, data: splitTextFixture(200)},
		{kind: "image", count: 100, data: splitImageFixture(100)},
		{kind: "image", count: 200, data: splitImageFixture(200)},
	}
	for _, fixture := range fixtures {
		fixture := fixture
		hash := sha256.Sum256(fixture.data)
		b.Logf("fixture kind=%s pages=%d bytes=%d sha256=%x", fixture.kind, fixture.count, len(fixture.data), hash)
		allPages := make([]int, fixture.count)
		perPage := make([][]int, fixture.count)
		for i := range allPages {
			allPages[i] = i + 1
			perPage[i] = []int{i + 1}
		}
		for _, grouping := range []struct {
			name   string
			ranges [][]int
		}{{"single_group", [][]int{allPages}}, {"one_page_per_output", perPage}} {
			grouping := grouping
			for _, method := range []string{"legacy", "split"} {
				method := method
				b.Run(fmt.Sprintf("%s_%d/%s/%s", fixture.kind, fixture.count, grouping.name, method), func(b *testing.B) {
					src := filepath.Join(b.TempDir(), "fixture.pdf")
					if err := os.WriteFile(src, fixture.data, 0o644); err != nil {
						b.Fatal(err)
					}
					outDir := b.TempDir()
					b.ReportAllocs()
					b.ResetTimer()
					for i := 0; i < b.N; i++ {
						var err error
						if method == "legacy" {
							err = splitLegacyBenchmark(context.Background(), src, outDir, grouping.ranges)
							if err != nil {
								b.Fatal(err)
							}
						} else if _, err = Split(context.Background(), src, outDir, grouping.ranges); err != nil {
							b.Fatal(err)
						}
						b.StopTimer()
						for i := range grouping.ranges {
							name := filepath.Join(outDir, fmt.Sprintf("fixture-%03d.pdf", i+1))
							if err := os.Remove(name); err != nil {
								b.Fatal(err)
							}
						}
						b.StartTimer()
					}
				})
			}
		}
	}
}
