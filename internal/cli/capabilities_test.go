package cli

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestCapabilitiesFastPathAndExactVersionOption(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX fixture")
	}
	root := t.TempDir()
	t.Setenv("HOME", root)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(root, "cache"))
	exe := filepath.Join(root, "soffice")
	marker := filepath.Join(root, "calls")
	t.Setenv("CAPABILITIES_CALLS", marker)
	writeCapabilitiesSoffice(t, exe, "printf x >> \"$CAPABILITIES_CALLS\"\nprintf 'LibreOffice 26.8.1\\n'")
	t.Setenv("DOCUMENT_SOFFICE", exe)

	fast := runCLI(t, "capabilities")
	fastOffice := fast["libreoffice"].(map[string]any)
	if fastOffice["available"] != true || fastOffice["path"] != exe {
		t.Fatalf("fast capability report has wrong availability/path: %#v", fastOffice)
	}
	if _, ok := fastOffice["version"]; ok {
		t.Fatalf("fast capability report unexpectedly contains version: %#v", fastOffice)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("default capabilities launched soffice; marker stat error = %v", err)
	}

	exact := runCLI(t, "capabilities", "--version")
	exactOffice := exact["libreoffice"].(map[string]any)
	if exactOffice["version"] != "LibreOffice 26.8.1" {
		t.Fatalf("exact capability version = %#v", exactOffice["version"])
	}
	if got := capabilityCallCount(t, marker); got != 1 {
		t.Fatalf("version probe calls = %d, want 1", got)
	}
	_ = runCLI(t, "capabilities", "--version")
	if got := capabilityCallCount(t, marker); got != 1 {
		t.Fatalf("cached version probe calls = %d, want 1", got)
	}

	writeCapabilitiesSoffice(t, exe, "printf x >> \"$CAPABILITIES_CALLS\"\nexit 1")
	failed := runCLI(t, "capabilities", "--version")
	failedOffice := failed["libreoffice"].(map[string]any)
	if failedOffice["available"] != true || failedOffice["path"] != exe || failedOffice["version"] != "" {
		t.Fatalf("version failure report lost availability/path or omitted empty version: %#v", failedOffice)
	}
	if got := capabilityCallCount(t, marker); got != 2 {
		t.Fatalf("version probe calls after failure = %d, want 2", got)
	}

	t.Setenv("DOCUMENT_SOFFICE", filepath.Join(root, "missing-soffice"))
	missing := runCLI(t, "capabilities")
	missingOffice := missing["libreoffice"].(map[string]any)
	if missingOffice["available"] != false {
		t.Fatalf("missing LibreOffice reported available: %#v", missingOffice)
	}
	if _, ok := missingOffice["path"]; ok {
		t.Fatalf("missing LibreOffice report has a path: %#v", missingOffice)
	}
}

func TestCapabilitiesExactVersionFallsBackFromUnusableCachedOffice(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("supported cached-office platform fixture")
	}
	for _, path := range standardLibreOfficePaths() {
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			t.Skipf("standard LibreOffice installation at %s takes precedence", path)
		}
	}

	root := t.TempDir()
	t.Setenv("HOME", root)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(root, "xdg-cache"))
	cache, err := os.UserCacheDir()
	if err != nil {
		t.Fatal(err)
	}
	platformPath := map[string]string{"darwin": filepath.Join("LibreOffice.app", "Contents", "MacOS", "soffice"), "linux": filepath.Join("program", "soffice")}[runtime.GOOS]
	makeCachedExecutable := func(version, hash, body string) string {
		t.Helper()
		path := filepath.Join(cache, "openvibely", "skill-tools", "libreoffice", version, runtime.GOOS, runtime.GOARCH, hash, platformPath)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		writeCapabilitiesSoffice(t, path, body)
		return path
	}
	marker := filepath.Join(root, "probes")
	t.Setenv("CAPABILITIES_CACHED_PROBE_MARKER", marker)
	makeCachedExecutable("26.10.0", strings.Repeat("a", 64), `printf '%s\n' newest >> "$CAPABILITIES_CACHED_PROBE_MARKER"
printf '%s\n' 'not LibreOffice'`)
	older := makeCachedExecutable("26.9.0", strings.Repeat("b", 64), `printf '%s\n' older >> "$CAPABILITIES_CACHED_PROBE_MARKER"
printf '%s\n' 'LibreOffice 26.9.0'`)
	emptyPath := filepath.Join(root, "empty-path")
	if err := os.Mkdir(emptyPath, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", emptyPath)
	t.Setenv("DOCUMENT_SOFFICE", "")

	report := runCLI(t, "capabilities", "--version")
	office := report["libreoffice"].(map[string]any)
	if office["available"] != true || office["path"] != older || office["version"] != "LibreOffice 26.9.0" {
		t.Fatalf("exact capability report did not select older working release: %#v", office)
	}
	probes, err := os.ReadFile(marker)
	if err != nil || string(probes) != "newest\nolder\n" {
		t.Fatalf("capability version probe sequence = %q, err = %v", probes, err)
	}

	repeated := runCLI(t, "capabilities", "--version")
	repeatedOffice := repeated["libreoffice"].(map[string]any)
	if repeatedOffice["available"] != true || repeatedOffice["path"] != older || repeatedOffice["version"] != "LibreOffice 26.9.0" {
		t.Fatalf("repeated exact capability report changed: %#v", repeatedOffice)
	}
	probes, err = os.ReadFile(marker)
	if err != nil || string(probes) != "newest\nolder\n" {
		t.Fatalf("repeated capability lookup re-probed cached executables: %q, err = %v", probes, err)
	}
}

func standardLibreOfficePaths() []string {
	switch runtime.GOOS {
	case "darwin":
		return []string{"/Applications/LibreOffice.app/Contents/MacOS/soffice"}
	case "linux":
		return []string{"/usr/lib/libreoffice/program/soffice", "/opt/libreoffice/program/soffice", "/usr/bin/soffice"}
	default:
		return nil
	}
}

func writeCapabilitiesSoffice(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body+"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
}

func capabilityCallCount(t *testing.T, path string) int {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Count(string(data), "x")
}
