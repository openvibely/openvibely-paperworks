package testsupport

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

// PresentationOutput mirrors the original unittest's output helper. Count all
// direct entries, including directories, without reserving or uniquifying a name.
func PresentationOutput(t testing.TB, root string) string {
	t.Helper()
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Join(root, strconv.Itoa(len(entries))+".pptx")
}
