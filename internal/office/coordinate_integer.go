package office

import (
	"fmt"
	"math"
	"math/big"
	"strings"
	"unicode"
)

// coordinateInteger follows Reference int(string, 10), rather than accepting
// floating-point OOXML coordinates or silently replacing parse failures with zero.
func coordinateInteger(raw string) (float64, error) {
	bad := func() (float64, error) { return 0, fmt.Errorf("invalid integer coordinate %q", raw) }
	s := strings.TrimFunc(raw, func(r rune) bool { return unicode.IsSpace(r) && !(r >= 0x1c && r <= 0x1f) })
	var b strings.Builder
	digit, separator := false, false
	for i, r := range s {
		if i == 0 && (r == '+' || r == '-') {
			b.WriteRune(r)
			continue
		}
		if r == '_' {
			if !digit || separator {
				return bad()
			}
			separator = true
			continue
		}
		value := -1
		for _, q := range unicode.Nd.R16 {
			if r >= rune(q.Lo) && r <= rune(q.Hi) && (r-rune(q.Lo))%rune(q.Stride) == 0 {
				value = int((r-rune(q.Lo))/rune(q.Stride)) % 10
				break
			}
		}
		if value < 0 {
			for _, q := range unicode.Nd.R32 {
				if uint32(r) >= q.Lo && uint32(r) <= q.Hi && (uint32(r)-q.Lo)%q.Stride == 0 {
					value = int((uint32(r)-q.Lo)/q.Stride) % 10
					break
				}
			}
		}
		if value < 0 {
			return bad()
		}
		b.WriteByte(byte('0' + value))
		digit = true
		separator = false
	}
	if !digit || separator {
		return bad()
	}
	n, ok := new(big.Int).SetString(b.String(), 10)
	if !ok {
		return bad()
	}
	f, _ := new(big.Float).SetInt(n).Float64()
	if math.IsInf(f, 0) {
		return bad()
	}
	return f, nil
}
