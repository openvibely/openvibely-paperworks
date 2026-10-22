package office

// Keep the previous scan as a test-only oracle for populated extent semantics.
func legacyTraceExtent(t *tracer, key traceExtentKey) ([2]int, bool) {
	extent := [2]int{}
	for coord, c := range t.wb.Sheets[key.sheet].Cells {
		if t.ctx.Err() != nil {
			return [2]int{}, false
		}
		if Child(c, NSS, "f") == nil && Child(c, NSS, "v") == nil && Child(c, NSS, "is") == nil {
			continue
		}
		col, row, ok := traceCoord(coord)
		if !ok {
			continue
		}
		axis, value := row, col
		if key.columns {
			axis, value = col, row
		}
		if axis < key.first || axis > key.last {
			continue
		}
		if extent[0] == 0 || value < extent[0] {
			extent[0] = value
		}
		if value > extent[1] {
			extent[1] = value
		}
	}
	return extent, true
}
