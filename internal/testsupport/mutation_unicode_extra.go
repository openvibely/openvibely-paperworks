package testsupport

import (
	"archive/zip"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"unicode/utf8"
)

// Keep the raw central name on zip.File for local-header checks. Only dictionary
// keys use the Unicode Path extra field. Multiple valid fields apply in order.
// The caller has already validated outer extra-field lengths and filename UTF-8.
func mutationExtraName(f *zip.File, warn func(string) error) (string, error) {
	name := mutationZIPName(f)
	for extra := f.Extra; len(extra) >= 4; {
		kind, n := binary.LittleEndian.Uint16(extra), int(binary.LittleEndian.Uint16(extra[2:]))
		data := extra[4 : 4+n]
		extra = extra[4+n:]
		if kind != 0x7075 {
			continue
		}
		if len(data) < 5 {
			return "", fmt.Errorf("corrupt Unicode path extra field: %w", zip.ErrFormat)
		}
		if data[0] != 1 || binary.LittleEndian.Uint32(data[1:]) != crc32.ChecksumIEEE([]byte(f.Name)) {
			continue
		}
		if !utf8.Valid(data[5:]) {
			return "", fmt.Errorf("invalid Unicode path UTF-8: %w", zip.ErrFormat)
		}
		if len(data) > 5 {
			name = referenceZIPMemberName(string(data[5:]))
		} else if warn != nil {
			if err := warn("Empty unicode path extra field (0x7075)"); err != nil {
				return "", err
			}
		}
	}
	return name, nil
}
