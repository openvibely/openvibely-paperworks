package equation

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"testing"
)

func bitmapFixture(t *testing.T) []byte {
	t.Helper()
	im := image.NewNRGBA(image.Rect(0, 0, 2, 1))
	im.SetNRGBA(0, 0, color.NRGBA{255, 80, 20, 128})
	im.SetNRGBA(1, 0, color.NRGBA{10, 200, 50, 255})
	var p bytes.Buffer
	if err := png.Encode(&p, im); err != nil {
		t.Fatal(err)
	}
	b := make([]byte, 94+p.Len())
	put16 := func(i int, v uint16) { binary.BigEndian.PutUint16(b[i:], v) }
	put32 := func(i int, v uint32) { binary.BigEndian.PutUint32(b[i:], v) }
	put32(0, 0x10000)
	put16(4, 2)
	copy(b[12:], "maxp")
	put32(20, 44)
	put32(24, 6)
	put16(48, 2)
	copy(b[28:], "sbix")
	put32(36, 50)
	put32(40, uint32(len(b)-50))
	put16(50, 1)
	put16(52, 1)
	put32(54, 1)
	put32(58, 12)
	put16(62, 32)
	put16(64, 72)
	put32(66, 16)
	put32(70, 26)
	put32(74, uint32(len(b)-62))
	put16(78, 3)
	put16(80, 0xfffe)
	copy(b[82:], "dupe")
	put16(86, 1)
	// Data starts at 88 for glyph 1: eight-byte header and PNG.
	b = append(b[:88], append(make([]byte, 8), p.Bytes()...)...)
	copy(b[92:], "png ")
	put32(40, uint32(len(b)-50))
	put32(74, uint32(len(b)-62))
	return b
}

func TestSystemBitmap(t *testing.T) {
	b := bitmapFixture(t)
	g, err := loadSystemBitmap(b, 0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if g.PPEM != 32 || g.PPI != 72 || g.OriginX != 3 || g.OriginY != -2 || g.Image.Bounds() != image.Rect(0, 0, 2, 1) {
		t.Fatalf("wrong bitmap metadata: %+v", g)
	}
	want := color.NRGBA{255, 80, 20, 128}
	if got := color.NRGBAModel.Convert(g.Image.At(0, 0)); got != want {
		t.Fatalf("colors: %v want %v", got, want)
	}
	g.PNG[0] = 0
	if b[96] != 137 {
		t.Fatal("bitmap aliases font memory")
	}
	for _, c := range [][3]int{{1, 0, 0}, {0, 2, 0}, {0, 0, 1}, {-1, 0, 0}} {
		if _, e := loadSystemBitmap(b, c[0], c[1], c[2]); e == nil {
			t.Fatal("accepted absent selection", c)
		}
	}
	for n := 0; n < len(b); n++ {
		if _, e := loadSystemBitmap(b[:n], 0, 0, 0); e == nil {
			t.Fatalf("accepted truncation %d", n)
		}
	}
	cycle := bytes.Clone(b)
	binary.BigEndian.PutUint16(cycle[86:], 0)
	if _, e := loadSystemBitmap(cycle, 0, 0, 0); e == nil {
		t.Fatal("accepted duplicate cycle")
	}
	invalid := bytes.Clone(b)
	binary.BigEndian.PutUint32(invalid[70:], 0xffffffff)
	if _, e := loadSystemBitmap(invalid, 0, 0, 0); e == nil {
		t.Fatal("accepted out-of-bounds glyph")
	}
}

// A file can be truncated after Stat; exercise short reads with the original size.
func TestSystemBitmapReaderAtBounds(t *testing.T) {
	data := bitmapFixture(t)
	for n := 0; n < len(data); n++ {
		if _, err := loadSystemBitmapReaderAt(bytes.NewReader(data[:n]), int64(len(data)), 0, 0, 0); err == nil {
			t.Fatalf("accepted short reader at %d", n)
		}
	}
	for _, c := range []struct {
		name   string
		offset int
		value  uint32
	}{
		{"table offset", 36, 0xffffffff}, {"table length", 40, 0xffffffff},
		{"strike offset", 58, 0xffffffff}, {"glyph offset", 70, 0xffffffff},
	} {
		t.Run(c.name, func(t *testing.T) {
			bad := bytes.Clone(data)
			binary.BigEndian.PutUint32(bad[c.offset:], c.value)
			if _, err := loadSystemBitmapReaderAt(bytes.NewReader(bad), int64(len(bad)), 0, 0, 0); err == nil {
				t.Fatal("accepted invalid range")
			}
			if c.offset != 70 {
				if _, err := largestSystemStrikeReaderAt(bytes.NewReader(bad), int64(len(bad))); err == nil {
					t.Fatal("accepted invalid strike directory")
				}
			}
		})
	}
	// TTC face directory offsets are absolute and must stay within the file.
	for _, base := range []uint32{0xfffffff0, uint32(len(data) - 8)} {
		bad := bytes.Clone(data)
		copy(bad, "ttcf")
		binary.BigEndian.PutUint32(bad[8:], 1)
		binary.BigEndian.PutUint32(bad[12:], base)
		if _, err := loadSystemBitmapReaderAt(bytes.NewReader(bad), int64(len(bad)), 0, 0, 0); err == nil {
			t.Fatal("accepted invalid TTC face offset")
		}
		if _, err := largestSystemStrikeReaderAt(bytes.NewReader(bad), int64(len(bad))); err == nil {
			t.Fatal("accepted invalid TTC strike directory")
		}
	}
	for n := 0; n < 66; n++ {
		if _, err := largestSystemStrikeReaderAt(bytes.NewReader(data[:n]), int64(len(data))); err == nil {
			t.Fatalf("accepted truncated strike header %d", n)
		}
	}
	// Extra font bytes must not be read just because the file is large.
	r := &countingFontReader{Reader: bytes.NewReader(data)}
	if _, err := loadSystemBitmapReaderAt(r, 256<<20, 0, 0, 0); err != nil {
		t.Fatal(err)
	}
	if r.total > len(data)*2 {
		t.Fatalf("read %d bytes for %d-byte fixture", r.total, len(data))
	}
	if _, err := loadSystemBitmapReaderAt(r, 256<<20+1, 0, 0, 0); err == nil {
		t.Fatal("accepted oversized font")
	}
}

type countingFontReader struct {
	*bytes.Reader
	total int
}

func (r *countingFontReader) ReadAt(p []byte, off int64) (int, error) {
	r.total += len(p)
	return r.Reader.ReadAt(p, off)
}
