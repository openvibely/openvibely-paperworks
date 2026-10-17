package pdf

import (
	"math"
	"strconv"
	"strings"
)

// reportNumber follows the pinned ReportLab fp_str precision policy for finite
// PDF drawing operands. Keep pdfNumber for APIs that promise full precision.
func reportNumber(v float64) string {
	a := math.Abs(v)
	if a <= 1e-7 {
		return "0"
	}
	digits := 6
	if a > 1 {
		digits = max(0, min(6, 6-int(math.Log10(a))))
	}
	s := strconv.FormatFloat(v, 'f', digits, 64)
	if digits > 0 {
		s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
	}
	if strings.HasPrefix(s, "0.") {
		s = s[1:]
	}
	return s
}
