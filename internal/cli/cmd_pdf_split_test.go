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

	"github.com/openvibely/openvibely-paperworks/internal/pdf"
)

func installPDFSplitStub(t *testing.T, stub func(context.Context, string, string, [][]int) ([]string, error)) {
	t.Helper()
	original := splitPDF
	splitPDF = stub
	t.Cleanup(func() { splitPDF = original })
}

func TestPDFSplitReportsOnlyCompletedOutputsOnFailure(t *testing.T) {
	source := filepath.Join("..", "..", "testdata", "reference-mirror", "library-dimensions.pdf")
	for _, tc := range []struct {
		name string
		err  error
	}{
		{name: "cancellation after completed group", err: context.Canceled},
		{name: "later output creation failure", err: errors.New("later output creation failed")},
		{name: "later output write failure", err: errors.New("later output write failed")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			outDir := filepath.Join(t.TempDir(), "split")
			completed := filepath.Join(outDir, "library-dimensions-001.pdf")
			incomplete := filepath.Join(outDir, "library-dimensions-002.pdf")
			installPDFSplitStub(t, func(_ context.Context, src, dir string, _ [][]int) ([]string, error) {
				if dir != outDir {
					t.Fatalf("split output directory = %q, want %q", dir, outDir)
				}
				data, err := os.ReadFile(src)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(completed, data, 0o644); err != nil {
					t.Fatal(err)
				}
				return []string{completed}, tc.err
			})

			var stdout, stderr bytes.Buffer
			code := Main([]string{"pdf-split", source, "--ranges", "1;1", "--output", outDir}, &stdout, &stderr)
			if code != ExitFailure {
				t.Fatalf("exit = %d, want %d; stderr: %s", code, ExitFailure, stderr.String())
			}
			if stdout.Len() != 0 {
				t.Fatalf("failure wrote JSON to stdout: %s", stdout.String())
			}
			for _, want := range []string{tc.err.Error(), "partial output directory: " + outDir, "completed files:", completed} {
				if !strings.Contains(stderr.String(), want) {
					t.Errorf("stderr %q does not contain %q", stderr.String(), want)
				}
			}
			if strings.Contains(stderr.String(), incomplete) {
				t.Errorf("stderr reports incomplete output path %q: %s", incomplete, stderr.String())
			}
			data, err := os.ReadFile(completed)
			if err != nil {
				t.Fatalf("completed path is missing: %v", err)
			}
			if _, err := pdf.PageCount(context.Background(), data); err != nil {
				t.Fatalf("completed path is not an openable PDF: %v", err)
			}
		})
	}
}

func TestPDFSplitFailureBeforeFirstOutputDoesNotReportOutputs(t *testing.T) {
	source := filepath.Join("..", "..", "testdata", "reference-mirror", "library-dimensions.pdf")
	outDir := filepath.Join(t.TempDir(), "split")
	injectedErr := errors.New("split failed before first output")
	installPDFSplitStub(t, func(context.Context, string, string, [][]int) ([]string, error) {
		return nil, injectedErr
	})

	var stdout, stderr bytes.Buffer
	code := Main([]string{"pdf-split", source, "--ranges", "1", "--output", outDir}, &stdout, &stderr)
	if code != ExitFailure {
		t.Fatalf("exit = %d, want %d; stderr: %s", code, ExitFailure, stderr.String())
	}
	if stdout.Len() != 0 {
		t.Fatalf("failure wrote JSON to stdout: %s", stdout.String())
	}
	if !strings.Contains(stderr.String(), injectedErr.Error()) {
		t.Fatalf("stderr %q does not contain the failure", stderr.String())
	}
	for _, absent := range []string{outDir, "partial output directory", "completed files"} {
		if strings.Contains(stderr.String(), absent) {
			t.Errorf("stderr implies output exists by containing %q: %s", absent, stderr.String())
		}
	}
}

func TestPDFSplitSuccessKeepsJSONFilesResult(t *testing.T) {
	source := filepath.Join("..", "..", "testdata", "reference-mirror", "library-dimensions.pdf")
	outDir := filepath.Join(t.TempDir(), "split")
	var stdout, stderr bytes.Buffer
	code := Main([]string{"pdf-split", source, "--ranges", "1", "--output", outDir}, &stdout, &stderr)
	if code != ExitOK {
		t.Fatalf("exit = %d, want %d; stderr: %s", code, ExitOK, stderr.String())
	}
	var result struct {
		Files []string `json:"files"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode stdout %q: %v", stdout.String(), err)
	}
	want := filepath.Join(outDir, "library-dimensions-001.pdf")
	if len(result.Files) != 1 || result.Files[0] != want {
		t.Fatalf("files = %#v, want [%q]", result.Files, want)
	}
	data, err := os.ReadFile(want)
	if err != nil {
		t.Fatalf("split output missing: %v", err)
	}
	if _, err := pdf.PageCount(context.Background(), data); err != nil {
		t.Fatalf("split output is not an openable PDF: %v", err)
	}
}
