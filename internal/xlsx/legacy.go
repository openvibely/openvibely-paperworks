package xlsx

// BIFF8 values-only reading. Formatting, formula evaluation and earlier BIFF
// versions are intentionally outside this API. Dates retain their Excel serials.
import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"os"
	"unicode/utf16"

	"github.com/richardlehane/mscfb"
)

const legacyLimit = 256 << 20

// LegacyBook contains every worksheet, in workbook order, with sparse cells.
type LegacyBook struct {
	Sheets []LegacySheet `json:"sheets"`
}
type LegacySheet struct {
	Name  string         `json:"name"`
	Cells map[string]any `json:"cells"`
}

// CellValue uses zero-based coordinates, like xlrd.cell_value. Blank cells
// return an empty string; invalid sheet/BIFF8 coordinates return an error.
func (b *LegacyBook) CellValue(sheet, row, col int) (any, error) {
	if sheet < 0 || sheet >= len(b.Sheets) || row < 0 || row >= 65536 || col < 0 || col >= 256 {
		return nil, fmt.Errorf("XLS cell outside worksheet bounds")
	}
	if v, ok := b.Sheets[sheet].Cells[legacyAddress(row, col)]; ok {
		return v, nil
	}
	return "", nil
}

func legacyAddress(row, col int) string {
	s := ""
	for col++; col > 0; col = (col - 1) / 26 {
		s = string(rune('A'+(col-1)%26)) + s
	}
	return fmt.Sprintf("%s%d", s, row+1)
}

// ReadLegacy reads a BIFF8 OLE workbook without conversion or preview limits.
// Errors never return a partial workbook. The compound stream is size bounded.
func ReadLegacy(path string) (*LegacyBook, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if info.Size() > legacyLimit {
		return nil, fmt.Errorf("XLS exceeds %d bytes", legacyLimit)
	}
	compound, err := mscfb.New(f)
	if err != nil {
		return nil, err
	}
	var raw []byte
	for entry, err := compound.Next(); ; entry, err = compound.Next() {
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if entry.Name != "Workbook" && entry.Name != "Book" {
			continue
		}
		if raw != nil {
			return nil, fmt.Errorf("ambiguous XLS workbook streams")
		}
		raw, err = io.ReadAll(io.LimitReader(compound, legacyLimit+1))
		if err != nil {
			return nil, err
		}
		if len(raw) > legacyLimit {
			return nil, fmt.Errorf("XLS workbook stream exceeds limit")
		}
	}
	if raw == nil {
		return nil, fmt.Errorf("XLS has no Workbook stream")
	}
	return readBIFF8(raw)
}

type biffRecord struct {
	id     uint16
	data   []byte
	offset int
}

var biffLE = binary.LittleEndian

func biffRecords(raw []byte, offset int) ([]biffRecord, error) {
	var records []biffRecord
	for offset >= 0 && offset+4 <= len(raw) {
		id, n := biffLE.Uint16(raw[offset:]), int(biffLE.Uint16(raw[offset+2:]))
		if n > 8224 || n > len(raw)-offset-4 {
			return nil, fmt.Errorf("invalid BIFF record at %d", offset)
		}
		records = append(records, biffRecord{id, raw[offset+4 : offset+4+n], offset})
		offset += 4 + n
		if id == 0x000a {
			if n != 0 {
				return nil, fmt.Errorf("invalid BIFF EOF")
			}
			return records, nil
		}
	}
	return nil, fmt.Errorf("truncated BIFF substream (missing EOF)")
}
func biffBOF(records []biffRecord, kind uint16) error {
	if len(records) == 0 || records[0].id != 0x0809 || len(records[0].data) < 16 || biffLE.Uint16(records[0].data) != 0x0600 || biffLE.Uint16(records[0].data[2:]) != kind {
		return fmt.Errorf("expected BIFF8 substream type %04x", kind)
	}
	return nil
}

