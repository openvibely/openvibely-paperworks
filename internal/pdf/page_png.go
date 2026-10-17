package pdf

import (
	"context"
	"encoding/binary"
	"fmt"
	"github.com/openvibely/openvibely-paperworks/internal/zlibcompat"
	"hash/crc32"
	"image"
	"io"
)

// encodePagePNG writes opaque RGB pages using the row selection of pinned
// Pillow 12.3.0's default PNG encoder: None, Up, Sub, Paeth, with strict
// improvement and no Average candidate. Go's standard encoder has different
// tie ordering and also considers Average. Compression follows the pinned Pillow zlib-ng level-6/memLevel-9 schedule.
func encodePagePNG(w io.Writer, im *image.RGBA) error {
	b := im.Bounds()
	if b.Dx() <= 0 || b.Dy() <= 0 || !im.Opaque() {
		return fmt.Errorf("page PNG requires a nonempty opaque image")
	}
	if err := pngWrite(w, []byte("\x89PNG\r\n\x1a\n")); err != nil {
		return err
	}
	header := make([]byte, 13)
	binary.BigEndian.PutUint32(header, uint32(b.Dx()))
	binary.BigEndian.PutUint32(header[4:], uint32(b.Dy()))
	header[8], header[9] = 8, 2 // RGB, eight bits/channel
	if err := pagePNGChunk(w, "IHDR", header); err != nil {
		return err
	}
	chunks := &pagePNGData{w: w}
	if int64(b.Dx())*3*int64(b.Dy())+int64(b.Dy()) > 256<<20 {
		return fmt.Errorf("page PNG filtered data exceeds 256 MiB")
	}
	filtered := make([]byte, 0, (b.Dx()*3+1)*b.Dy())
	row, prior := make([]byte, b.Dx()*3), make([]byte, b.Dx()*3)
	best, candidate := make([]byte, len(row)+1), make([]byte, len(row)+1)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x, i := b.Min.X, 0; x < b.Max.X; x, i = x+1, i+3 {
			o := im.PixOffset(x, y)
			copy(row[i:i+3], im.Pix[o:o+3])
		}
		pagePNGFilter(row, prior, best, candidate)
		filtered = append(filtered, best...)
		row, prior = prior, row
	}
	encoded, err := zlibcompat.ZlibNGPage(context.Background(), filtered, len(best))
	if err != nil {
		return err
	}
	if _, err := chunks.Write(encoded); err != nil {
		return err
	}
	if err := chunks.flush(); err != nil {
		return err
	}
	return pagePNGChunk(w, "IEND", nil)
}

func pagePNGFilter(row, prior, best, candidate []byte) {
	best[0] = 0
	copy(best[1:], row)
	score := func(v byte) int {
		if v < 128 {
			return int(v)
		}
		return 256 - int(v)
	}
	sum := 0
	for _, v := range row {
		sum += score(v)
	}
	for _, filter := range []byte{2, 1, 4} {
		if sum == 0 {
			break
		}
		candidate[0] = filter
		s := 0
		for i, v := range row {
			left, above, upperLeft := 0, int(prior[i]), 0
			if i >= 3 {
				left, upperLeft = int(row[i-3]), int(prior[i-3])
			}
			predictor := above
			if filter == 1 {
				predictor = left
			} else if filter == 4 {
				pa, pb, pc := absPNG(above-upperLeft), absPNG(left-upperLeft), absPNG(left+above-2*upperLeft)
				if pa <= pb && pa <= pc {
					predictor = left
				} else if pb > pc {
					predictor = upperLeft
				}
			}
			candidate[i+1] = v - byte(predictor)
			s += score(candidate[i+1])
		}
		if s < sum {
			copy(best, candidate)
			sum = s
		}
	}
}

func absPNG(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

func pngWrite(w io.Writer, p []byte) error {
	n, err := w.Write(p)
	if err == nil && n != len(p) {
		return io.ErrShortWrite
	}
	return err
}

func pagePNGChunk(w io.Writer, kind string, data []byte) error {
	var h [8]byte
	binary.BigEndian.PutUint32(h[:4], uint32(len(data)))
	copy(h[4:], kind)
	crc := crc32.NewIEEE()
	crc.Write(h[4:])
	crc.Write(data)
	var tail [4]byte
	binary.BigEndian.PutUint32(tail[:], crc.Sum32())
	for _, p := range [][]byte{h[:], data, tail[:]} {
		if err := pngWrite(w, p); err != nil {
			return err
		}
	}
	return nil
}

type pagePNGData struct {
	w   io.Writer
	buf [65536]byte
	n   int
}

func (p *pagePNGData) Write(b []byte) (int, error) {
	written := 0
	for len(b) > 0 {
		n := copy(p.buf[p.n:], b)
		p.n += n
		written += n
		b = b[n:]
		if p.n == len(p.buf) {
			if err := p.flush(); err != nil {
				return written, err
			}
		}
	}
	return written, nil
}

func (p *pagePNGData) flush() error {
	if p.n == 0 {
		return nil
	}
	if err := pagePNGChunk(p.w, "IDAT", p.buf[:p.n]); err != nil {
		return err
	}
	p.n = 0
	return nil
}
