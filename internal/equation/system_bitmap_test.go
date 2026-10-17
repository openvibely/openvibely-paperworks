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
