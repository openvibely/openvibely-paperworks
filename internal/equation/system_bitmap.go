package equation

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/png"
	"io"
)

// systemBitmap is an explicitly selected sbix strike. Origins are signed pixel
// offsets from the design origin to the bitmap's lower left, not layout metrics.
// No host discovery, strike heuristic, shaping, or resampling is performed here.
type systemBitmap struct {
	Image            image.Image
	PNG              []byte
	OriginX, OriginY int16
	PPEM, PPI        uint16
}

// loadSystemBitmap reads bounded OpenType/TTC sbix PNG glyphs. The caller must
// establish face, glyph and strike selection independently. Unsupported image
// formats fail explicitly; empty outline/color data never counts as a glyph.
func loadSystemBitmap(data []byte, face, glyph, strike int) (systemBitmap, error) {
	return loadSystemBitmapReaderAt(bytes.NewReader(data), int64(len(data)), face, glyph, strike)
}

// fontRange keeps every read within its enclosing font/table/strike boundary.
type fontRange struct {
	r         io.ReaderAt
	off, size uint64
}

func (r fontRange) section(off, n uint64) (fontRange, bool) {
	if off > r.size || n > r.size-off {
		return fontRange{}, false
	}
	return fontRange{r.r, r.off + off, n}, true
}

func (r fontRange) read(off, n uint64) ([]byte, bool) {
	sub, ok := r.section(off, n)
	if !ok {
		return nil, false
	}
	b := make([]byte, int(n))
	count, err := sub.r.ReadAt(b, int64(sub.off))
	return b, count == len(b) && err == nil
}

func loadSystemBitmapReaderAt(reader io.ReaderAt, size int64, face, glyph, strike int) (systemBitmap, error) {
	var out systemBitmap
	bad := func() (systemBitmap, error) { return out, fmt.Errorf("invalid or unsupported sbix glyph") }
	if size < 12 || size > 256<<20 || face < 0 || glyph < 0 || strike < 0 {
		return bad()
	}
	data := fontRange{reader, 0, uint64(size)}
	tables, ok := readSFNTDirectory(data, face)
	if !ok {
		return bad()
	}
	u16 := func(b []byte) uint16 { return binary.BigEndian.Uint16(b) }
	u32 := func(b []byte) uint64 { return uint64(binary.BigEndian.Uint32(b)) }
	maxp, ok := tables["maxp"].read(0, 6)
	sb := tables["sbix"]
	sbhead, sbOK := sb.read(0, 8)
	if !ok || !sbOK || u16(sbhead) != 1 {
		return bad()
	}
	ng := uint64(u16(maxp[4:]))
	ns := u32(sbhead[4:])
	if uint64(glyph) >= ng || uint64(strike) >= ns || ns > 65536 {
		return bad()
	}
	offsets, ok := sb.read(8, ns*4)
	if !ok {
		return bad()
	}
	start := u32(offsets[strike*4:])
	end := sb.size
	if uint64(strike)+1 < ns {
		end = u32(offsets[(strike+1)*4:])
	}
	if start < 8+4*ns || start > end {
		return bad()
	}
	st, ok := sb.section(start, end-start)
	if !ok || st.size < 4 {
		return bad()
	}
	sthead, ok := st.read(0, 4)
	if !ok {
		return bad()
	}
	out.PPEM, out.PPI = u16(sthead), u16(sthead[2:])
	if out.PPEM == 0 {
		return bad()
	}
	goff, ok := st.read(4, (ng+1)*4)
	if !ok {
		return bad()
	}
	seen := map[int]bool{}
	for {
		if uint64(glyph) >= ng || seen[glyph] || len(seen) >= 64 {
			return bad()
		}
		seen[glyph] = true
		a, z := u32(goff[glyph*4:]), u32(goff[(glyph+1)*4:])
		if a < 4+(ng+1)*4 || z < a {
			return bad()
		}
		b, ok := st.read(a, z-a)
		if !ok || len(b) < 8 {
			return bad()
		}
		if len(seen) == 1 {
			out.OriginX, out.OriginY = int16(u16(b)), int16(u16(b[2:]))
		}
		switch string(b[4:8]) {
		case "dupe":
			if len(b) != 10 {
				return bad()
			}
			glyph = int(u16(b[8:]))
			continue
		case "png ":
			cfg, e := png.DecodeConfig(bytes.NewReader(b[8:]))
			if e != nil || cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > 4096 || cfg.Height > 4096 || int64(cfg.Width)*int64(cfg.Height) > 16000000 {
				return bad()
			}
			im, e := png.Decode(bytes.NewReader(b[8:]))
			if e != nil {
				return out, e
			}
			out.Image = im
			out.PNG = b[8:]
			return out, nil
		default:
			return bad()
		}
	}
}
