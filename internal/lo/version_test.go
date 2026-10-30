package lo

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestVersionOutputAndFailureCleanup(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX fixture")
	}
	dir := t.TempDir()
	exe := filepath.Join(dir, "soffice")
	marker := filepath.Join(dir, "profile-path")
	t.Setenv("DOCUMENT_SOFFICE", exe)
	t.Setenv("LO_PROFILE_MARKER", marker)

	writeVersionScript(t, exe, "printf '%s\\n' \"${XDG_CACHE_HOME%/*}\" > \"$LO_PROFILE_MARKER\"\nprintf '  LibreOffice 26.8.1  \\n' ")
	if got := Version(context.Background()); got != "LibreOffice 26.8.1" {
		t.Fatalf("Version output = %q", got)
	}
	assertVersionProfileCleaned(t, marker)

	writeVersionScript(t, exe, "printf '%s\\n' \"${XDG_CACHE_HOME%/*}\" > \"$LO_PROFILE_MARKER\"\nprintf 'partial output\\n'\nexit 1")
	if got := Version(context.Background()); got != "" {
		t.Fatalf("Version on command failure = %q", got)
	}
	assertVersionProfileCleaned(t, marker)
}

func TestRunVersionCommandTimeoutCleansProfile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX fixture")
	}
	dir := t.TempDir()
	exe := filepath.Join(dir, "soffice")
	marker := filepath.Join(dir, "profile-path")
	t.Setenv("LO_PROFILE_MARKER", marker)
	writeVersionScript(t, exe, "printf '%s\\n' \"${XDG_CACHE_HOME%/*}\" > \"$LO_PROFILE_MARKER\"\nexec sleep 10")

	if got := versionForExecutable(context.Background(), exe, 200*time.Millisecond); got != "" {
		t.Fatalf("Version after timeout = %q", got)
	}
	assertVersionProfileCleaned(t, marker)
}

func TestVersionHonorsContextCancellationAndCleansProfile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX fixture")
	}
	dir := t.TempDir()
	exe := filepath.Join(dir, "soffice")
	marker := filepath.Join(dir, "profile-path")
	t.Setenv("DOCUMENT_SOFFICE", exe)
	t.Setenv("LO_PROFILE_MARKER", marker)
	writeVersionScript(t, exe, "printf '%s\\n' \"${XDG_CACHE_HOME%/*}\" > \"$LO_PROFILE_MARKER\"\nexec sleep 10")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cancelAfterLaunch := make(chan struct{})
	go func() {
		defer close(cancelAfterLaunch)
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			if _, err := os.Stat(marker); err == nil {
				cancel()
				return
			}
			time.Sleep(5 * time.Millisecond)
		}
		cancel()
	}()

	if got := Version(ctx); got != "" {
		t.Fatalf("Version after cancellation = %q", got)
	}
	<-cancelAfterLaunch
	if _, err := os.Stat(marker); err != nil {
		t.Fatal("soffice fixture did not start before cancellation:", err)
	}
	assertVersionProfileCleaned(t, marker)
}
