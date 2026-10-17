package testsupport

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMutationSerializationFailureKeepsPartialArchive(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.zip")
	parts := &OrderedZIP{Parts: []ZIPPart{{"first", "saved"}, {strings.Repeat("x", 65536), "invalid"}}}
	if err := SaveStoredZIP(path, parts); err == nil {
		t.Fatal("oversized member accepted")
	}
	z, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer z.Close()
	if len(z.File) != 1 || z.File[0].Name != "first" {
		t.Fatal("lost completed member", z.File)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal("failure removed output", err)
	}
}

// Output acquisition failures must not create parents or disturb an existing
// directory. Reference opens the destination only after its callback succeeds.
func TestMutationDestinationOpenFailures(t *testing.T) {
	for _, kind := range []string{"directory", "missing-parent"} {
		t.Run(kind, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, "target")
			if kind == "directory" {
				if err := os.Mkdir(path, 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(path, "sentinel"), []byte("keep"), 0600); err != nil {
					t.Fatal(err)
				}
			} else {
				path = filepath.Join(path, "out.zip")
			}
			if err := SaveStoredZIP(path, &OrderedZIP{Parts: []ZIPPart{{"first", "saved"}, {"second", "data"}}}); err == nil {
				t.Fatal("accepted invalid destination")
			}
			if kind == "directory" {
				b, err := os.ReadFile(filepath.Join(path, "sentinel"))
				if err != nil || string(b) != "keep" {
					t.Fatal(string(b), err)
				}
			} else if _, err := os.Stat(filepath.Dir(path)); !os.IsNotExist(err) {
				t.Fatal("created parent", err)
			}
		})
	}
}
