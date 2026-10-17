package testsupport

import (
	"fmt"
	"strconv"
	"strings"
)

// ZipFile formats warning names using Reference's str repr, not Go's quoted string.
func mutationNameRepr(s string) string {
	quote := byte('\'')
	if strings.Contains(s, "'") && !strings.Contains(s, `"`) {
		quote = '"'
	}
	var b strings.Builder
	b.WriteByte(quote)
	for _, r := range s {
		switch {
		case r == rune(quote) || r == '\\':
			b.WriteByte('\\')
			b.WriteRune(r)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\r':
			b.WriteString(`\r`)
		case r == '\t':
			b.WriteString(`\t`)
		case strconv.IsPrint(r):
			b.WriteRune(r)
		case r <= 255:
			fmt.Fprintf(&b, `\x%02x`, r)
		case r <= 65535:
			fmt.Fprintf(&b, `\u%04x`, r)
		default:
			fmt.Fprintf(&b, `\U%08x`, r)
		}
	}
	b.WriteByte(quote)
	return b.String()
}
