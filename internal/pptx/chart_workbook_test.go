package pptx

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"
)

func TestCategoryWorkbookReference(t *testing.T) {
	raw, e := os.ReadFile("testdata/chart-workbook.json")
	if e != nil {
		t.Fatal(e)
	}
	var cases []struct {
		Name       string
		Categories []string
		Series     []Series
		SHA256     string
	}
	if e = json.Unmarshal(raw, &cases); e != nil {
		t.Fatal(e)
	}
	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			b, e := categoryWorkbook(Chart{Categories: c.Categories, Series: c.Series}, time.Date(2024, 2, 3, 4, 5, 6, 0, time.UTC))
			if e != nil {
				t.Fatal(e)
			}
			if got := fmt.Sprintf("%x", sha256.Sum256(b)); got != c.SHA256 {
				t.Errorf("whole XLSX %s want %s", got, c.SHA256)

			}
		})
	}
}

func TestCategoryWorkbookBoundary(t *testing.T) {
	for _, text := range []string{"", "=A1", "https://example.com", "a\nb", "_x0041_"} {
		if categoryWorkbookSupported(Chart{Categories: []string{text}, Series: []Series{{Name: "s", Values: []float64{1}}}}) {
			t.Fatalf("unsupported string %q", text)
		}
	}
	if categoryWorkbookSupported(Chart{Type: "scatter", Categories: []string{"1"}, Series: []Series{{Name: "s", Values: []float64{1}}}}) {
		t.Fatal("scatter must retain numeric X semantics")
	}
}
