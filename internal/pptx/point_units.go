package pptx

import "math"

const emuPerPoint = 12700

type pointRounding uint8

const (
	truncatePointEMU pointRounding = iota
	roundPointEMU
)

// pointsToEMU converts points to EMUs, truncating fractional EMUs unless
// roundPointEMU is requested, which rounds to the nearest integer (half away
// from zero).
func pointsToEMU(points float64, rounding pointRounding) int64 {
	value := points * emuPerPoint
	if rounding == roundPointEMU {
		value = math.Round(value)
	}
	return int64(value)
}
