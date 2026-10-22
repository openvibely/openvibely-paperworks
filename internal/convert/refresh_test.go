package convert

import (
	"archive/zip"
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openvibely/openvibely-paperworks/internal/lo"
	"github.com/openvibely/openvibely-paperworks/internal/testdeps"
)

func TestLibreOfficeWholeSheetPreviewUsesOneProcessForChartWorkbook(t *testing.T) {
	if err := testdeps.Office(lo.Find); err != nil {
		t.Skip("LibreOffice unavailable: " + err.Error())
	}
	realSoffice, err := lo.Find()
	if err != nil {
		t.Skip("LibreOffice unavailable: " + err.Error())
	}
	dir := t.TempDir()
	marker := filepath.Join(dir, "launches")
	wrapper := filepath.Join(dir, "soffice-count")
	script := "#!/bin/sh\nprintf 'launch\\n' >> " + shellQuote(marker) + "\nexec " + shellQuote(realSoffice) + " \"$@\"\n"
	if err := os.WriteFile(wrapper, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DOCUMENT_SOFFICE", wrapper)
	src := "../../testdata/fixtures/reference-pie.xlsx"
	before, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	result, err := Convert(context.Background(), src, filepath.Join(dir, "preview"), Options{WholeSheets: true})
	if err != nil {
		t.Fatal(err)
	}
	out := result["output"].(string)
	data, err := os.ReadFile(out)
	if err != nil || len(data) == 0 || !bytes.HasPrefix(data, []byte("%PDF-")) {
		t.Fatalf("whole-sheet PDF missing or invalid: %v", err)
	}
	launches, err := os.ReadFile(marker)
	if err != nil || len(strings.Fields(string(launches))) != 1 {
		t.Fatalf("whole-sheet preview LibreOffice launches = %d, want 1: %v", len(strings.Fields(string(launches))), err)
	}
	after, err := os.ReadFile(src)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("source workbook changed", err)
	}
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

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
