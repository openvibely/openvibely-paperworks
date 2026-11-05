//go:build linux || darwin || windows

package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRenamePreviewExclusivePublishesFreshDirectory(t *testing.T) {
	root := t.TempDir()
	stage := filepath.Join(root, "stage")
	out := filepath.Join(root, "out")
	if err := os.Mkdir(stage, 0700); err != nil {
		t.Fatal(err)
	}
	want := []byte("staged output")
	if err := os.WriteFile(filepath.Join(stage, "page.png"), want, 0600); err != nil {
		t.Fatal(err)
	}

	if err := renamePreviewExclusive(stage, out); err != nil {
		t.Fatalf("publish fresh destination: %v", err)
	}
	if _, err := os.Lstat(stage); !os.IsNotExist(err) {
		t.Fatalf("stage path remains after publication: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(out, "page.png"))
	if err != nil || string(got) != string(want) {
		t.Fatalf("published output = %q, %v; want %q", got, err, want)
	}
}
