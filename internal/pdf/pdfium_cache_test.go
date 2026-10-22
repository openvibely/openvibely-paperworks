package pdf

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/klippa-app/go-pdfium/requests"
)

func TestCompilationCacheDir(t *testing.T) {
	t.Setenv("PAPERWORKS_WASM_CACHE", "")
	t.Setenv("XDG_CACHE_HOME", "/tmp/x")
	if dir, ok := compilationCacheDir(); !ok || dir != "/tmp/x/paperworks/wazero" {
		t.Fatalf("XDG dir = %q %v", dir, ok)
	}
	t.Setenv("XDG_CACHE_HOME", "relative")
	if _, ok := compilationCacheDir(); ok {
		t.Fatal("relative XDG_CACHE_HOME must disable the cache")
	}
	t.Setenv("PAPERWORKS_WASM_CACHE", "/tmp/override")
	if dir, ok := compilationCacheDir(); !ok || dir != "/tmp/override" {
		t.Fatalf("override dir = %q %v", dir, ok)
	}
	t.Setenv("PAPERWORKS_WASM_CACHE", "off")
	if _, ok := compilationCacheDir(); ok {
		t.Fatal("off must disable the cache")
	}
}

func TestCompilationCacheRejectsUnusableDirs(t *testing.T) {
	t.Setenv("PAPERWORKS_WASM_CACHE", "")
	t.Setenv("XDG_CACHE_HOME", "/dev/null/unusable-cache")
	if c, _ := compilationCache(); c != nil {
		t.Fatal("expected no cache under /dev/null")
	}
	shared := filepath.Join(t.TempDir(), "shared")
	if err := os.Mkdir(shared, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(shared, 0o777); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PAPERWORKS_WASM_CACHE", shared)
	if c, _ := compilationCache(); c != nil {
		t.Fatal("expected no cache in a world-writable directory")
	}
	dir := filepath.Join(t.TempDir(), "private")
	t.Setenv("PAPERWORKS_WASM_CACHE", dir)
	c, _ := compilationCache()
	if c == nil {
		t.Fatal("expected a cache in a private directory")
	}
	_ = c.Close(context.Background())
	if st, err := os.Stat(dir); err != nil || st.Mode().Perm() != 0o700 {
		t.Fatalf("cache dir mode = %v, %v", st.Mode(), err)
	}
}

func checkPool(t *testing.T) {
	t.Helper()
	p, err := initPool()
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	inst, err := p.GetInstance(0)
	if err != nil {
		t.Fatal(err)
	}
	defer inst.Close()
	if _, err := inst.FPDF_GetLastError(&requests.FPDF_GetLastError{}); err != nil {
		t.Fatal(err)
	}
}

func TestInitPoolFallsBackWithoutCache(t *testing.T) {
	if testing.Short() {
		t.Skip("compiles PDFium")
	}
	t.Setenv("PAPERWORKS_WASM_CACHE", "")
	t.Setenv("XDG_CACHE_HOME", "/dev/null/unusable-cache")
	checkPool(t)
}

func TestInitPoolRecoversFromCorruptCache(t *testing.T) {
	if testing.Short() {
		t.Skip("compiles PDFium")
	}
	dir := t.TempDir()
	t.Setenv("PAPERWORKS_WASM_CACHE", dir)
	checkPool(t)
	var entries int
	err := filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		entries++
		return os.WriteFile(p, []byte("not a wazero cache entry"), 0o600)
	})
	if err != nil {
		t.Fatal(err)
	}
	if entries == 0 {
		t.Fatal("expected cache entries after first init")
	}
	checkPool(t)
	if stale, _ := filepath.Glob(filepath.Join(dir, "wazero-*")); len(stale) != 0 {
		t.Fatalf("corrupt entries were not evicted: %v", stale)
	}
	checkPool(t)
	if fresh, _ := filepath.Glob(filepath.Join(dir, "wazero-*", "*")); len(fresh) == 0 {
		t.Fatal("cache was not repopulated")
	}
}
