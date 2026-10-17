package pdf

import (
	"context"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFlowPaginationHeadersAndRenderMargins(t *testing.T) {
	ctx := context.Background()
	rows := [][]string{}
	for i := 1; i <= 65; i++ {
		rows = append(rows, []string{fmt.Sprintf("Item-%03d", i), fmt.Sprint(i * 10)})
	}
	spec := CreateSpec{Paper: "A4", Flow: &FlowSpec{Blocks: []FlowBlock{
		{Kind: "heading", Text: "Flowing revenue report"},
		{Kind: "paragraph", Text: strings.Repeat("Revenue includes subscriptions and services. ", 16)},
		{Kind: "table", Header: []string{"Item", "Amount"}, Rows: rows, ColumnWidths: []int{75, 25}},
		{Kind: "paragraph", Text: "End of report."},
	}}}
	dest := filepath.Join(t.TempDir(), "flow.pdf")
	if err := Create(ctx, spec, ".", dest); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	text, err := PageText(ctx, raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(text) < 3 {
		t.Fatalf("expected paginated report, got %d", len(text))
	}
	all := strings.Join(text, "\n")
	for i := 1; i <= 65; i++ {
		key := fmt.Sprintf("Item-%03d", i)
		if strings.Count(all, key) != 1 {
			t.Fatalf("missing or duplicated %s", key)
		}
	}
	if !strings.Contains(all, "End of report.") {
		t.Fatal("missing final paragraph")
	}
	for p, s := range text {
		if strings.Contains(s, "Item-") && !strings.Contains(s, "Amount") {
			t.Fatalf("page %d missing repeated header", p+1)
		}
	}
	n, err := RenderImages(ctx, raw, RenderOptions{DPI: 72}, func(page int, img *image.RGBA) error {
		ink := 0
		for y := 0; y < img.Bounds().Dy(); y++ {
			for x := 0; x < img.Bounds().Dx(); x++ {
				c := img.RGBAAt(x, y)
				if c.R < 200 || c.G < 200 || c.B < 200 {
					ink++
					if x < 65 || x > img.Bounds().Dx()-65 || y < 60 || y > img.Bounds().Dy()-65 {
						return fmt.Errorf("page %d paints outside margins at %d,%d", page, x, y)
					}
				}
			}
		}
		if ink < 200 {
			return fmt.Errorf("page %d has no meaningful content", page)
		}
		return nil
	})
	if err != nil || n != len(text) {
		t.Fatal(n, err)
	}
}
func TestFlowRejectsInvalidLayout(t *testing.T) {
	for _, f := range []*FlowSpec{
		{Margin: 500, Blocks: []FlowBlock{{Kind: "paragraph", Text: "x"}}},
		{Blocks: []FlowBlock{{Kind: "paragraph", Text: strings.Repeat("X", 300)}}},
		{Blocks: []FlowBlock{{Kind: "table", Header: []string{"a", "b"}, Rows: [][]string{{"x"}}}}},
		{Blocks: []FlowBlock{{Kind: "table", Header: []string{"a"}, Rows: [][]string{{strings.Repeat("X", 200)}}}}},
		{Blocks: []FlowBlock{{Kind: "bogus"}}},
	} {
		out := filepath.Join(t.TempDir(), "bad.pdf")
		if err := Create(context.Background(), CreateSpec{Flow: f}, ".", out); err == nil {
			t.Fatal("accepted invalid flow")
		}
		if _, err := os.Stat(out); !os.IsNotExist(err) {
			t.Fatal("created invalid output")
		}
	}
}
func TestFlowHeadingKeptWithFollowingTable(t *testing.T) {
	pages, err := flowPages(context.Background(), "A4", &FlowSpec{Blocks: []FlowBlock{{Kind: "spacer", Height: 660}, {Kind: "heading", Text: "Keep me"}, {Kind: "table", Header: []string{"Item"}, Rows: [][]string{{"Value"}}}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) != 2 || len(pages[0].Text) != 0 || len(pages[1].Text) != 1 || len(pages[1].Tables) != 1 {
		t.Fatal("heading orphaned")
	}
}

func TestFlowCancellationAndConflictingModes(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	spec := CreateSpec{Flow: &FlowSpec{Blocks: []FlowBlock{{Kind: "paragraph", Text: "Cancelled"}}}}
	out := filepath.Join(t.TempDir(), "out.pdf")
	if err := Create(ctx, spec, ".", out); err != context.Canceled {
		t.Fatal(err)
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Fatal("cancelled output exists")
	}
	spec.Pages = []CreatePage{{}}
	if err := Create(context.Background(), spec, ".", out); err == nil {
		t.Fatal("conflicting modes accepted")
	}
}

func TestWrappedFlowTablePagination(t *testing.T) {
	ctx := context.Background()
	rows := make([][]string, 35)
	for i := range rows {
		rows[i] = []string{fmt.Sprintf("Record-%02d", i), "First sentence fits across several lines.\nSecond sentence stays in this cell."}
	}
	flow := &FlowSpec{Blocks: []FlowBlock{{Kind: "spacer", Height: 580}, {Kind: "heading", Text: "Wrapped report"}, {Kind: "table", Header: []string{"Record identifier", "Detailed description repeated on every page"}, ColumnWidths: []int{18, 82}, Rows: rows}}}
	pages, err := flowPages(ctx, "A4", flow)
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) < 3 || len(pages[0].Text) != 0 {
		t.Fatal("heading orphaned or table did not paginate")
	}
	for _, p := range pages {
		for _, table := range p.Tables {
			if table.Header == nil || !strings.Contains(table.Header.Values[0], "\n") {
				t.Fatal("header not wrapped")
			}
			for _, row := range table.Values {
				if !strings.Contains(row[1], "\n") {
					t.Fatal("cell not wrapped")
				}
			}
		}
	}
	out := filepath.Join(t.TempDir(), "wrapped.pdf")
	if err := Create(ctx, CreateSpec{Flow: flow}, ".", out); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	text, err := PageText(ctx, raw)
	if err != nil {
		t.Fatal(err)
	}
	all := strings.Join(text, "\n")
	for i := range rows {
		if strings.Count(all, fmt.Sprintf("Record-%02d", i)) != 1 {
			t.Fatal("lost/duplicate row", i)
		}
	}
	for i, s := range text {
		if strings.Contains(s, "Record-") && !strings.Contains(s, "Detailed") {
			t.Fatal("missing header", i)
		}
	}
	if strings.Count(all, "Second sentence") != len(rows) {
		t.Fatal("lost explicit second lines")
	}
	_, err = RenderImages(ctx, raw, RenderOptions{DPI: 72}, func(page int, img *image.RGBA) error {
		for y := 0; y < img.Bounds().Dy(); y++ {
			for x := 0; x < img.Bounds().Dx(); x++ {
				c := img.RGBAAt(x, y)
				if (c.R < 200 || c.G < 200 || c.B < 200) && (x < 65 || x > img.Bounds().Dx()-65 || y < 60 || y > img.Bounds().Dy()-65) {
					return fmt.Errorf("page %d outside margins", page)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestOversizeWrappedFlowRowRejected(t *testing.T) {
	out := filepath.Join(t.TempDir(), "too-tall.pdf")
	err := Create(context.Background(), CreateSpec{Flow: &FlowSpec{Blocks: []FlowBlock{{Kind: "table", Header: []string{"Header"}, Rows: [][]string{{strings.Repeat("line\n", 100)}}}}}}, ".", out)
	if err == nil {
		t.Fatal("accepted row taller than page")
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Fatal("created invalid output")
	}
}
