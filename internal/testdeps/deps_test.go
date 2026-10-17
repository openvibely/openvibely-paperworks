package testdeps

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestOfficeDiscoveryPrecedence(t *testing.T) {
	cache := t.TempDir()
	cached := filepath.Join(cache, "libreoffice-25.8.7-"+arch(), "LibreOffice.app", "Contents", "MacOS", "soffice")
	if err := os.MkdirAll(filepath.Dir(cached), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cached, []byte("test executable"), 0755); err != nil {
		t.Fatal(err)
	}
	installed := func() (string, error) { return "/installed/soffice", nil }
	if got, err := resolveOffice("", cache, installed); err != nil || got != "/installed/soffice" {
		t.Fatal(got, err)
	}
	missing := errors.New("missing office")
	absent := func() (string, error) { return "", missing }
	if _, err := resolveOffice("/invalid/override", cache, absent); err != missing {
		t.Fatalf("override error masked: %v", err)
	}
	got, err := resolveOffice("", cache, absent)
	if runtime.GOOS == "darwin" {
		if err != nil || got != cached {
			t.Fatal(got, err)
		}
	} else if err == nil || !strings.Contains(err.Error(), "make deps") {
		t.Fatal(err)
	}
	if err := os.Remove(cached); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveOffice("", cache, absent); err == nil || !strings.Contains(err.Error(), "make deps") {
		t.Fatal(err)
	}
}

func TestReaderOverrideAndIntegrity(t *testing.T) {
	cache := t.TempDir()
	cached := filepath.Join(cache, "xlsx-0.20.3.full.min.js")
	if err := os.WriteFile(cached, []byte("wrong reader"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := reader("", cache); err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatal(err)
	}
	explicit := filepath.Join(cache, "explicit.js")
	if _, err := reader(explicit, cache); err == nil || !strings.Contains(err.Error(), explicit) {
		t.Fatalf("explicit path masked by cache: %v", err)
	}
	if err := os.Remove(cached); err != nil {
		t.Fatal(err)
	}
	if _, err := reader("", cache); err == nil || !strings.Contains(err.Error(), "make deps") {
		t.Fatal(err)
	}
}

func TestCacheOverride(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", dir)
	got, err := cacheDir()
	if err != nil || got != filepath.Join(dir, "paperworks-tests") {
		t.Fatal(got, err)
	}
}
