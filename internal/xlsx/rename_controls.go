package xlsx

import (
	"bytes"
	"encoding/xml"
	"io"
	"regexp"
	"strings"

	"github.com/beevik/etree"
)

// Match formula elements without parsing the surrounding legacy VML, which may
// contain malformed XML. Opening tags may carry attributes (including local
// namespace declarations), and formula text may mix entity escapes, CDATA,
// comments, and processing instructions.
// Standalone comments, CDATA, and processing instructions are consumed first
// so embedded example elements are not rewritten. Only the matched element's content is replaced.
var vmlControlFormula = regexp.MustCompile(`(?s)<!--.*?-->|<!\[CDATA\[.*?\]\]>|<\?.*?\?>|<((?:[\pL_][\pL\pN_.-]*:)?Fmla(?:Link|Range|Txbx))(?:\s+(?:[^"'<>]|"[^"]*"|'[^']*')*)?>((?:[^<]|<!\[CDATA\[.*?\]\]>|<!--.*?-->|<\?.*?\?>)*)</((?:[\pL_][\pL\pN_.-]*:)?Fmla(?:Link|Range|Txbx))\s*>`)

func renameVMLControlFormulas(data []byte, source, target string) []byte {
	return vmlControlFormula.ReplaceAllFunc(data, func(element []byte) []byte {
		m := vmlControlFormula.FindSubmatchIndex(element)
		if m[2] < 0 || !bytes.Equal(element[m[2]:m[3]], element[m[6]:m[7]]) {
			return element
		}
		text := element[m[4]:m[5]]
		next := renameVMLFormulaContent(text, source, target)
		if bytes.Equal(next, text) {
			return element
		}
		var out bytes.Buffer
		out.Write(element[:m[4]])
		out.Write(next)
		out.Write(element[m[5]:])
		return out.Bytes()
	})
}

// Decode just the formula's content, retaining the original byte ranges of text
// nodes. Formula parsing sees concatenated character data, while XML comments
// and processing instructions never become part of the formula or get rewritten.
func renameVMLFormulaContent(content []byte, source, target string) []byte {
	type textChunk struct {
		start, end, formulaStart int
		text                     string
	}
	var chunks []textChunk
	var formula strings.Builder
	wrapped := append(append([]byte("<f>"), content...), []byte("</f>")...)
	decoder := xml.NewDecoder(bytes.NewReader(wrapped))
	depth := 0
	for {
		start := int(decoder.InputOffset()) - len("<f>")
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return content
		}
		switch token := token.(type) {
		case xml.StartElement:
			depth++
			if depth != 1 { // Formula elements have simple content, not child elements.
				return content
			}
		case xml.EndElement:
			depth--
		case xml.CharData:
			chunks = append(chunks, textChunk{start, int(decoder.InputOffset()) - len("<f>"), formula.Len(), string(token)})
			formula.Write(token)
		case xml.Directive:
			return content
		}
	}
	type edit struct {
		start, end int
		text       string
	}
	var edits []edit
	renameFormulaSheetWithEdits(formula.String(), source, target, func(start, end int, replacement string) {
		edits = append(edits, edit{start, end, replacement})
	})
	if len(edits) == 0 {
		return content
	}
	var out bytes.Buffer
	cursor, firstEdit := 0, 0
	for _, chunk := range chunks {
		chunkEnd := chunk.formulaStart + len(chunk.text)
		for firstEdit < len(edits) && edits[firstEdit].end <= chunk.formulaStart {
			firstEdit++
		}
		var next strings.Builder
		pos := 0
		for i := firstEdit; i < len(edits) && edits[i].start < chunkEnd; i++ {
			e := edits[i]
			start := max(e.start-chunk.formulaStart, 0)
			end := min(e.end-chunk.formulaStart, len(chunk.text))
			next.WriteString(chunk.text[pos:start])
			// A qualifier split by comments/CDATA is replaced at its first
			// character. Later fragments are removed, leaving comments intact.
			if e.start >= chunk.formulaStart {
				next.WriteString(e.text)
			}
			pos = end
		}
		next.WriteString(chunk.text[pos:])
		if next.String() != chunk.text {
			out.Write(content[cursor:chunk.start])
			_ = xml.EscapeText(&out, []byte(next.String()))
			cursor = chunk.end
		}
	}
	out.Write(content[cursor:])
	return out.Bytes()
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
