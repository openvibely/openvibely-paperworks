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
	unrelated := filepath.Join(dir, "wazero-unrelated")
	if err := os.Mkdir(unrelated, 0o700); err != nil {
		t.Fatal(err)
	}
	checkPool(t)
	versionDir := filepath.Join(dir, wazeroCacheSubdir())
	if _, err := os.Stat(versionDir); !os.IsNotExist(err) {
		t.Fatalf("corrupt entries were not evicted: %v", err)
	}
	if _, err := os.Stat(unrelated); err != nil {
		t.Fatalf("cleanup removed an unrelated directory: %v", err)
	}
	checkPool(t)
	if fresh, _ := filepath.Glob(filepath.Join(versionDir, "*")); len(fresh) == 0 {
		t.Fatal("cache was not repopulated")
	}
}

func TestCompilationCacheVersionDirMatchesWazero(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PAPERWORKS_WASM_CACHE", dir)
	c, versionDir := compilationCache()
	if c == nil {
		t.Fatal("expected a cache")
	}
	_ = c.Close(context.Background())
	entries, err := filepath.Glob(filepath.Join(dir, "wazero-*"))
	if err != nil || len(entries) != 1 || filepath.Base(entries[0]) != filepath.Base(versionDir) {
		t.Fatalf("wazero dirs %v, expected only %s", entries, versionDir)
	}
}

func TestCompilationCacheRejectsForeignOwner(t *testing.T) {
	if _, ok := fileOwner(mustStat(t, t.TempDir())); !ok {
		t.Skip("ownership is not available on this platform")
	}
	dir := filepath.Join(t.TempDir(), "cache")
	t.Setenv("PAPERWORKS_WASM_CACHE", dir)
	if c, _ := compilationCache(); c == nil {
		t.Fatal("expected a cache owned by the current user")
	} else {
		_ = c.Close(context.Background())
	}
	uid := cacheUID
	cacheUID = func() int { return uid() + 1 }
	t.Cleanup(func() { cacheUID = uid })
	if c, _ := compilationCache(); c != nil {
		t.Fatal("expected no cache in a directory owned by another user")
	}
}

func TestCompilationCacheRejectsUnsafeParent(t *testing.T) {
	if _, ok := fileOwner(mustStat(t, t.TempDir())); !ok {
		t.Skip("ownership is not available on this platform")
	}
	shared := t.TempDir()
	if err := os.Chmod(shared, 0o777); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PAPERWORKS_WASM_CACHE", filepath.Join(shared, "cache"))
	if c, _ := compilationCache(); c != nil {
		t.Fatal("expected no cache below a world-writable non-sticky directory")
	}
	if err := os.Chmod(shared, 0o777|os.ModeSticky); err != nil {
		t.Fatal(err)
	}
	c, _ := compilationCache()
	if c == nil {
		t.Fatal("expected a cache below a sticky shared directory")
	}
	_ = c.Close(context.Background())
}

func TestCompilationCacheRejectsReadOnlyVersionDir(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root bypasses permissions")
	}
	dir := t.TempDir()
	versionDir := filepath.Join(dir, wazeroCacheSubdir())
	if err := os.Mkdir(versionDir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(versionDir, 0o700) })
	t.Setenv("PAPERWORKS_WASM_CACHE", dir)
	if c, _ := compilationCache(); c != nil {
		t.Fatal("expected no cache when entries cannot be written")
	}
}

func mustStat(t *testing.T, p string) os.FileInfo {
	t.Helper()
	st, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	return st
}
