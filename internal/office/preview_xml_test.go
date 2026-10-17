package office

import (
	"bytes"
	"encoding/xml"
	"io"
	"testing"

	"github.com/beevik/etree"
)

// Normalize literal attribute whitespace before entity decoding, as libxml2's
// xmlParseAttValueInternal does. Applying this to decoded values would corrupt
// numeric references (&#9;, &#10;, &#13;). RawToken offsets isolate start tags
// from comments, CDATA, processing instructions and document declarations.
// This addresses UTF-8 OOXML input; DTD defaults/entities and other encodings
// still require separate correspondence review.
func referencePreviewReadXML(doc *etree.Document, raw []byte) error {
	decoder := xml.NewDecoder(bytes.NewReader(raw))
	normalized := append([]byte(nil), raw...)
	var removed []int
	for {
		start := int(decoder.InputOffset())
		token, err := decoder.RawToken()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if _, ok := token.(xml.StartElement); !ok {
			continue
		}
		end := int(decoder.InputOffset())
		var quote byte
		for i := start; i < end; i++ {
			c := raw[i]
			if quote == 0 {
				if c == '\'' || c == '"' {
					quote = c
				}
				continue
			}
			if c == quote {
				quote = 0
				continue
			}
			if c == '\t' || c == '\n' || c == '\r' {
				normalized[i] = ' '
				// XML end-of-line processing treats CRLF as one character. Remove the
				// CR here; the following LF supplies its single normalized space.
				if c == '\r' && i+1 < end && raw[i+1] == '\n' {
					removed = append(removed, i)
				}
			}
		}
	}
	// Remove only the CR positions recorded above, preserving all other bytes.
	for i := len(removed) - 1; i >= 0; i-- {
		pos := removed[i]
		normalized = append(normalized[:pos], normalized[pos+1:]...)
	}
	return doc.ReadFromBytes(normalized)
}

func TestReferencePreviewParserAttributeNormalization(t *testing.T) {
	for _, c := range []struct{ name, input, want string }{
		{"literal", "<root a=\"x\t y\r\nz\">a\r\nb\rc</root>", "<root a=\"x  y z\">a\nb\nc</root>"},
		{"references", `<root a="&#9;&#10;&#13;">&#13;é😀</root>`, `<root a="&#9;&#10;&#13;">&#13;&#233;&#128512;</root>`},
		{"single-quote", "<root a='a\rb\nc\td'/>", `<root a="a b c d"/>`},
		{"tokens", "<root a='x\ty'><!--a\tb--><?pi a\tb?><![CDATA[a\tb]]></root>", "<root a=\"x y\"><!--a\tb--><?pi a\tb?>a\tb</root>"},
	} {
		t.Run(c.name, func(t *testing.T) {
			raw := []byte(c.input)
			before := append([]byte(nil), raw...)
			d := etree.NewDocument()
			if err := referencePreviewReadXML(d, raw); err != nil {
				t.Fatal(err)
			}
			got, err := referencePreviewSerializeElement(d.Root())
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != c.want {
				t.Fatalf("got %q want %q", got, c.want)
			}
			if !bytes.Equal(raw, before) {
				t.Fatal("parser changed source bytes")
			}
		})
	}
}
