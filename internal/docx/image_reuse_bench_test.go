package docx

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openvibely/openvibely-paperworks/internal/opc"
)

const imageBenchmarkBytes = 1 << 20

func benchmarkJPEG(t testing.TB, seed int64) []byte {
	t.Helper()
	rng := rand.New(rand.NewSource(seed))
	pixels := image.NewRGBA(image.Rect(0, 0, 128, 128))
	for y := 0; y < 128; y++ {
		for x := 0; x < 128; x++ {
			pixels.SetRGBA(x, y, color.RGBA{R: uint8(rng.Intn(256)), G: uint8(rng.Intn(256)), B: uint8(rng.Intn(256)), A: 255})
		}
	}
	var encoded bytes.Buffer
	if err := jpeg.Encode(&encoded, pixels, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatal(err)
	}
	jpegData := encoded.Bytes()
	if len(jpegData) > imageBenchmarkBytes-4 {
		t.Fatalf("base JPEG too large: %d bytes", len(jpegData))
	}

	// Pad with valid JPEG comment segments before EOI to make each benchmark
	// image exactly 1 MiB without changing its decoded pixels.
	padding := imageBenchmarkBytes - len(jpegData)
	var out bytes.Buffer
	out.Grow(imageBenchmarkBytes)
	out.Write(jpegData[:len(jpegData)-2])
	for padding > 0 {
		segmentSize := min(padding, 65535)
		if remainder := padding - segmentSize; remainder > 0 && remainder < 4 {
			segmentSize -= 4 - remainder
		}
		if segmentSize < 4 {
			t.Fatalf("cannot encode JPEG comment segment of %d bytes", segmentSize)
		}
		out.Write([]byte{0xff, 0xfe})
		binary.Write(&out, binary.BigEndian, uint16(segmentSize-2))
		payload := make([]byte, segmentSize-4)
		_, _ = rng.Read(payload)
		out.Write(payload)
		padding -= segmentSize
	}
	out.Write(jpegData[len(jpegData)-2:])
	if out.Len() != imageBenchmarkBytes {
		t.Fatalf("benchmark JPEG has %d bytes; want %d", out.Len(), imageBenchmarkBytes)
	}
	return out.Bytes()
}

func benchmarkImageSpec(paths []string) Spec {
	blocks := make([]Block, len(paths))
	for i, path := range paths {
		blocks[i] = Block{Type: "image", Path: path, WidthIn: 2, Description: "benchmark image"}
	}
	return Spec{Page: Page{WidthIn: 8.5, HeightIn: 11, MarginIn: 1}, Blocks: blocks}
}

func BenchmarkBuildImageReuse(b *testing.B) {
	dir := b.TempDir()
	repeated := benchmarkJPEG(b, 1)
	repeatedPath := filepath.Join(dir, "repeated.jpg")
	if err := os.WriteFile(repeatedPath, repeated, 0o600); err != nil {
		b.Fatal(err)
	}
	repeatedPaths := make([]string, 100)
	for i := range repeatedPaths {
		repeatedPaths[i] = repeatedPath
	}

	distinctDir := filepath.Join(dir, "distinct")
	if err := os.Mkdir(distinctDir, 0o700); err != nil {
		b.Fatal(err)
	}
	distinctPaths := make([]string, 100)
	for i := range distinctPaths {
		name := filepath.Join(distinctDir, fmt.Sprintf("image-%03d.jpg", i))
		data := benchmarkJPEG(b, int64(i+2))
		if err := os.WriteFile(name, data, 0o600); err != nil {
			b.Fatal(err)
		}
		distinctPaths[i] = name
	}

	for _, tc := range []struct {
		name  string
		paths []string
	}{
		{name: "repeated-identical", paths: repeatedPaths},
		{name: "100-distinct", paths: distinctPaths},
	} {
		b.Run(tc.name, func(b *testing.B) {
			spec := benchmarkImageSpec(tc.paths)
			b.ReportAllocs()
			b.ResetTimer()
			var output []byte
			for i := 0; i < b.N; i++ {
				var err error
				output, err = Build(spec, "")
				if err != nil {
					b.Fatal(err)
				}
			}
			b.StopTimer()
			pkg, err := opc.Read(output)
			if err != nil {
				b.Fatal(err)
			}
			mediaBytes := 0
			for _, part := range pkg.Parts {
				if strings.HasPrefix(part.Name, "word/media/") {
					mediaBytes += len(part.Data)
				}
			}
			b.ReportMetric(float64(len(output)), "output-B")
			b.ReportMetric(float64(mediaBytes), "media-B")
		})
	}
}
