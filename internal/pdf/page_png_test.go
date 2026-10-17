package pdf

import (
	"bytes"
	"compress/zlib"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"testing"
)

func pagePNGFiltered(t *testing.T, b []byte) []byte {
	t.Helper()
	var data []byte
	for i := 8; i < len(b); {
		if len(b)-i < 12 {
			t.Fatal("truncated PNG chunk")
		}
		n := int(binary.BigEndian.Uint32(b[i:]))
		if n > len(b)-i-12 {
			t.Fatal("truncated PNG payload")
		}
		if string(b[i+4:i+8]) == "IDAT" {
			if n > 65536 {
				t.Fatal("oversized IDAT")
			}
			data = append(data, b[i+8:i+8+n]...)
		}
		i += n + 12
	}
	r, err := zlib.NewReader(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	raw, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestPagePNGDefaultPillowFilters(t *testing.T) {
	// SHA256 values from Pillow 12.3.0 Image.frombytes("RGB",...).save(),
	// default optimize=False. Hash only inflated IDAT: compression is not equal.
	for _, c := range []struct {
		name string
		w, h int
		hash string
	}{
		{"white", 5, 4, "3c0704f6d03b6174f7b202d61bb9c253b4c3ee749946cca6261ec5b879ff5c52"},
		{"black", 5, 4, "f5a5fd42d16a20302798ef6ed309979b43003d2320d9f0e8ea9831a92759fb4b"},
		{"gray", 5, 4, "32269592e8e44d894491caccef1ba72a7863de0f2d8ab526ae6b088c17b91adf"},
		{"gradient", 19, 11, "4f7c1adbb166c07d9f401c040931c530c3ec812a9cca6a8ead7ef5505996a978"},
		{"pattern", 23, 17, "4c8f2a3b84cedf236373c40fc74976ffc269c02f8b0239cef13d0fcf5bf18aed"},
		{"noise", 311, 277, "c1cb89c8b20cf51338fe16534c25ea6529aa52b97613b34aaed0829605c83bcb"},
	} {
		t.Run(c.name, func(t *testing.T) {
			// Exercise non-zero origin and padded stride as well as opaque grayscale.
			parent := image.NewRGBA(image.Rect(0, 0, c.w+4, c.h+4))
			im := parent.SubImage(image.Rect(2, 2, c.w+2, c.h+2)).(*image.RGBA)
			for y := 0; y < c.h; y++ {
				for x := 0; x < c.w; x++ {
					p := color.RGBA{A: 255}
					switch c.name {
					case "white":
						p.R, p.G, p.B = 255, 255, 255
					case "gray":
						p.R, p.G, p.B = 128, 128, 128
					case "gradient":
						p.R, p.G, p.B = byte(x*11), byte(y*23), byte((x+y)*7)
					case "pattern":
						p.R, p.G, p.B = byte(x%3*90), byte(y%3*90), byte((x+y)%5*50)
					case "noise":
						n := uint32((y*c.w + x) * 3)
						p.R, p.G, p.B = byte((n+1)*2654435761>>24), byte((n+2)*2654435761>>24), byte((n+3)*2654435761>>24)
					}
					im.SetRGBA(x+2, y+2, p)
				}
			}
			var b bytes.Buffer
			if err := encodePagePNG(&b, im); err != nil {
				t.Fatal(err)
			}
			raw := pagePNGFiltered(t, b.Bytes())
			if got := fmt.Sprintf("%x", sha256.Sum256(raw)); got != c.hash {
				t.Fatalf("filtered SHA %s != %s", got, c.hash)
			}
			decoded, err := png.Decode(bytes.NewReader(b.Bytes()))
			if err != nil {
				t.Fatal(err)
			}
			for y := 0; y < c.h; y++ {
				for x := 0; x < c.w; x++ {
					if color.RGBAModel.Convert(decoded.At(x, y)) != im.RGBAAt(x+2, y+2) {
						t.Fatalf("pixel %d,%d changed", x, y)
					}
				}
			}
		})
	}
}

type pagePNGShortWriter struct{}

func (pagePNGShortWriter) Write(p []byte) (int, error) { return 0, nil }
func TestPagePNGWriteBoundaries(t *testing.T) {
	im := image.NewRGBA(image.Rect(0, 0, 1, 1))
	im.SetRGBA(0, 0, color.RGBA{A: 255})
	if err := encodePagePNG(pagePNGShortWriter{}, im); err != io.ErrShortWrite {
		t.Fatalf("short write: %v", err)
	}
	im.SetRGBA(0, 0, color.RGBA{})
	var b bytes.Buffer
	if err := encodePagePNG(&b, im); err == nil || b.Len() != 0 {
		t.Fatal("transparent input silently changed")
	}
	var out bytes.Buffer
	cw := &pagePNGData{w: &out}
	payload := bytes.Repeat([]byte{42}, 131073)
	if n, err := cw.Write(payload); n != len(payload) || err != nil {
		t.Fatal(n, err)
	}
	if err := cw.flush(); err != nil {
		t.Fatal(err)
	}
	for i, want := 0, 0; i < out.Len(); want++ {
		n := int(binary.BigEndian.Uint32(out.Bytes()[i:]))
		expected := 65536
		if want == 2 {
			expected = 1
		}
		if n != expected {
			t.Fatal(n, expected)
		}
		i += n + 12
	}
}

// Complete PNG captured from pinned Pillow 12.3.0, including multiple IDAT
// chunks. This checks encoding independently of PDF rasterization.
func TestPagePNGCompletePillow(t *testing.T) {
	im := image.NewRGBA(image.Rect(0, 0, 300, 240))
	for y := 0; y < 240; y++ {
		for x := 0; x < 300; x++ {
			im.SetRGBA(x, y, color.RGBA{uint8(x*17 + y*3), uint8(x*x + y*11), uint8(x + y*y), 255})
		}
	}
	var b bytes.Buffer
	if err := encodePagePNG(&b, im); err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(b.Bytes())); got != "0386f9bf58438a5d3adba6c1c8d5e5916a8ce5fc585fe8dd6492324faa5adf40" {
		t.Fatalf("PNG hash %s", got)
	}
}
