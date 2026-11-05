// Package xmltext provides shared escaping for XML text and attribute values.
package xmltext

import (
	"strings"
	"unicode/utf8"
)

// QuoteStyle controls how quote characters are escaped.
type QuoteStyle uint8

const (
	// NumericQuotes escapes double and single quotes as numeric character references.
	// This matches encoding/xml.EscapeText.
	NumericQuotes QuoteStyle = iota
	// NamedDoubleQuote escapes double quotes as &quot; and leaves apostrophes literal.
	NamedDoubleQuote
)

// Options selects formatting details that callers may need to preserve.
type Options struct {
	Quotes            QuoteStyle
	EscapeWhitespace  bool
	ReplaceInvalidXML bool
}

// Escape returns s escaped for use as XML character data or a double-quoted
// attribute value. Control-character filtering is a caller policy and is not
// performed here.
func Escape(s string, options Options) string {
	var out strings.Builder
	out.Grow(len(s))
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		i += size
		switch r {
		case '&':
			out.WriteString("&amp;")
		case '<':
			out.WriteString("&lt;")
		case '>':
			out.WriteString("&gt;")
		case '"':
			if options.Quotes == NamedDoubleQuote {
				out.WriteString("&quot;")
			} else {
				out.WriteString("&#34;")
			}
		case '\'':
			if options.Quotes == NamedDoubleQuote {
				out.WriteRune(r)
			} else {
				out.WriteString("&#39;")
			}
		case '\t':
			if options.EscapeWhitespace {
				out.WriteString("&#x9;")
			} else {
				out.WriteRune(r)
			}
		case '\n':
			if options.EscapeWhitespace {
				out.WriteString("&#xA;")
			} else {
				out.WriteRune(r)
			}
		case '\r':
			if options.EscapeWhitespace {
				out.WriteString("&#xD;")
			} else {
				out.WriteRune(r)
			}
		default:
			if (r == utf8.RuneError && size == 1) || (options.ReplaceInvalidXML && !inXMLCharacterRange(r)) {
				out.WriteRune(utf8.RuneError)
			} else {
				out.WriteString(s[i-size : i])
			}
		}
	}
	return out.String()
}

func inXMLCharacterRange(r rune) bool {
	return r == 0x9 || r == 0xA || r == 0xD ||
		(r >= 0x20 && r <= 0xD7FF) ||
		(r >= 0xE000 && r <= 0xFFFD) ||
		(r >= 0x10000 && r <= 0x10FFFF)
}
