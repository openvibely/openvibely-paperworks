package xmltext

import (
	"bytes"
	"encoding/xml"
	"testing"
)

func TestEscapeMatchesEncodingXML(t *testing.T) {
	input := "A & B < C > \"quoted\" 'apostrophe' café\tline\nnext\rend\x01\uFFFE\xff"
	var want bytes.Buffer
	if err := xml.EscapeText(&want, []byte(input)); err != nil {
		t.Fatal(err)
	}
	got := Escape(input, Options{EscapeWhitespace: true, ReplaceInvalidXML: true})
	if got != want.String() {
		t.Fatalf("Escape() = %q, encoding/xml = %q", got, want.String())
	}
}

func TestEscapeSupportsCallerQuoteAndWhitespacePolicy(t *testing.T) {
	input := "A & B < C > \"quoted\" 'apostrophe' café\tline\nnext\rend"
	want := "A &amp; B &lt; C &gt; &quot;quoted&quot; 'apostrophe' café\tline\nnext\rend"
	got := Escape(input, Options{Quotes: NamedDoubleQuote})
	if got != want {
		t.Fatalf("Escape() = %q, want %q", got, want)
	}
}
