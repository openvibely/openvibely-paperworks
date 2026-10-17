package pdf

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// A deterministic context cancels during line iteration, without timing races.
type lineCancelContext struct {
	context.Context
	calls int
}

func (c *lineCancelContext) Err() error {
	c.calls++
	if c.calls >= 5 {
		return context.Canceled
	}
	return nil
}

func TestFlowBlankLineCancellationAndLimits(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, text := range []string{"", "\n", strings.Repeat("\n", 999999)} {
		if _, err := wrapFlow(ctx, text, "Helvetica", 12, 400); !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	}
	mid := &lineCancelContext{Context: context.Background()}
	if lines, err := wrapFlow(mid, strings.Repeat("\n", 999999), "Helvetica", 12, 400); !errors.Is(err, context.Canceled) || lines != nil {
		t.Fatalf("lines=%d error=%v", len(lines), err)
	}
	lines, err := wrapFlow(context.Background(), strings.Repeat("\n", 9999), "Helvetica", 12, 400)
	if err != nil || len(lines) != 10000 {
		t.Fatal(len(lines), err)
	}
	for _, text := range []string{strings.Repeat("\n", 10000), strings.Repeat("a ", 10001)} {
		if lines, err := wrapFlow(context.Background(), text, "Helvetica", 12, 7); err == nil || !strings.Contains(err.Error(), "10000 lines") || lines != nil {
			t.Fatal(len(lines), err)
		}
	}
	for _, kind := range []string{"paragraph", "heading", "figure"} {
		out := filepath.Join(t.TempDir(), "rejected.pdf")
		b := FlowBlock{Kind: kind, Text: strings.Repeat("\n", 10000), Caption: strings.Repeat("\n", 10000), Src: "unused.png", Width: 100, Height: 100}
		err := Create(context.Background(), CreateSpec{Flow: &FlowSpec{Blocks: []FlowBlock{b}}}, ".", out)
		if err == nil || !strings.Contains(err.Error(), "10000 lines") {
			t.Fatal(kind, err)
		}
		if _, err := os.Stat(out); !os.IsNotExist(err) {
			t.Fatal(err)
		}
	}
}

func TestFlowLogicalLinePreservation(t *testing.T) {
	for _, text := range []string{"a\n\nb\n", "a\r\n\r\nb\r\n"} {
		got, err := wrapFlow(context.Background(), text, "Helvetica", 12, 400)
		if err != nil || !reflect.DeepEqual(got, []string{"a", "", "b", ""}) {
			t.Fatal(got, err)
		}
	}
}
