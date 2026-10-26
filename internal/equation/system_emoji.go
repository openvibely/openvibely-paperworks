package equation

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"os"

	"github.com/luo-studio/go-tex/tex/path"
	"golang.org/x/image/font/sfnt"
)

// The reference's serif fallback on macOS selects Apple Color Emoji. Keep that
// font as a runtime input; neither its bitmap nor a captured mask is embedded.
// Other hosts can supply this same font explicitly. Missing fonts fail clearly.
func newSystemEmojiResolver(ctx context.Context) (func(rune) ([]path.Command, error), func()) {
	var file *os.File
	var size int64
	var face *sfnt.Font
	var strike int
	var initErr error
	initialized := false
	return func(cp rune) ([]path.Command, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if !initialized {
			initialized = true
			filename := os.Getenv("PAPERWORKS_EMOJI_FONT")
			if filename == "" {
				filename = "/System/Library/Fonts/Apple Color Emoji.ttc"
			}
			var info os.FileInfo
			file, initErr = os.Open(filename)
			if initErr == nil {
				info, initErr = file.Stat()
			}
			if initErr == nil && (info.Size() <= 0 || info.Size() > 256<<20) {
				initErr = fmt.Errorf("emoji font exceeds size limit")
			}
			if initErr == nil {
				size = info.Size()
			}
			if initErr == nil {
				var col *sfnt.Collection
				col, initErr = sfnt.ParseCollectionReaderAt(io.NewSectionReader(file, 0, size))
				if initErr == nil {
					face, initErr = col.Font(0)
				}
				if initErr == nil {
					var buffer sfnt.Buffer
					var name string
					name, initErr = face.Name(&buffer, sfnt.NameIDPostScript)
					if initErr == nil && name != "AppleColorEmoji" {
						initErr = fmt.Errorf("unsupported bitmap font profile %q", name)
					}
					if initErr == nil {
						strike, initErr = largestSystemStrikeReaderAt(file, size)
					}
				}
			}
		}
		if initErr != nil {
			return nil, fmt.Errorf("system Unicode U+%04X requires Apple Color Emoji (PAPERWORKS_EMOJI_FONT): %w", cp, initErr)
		}
		var buffer sfnt.Buffer
		glyph, err := face.GlyphIndex(&buffer, cp)
		if err != nil {
			return nil, err
		}
		if glyph == 0 {
			return nil, fmt.Errorf("system emoji font has no glyph for U+%04X", cp)
		}
		bitmap, err := loadSystemBitmapReaderAt(file, size, 0, int(glyph), strike)
		if err != nil {
			return nil, err
		}
		// MathJax's unknown SVG text uses 884 local units. Quartz's Apple emoji
		// profile places the square bitmap at 1.25em, with a .25em descent and a
		// half-strike-pixel x bearing, snapped to the nearest local pixel.
		const size = 884.0
		unit := size / float64(bitmap.PPEM)
		x := math.Floor((float64(bitmap.OriginX)+.5)*unit + .5)
		y := float64(bitmap.OriginY)*unit - size*.25
		mask, left, top, err := systemEmojiMask(bitmap, x, y, size*1.25, size*1.25)
		if err != nil {
			return nil, err
		}
		commands, err := systemMaskRuns(mask, left, top)
		if err != nil {
			return nil, err
		}
		for i := range commands {
			commands[i].X /= 1000
			commands[i].Y /= 1000
		}
		return commands, ctx.Err()
	}, func() {
		if file != nil {
			_ = file.Close()
		}
	}
}

// Select the largest available strike for the 884-unit SVG-local text path.
// Bounds are checked independently of the glyph decoder before indexing tables.
func largestSystemStrike(data []byte) (int, error) {
	return largestSystemStrikeReaderAt(bytes.NewReader(data), int64(len(data)))
}

func largestSystemStrikeReaderAt(reader io.ReaderAt, size int64) (int, error) {
	bad := fmt.Errorf("invalid emoji font strike directory")
	if size < 12 || size > 256<<20 {
		return 0, bad
	}
	data := fontRange{reader, 0, uint64(size)}
	tables, ok := readSFNTDirectory(data, 0)
	if !ok {
		return 0, bad
	}
	u32 := func(b []byte) uint64 { return uint64(binary.BigEndian.Uint32(b)) }
	if sb, exists := tables["sbix"]; exists {
		head, ok := sb.read(0, 8)
		if !ok {
			return 0, bad
		}
		count := u32(head[4:])
		if count == 0 || count > 65536 {
			return 0, bad
		}
		offsets, ok := sb.read(8, 4*count)
		if !ok {
			return 0, bad
		}
		best, maxPPEM := 0, uint16(0)
		for i := uint64(0); i < count; i++ {
			head, ok := sb.read(u32(offsets[4*i:]), 4)
			if !ok {
				return 0, bad
			}
			ppem := binary.BigEndian.Uint16(head)
			if ppem > maxPPEM {
				best, maxPPEM = int(i), ppem
			}
		}
		if maxPPEM == 0 {
			return 0, bad
		}
		return best, nil
	}
	return 0, bad
}
