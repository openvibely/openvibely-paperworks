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

func bitmapTTCFixture(t *testing.T, font []byte, faces int) []byte {
	return bitmapTTCVersionFixture(t, font, faces, 0x00010000)
}

func bitmapTTCVersionFixture(t *testing.T, font []byte, faces int, version uint32) []byte {
	t.Helper()
	if faces < 1 {
		t.Fatal("TTC fixture needs at least one face")
	}
	headerSize := 12 + 4*faces
	if version == 0x00020000 {
		headerSize += 12
	}
	b := make([]byte, headerSize+faces*len(font))
	copy(b, "ttcf")
	binary.BigEndian.PutUint32(b[4:], version)
	binary.BigEndian.PutUint32(b[8:], uint32(faces))
	for face := 0; face < faces; face++ {
		base := headerSize + face*len(font)
		binary.BigEndian.PutUint32(b[12+4*face:], uint32(base))
		copy(b[base:], font)
		tableCount := int(binary.BigEndian.Uint16(b[base+4:]))
		for i := 0; i < tableCount; i++ {
			offset := base + 12 + i*16 + 8
			fontOffset := binary.BigEndian.Uint32(b[offset:])
			binary.BigEndian.PutUint32(b[offset:], fontOffset+uint32(base))
		}
	}
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
	unsupported := bytes.Clone(b)
	copy(unsupported[82:], "jpg ")
	if _, e := loadSystemBitmap(unsupported, 0, 0, 0); e == nil {
		t.Fatal("accepted unsupported sbix glyph data")
	}
	invalid := bytes.Clone(b)
	binary.BigEndian.PutUint32(invalid[70:], 0xffffffff)
	if _, e := loadSystemBitmap(invalid, 0, 0, 0); e == nil {
		t.Fatal("accepted out-of-bounds glyph")
	}
}

func TestSystemBitmapTTFAndTTC(t *testing.T) {
	font := bitmapFixture(t)
	ttc := bitmapTTCFixture(t, font, 2)
	ttcV2 := bitmapTTCVersionFixture(t, font, 1, 0x00020000)
	for _, tc := range []struct {
		name string
		data []byte
		face int
	}{
		{name: "ttf", data: font},
		{name: "ttc first face", data: ttc},
		{name: "ttc second face", data: ttc, face: 1},
		{name: "ttc version 2", data: ttcV2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			glyph, err := loadSystemBitmap(tc.data, tc.face, 0, 0)
			if err != nil {
				t.Fatal(err)
			}
			if glyph.Image.Bounds() != image.Rect(0, 0, 2, 1) {
				t.Fatalf("unexpected image bounds: %v", glyph.Image.Bounds())
			}
		})
	}
	for name, collection := range map[string][]byte{"TTC v1": ttc, "TTC v2": ttcV2} {
		if strike, err := largestSystemStrike(collection); err != nil || strike != 0 {
			t.Errorf("%s strike selection = %d, %v; want 0, nil", name, strike, err)
		}
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
		{"maxp table offset", 20, 0xffffffff}, {"maxp table length", 24, 0xffffffff},
		{"sbix table offset", 36, 0xffffffff}, {"sbix table length", 40, 0xffffffff},
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
	// TTC offsets are absolute and may not point into the collection header or
	// outside the file. The shared reader also rejects malformed TTC headers.
	ttc := bitmapTTCFixture(t, data, 1)
	for _, tc := range []struct {
		name   string
		offset int
		value  uint32
	}{
		{"offset outside file", 12, 0xfffffff0},
		{"offset into header", 12, 12},
		{"unsupported version", 4, 0x00030000},
		{"zero face count", 8, 0},
		{"excessive face count", 8, 65537},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bad := bytes.Clone(ttc)
			binary.BigEndian.PutUint32(bad[tc.offset:], tc.value)
			if _, err := loadSystemBitmapReaderAt(bytes.NewReader(bad), int64(len(bad)), 0, 0, 0); err == nil {
				t.Fatal("accepted invalid TTC face directory")
			}
			if _, err := largestSystemStrikeReaderAt(bytes.NewReader(bad), int64(len(bad))); err == nil {
				t.Fatal("accepted invalid TTC strike directory")
			}
		})
	}
	for n := 0; n < 16; n++ {
		if _, err := largestSystemStrikeReaderAt(bytes.NewReader(ttc[:n]), int64(n)); err == nil {
			t.Fatalf("accepted truncated TTC header %d", n)
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
