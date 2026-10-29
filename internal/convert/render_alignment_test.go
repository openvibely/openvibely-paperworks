package convert

import (
	"context"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/openvibely/openvibely-paperworks/internal/pdf"
)

func alignmentPage(c color.Color) image.Image {
	page := image.NewRGBA(image.Rect(0, 0, 48, 48))
	draw.Draw(page, page.Bounds(), image.NewUniform(c), image.Point{}, draw.Src)
	return page
}

func alignmentPages(colors ...color.Color) []image.Image {
	pages := make([]image.Image, len(colors))
	for i, c := range colors {
		pages[i] = alignmentPage(c)
	}
	return pages
}

func alignmentPairs(pairs ...int) []image.Image {
	palette := []color.Color{
		color.RGBA{R: 255, A: 255},
		color.RGBA{G: 255, A: 255},
		color.RGBA{B: 255, A: 255},
		color.RGBA{R: 255, B: 255, A: 255},
	}
	pages := make([]image.Image, len(pairs))
	for i, p := range pairs {
		pages[i] = alignmentPage(palette[p])
	}
	return pages
}

func matched(before, after int) pageAlignment {
	return pageAlignment{before: before, after: after, kind: pageMatch}
}

func removed(before int) pageAlignment {
	return pageAlignment{before: before, kind: pageRemoved}
}

func added(after int) pageAlignment {
	return pageAlignment{after: after, kind: pageAdded}
}

func TestAlignPagesInsertionsAndRemovals(t *testing.T) {
	cases := []struct {
		name          string
		before, after []image.Image
		want          []pageAlignment
	}{
		{"leading insertion", alignmentPairs(0, 1, 2), alignmentPairs(3, 0, 1, 2), []pageAlignment{added(0), matched(0, 1), matched(1, 2), matched(2, 3)}},
		{"trailing insertion", alignmentPairs(0, 1, 2), alignmentPairs(0, 1, 2, 3), []pageAlignment{matched(0, 0), matched(1, 1), matched(2, 2), added(3)}},
		{"middle insertion", alignmentPairs(0, 1, 2), alignmentPairs(0, 3, 1, 2), []pageAlignment{matched(0, 0), added(1), matched(1, 2), matched(2, 3)}},
		{"leading removal", alignmentPairs(0, 1, 2), alignmentPairs(1, 2), []pageAlignment{removed(0), matched(1, 0), matched(2, 1)}},
		{"trailing removal", alignmentPairs(0, 1, 2), alignmentPairs(0, 1), []pageAlignment{matched(0, 0), matched(1, 1), removed(2)}},
		{"middle removal", alignmentPairs(0, 1, 2), alignmentPairs(0, 2), []pageAlignment{matched(0, 0), removed(1), matched(2, 1)}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := alignPages(tc.before, tc.after); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("alignment = %#v, want %#v", got, tc.want)
			}
		})
	}
}

func TestAlignPagesRepeatedIdenticalPagesAreDeterministic(t *testing.T) {
	page := alignmentPage(color.RGBA{R: 80, G: 120, B: 180, A: 255})
	before := []image.Image{page, page}
	after := []image.Image{page, page, page}
	want := []pageAlignment{matched(0, 0), matched(1, 1), added(2)}
	for i := 0; i < 5; i++ {
		if got := alignPages(before, after); !reflect.DeepEqual(got, want) {
			t.Fatalf("run %d alignment = %#v, want %#v", i, got, want)
		}
	}
}

func writeAlignmentPNG(t *testing.T, path string, img image.Image) string {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, img); err != nil {
		f.Close()
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

func runPageCompare(t *testing.T, beforePages, afterPages []image.Image) (map[string]any, string) {
	t.Helper()
	root := t.TempDir()
	pagePaths := func(name string, pages []image.Image) []string {
		paths := make([]string, len(pages))
		for i, page := range pages {
			paths[i] = writeAlignmentPNG(t, filepath.Join(root, name+"-"+string(rune('a'+i))+".png"), page)
		}
		return paths
	}
	beforePaths := pagePaths("before", beforePages)
	afterPaths := pagePaths("after", afterPages)
	ctx := withDeps(dependencies{render: func(_ context.Context, input, _ string, _ pdf.RenderOptions) ([]string, error) {
		switch input {
		case "before.pdf":
			return beforePaths, nil
		case "after.pdf":
			return afterPaths, nil
		default:
			t.Fatalf("unexpected render input %q", input)
			return nil, nil
		}
	}})
	output := filepath.Join(root, "comparison")
	result, err := Compare(ctx, "before.pdf", "after.pdf", output, pdf.RenderOptions{})
	if err != nil {
		t.Fatal(err)
	}
	return result, output
}

func TestComparePageAlignmentReportsShiftsAndPixelDiffs(t *testing.T) {
	beforeChanged := image.NewRGBA(image.Rect(0, 0, 48, 48))
	draw.Draw(beforeChanged, beforeChanged.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	draw.Draw(beforeChanged, image.Rect(8, 10, 24, 36), image.NewUniform(color.Black), image.Point{}, draw.Src)
	afterChanged := image.NewRGBA(image.Rect(0, 0, 48, 48))
	draw.Draw(afterChanged, afterChanged.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	draw.Draw(afterChanged, image.Rect(8, 10, 24, 36), image.NewUniform(color.RGBA{R: 200, G: 20, B: 20, A: 255}), image.Point{}, draw.Src)

	before := []image.Image{
		alignmentPage(color.RGBA{R: 230, G: 40, B: 40, A: 255}),
		beforeChanged,
		alignmentPage(color.RGBA{B: 230, G: 40, A: 255}),
	}
	after := []image.Image{
		before[0],
		alignmentPage(color.RGBA{R: 220, G: 220, B: 30, A: 255}),
		afterChanged,
		before[2],
	}
	result, output := runPageCompare(t, before, after)
	pages := result["pages"].([]PageChange)
	if len(pages) != 4 {
		t.Fatalf("pages = %#v", pages)
	}
	want := []PageChange{
		{Page: 1, Status: "same", Image: filepath.Join(output, "diff-0001.png")},
		{Page: 2, Status: "added"},
		{Page: 3, Status: "changed", BeforePage: 2, AfterPage: 3, Bounds: []int{8, 10, 24, 36}, Image: filepath.Join(output, "diff-0003.png")},
		{Page: 4, Status: "same", BeforePage: 3, AfterPage: 4, Image: filepath.Join(output, "diff-0004.png")},
	}
	if !reflect.DeepEqual(pages, want) {
		t.Fatalf("pages = %#v, want %#v", pages, want)
	}
	changedDiff, err := loadPNG(pages[2].Image)
	if err != nil {
		t.Fatal(err)
	}
	if _, bounds := diffImages(beforeChanged, afterChanged); bounds == nil {
		t.Fatal("test images unexpectedly have no pixel differences")
	}
	if changedDiff.Bounds().Empty() {
		t.Fatal("changed page diff image is empty")
	}
}
