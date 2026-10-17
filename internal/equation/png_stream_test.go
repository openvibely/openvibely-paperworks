package equation

import (
	"bytes"
	"compress/zlib"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"image"
	"io"
	"os"
	"testing"
)

// Independently captured filtered rows and complete PNG hashes from the pinned
// renderer. This isolates PNG encoding from the still-incomplete layout engine.
func TestPinnedCompletePNGEncoding(t *testing.T) {
	var f struct {
		Cases []struct {
			Name     string
			Source   string
			Hash     string `json:"source_sha256"`
			RowBytes int    `json:"row_bytes"`
			Stream   []byte
		}
	}
	b, e := os.ReadFile("../zlibcompat/testdata/ng-png.json")
	if e != nil {
		t.Fatal(e)
	}
	if e = json.Unmarshal(b, &f); e != nil {
		t.Fatal(e)
	}
	for _, c := range f.Cases {
		if c.Source == "" {
			continue
		}
		t.Run(c.Name, func(t *testing.T) {
			z, e := zlib.NewReader(bytes.NewReader(c.Stream))
			if e != nil {
				t.Fatal(e)
			}
			raw, e := io.ReadAll(z)
			if e != nil {
				t.Fatal(e)
			}
			z.Close()
			w, h := (c.RowBytes-1)/4, len(raw)/c.RowBytes
			straight := image.NewNRGBA(image.Rect(0, 0, w, h))
			for y := 0; y < h; y++ {
				row := raw[y*c.RowBytes : (y+1)*c.RowBytes]
				if row[0] != 0 {
					t.Fatal("reference is not unfiltered RGBA")
				}
				copy(straight.Pix[y*straight.Stride:], row[1:])
			}
			png, e := encodePNG(context.Background(), straight, 300)
			if e != nil {
				t.Fatal(e)
			}
			if got := fmt.Sprintf("%x", sha256.Sum256(png)); got != c.Hash {
				t.Fatalf("whole PNG %s want %s", got, c.Hash)
			}
		})
	}
}
