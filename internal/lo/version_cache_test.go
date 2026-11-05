package lo

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestVersionCacheAvoidsProbeAndTracksExecutableReplacement(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX fixture")
	}
	dir := t.TempDir()
	exe := filepath.Join(dir, "soffice")
	cache := filepath.Join(dir, "cache")
	calls := filepath.Join(dir, "calls")
	versionFile := filepath.Join(dir, "reported-version")
	t.Setenv("LO_VERSION_CALLS", calls)
	t.Setenv("LO_VERSION_OUTPUT", versionFile)
	if err := os.WriteFile(versionFile, []byte("LibreOffice 1.0\n"), 0600); err != nil {
		t.Fatal(err)
	}
	script := "printf x >> \"$LO_VERSION_CALLS\"\ncat \"$LO_VERSION_OUTPUT\""
	writeVersionScript(t, exe, script)
	oldIdentity, err := identifyExecutable(exe)
	if err != nil {
		t.Fatal(err)
	}

	if got := versionForExecutableWithCache(context.Background(), exe, time.Second, cache); got != "LibreOffice 1.0" {
		t.Fatalf("first version = %q", got)
	}
	if got := versionForExecutableWithCache(context.Background(), exe, time.Second, cache); got != "LibreOffice 1.0" {
		t.Fatalf("cached version = %q", got)
	}
	if got := versionCallCount(t, calls); got != 1 {
		t.Fatalf("version probe calls after cache hit = %d, want 1", got)
	}

	oldInfo, err := os.Stat(exe)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(versionFile, []byte("LibreOffice 2.0\n"), 0600); err != nil {
		t.Fatal(err)
	}
	replacement := filepath.Join(dir, "replacement")
	writeVersionScript(t, replacement, script)
	if err := os.Chtimes(replacement, oldInfo.ModTime(), oldInfo.ModTime()); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(replacement, exe); err != nil {
		t.Fatal(err)
	}
	newInfo, err := os.Stat(exe)
	if err != nil {
		t.Fatal(err)
	}
	newIdentity, err := identifyExecutable(exe)
	if err != nil {
		t.Fatal(err)
	}
	if newInfo.Size() != oldInfo.Size() || !newInfo.ModTime().Equal(oldInfo.ModTime()) ||
		newIdentity.SHA256 != oldIdentity.SHA256 || newIdentity.FileID == oldIdentity.FileID {
		t.Fatalf("replacement fixture did not preserve content/size/time and change file identity: old=%+v new=%+v", oldIdentity, newIdentity)
	}
	if got := versionForExecutableWithCache(context.Background(), exe, time.Second, cache); got != "LibreOffice 2.0" {
		t.Fatalf("version after executable replacement = %q", got)
	}
	if got := versionCallCount(t, calls); got != 2 {
		t.Fatalf("version probe calls after replacement = %d, want 2", got)
	}

	if err := os.WriteFile(versionFile, []byte("LibreOffice 3.0\n"), 0600); err != nil {
		t.Fatal(err)
	}
	changedTime := newInfo.ModTime().Add(2 * time.Second)
	if err := os.Chtimes(exe, changedTime, changedTime); err != nil {
		t.Fatal(err)
	}
	modifiedIdentity, err := identifyExecutable(exe)
	if err != nil {
		t.Fatal(err)
	}
	if modifiedIdentity.FileID != newIdentity.FileID || modifiedIdentity.SHA256 != newIdentity.SHA256 ||
		modifiedIdentity.ModTimeNS == newIdentity.ModTimeNS {
		t.Fatalf("modification-time fixture did not isolate the timestamp change: before=%+v after=%+v", newIdentity, modifiedIdentity)
	}
	if got := versionForExecutableWithCache(context.Background(), exe, time.Second, cache); got != "LibreOffice 3.0" {
		t.Fatalf("version after modification-time change = %q", got)
	}
	if got := versionCallCount(t, calls); got != 3 {
		t.Fatalf("version probe calls after modification-time change = %d, want 3", got)
	}
}

func TestVersionProbeFailureIsNotCached(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX fixture")
	}
	dir := t.TempDir()
	exe := filepath.Join(dir, "soffice")
	calls := filepath.Join(dir, "calls")
	t.Setenv("LO_VERSION_CALLS", calls)
	writeVersionScript(t, exe, "printf x >> \"$LO_VERSION_CALLS\"\nexit 1")
	cache := filepath.Join(dir, "cache")
	for i := 0; i < 2; i++ {
		if got := versionForExecutableWithCache(context.Background(), exe, time.Second, cache); got != "" {
			t.Fatalf("version after failed probe = %q", got)
		}
	}
	if got := versionCallCount(t, calls); got != 2 {
		t.Fatalf("failed probe calls = %d, want 2", got)
	}
}

func versionCallCount(t *testing.T, path string) int {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return 0
		}
		t.Fatal(err)
	}
	return strings.Count(string(data), "x")
}

func TestVersionCachePathSeparatesExecutables(t *testing.T) {
	root := t.TempDir()
	first := versionCachePath(root, filepath.Join("/", "opt", "one", "soffice"))
	second := versionCachePath(root, filepath.Join("/", "opt", "two", "soffice"))
	if first == second {
		t.Fatalf("cache paths collide: %s", first)
	}
	if !strings.HasSuffix(first, ".json") {
		t.Fatalf("cache path has unexpected suffix: %s", first)
	}
}
