package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestPDFSelectWritesJSONPageList(t *testing.T) {
	source := filepath.Join("..", "..", "testdata", "reference-mirror", "library-dimensions.pdf")
	out := filepath.Join(t.TempDir(), "selected.pdf")
	var stdout, stderr bytes.Buffer
	code := Main([]string{"pdf-select", source, "--pages", "1", "--output", out}, &stdout, &stderr)
	if code != ExitOK {
		t.Fatalf("pdf-select exit = %d: %s", code, stderr.String())
	}
	var result struct {
		Output string `json:"output"`
		Pages  []int  `json:"pages"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode output %q: %v", stdout.String(), err)
	}
	if result.Output != out || len(result.Pages) != 1 || result.Pages[0] != 1 {
		t.Fatalf("result = %+v, want output %q and pages [1]", result, out)
	}
	if _, err := os.Stat(out); err != nil {
		t.Fatalf("selected output not written: %v", err)
	}
}

func TestPDFSelectRejectsInvalidInputsAndExistingOutput(t *testing.T) {
	source := filepath.Join("..", "..", "testdata", "reference-mirror", "library-dimensions.pdf")
	for _, tc := range []struct {
		name  string
		pages string
	}{
		{name: "empty", pages: ""},
		{name: "out of range", pages: "999"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out := filepath.Join(t.TempDir(), "selected.pdf")
			args := []string{"pdf-select", source, "--pages", tc.pages, "--output", out}
			var stdout, stderr bytes.Buffer
			if code := Main(args, &stdout, &stderr); code == ExitOK {
				t.Fatalf("selection unexpectedly succeeded: %s", stdout.String())
			}
		})
	}

	t.Run("malformed PDF", func(t *testing.T) {
		input := filepath.Join(t.TempDir(), "broken.pdf")
		if err := os.WriteFile(input, []byte("not a PDF"), 0o600); err != nil {
			t.Fatal(err)
		}
		var stdout, stderr bytes.Buffer
		if code := Main([]string{"pdf-select", input, "--pages", "1", "--output", filepath.Join(t.TempDir(), "out.pdf")}, &stdout, &stderr); code == ExitOK {
			t.Fatalf("malformed PDF unexpectedly succeeded: %s", stdout.String())
		}
	})

	t.Run("existing output", func(t *testing.T) {
		out := filepath.Join(t.TempDir(), "existing.pdf")
		if err := os.WriteFile(out, []byte("keep"), 0o600); err != nil {
			t.Fatal(err)
		}
		_, err := commands["pdf-select"].Run(context.Background(), Args{
			Pos:   []string{source},
			Flags: map[string]string{"pages": "1", "output": out},
		})
		if !errors.Is(err, os.ErrExist) {
			t.Fatalf("error = %v, want os.ErrExist", err)
		}
		got, readErr := os.ReadFile(out)
		if readErr != nil || string(got) != "keep" {
			t.Fatalf("existing output changed: %q, %v", got, readErr)
		}
	})
}

func TestPDFSelectHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := commands["pdf-select"].Run(ctx, Args{
		Pos:   []string{filepath.Join("..", "..", "testdata", "reference-mirror", "library-dimensions.pdf")},
		Flags: map[string]string{"pages": "1", "output": filepath.Join(t.TempDir(), "selected.pdf")},
	})
	if err == nil {
		t.Fatal("cancelled selection unexpectedly succeeded")
	}
}