func readBIFF8(raw []byte) (*LegacyBook, error) {
	records, err := biffRecords(raw, 0)
	if err != nil {
		return nil, err
	}
	if err = biffBOF(records, 5); err != nil {
		return nil, err
	}
	type sheetRef struct {
		name   string
		offset int
	}
	var refs []sheetRef
	var strings []string
	seenSST := false
	for i := 1; i < len(records); i++ {
		r := records[i]
		switch r.id {
		case 0x002f:
			return nil, fmt.Errorf("encrypted XLS is unsupported")
		case 0x0085:
			if len(r.data) < 8 {
				return nil, fmt.Errorf("truncated BOUNDSHEET")
			}
			if r.data[5] != 0 {
				return nil, fmt.Errorf("XLS non-worksheet substream is unsupported")
			}
			n := int(r.data[6])
			flags := r.data[7]
			width := 1
			if flags&1 != 0 {
				width = 2
			}
			if flags > 1 || n == 0 || len(r.data) != 8+n*width {
				return nil, fmt.Errorf("invalid BOUNDSHEET name")
			}
			name, err := biffText(r.data[8:], width)
			if err != nil {
				return nil, err
			}
			refs = append(refs, sheetRef{name, int(biffLE.Uint32(r.data))})
		case 0x00fc:
			if seenSST || len(r.data) < 8 {
				return nil, fmt.Errorf("invalid SST")
			}
			seenSST = true
			count := int(biffLE.Uint32(r.data[4:]))
			if count > 1000000 || uint32(count) > biffLE.Uint32(r.data) {
				return nil, fmt.Errorf("invalid SST count")
			}
			chunks := [][]byte{r.data[8:]}
			for i+1 < len(records) && records[i+1].id == 0x003c {
				i++
				chunks = append(chunks, records[i].data)
			}
			cursor := biffStringCursor{chunks: chunks}
			for j := 0; j < count; j++ {
				value, err := cursor.text()
				if err != nil {
					return nil, fmt.Errorf("SST string %d: %w", j, err)
				}
				strings = append(strings, value)
			}
		}
	}
	book := &LegacyBook{Sheets: []LegacySheet{}}
	names, offsets := map[string]bool{}, map[int]bool{}
	globalEnd := records[len(records)-1].offset + 4
	for _, ref := range refs {
		if names[ref.name] || offsets[ref.offset] || ref.offset < globalEnd {
			return nil, fmt.Errorf("duplicate or invalid XLS worksheet reference")
		}
		names[ref.name] = true
		offsets[ref.offset] = true
		rows, err := biffRecords(raw, ref.offset)
		if err != nil {
			return nil, err
		}
		if err = biffBOF(rows, 0x10); err != nil {
			return nil, err
		}
		sheet := LegacySheet{Name: ref.name, Cells: map[string]any{}}
		if err = sheet.readCells(rows, strings); err != nil {
			return nil, fmt.Errorf("worksheet %q: %w", ref.name, err)
		}
		book.Sheets = append(book.Sheets, sheet)
	}
	return book, nil
}

// A CONTINUE record starts with an encoding byte only when continuing the
// character array, not between strings or in rich-text/phonetic payloads.
type biffStringCursor struct {
	chunks     [][]byte
	chunk, pos int
}

