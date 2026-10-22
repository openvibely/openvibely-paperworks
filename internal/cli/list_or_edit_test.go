package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openvibely/openvibely-paperworks/internal/opc"
)

type fakeSpec struct {
	Action string `json:"action"`
}

func runFake(ctx context.Context, a Args) (string, []int, error) {
	return listOrEdit(ctx, a, "fake", "PPTX", fakeSpec{},
		func(s fakeSpec) string { return s.Action },
		func(*opc.Package, fakeSpec) ([]int, error) { return nil, nil })
}

func TestListOrEdit(t *testing.T) {
	const fixture = "../../testdata/fixtures/reference-comments.pptx"
	dir := t.TempDir()
	in := absPath(fixture)
	edit := writeSpec(t, dir, "edit.json", map[string]any{"action": "add"})
	list := writeSpec(t, dir, "list.json", map[string]any{"action": "list"})
	var usage UsageError

	file, r, err := runFake(context.Background(), Args{Pos: []string{fixture}})
	if err != nil || file != in || r == nil || len(r) != 0 {
		t.Fatal("list", file, r, err)
	}
	if _, _, err = runFake(context.Background(), Args{Pos: []string{fixture}, Flags: map[string]string{"requirements": list, "output": filepath.Join(dir, "x.pptx")}}); !errors.As(err, &usage) || err.Error() != "list is read-only; omit --output" {
		t.Fatal("list with output", err)
	}
	if _, _, err = runFake(context.Background(), Args{Pos: []string{filepath.Join(dir, "a.xlsx")}}); !errors.As(err, &usage) || err.Error() != "fake requires PPTX" {
		t.Fatal("extension", err)
	}
	if _, _, err = runFake(context.Background(), Args{Pos: []string{fixture}, Flags: map[string]string{"requirements": edit}}); !errors.As(err, &usage) {
		t.Fatal("edit without output", err)
	}
	if _, _, err = runFake(context.Background(), Args{Pos: []string{fixture}, Flags: map[string]string{"requirements": edit, "output": fixture}}); err == nil || !strings.Contains(err.Error(), "exists") && !strings.Contains(err.Error(), "differ") {
		t.Fatal("output equals input", err)
	}

	out := filepath.Join(dir, "new.pptx")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err = runFake(ctx, Args{Pos: []string{fixture}, Flags: map[string]string{"requirements": edit, "output": out}}); !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled edit", err)
	}
	if _, err = os.Stat(out); !os.IsNotExist(err) {
		t.Fatal("cancelled edit published", err)
	}
	if _, _, err = runFake(ctx, Args{Pos: []string{fixture}}); !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled list", err)
	}

	file, _, err = runFake(context.Background(), Args{Pos: []string{fixture}, Flags: map[string]string{"requirements": edit, "output": out}})
	if err != nil || file != out {
		t.Fatal("edit", file, err)
	}
	if _, err = os.Stat(out); err != nil {
		t.Fatal(err)
	}
}

func TestListOrEditResultShape(t *testing.T) {
	const fixture = "../../testdata/fixtures/reference-bar.pptx"
	for _, tc := range []struct{ cmd, key string }{{"extended-charts", "charts"}, {"slide-comments", "comments"}} {
		var stdout, stderr bytes.Buffer
		if code := Main([]string{tc.cmd, fixture}, &stdout, &stderr); code != 0 {
			t.Fatal(tc.cmd, code, stderr.String())
		}
		var r map[string]any
		if err := json.Unmarshal(stdout.Bytes(), &r); err != nil {
			t.Fatal(err)
		}
		if r["file"] != absPath(fixture) {
			t.Fatal(tc.cmd, "file", r["file"])
		}
		if list, ok := r[tc.key].([]any); !ok || len(list) != 0 {
			t.Fatal(tc.cmd, "empty list", stdout.String())
		}
	}
	var stdout, stderr bytes.Buffer
	if code := Main([]string{"extended-charts", "../../testdata/fixtures/reference-bar.xlsx"}, &stdout, &stderr); code != 2 {
		t.Fatal("wrong extension exit", code, stderr.String())
	}
}
