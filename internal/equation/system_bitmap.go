package equation

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/png"
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
	var out systemBitmap
	bad := func() (systemBitmap, error) { return out, fmt.Errorf("invalid or unsupported sbix glyph") }
	if len(data) < 12 || len(data) > 256<<20 || face < 0 || glyph < 0 || strike < 0 {
		return bad()
	}
	u16 := func(b []byte) uint16 { return binary.BigEndian.Uint16(b) }
	u32 := func(b []byte) uint64 { return uint64(binary.BigEndian.Uint32(b)) }
	slice := func(b []byte, off, n uint64) ([]byte, bool) {
		if off > uint64(len(b)) || n > uint64(len(b))-off {
			return nil, false
		}
		return b[off : off+n], true
	}
	base := uint64(0)
	if string(data[:4]) == "ttcf" {
		n := u32(data[8:])
		if uint64(face) >= n || n > 65536 {
			return bad()
		}
		p, ok := slice(data, 12+4*uint64(face), 4)
		if !ok {
			return bad()
		}
		base = u32(p)
	} else if face != 0 {
		return bad()
	}
	head, ok := slice(data, base, 12)
	if !ok {
		return bad()
	}
	n := uint64(u16(head[4:]))
	records, ok := slice(data, base+12, n*16)
	if !ok {
		return bad()
	}
	tables := map[string][]byte{}
	for i := uint64(0); i < n; i++ {
		r := records[i*16:]
		tag := string(r[:4])
		if tag != "maxp" && tag != "sbix" {
			continue
		}
		if _, exists := tables[tag]; exists {
			return bad()
		}
		b, ok := slice(data, u32(r[8:]), u32(r[12:]))
		if !ok {
			return bad()
		}
		tables[tag] = b
	}
	maxp, sb := tables["maxp"], tables["sbix"]
	if len(maxp) < 6 || len(sb) < 8 || u16(sb) != 1 {
		return bad()
	}
	ng := uint64(u16(maxp[4:]))
	ns := u32(sb[4:])
	if uint64(glyph) >= ng || uint64(strike) >= ns || ns > 65536 {
		return bad()
	}
	offsets, ok := slice(sb, 8, ns*4)
	if !ok {
		return bad()
	}
	start := u32(offsets[strike*4:])
	end := uint64(len(sb))
	if uint64(strike)+1 < ns {
		end = u32(offsets[(strike+1)*4:])
	}
	if start < 8+4*ns || start > end {
		return bad()
	}
	st, ok := slice(sb, start, end-start)
	if !ok || len(st) < 4 {
		return bad()
	}
	out.PPEM, out.PPI = u16(st), u16(st[2:])
	if out.PPEM == 0 {
		return bad()
	}
	goff, ok := slice(st, 4, (ng+1)*4)
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
		b, ok := slice(st, a, z-a)
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
			out.PNG = bytes.Clone(b[8:])
			return out, nil
		default:
			return bad()
		}
	}
}
