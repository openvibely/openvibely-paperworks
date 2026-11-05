//go:build !darwin && !linux && !windows

package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRenamePreviewExclusiveFailsClosedWithoutNativeNoReplace(t *testing.T) {
	root := t.TempDir()
	stage := filepath.Join(root, "stage")
	out := filepath.Join(root, "out")
	if err := os.Mkdir(stage, 0700); err != nil {
		t.Fatal(err)
	}
	want := []byte("staged output")
	stageFile := filepath.Join(stage, "page.png")
	if err := os.WriteFile(stageFile, want, 0600); err != nil {
		t.Fatal(err)
	}

	if err := renamePreviewExclusive(stage, out); err == nil {
		t.Fatal("publication succeeded without an atomic no-replace operation")
	}
	if _, err := os.Lstat(out); !os.IsNotExist(err) {
		t.Fatalf("unsupported publication created destination: %v", err)
	}
	got, err := os.ReadFile(stageFile)
	if err != nil || string(got) != string(want) {
		t.Fatalf("staged output changed: %q, %v", got, err)
	}
}
