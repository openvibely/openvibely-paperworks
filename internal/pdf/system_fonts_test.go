package pdf

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
)

func TestFontFilesystemAncestorsAndConfinement(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "sample.ttf"), []byte("font bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "secret")
	if err := os.WriteFile(outside, []byte("not a font"), 0600); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	fonts := fontFS{root.FS()}
	if err := fstest.TestFS(fonts, "usr/share/fonts/sample.ttf"); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{".", "usr", "usr/share", "usr/share/fonts"} {
		info, err := fs.Stat(fonts, path)
		if err != nil || !info.IsDir() {
			t.Fatalf("ancestor %s: %v %v", path, info, err)
		}
	}
	data, err := fs.ReadFile(fonts, "usr/share/fonts/sample.ttf")
	if err != nil || string(data) != "font bytes" {
		t.Fatalf("font: %q %v", data, err)
	}
	for _, path := range []string{"../secret", "usr/share/fonts/../secret", "etc/passwd", outside} {
		if _, err := fs.ReadFile(fonts, path); err == nil {
			t.Fatalf("exposed %s", path)
		}
	}
	if err := os.Symlink(outside, filepath.Join(dir, "escape.ttf")); err == nil {
		if _, err := fs.ReadFile(fonts, "usr/share/fonts/escape.ttf"); err == nil {
			t.Fatal("symlink escaped font root")
		}
	}
}
