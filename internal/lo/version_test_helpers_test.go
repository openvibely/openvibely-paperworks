package lo

import (
	"errors"
	"os"
	"strings"
	"testing"
)

func writeVersionScript(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body+"\n"), 0700); err != nil {
		t.Fatal(err)
	}
}

func assertVersionProfileCleaned(t *testing.T, marker string) {
	t.Helper()
	data, err := os.ReadFile(marker)
	if err != nil {
		t.Fatalf("read profile marker: %v", err)
	}
	profileRoot := strings.TrimSpace(string(data))
	if profileRoot == "" {
		t.Fatal("script did not record its temporary profile root")
	}
	if _, err := os.Stat(profileRoot); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("temporary profile root %q still exists: %v", profileRoot, err)
	}
}
