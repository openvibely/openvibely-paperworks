package equation

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"testing"
)

// Compare complete files emitted by the unchanged pinned helper. This covers
// shared arrow assembly, padded labels, font metrics, and nested transforms.
func TestPinnedLabeledArrowsPNG(t *testing.T) {
	var fixture struct {
		Cases []struct {
			Inline bool   `json:"inline"`
			Name   string `json:"name"`
			Tex    string `json:"tex"`
			Hash   string `json:"png_sha256"`
		}
	}
	raw, err := os.ReadFile("testdata/labeled-arrows.json")
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	if len(fixture.Cases) != 12 {
		t.Fatalf("missing reference cases: %d", len(fixture.Cases))
	}
	for _, c := range fixture.Cases {
		t.Run(c.Name, func(t *testing.T) {
			options := Defaults()
			options.Inline = c.Inline
			png, err := PNG(context.Background(), c.Tex, options)
			if err != nil {
				t.Fatal(err)
			}
			if got := fmt.Sprintf("%x", sha256.Sum256(png)); got != c.Hash {
				t.Fatalf("complete PNG hash %s, want %s", got, c.Hash)
			}
		})
	}
}
