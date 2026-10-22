package xlsx

import (
	"bytes"
	"encoding/xml"
	"regexp"

	"github.com/beevik/etree"
)

// Match formula elements without parsing the surrounding legacy VML, which may
// contain malformed XML. Opening tags may carry attributes (including local
// namespace declarations), and formula text may mix entity escapes and CDATA.
// Standalone comments and CDATA are consumed first so embedded example elements
// are not rewritten. Only the matched element's content is replaced.
var vmlControlFormula = regexp.MustCompile(`(?s)<!--.*?-->|<!\[CDATA\[.*?\]\]>|<((?:[\pL_][\pL\pN_.-]*:)?Fmla(?:Link|Range|Txbx))(?:\s+(?:[^"'<>]|"[^"]*"|'[^']*')*)?>((?:[^<]|<!\[CDATA\[.*?\]\]>)*)</((?:[\pL_][\pL\pN_.-]*:)?Fmla(?:Link|Range|Txbx))\s*>`)

func renameVMLControlFormulas(data []byte, source, target string) []byte {
	return vmlControlFormula.ReplaceAllFunc(data, func(element []byte) []byte {
		m := vmlControlFormula.FindSubmatchIndex(element)
		if m[2] < 0 || !bytes.Equal(element[m[2]:m[3]], element[m[6]:m[7]]) {
			return element
		}
		// Decode XML entities only in the text being edited, without parsing or
		// reserializing the surrounding legacy drawing.
		var formula string
		text := element[m[4]:m[5]]
		wrapped := append(append([]byte("<f>"), text...), []byte("</f>")...)
		if err := xml.Unmarshal(wrapped, &formula); err != nil {
			return element
		}
		next := renameFormulaSheet(formula, source, target)
		if next == formula {
			return element
		}
		var out bytes.Buffer
		out.Write(element[:m[4]])
		_ = xml.EscapeText(&out, []byte(next))
		out.Write(element[m[5]:])
		return out.Bytes()
	})
}

func renameControlAttributes(e *etree.Element, source, target string) bool {
	changed := false
	for i := range e.Attr {
		a := &e.Attr[i]
		switch a.Key {
		case "fmlaLink", "fmlaRange", "fmlaTxbx":
			if next := renameFormulaSheet(a.Value, source, target); next != a.Value {
				a.Value = next
				changed = true
			}
		}
	}
	return changed
}
