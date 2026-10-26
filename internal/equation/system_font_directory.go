package equation

import "encoding/binary"

// readSFNTDirectory selects one SFNT face from a TTF or TTC and bounds every
// table record against the font. Callers keep their own table-specific checks.
func readSFNTDirectory(data fontRange, face int) (map[string]fontRange, bool) {
	if face < 0 {
		return nil, false
	}
	header, ok := data.read(0, 12)
	if !ok {
		return nil, false
	}
	base := uint64(0)
	if string(header[:4]) == "ttcf" {
		version := binary.BigEndian.Uint32(header[4:8])
		if version != 0x00010000 && version != 0x00020000 {
			return nil, false
		}
		faceCount := uint64(binary.BigEndian.Uint32(header[8:12]))
		if faceCount == 0 || faceCount > 65536 || uint64(face) >= faceCount {
			return nil, false
		}
		collectionHeaderSize := uint64(12) + faceCount*4
		if version == 0x00020000 {
			collectionHeaderSize += 12
		}
		if _, ok := data.section(0, collectionHeaderSize); !ok {
			return nil, false
		}
		faceOffsets, ok := data.read(12+uint64(face)*4, 4)
		if !ok {
			return nil, false
		}
		base = uint64(binary.BigEndian.Uint32(faceOffsets))
		if base < collectionHeaderSize {
			return nil, false
		}
	} else if face != 0 {
		return nil, false
	}

	header, ok = data.read(base, 12)
	if !ok {
		return nil, false
	}
	tableCount := uint64(binary.BigEndian.Uint16(header[4:6]))
	records, ok := data.read(base+12, tableCount*16)
	if !ok {
		return nil, false
	}
	tables := make(map[string]fontRange, tableCount)
	for i := uint64(0); i < tableCount; i++ {
		record := records[i*16 : (i+1)*16]
		tag := string(record[:4])
		if _, exists := tables[tag]; exists {
			return nil, false
		}
		table, ok := data.section(uint64(binary.BigEndian.Uint32(record[8:12])), uint64(binary.BigEndian.Uint32(record[12:16])))
		if !ok {
			return nil, false
		}
		tables[tag] = table
	}
	return tables, true
}
