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
// CSS padding, borders, nested transforms, color recovery and PNG serialization.
func TestPinnedDecorationBoxesPNG(t *testing.T) {
	var fixture struct {
		Cases []struct {
			Name string `json:"name"`
			Tex  string `json:"tex"`
			Hash string `json:"png_sha256"`
		}
	}
	raw, err := os.ReadFile("testdata/decoration-boxes.json")
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
			png, err := PNG(context.Background(), c.Tex, Defaults())
			if err != nil {
				t.Fatal(err)
			}
			if got := fmt.Sprintf("%x", sha256.Sum256(png)); got != c.Hash {
				t.Fatalf("complete PNG hash %s, want %s", got, c.Hash)
			}
		})
	}
}
