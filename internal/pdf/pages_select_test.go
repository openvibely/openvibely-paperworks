package pdf

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestSelectPagesBySpecMatchesCollectAndPreservesOrder(t *testing.T) {
	input := splitTextFixture(4)
	cases := []struct {
		name string
		spec string
		want []int
	}{
		{name: "single", spec: "2", want: []int{2}},
		{name: "ordered", spec: "1-3", want: []int{1, 2, 3}},
		{name: "reordered", spec: "3,1,2", want: []int{3, 1, 2}},
		{name: "duplicate", spec: "2,2,1", want: []int{2, 2, 1}},
		{name: "first and last", spec: "1,4", want: []int{1, 4}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, pages, err := SelectPagesBySpec(context.Background(), input, tc.spec)
			if err != nil {
				t.Fatal(err)
			}
			if !equalPages(pages, tc.want) {
				t.Fatalf("pages = %v, want %v", pages, tc.want)
			}
			want, err := SelectPages(context.Background(), input, tc.want)
			if err != nil {
				t.Fatal(err)
			}
			gotStreams := selectTestPageContents(t, got)
			wantStreams := selectTestPageContents(t, want)
			if len(gotStreams) != len(wantStreams) {
				t.Fatalf("got %d page content streams, want %d", len(gotStreams), len(wantStreams))
			}
			for i := range gotStreams {
				if !bytes.Equal(gotStreams[i], wantStreams[i]) {
					t.Fatalf("page %d content bytes differ", i+1)
				}
			}
			texts, err := PageText(context.Background(), got)
			if err != nil {
				t.Fatal(err)
			}
			if len(texts) != len(tc.want) {
				t.Fatalf("got %d pages, want %d", len(texts), len(tc.want))
			}
			for i, page := range tc.want {
				if !strings.Contains(texts[i], fmt.Sprintf("Page %03d", page)) {
					t.Fatalf("output page %d does not match source page %d: %q", i+1, page, texts[i])
				}
			}
		})
	}
}

func TestSelectPagesRejectsInvalidPageNumbers(t *testing.T) {
	input := splitTextFixture(2)
	for _, page := range []int{-1, -2, 0, 3} {
		t.Run(fmt.Sprintf("page_%d", page), func(t *testing.T) {
			if _, err := SelectPages(context.Background(), input, []int{page}); err == nil {
				t.Fatalf("SelectPages(%d) unexpectedly succeeded", page)
			}
		})
	}
}

func TestSelectPagesBySpecRejectsEmptyAndOutOfRangeSelections(t *testing.T) {
	input := splitTextFixture(2)
	for _, spec := range []string{"", "0", "3", "2-1", "x"} {
		t.Run("selection_"+strings.ReplaceAll(spec, "/", "_"), func(t *testing.T) {
			_, _, err := SelectPagesBySpec(context.Background(), input, spec)
			var selectionErr PageSelectionError
			if !errors.As(err, &selectionErr) {
				t.Fatalf("error = %v, want PageSelectionError", err)
			}
		})
	}
}

func TestSelectPagesBySpecRejectsMalformedPDFAndCancellation(t *testing.T) {
	if _, _, err := SelectPagesBySpec(context.Background(), []byte("not a PDF"), "1"); err == nil {
		t.Fatal("malformed PDF unexpectedly succeeded")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := SelectPagesBySpec(ctx, splitTextFixture(1), "1"); err == nil {
		t.Fatal("cancelled selection unexpectedly succeeded")
	}
}

func TestSelectPagesBySpecHasOnePreparationPass(t *testing.T) {
	passes := 0
	if _, _, err := selectPagesBySpec(context.Background(), splitTextFixture(2), "2,1", func() { passes++ }); err != nil {
		t.Fatal(err)
	}
	if passes != 1 {
		t.Fatalf("preparation passes = %d, want 1", passes)
	}
}

func selectTestPageContents(t *testing.T, data []byte) [][]byte {
	t.Helper()
	c, err := Load(context.Background(), data)
	if err != nil {
		t.Fatal(err)
	}
	out := make([][]byte, c.PageCount)
	for i := 1; i <= c.PageCount; i++ {
		page, _, _, err := c.PageDict(context.Background(), i, false)
		if err != nil {
			t.Fatal(err)
		}
		out[i-1], err = c.PageContent(page, i)
		if err != nil {
			t.Fatal(err)
		}
	}
	return out
}

func equalPages(a, b []int) bool {
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
