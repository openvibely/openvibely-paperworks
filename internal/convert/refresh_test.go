package convert

import (
	"archive/zip"
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestLibreOfficeExplicitPowerPointRefresh(t *testing.T) {
	referenceRuntimeOffice(t)
	src := "../../testdata/fixtures/reference-bar.pptx"
	before, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	result, err := Convert(context.Background(), src, filepath.Join(t.TempDir(), "refreshed"), Options{Refresh: true})
	if err != nil {
		t.Fatal(err)
	}
	out := result["output"].(string)
	st, err := os.Stat(out)
	if err != nil || st.Size() == 0 {
		t.Fatalf("refreshed presentation missing or empty: %v", err)
	}
	archive, err := zip.OpenReader(out)
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Close()
	found := false
	for _, file := range archive.File {
		if file.Name == "ppt/presentation.xml" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("refreshed presentation package is incomplete")
	}
	after, err := os.ReadFile(src)
	if err != nil || string(before) != string(after) {
		t.Fatal("source presentation changed", err)
	}
}
