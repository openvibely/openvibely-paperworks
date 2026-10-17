package zlibcompat

import (
	"bytes"
	"compress/zlib"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"testing"
)

func TestPinnedNGRows(t *testing.T) {
	testPinnedNG(t, "testdata/ng-rows.json", ZlibNGRows)
}
func TestPinnedNGPNG(t *testing.T) {
	testPinnedNG(t, "testdata/ng-png.json", func(b []byte, n int) ([]byte, error) { return ZlibNGPNG(context.Background(), b, n) })
}
func testPinnedNG(t *testing.T, path string, compress func([]byte, int) ([]byte, error)) {
	var f struct {
		Cases []struct {
			Name     string
			RowBytes int    `json:"row_bytes"`
			RawSHA   string `json:"raw_sha256"`
			Stream   []byte
		}
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(b, &f); err != nil {
		t.Fatal(err)
	}
	for _, c := range f.Cases {
		t.Run(c.Name, func(t *testing.T) {
			z, err := zlib.NewReader(bytes.NewReader(c.Stream))
			if err != nil {
				t.Fatal(err)
			}
			raw, err := io.ReadAll(z)
			if err != nil {
				t.Fatal(err)
			}
			z.Close()
			if fmt.Sprintf("%x", sha256.Sum256(raw)) != c.RawSHA {
				t.Fatal("input hash")
			}
			got, err := compress(raw, c.RowBytes)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, c.Stream) {
				first := 0
				for first < min(len(got), len(c.Stream)) && got[first] == c.Stream[first] {
					first++
				}
				t.Fatalf("compressed bytes differ at %d; lengths %d/%d", first, len(got), len(c.Stream))
			}
		})
	}
}
