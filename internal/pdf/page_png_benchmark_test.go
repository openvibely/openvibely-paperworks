package pdf

import (
	"fmt"
	"image"
	"io"
	"testing"
)

func BenchmarkPagePNG(b *testing.B) {
	for _, size := range []struct {
		name          string
		width, height int
	}{
		{"5MP", 2500, 2000},
		{"20MP", 5000, 4000},
		{"40MP", 8000, 5000},
	} {
		for _, pattern := range []string{"high-entropy", "repeated-background"} {
			b.Run(fmt.Sprintf("%s/%s", size.name, pattern), func(b *testing.B) {
				im := pagePNGBenchmarkImage(size.width, size.height, pattern == "high-entropy")
				b.SetBytes(int64(size.width) * int64(size.height) * 3)
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					if err := encodePagePNG(io.Discard, im); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}

func pagePNGBenchmarkImage(width, height int, highEntropy bool) *image.RGBA {
	im := image.NewRGBA(image.Rect(0, 0, width, height))
	if !highEntropy {
		for i := 3; i < len(im.Pix); i += 4 {
			im.Pix[i] = 255
		}
		for i := 0; i < len(im.Pix); i += 4 {
			im.Pix[i], im.Pix[i+1], im.Pix[i+2] = 0x32, 0x78, 0xc4
		}
		return im
	}
	state := uint32(0x6d2b79f5)
	for i := 0; i < len(im.Pix); i += 4 {
		state ^= state << 13
		state ^= state >> 17
		state ^= state << 5
		im.Pix[i], im.Pix[i+1], im.Pix[i+2], im.Pix[i+3] = byte(state), byte(state>>8), byte(state>>16), 255
	}
	return im
}
