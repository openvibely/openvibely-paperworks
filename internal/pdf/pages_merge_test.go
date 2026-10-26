package pdf

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu"
)

func mergeTestPDF(boxes ...[2]int) []byte {
	kids := make([]string, len(boxes))
	objs := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"",
	}
	for i, box := range boxes {
		objNum := i + 3
		kids[i] = fmt.Sprintf("%d 0 R", objNum)
		objs = append(objs, fmt.Sprintf(
			"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 %d %d] /Resources << >> >>",
			box[0], box[1],
		))
	}
	objs[1] = fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", strings.Join(kids, " "), len(boxes))
	return rawPDF(objs)
}

func mergeTestBookmarkedPDF(t *testing.T, boxes [][2]int, bookmarks []pdfcpu.Bookmark) (string, []byte) {
	t.Helper()
	ctx := context.Background()
	source := mergeTestPDF(boxes...)
	var out bytes.Buffer
	if err := api.AddBookmarks(ctx, bytes.NewReader(source), &out, bookmarks, false, config()); err != nil {
		t.Fatal(err)
	}
	data := out.Bytes()
	return writeTemp(t, "bookmarked.pdf", data), append([]byte(nil), data...)
}

func readMergeTestBookmarks(t *testing.T, path string) []pdfcpu.Bookmark {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	bookmarks, err := api.Bookmarks(context.Background(), bytes.NewReader(data), config())
	if err != nil {
		t.Fatal(err)
	}
	return bookmarks
}

func assertMergeTestDims(t *testing.T, path string, want [][2]int) {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	dims, err := api.PageDims(context.Background(), file, config())
	if err != nil {
		t.Fatal(err)
	}
	if len(dims) != len(want) {
		t.Fatalf("got %d page dimensions, want %d", len(dims), len(want))
	}
	for i, dim := range dims {
		if dim.Width != float64(want[i][0]) || dim.Height != float64(want[i][1]) {
			t.Errorf("page %d dimensions = %v, want %dx%d", i+1, dim, want[i][0], want[i][1])
		}
	}
}

func TestMergePreservesBookmarksAfterCover(t *testing.T) {
	ctx := context.Background()
	coverData := mergeTestPDF([2]int{200, 200})
	cover := writeTemp(t, "cover.pdf", coverData)
	report, reportData := mergeTestBookmarkedPDF(t, [][2]int{{300, 300}, {400, 400}, {500, 500}}, []pdfcpu.Bookmark{
		{
			Title:    "Report contents",
			PageFrom: 1,
			Kids: []pdfcpu.Bookmark{{
				Title:    "Chapter one",
				PageFrom: 2,
				Kids:     []pdfcpu.Bookmark{{Title: "Section 1.1", PageFrom: 3}},
			}},
		},
	})
	out := filepath.Join(t.TempDir(), "merged.pdf")
	if pages, err := Merge(ctx, []string{cover, report}, out); err != nil || pages != 4 {
		t.Fatalf("Merge() = (%d, %v), want (4, nil)", pages, err)
	}

	bookmarks := readMergeTestBookmarks(t, out)
	if len(bookmarks) != 1 {
		t.Fatalf("got %d top-level bookmarks, want 1: %+v", len(bookmarks), bookmarks)
	}
	root := bookmarks[0]
	if root.Title != "Report contents" || root.PageFrom != 2 || len(root.Kids) != 1 {
		t.Fatalf("unexpected report bookmark after cover: %+v", root)
	}
	chapter := root.Kids[0]
	if chapter.Title != "Chapter one" || chapter.PageFrom != 3 || len(chapter.Kids) != 1 {
		t.Fatalf("unexpected chapter bookmark: %+v", chapter)
	}
	if section := chapter.Kids[0]; section.Title != "Section 1.1" || section.PageFrom != 4 {
		t.Fatalf("unexpected section bookmark: %+v", section)
	}

	assertMergeTestDims(t, out, [][2]int{{200, 200}, {300, 300}, {400, 400}, {500, 500}})
	gotCover, err := os.ReadFile(cover)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(gotCover, coverData) {
		t.Fatal("merge changed the cover input")
	}
	gotReport, err := os.ReadFile(report)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(gotReport, reportData) {
		t.Fatal("merge changed the bookmarked report input")
	}
}

func TestMergeRetainsDuplicateBookmarkTitlesFromMultipleSources(t *testing.T) {
	ctx := context.Background()
	first, _ := mergeTestBookmarkedPDF(t, [][2]int{{100, 100}, {110, 110}}, []pdfcpu.Bookmark{{
		Title:    "Introduction",
		PageFrom: 1,
		Kids:     []pdfcpu.Bookmark{{Title: "Overview", PageFrom: 2}},
	}})
	second, _ := mergeTestBookmarkedPDF(t, [][2]int{{200, 200}, {210, 210}, {220, 220}}, []pdfcpu.Bookmark{{
		Title:    "Introduction",
		PageFrom: 2,
		Kids:     []pdfcpu.Bookmark{{Title: "Overview", PageFrom: 3}},
	}})
	out := filepath.Join(t.TempDir(), "merged.pdf")
	if pages, err := Merge(ctx, []string{first, second}, out); err != nil || pages != 5 {
		t.Fatalf("Merge() = (%d, %v), want (5, nil)", pages, err)
	}

	bookmarks := readMergeTestBookmarks(t, out)
	if len(bookmarks) != 2 {
		t.Fatalf("got %d top-level bookmarks, want 2: %+v", len(bookmarks), bookmarks)
	}
	wantRoots := []int{1, 4}
	wantKids := []int{2, 5}
	for i, bookmark := range bookmarks {
		if bookmark.Title != "Introduction" || bookmark.PageFrom != wantRoots[i] || len(bookmark.Kids) != 1 {
			t.Errorf("bookmark %d = %+v, want duplicate title targeting page %d", i, bookmark, wantRoots[i])
			continue
		}
		if child := bookmark.Kids[0]; child.Title != "Overview" || child.PageFrom != wantKids[i] {
			t.Errorf("bookmark %d child = %+v, want destination page %d", i, child, wantKids[i])
		}
	}
	assertMergeTestDims(t, out, [][2]int{{100, 100}, {110, 110}, {200, 200}, {210, 210}, {220, 220}})
}
