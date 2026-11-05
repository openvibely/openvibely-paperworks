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

func TestZlibNGPageStreamMatchesBatch(t *testing.T) {
	const rowBytes, rows = 701, 190
	for _, tc := range []struct {
		name string
		fill func([]byte)
	}{
		{
			name: "high-entropy",
			fill: func(p []byte) {
				state := uint32(0x12345678)
				for i := range p {
					state ^= state << 13
					state ^= state >> 17
					state ^= state << 5
					p[i] = byte(state)
				}
			},
		},
		{
			name: "repeated-background",
			fill: func(p []byte) {
				for i := range p {
					p[i] = 0x7f
				}
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := make([]byte, rowBytes*rows)
			tc.fill(input)
			want, err := ZlibNGPage(context.Background(), input, rowBytes)
			if err != nil {
				t.Fatal(err)
			}
			var got bytes.Buffer
			stream, err := NewZlibNGPageStream(context.Background(), &got, rowBytes)
			if err != nil {
				t.Fatal(err)
			}
			for i := 0; i < len(input); i += rowBytes {
				if err := stream.WriteRow(input[i : i+rowBytes]); err != nil {
					t.Fatal(err)
				}
			}
			if err := stream.Close(); err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got.Bytes(), want) {
				t.Fatalf("stream differs from batch: lengths %d/%d", got.Len(), len(want))
			}
		})
	}
}

func TestZlibNGPageStreamShortWriter(t *testing.T) {
	w := &ngShortWriter{limit: 2}
	stream, err := NewZlibNGPageStream(context.Background(), w, 4)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 100 && stream.err == nil; i++ {
		if err := stream.WriteRow([]byte{1, 2, 3, 4}); err != nil {
			if err != io.ErrShortWrite {
				t.Fatal(err)
			}
			return
		}
	}
	if err := stream.Close(); err != io.ErrShortWrite {
		t.Fatalf("Close() = %v, want %v", err, io.ErrShortWrite)
	}
}

type ngShortWriter struct {
	limit int
	bytes int
}

func (w *ngShortWriter) Write(p []byte) (int, error) {
	if w.bytes >= w.limit {
		return 0, nil
	}
	n := min(len(p), w.limit-w.bytes)
	w.bytes += n
	return n, nil
}

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
