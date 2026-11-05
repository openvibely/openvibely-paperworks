package docx

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openvibely/openvibely-paperworks/internal/opc"
)

func TestCreatePageSizeDefaultsIndependently(t *testing.T) {
	tests := []struct {
		name       string
		page       Page
		wantWidth  string
		wantHeight string
	}{
		{name: "default size", wantWidth: "12240", wantHeight: "15840"},
		{name: "height only", page: Page{HeightIn: 7.25}, wantWidth: "12240", wantHeight: "10440"},
		{name: "width only", page: Page{WidthIn: 7.25}, wantWidth: "10440", wantHeight: "15840"},
		{name: "tiny positive dimensions", page: Page{WidthIn: 0.0001, HeightIn: 0.0001}, wantWidth: "1", wantHeight: "1"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			output := filepath.Join(t.TempDir(), "page-size.docx")
			if err := Create(Spec{Page: tc.page}, "", output); err != nil {
				t.Fatal(err)
			}
			raw, err := os.ReadFile(output)
			if err != nil {
				t.Fatal(err)
			}
			assertPageSize(t, raw, tc.wantWidth, tc.wantHeight)
		})
	}
}

func TestCreateRejectsPageDimensionsThatCouldSerializeInvalidly(t *testing.T) {
	tests := []struct {
		name      string
		page      Page
		wantError string
	}{
		{name: "negative tiny width", page: Page{WidthIn: -0.0001, HeightIn: 11}, wantError: "page width_in"},
		{name: "not-a-number height", page: Page{WidthIn: 8.5, HeightIn: math.NaN()}, wantError: "page height_in"},
		{name: "infinite width", page: Page{WidthIn: math.Inf(1), HeightIn: 11}, wantError: "page width_in"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			output := filepath.Join(t.TempDir(), "invalid-page-size.docx")
			err := Create(Spec{Page: tc.page}, "", output)
			if err == nil || !strings.Contains(err.Error(), tc.wantError) {
				t.Fatalf("Create error = %v, want an error containing %q", err, tc.wantError)
			}
			if _, err := os.Stat(output); !os.IsNotExist(err) {
				t.Fatalf("invalid page dimensions produced output: stat error %v", err)
			}
		})
	}
}

func TestBuildPageSizeDefaultsWithoutCreate(t *testing.T) {
	raw, err := Build(Spec{}, "")
	if err != nil {
		t.Fatal(err)
	}
	assertPageSize(t, raw, "12240", "15840")
}

func assertPageSize(t *testing.T, raw []byte, wantWidth, wantHeight string) {
	t.Helper()
	pkg, err := opc.Read(raw)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := pkg.XML("word/document.xml")
	if err != nil {
		t.Fatal(err)
	}
	pageSize := doc.Root().FindElement("w:body/w:sectPr/w:pgSz")
	if pageSize == nil {
		t.Fatal("DOCX page size element missing")
	}
	gotWidth := pageSize.SelectAttrValue("w", "")
	gotHeight := pageSize.SelectAttrValue("h", "")
	if gotWidth == "0" || gotHeight == "0" {
		t.Fatalf("DOCX page has a zero dimension: width=%s height=%s", gotWidth, gotHeight)
	}
	if gotWidth != wantWidth || gotHeight != wantHeight {
		t.Fatalf("DOCX page size = %s x %s twips, want %s x %s", gotWidth, gotHeight, wantWidth, wantHeight)
	}
	if gotWidth == "" || gotHeight == "" {
		t.Fatalf("DOCX page size is incomplete: width=%s height=%s", gotWidth, gotHeight)
	}
}
