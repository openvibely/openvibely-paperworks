package testsupport

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func TestReferenceStoredZIPSerialization(t *testing.T) {
	// Unchanged PresentationNative.mutate, CReference 3.12.12 zipfile, with only
	// localtime controlled to 2024-01-02 03:04:06. Full archive hashes, not a
	// normalized ZIP comparison. The fixture below retains the expected bytes.
	many := make([]ZIPPart, 65536)
	for i := range many {
		many[i] = ZIPPart{strconv.Itoa(i), ""}
	}
	cases := []struct {
		name  string
		parts []ZIPPart
		hash  string
	}{
		{"normal", []ZIPPart{{"z", "first"}, {"a", "last"}}, "c96ada31ac5ba63344232a673d1709546e7e7c23da78d4a4350df10f9365788a"},
		{"directory", []ZIPPart{{"original", "keep"}, {"folder/", "value"}, {"folder/child", "text"}}, "c40a6950152241aa8c09f4087a8b97dedbf6623a9c49c2109d81da669450cbe6"},
		{"unicode", []ZIPPart{{"café", "λ"}, {"emoji-😀", "你好"}, {"empty", ""}}, "f0dbeeb89989a579881b8bc1828f8dfb873ad64e6a1f3ccef8e18cf4428f2618"},
		{"nul", []ZIPPart{{"key\x00one", "first"}, {"key\x00two", "second"}}, "b34014e5f1c1978ac4c57956a5608ad95c52e458b1e113906d08e5ae0b13b012"},
		{"empty", nil, "8739c76e681f900923b900c9df0ef75cf421d39cabb54650c4b9ad19b6a76d85"},
		{"zip64-count", many, "0c61ec1565dda81178a4c5e4e7e447534cda86434efebf4ff7dbdb2da37628b3"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var b bytes.Buffer
			err := writeReferenceStoredZIP(&b, &OrderedZIP{Parts: tc.parts}, func() time.Time { return time.Date(2024, 1, 2, 3, 4, 6, 0, time.Local) })
			if err != nil {
				t.Fatal(err)
			}
			if got := fmt.Sprintf("%x", sha256.Sum256(b.Bytes())); got != tc.hash {
				t.Fatalf("full ZIP hash %s, want %s", got, tc.hash)
			}
			path := filepath.Join(t.TempDir(), "result.zip")
			if err := os.WriteFile(path, b.Bytes(), 0600); err != nil {
				t.Fatal(err)
			}
			z, err := zip.OpenReader(path)
			if err != nil {
				t.Fatal(err)
			}
			defer z.Close()
			if len(z.File) != len(tc.parts) {
				t.Fatal("member count", len(z.File))
			}
		})
	}
}
