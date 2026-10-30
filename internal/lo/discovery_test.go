package lo

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func cachedFixture(t *testing.T, root, version, platform, arch, hash, rel string) string {
	t.Helper()
	p := filepath.Join(root, "openvibely", "skill-tools", "libreoffice", version, platform, arch, hash, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("fixture"), 0700); err != nil {
		t.Fatal(err)
	}
	return p
}
func TestCachedOfficeVersionSelection(t *testing.T) {
	root := t.TempDir()
	hash := strings.Repeat("a", 64)
	rel := "LibreOffice.app/Contents/MacOS/soffice"
	older := cachedFixture(t, root, "26.9.0", "darwin", "arm64", hash, rel)
	newest := cachedFixture(t, root, "26.10.0", "darwin", "arm64", hash, rel)
	for _, v := range []string{"local", "27.0", "v99.0.0"} {
		cachedFixture(t, root, v, "darwin", "arm64", hash, rel)
	}
	cachedFixture(t, root, "99.0.0", "darwin", "amd64", hash, rel)
	cachedFixture(t, root, "99.0.0", "darwin", "arm64", "staging", rel)
	cachedFixture(t, root, "26.10.0-rc.1", "darwin", "arm64", hash, rel)
	var calls []string
	got, err := findCachedOffice(root, "darwin", "arm64", func(p string) bool { calls = append(calls, p); return true })
	if err != nil || got != newest || len(calls) != 1 {
		t.Fatalf("%s %v %v", got, err, calls)
	}
	got, err = findCachedOffice(root, "darwin", "arm64", func(p string) bool { return p == older })
	if err != nil || got != older {
		t.Fatalf("%s %v", got, err)
	}
	if _, err = findCachedOffice(root, "darwin", "arm64", func(string) bool { return false }); !errors.Is(err, ErrMissing) {
		t.Fatal(err)
	}
}
func TestCachedOfficePlatformPaths(t *testing.T) {
	for _, tc := range []struct{ os, arch, rel string }{{"darwin", "arm64", "LibreOffice.app/Contents/MacOS/soffice"}, {"linux", "amd64", "program/soffice"}, {"windows", "amd64", "program/soffice.exe"}} {
		t.Run(tc.os, func(t *testing.T) {
			root := t.TempDir()
			want := cachedFixture(t, root, "1.0.0", tc.os, tc.arch, strings.Repeat("b", 64), tc.rel)
			got, err := findCachedOffice(root, tc.os, tc.arch, func(string) bool { return true })
			if err != nil || got != want {
				t.Fatalf("%s %v", got, err)
			}
		})
	}
	if _, err := findCachedOffice(t.TempDir(), "linux", "amd64", func(string) bool { t.Fatal("unexpected probe"); return true }); !errors.Is(err, ErrMissing) {
		t.Fatal(err)
	}
}
func TestCachedOfficeProbeAndOverride(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX fixture")
	}
	dir := t.TempDir()
	exe := filepath.Join(dir, "soffice")
	profileMarker := filepath.Join(dir, "profile-path")
	argsMarker := filepath.Join(dir, "args")
	t.Setenv("LO_PROFILE_MARKER", profileMarker)
	t.Setenv("LO_ARGS_MARKER", argsMarker)
	for _, tc := range []struct {
		body string
		want bool
	}{{"echo 'LibreOffice 26.8.1'", true}, {"echo 'other tool'", false}, {"exit 1", false}} {
		writeVersionScript(t, exe, "printf '%s\\n' \"${XDG_CACHE_HOME%/*}\" > \"$LO_PROFILE_MARKER\"\nprintf '%s\\n' \"$*\" > \"$LO_ARGS_MARKER\"\n"+tc.body)
		if got := probeCachedOffice(exe); got != tc.want {
			t.Fatalf("%q: %v", tc.body, got)
		}
		assertVersionProfileCleaned(t, profileMarker)
		args, err := os.ReadFile(argsMarker)
		if err != nil || !strings.Contains(string(args), "--headless --version") {
			t.Fatalf("probe args = %q, err = %v", args, err)
		}
	}
	t.Setenv("DOCUMENT_SOFFICE", exe)
	if got, err := Find(); err != nil || got != exe {
		t.Fatalf("override: %s %v", got, err)
	}
	t.Setenv("DOCUMENT_SOFFICE", filepath.Join(dir, "missing"))
	if _, err := Find(); err == nil || !strings.Contains(err.Error(), "DOCUMENT_SOFFICE") {
		t.Fatal(err)
	}
}

func TestCachedOfficePreservedDEBLayout(t *testing.T) {
	for _, arch := range []string{"amd64", "arm64"} {
		root := t.TempDir()
		want := cachedFixture(t, root, "26.8.1", "linux", arch, strings.Repeat("b", 64), "opt/libreoffice26.8/program/soffice")
		got, err := findCachedOffice(root, "linux", arch, func(string) bool { return true })
		if err != nil || got != want {
			t.Fatalf("%s %v", got, err)
		}
	}
}
