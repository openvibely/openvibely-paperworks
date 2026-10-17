package xlsx

import (
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func legacyRecord(id uint16, data []byte) []byte {
	out := make([]byte, 4+len(data))
	binary.LittleEndian.PutUint16(out, id)
	binary.LittleEndian.PutUint16(out[2:], uint16(len(data)))
	copy(out[4:], data)
	return out
}
func legacyBOF(kind uint16) []byte {
	data := make([]byte, 16)
	biffLE.PutUint16(data, 0x0600)
	biffLE.PutUint16(data[2:], kind)
	return legacyRecord(0x0809, data)
}
func legacyTestStream(cells []byte) []byte {
	bound := []byte{0, 0, 0, 0, 0, 0, 1, 0, 'S'}
	global := legacyBOF(5)
	offset := len(global) + len(bound) + 4 + 4
	biffLE.PutUint32(bound, uint32(offset))
	global = append(global, legacyRecord(0x85, bound)...)
	global = append(global, legacyRecord(0xa, nil)...)
	global = append(global, legacyBOF(0x10)...)
	global = append(global, cells...)
	return append(global, legacyRecord(0xa, nil)...)
}
func TestLegacyValuesAndBounds(t *testing.T) {
	number := make([]byte, 14)
	biffLE.PutUint64(number[6:], math.Float64bits(42.5))
	boolean := []byte{1, 0, 0, 0, 0, 0, 1, 0}
	errcell := []byte{2, 0, 0, 0, 0, 0, 7, 1}
	// MULRK encodes -12 and 1.25 as integer RK values, second scaled by 100.
	multi := []byte{3, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 0}
	negative := int32(-12)
	biffLE.PutUint32(multi[6:], uint32(negative<<2)|2)
	biffLE.PutUint32(multi[12:], 125<<2|3)
	formula := make([]byte, 22)
	formula[0] = 4
	biffLE.PutUint64(formula[6:], math.Float64bits(123))
	formulaString := make([]byte, 22)
	formulaString[0] = 5
	formulaString[12] = 255
	formulaString[13] = 255
	cells := legacyRecord(0x203, number)
	for _, r := range []struct {
		id uint16
		d  []byte
	}{{0x205, boolean}, {0x205, errcell}, {0xbd, multi}, {6, formula}, {6, formulaString}, {0x207, []byte{3, 0, 0, 'a', 'b', 'c'}}} {
		cells = append(cells, legacyRecord(r.id, r.d)...)
	}
	book, err := readBIFF8(legacyTestStream(cells))
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		row, col int
		want     any
	}{{0, 0, 42.5}, {1, 0, true}, {2, 0, map[string]any{"error_code": 7}}, {3, 0, float64(-12)}, {3, 1, 1.25}, {4, 0, float64(123)}, {5, 0, "abc"}, {60000, 250, ""}} {
		got, err := book.CellValue(0, tc.row, tc.col)
		if err != nil || !reflect.DeepEqual(got, tc.want) {
			t.Fatalf("cell %d,%d=%v (%v), want %v", tc.row, tc.col, got, err, tc.want)
		}
	}
	for _, v := range [][3]int{{-1, 0, 0}, {1, 0, 0}, {0, -1, 0}, {0, 65536, 0}, {0, 0, 256}} {
		if _, err := book.CellValue(v[0], v[1], v[2]); err == nil {
			t.Fatal("accepted invalid coordinate", v)
		}
	}
}
func TestLegacyContinuedStrings(t *testing.T) {
	for _, tc := range []struct {
		name   string
		chunks [][]byte
		want   []string
	}{
		{"compression-switch", [][]byte{{4, 0, 0, 'A', 0xe9}, {1, 0x2d, 0x4e, 0x87, 0x65}}, []string{"Aé中文"}},
		{"between-strings-no-option-byte", [][]byte{{1, 0, 0, 'A'}, {1, 0, 0, 'B'}}, []string{"A", "B"}},
		{"rich-and-phonetic-continuation", [][]byte{{1, 0, 12, 1, 0, 3, 0, 0, 0, 'A', 0, 0}, {0, 0, 9, 9, 9, 1, 0, 0, 'B'}}, []string{"A", "B"}},
		{"surrogate-pair", [][]byte{{2, 0, 1, 0x3d, 0xd8}, {1, 0, 0xde}}, []string{"😀"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := biffStringCursor{chunks: tc.chunks}
			for _, want := range tc.want {
				got, err := c.text()
				if err != nil || got != want {
					t.Fatalf("got %q (%v), want %q", got, err, want)
				}
			}
		})
	}
}
func TestLegacyRejectsMalformedInput(t *testing.T) {
	for _, tc := range []struct {
		name string
		raw  []byte
	}{
		{"empty", nil}, {"truncated-header", []byte{9, 8, 16}},
		{"missing-eof", legacyBOF(5)},
		{"truncated-cell", legacyTestStream(legacyRecord(0x203, []byte{0}))},
		{"bad-sst-index", legacyTestStream(legacyRecord(0xfd, make([]byte, 10)))},
		{"missing-formula-string", legacyTestStream(legacyRecord(6, []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 255, 255, 0, 0, 0, 0, 0, 0, 0, 0}))},
		{"unsupported-rich-label", legacyTestStream(legacyRecord(0xd6, nil))},
		{"encrypted", append(append(legacyBOF(5), legacyRecord(0x2f, []byte{0, 0})...), legacyRecord(0xa, nil)...)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			book, err := readBIFF8(tc.raw)
			if err == nil || book != nil {
				t.Fatalf("wanted error without partial workbook: %v %v", book, err)
			}
		})
	}
	for _, chunks := range [][][]byte{{{2, 0, 0, 'A'}}, {{1, 0, 1, 0x00}}, {{1, 0, 1, 0x00, 0xd8}}, {{2, 0, 0, 'A'}, {2, 'B'}}} {
		c := biffStringCursor{chunks: chunks}
		if _, err := c.text(); err == nil {
			t.Fatal("accepted malformed string", chunks)
		}
	}
	path := filepath.Join(t.TempDir(), "invalid.xls")
	if err := os.WriteFile(path, []byte("not OLE"), 0600); err != nil {
		t.Fatal(err)
	}
	if book, err := ReadLegacy(path); err == nil || book != nil {
		t.Fatalf("bad compound document: %v %v", book, err)
	}
}
