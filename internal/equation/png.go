package equation

import (
	"bytes"
	"context"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/color"
	"math"

	"github.com/openvibely/openvibely-paperworks/internal/zlibcompat"
)

// encodePNG preserves the pinned Sharp PNG defaults: eight-bit straight RGBA,
// no row filtering, density metadata, and zlib-ng row/output-buffer scheduling.
func encodePNG(ctx context.Context, im image.Image, dpi float64) ([]byte, error) {
	b := im.Bounds()
	rowBytes := 1 + 4*b.Dx()
	scanlines := make([]byte, rowBytes*b.Dy())
	for y := b.Min.Y; y < b.Max.Y; y++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		row := scanlines[(y-b.Min.Y)*rowBytes : (y-b.Min.Y+1)*rowBytes]
		for x, i := b.Min.X, 1; x < b.Max.X; x, i = x+1, i+4 {
			c := color.NRGBAModel.Convert(im.At(x, y)).(color.NRGBA)
			// The pinned SVG loader recovers straight channels using byte-scale
			// integer division (libvips foreign/cairo.c), not Go's 16-bit color
			// conversion. Preserve straight input images without reconversion.
			if rgba, ok := im.(*image.RGBA); ok {
				v := rgba.RGBAAt(x, y)
				c = color.NRGBA{v.R, v.G, v.B, v.A}
				if v.A != 0 && v.A != 255 {
					c.R = uint8(255 * uint16(v.R) / uint16(v.A))
					c.G = uint8(255 * uint16(v.G) / uint16(v.A))
					c.B = uint8(255 * uint16(v.B) / uint16(v.A))
				}
			}
			row[i], row[i+1], row[i+2], row[i+3] = c.R, c.G, c.B, c.A
		}
	}
	compressed, err := zlibcompat.ZlibNGPNG(ctx, scanlines, rowBytes)
	if err != nil {
		return nil, err
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var out bytes.Buffer
	out.WriteString("\x89PNG\r\n\x1a\n")
	chunk := func(kind string, data []byte) {
		var n [4]byte
		binary.BigEndian.PutUint32(n[:], uint32(len(data)))
		out.Write(n[:])
		out.WriteString(kind)
		out.Write(data)
		crc := crc32.NewIEEE()
		crc.Write([]byte(kind))
		crc.Write(data)
		binary.BigEndian.PutUint32(n[:], crc.Sum32())
		out.Write(n[:])
	}
	var header [13]byte
	binary.BigEndian.PutUint32(header[:4], uint32(b.Dx()))
	binary.BigEndian.PutUint32(header[4:8], uint32(b.Dy()))
	header[8], header[9] = 8, 6
	chunk("IHDR", header[:])
	var density [9]byte
	ppm := uint32(math.Floor(dpi/0.0254 + .5))
	binary.BigEndian.PutUint32(density[:4], ppm)
	binary.BigEndian.PutUint32(density[4:8], ppm)
	density[8] = 1 // metres
	chunk("pHYs", density[:])
	for len(compressed) > 8192 {
		chunk("IDAT", compressed[:8192])
		compressed = compressed[8192:]
	}
	chunk("IDAT", compressed)
	chunk("IEND", nil)
	return out.Bytes(), nil
}