func (c *biffStringCursor) take(n int) ([]byte, error) {
	var out []byte
	for n > 0 {
		for c.chunk < len(c.chunks) && c.pos == len(c.chunks[c.chunk]) {
			c.chunk++
			c.pos = 0
		}
		if c.chunk >= len(c.chunks) {
			return nil, io.ErrUnexpectedEOF
		}
		k := min(n, len(c.chunks[c.chunk])-c.pos)
		out = append(out, c.chunks[c.chunk][c.pos:c.pos+k]...)
		c.pos += k
		n -= k
	}
	return out, nil
}
func (c *biffStringCursor) text() (string, error) {
	header, err := c.take(3)
	if err != nil {
		return "", err
	}
	count := int(biffLE.Uint16(header))
	flags := header[2]
	if flags & ^byte(0x0d) != 0 {
		return "", fmt.Errorf("invalid BIFF string flags")
	}
	rich, phonetic := 0, 0
	if flags&8 != 0 {
		b, e := c.take(2)
		if e != nil {
			return "", e
		}
		rich = int(biffLE.Uint16(b)) * 4
	}
	if flags&4 != 0 {
		b, e := c.take(4)
		if e != nil {
			return "", e
		}
		phonetic = int(biffLE.Uint32(b))
		if phonetic > legacyLimit {
			return "", fmt.Errorf("oversized phonetic string")
		}
	}
	units := make([]uint16, 0, count)
	for len(units) < count {
		if c.chunk >= len(c.chunks) {
			return "", io.ErrUnexpectedEOF
		}
		if c.pos == len(c.chunks[c.chunk]) {
			c.chunk++
			c.pos = 0
			if c.chunk >= len(c.chunks) || len(c.chunks[c.chunk]) == 0 {
				return "", io.ErrUnexpectedEOF
			}
			flags = c.chunks[c.chunk][0]
			c.pos = 1
			if flags > 1 {
				return "", fmt.Errorf("invalid CONTINUE encoding")
			}
		}
		width := 1
		if flags&1 != 0 {
			width = 2
		}
		if len(c.chunks[c.chunk])-c.pos < width {
			return "", fmt.Errorf("split UTF-16 code unit")
		}
		b := c.chunks[c.chunk][c.pos : c.pos+width]
		c.pos += width
		u := uint16(b[0])
		if width == 2 {
			u = biffLE.Uint16(b)
		}
		units = append(units, u)
	}
	if _, err = c.take(rich + phonetic); err != nil {
		return "", err
	}
	return biffUnits(units)
}
func biffUnits(units []uint16) (string, error) {
	for i := 0; i < len(units); i++ {
		u := units[i]
		if u >= 0xd800 && u <= 0xdbff {
			if i+1 == len(units) || units[i+1] < 0xdc00 || units[i+1] > 0xdfff {
				return "", fmt.Errorf("invalid UTF-16 surrogate")
			}
			i++
		} else if u >= 0xdc00 && u <= 0xdfff {
			return "", fmt.Errorf("invalid UTF-16 surrogate")
		}
	}
	return string(utf16.Decode(units)), nil
}
func biffText(raw []byte, width int) (string, error) {
	units := make([]uint16, len(raw)/width)
	for i := range units {
		units[i] = uint16(raw[i*width])
		if width == 2 {
			units[i] = biffLE.Uint16(raw[i*width:])
		}
	}
	return biffUnits(units)
}
func biffRK(b []byte) float64 {
	bits := biffLE.Uint32(b)
	var value float64
	if bits&2 != 0 {
		value = float64(int32(bits) >> 2)
	} else {
		value = math.Float64frombits(uint64(bits&^3) << 32)
	}
	if bits&1 != 0 {
		value /= 100
	}
	return value
}
func (s *LegacySheet) readCells(records []biffRecord, shared []string) error {
	put := func(data []byte, value any) error {
		row, col := int(biffLE.Uint16(data)), int(biffLE.Uint16(data[2:]))
		if col >= 256 {
			return fmt.Errorf("invalid BIFF column")
		}
		key := legacyAddress(row, col)
		if _, ok := s.Cells[key]; ok {
			return fmt.Errorf("duplicate cell %s", key)
		}
		if n, ok := value.(float64); ok && (math.IsNaN(n) || math.IsInf(n, 0)) {
			return fmt.Errorf("nonfinite cell %s", key)
		}
		s.Cells[key] = value
		return nil
	}
	for i := 1; i < len(records); i++ {
		r := records[i]
		d := r.data
		required := map[uint16]int{0x0203: 14, 0x00fd: 10, 0x027e: 10, 0x00bd: 12, 0x0205: 8, 0x0204: 9, 0x0006: 22}
		if n, ok := required[r.id]; ok && len(d) < n {
			return fmt.Errorf("truncated cell record %04x", r.id)
		}
		var value any
		switch r.id {
		case 0x0203:
			value = math.Float64frombits(biffLE.Uint64(d[6:]))
		case 0x00fd:
			idx := int(biffLE.Uint32(d[6:]))
			if idx >= len(shared) {
				return fmt.Errorf("SST index out of range")
			}
			value = shared[idx]
		case 0x027e:
			value = biffRK(d[6:])
		case 0x00bd:
			first, last := int(biffLE.Uint16(d[2:])), int(biffLE.Uint16(d[len(d)-2:]))
			if last < first || last >= 256 || len(d) != 6+6*(last-first+1) {
				return fmt.Errorf("invalid MULRK")
			}
			for col := first; col <= last; col++ {
				head := bytes.Clone(d[:4])
				biffLE.PutUint16(head[2:], uint16(col))
				if err := put(head, biffRK(d[6+6*(col-first):])); err != nil {
					return err
				}
			}
			continue
		case 0x0205:
			if d[7] > 1 {
				return fmt.Errorf("invalid BOOLERR")
			}
			if d[7] == 1 {
				value = map[string]any{"error_code": int(d[6])}
			} else {
				if d[6] > 1 {
					return fmt.Errorf("invalid boolean")
				}
				value = d[6] != 0
			}
		case 0x0204:
			cursor := biffStringCursor{chunks: [][]byte{d[6:]}}
			v, err := cursor.text()
			if err != nil {
				return err
			}
			value = v
		case 0x0006:
			if int(biffLE.Uint16(d[20:])) > len(d)-22 {
				return fmt.Errorf("truncated formula tokens")
			}
			if d[12] != 255 || d[13] != 255 {
				value = math.Float64frombits(biffLE.Uint64(d[6:]))
			} else {
				switch d[6] {
				case 0:
					// Shared/array formula definitions may precede the cached STRING.
					for i+1 < len(records) && (records[i+1].id == 0x04bc || records[i+1].id == 0x0221 || records[i+1].id == 0x0236) {
						i++
					}
					i++
					if i >= len(records) || records[i].id != 0x0207 {
						return fmt.Errorf("formula has no cached STRING")
					}
					chunks := [][]byte{records[i].data}
					for i+1 < len(records) && records[i+1].id == 0x003c {
						i++
						chunks = append(chunks, records[i].data)
					}
					cursor := biffStringCursor{chunks: chunks}
					v, err := cursor.text()
					if err != nil {
						return err
					}
					value = v
				case 1:
					if d[8] > 1 {
						return fmt.Errorf("invalid formula boolean")
					}
					value = d[8] != 0
				case 2:
					value = map[string]any{"error_code": int(d[8])}
				case 3:
					value = ""
				default:
					return fmt.Errorf("invalid formula cached type")
				}
			}
		case 0x002f:
			return fmt.Errorf("encrypted XLS is unsupported")
		case 0x00d6, 0x0207, 0x0002, 0x0003, 0x0004, 0x0206, 0x0406:
			return fmt.Errorf("unsupported cell record %04x", r.id)
		case 0x0809:
			return fmt.Errorf("nested BIFF substream is unsupported")
		default:
			continue // Formatting, dimensions, row indexes, and other non-value records.
		}
		if err := put(d, value); err != nil {
			return err
		}
	}
	return nil
}
