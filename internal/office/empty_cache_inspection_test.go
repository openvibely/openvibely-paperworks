package office

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestHistoricalEmptyStringCacheInspection(t *testing.T) {
	for _, tc := range []struct {
		name   string
		issues []string
	}{
		{"historical", []string{}},
		{"content-types", []string{}},
		{"missing-cache", []string{"xl/worksheets/sheet1.xml:A1 has no formula cache"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := filepath.Join("..", "..", "testdata", "regression", "empty-cache-baseline", tc.name+".xlsx")
			before, err := os.ReadFile(source)
			if err != nil {
				t.Fatal(err)
			}
			report, err := PackageReport(source)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(report["issues"], tc.issues) {
				t.Fatalf("issues = %#v", report["issues"])
			}
			after, err := os.ReadFile(source)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatal("source changed", err)
			}
		})
	}
}
