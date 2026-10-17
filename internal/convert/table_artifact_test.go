package convert

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"path/filepath"
	"testing"
)

// This digest comes from unchanged pinned document_check.exchange_table on the
// original library-exchange.xlsx, not from the Go serializer. It covers table
// defaults, header bold/repeat, text escaping, dates, alignment and line breaks.
func TestTableExchangePinnedDocumentMember(t *testing.T) {
	out := filepath.Join(t.TempDir(), "table.docx")
	if _, err := TableExchange("../../testdata/reference-mirror/library-exchange.xlsx", out, "Evidence", "A1:D2", 0); err != nil {
		t.Fatal(err)
	}
	p, err := opc.Open(out)
	if err != nil {
		t.Fatal(err)
	}
	got := fmt.Sprintf("%x", sha256.Sum256(p.Get("word/document.xml")))
	const want = "f32fac7b538c27658359b1b44b996233340288d57b4830419240561577ba49ad"
	if got != want {
		t.Fatalf("pinned document member mismatch: %s", got)
	}
}

// Literal outcomes from pinned Reference csv.writer, including the cases where
// encoding/csv's default quoting policy differs.
func TestTableExchangeCSVDialect(t *testing.T) {
	for _, tc := range []struct {
		fields []string
		want   string
	}{
		{[]string{" Header ", " x ", "space\u00a0", `\.`}, " Header , x ,space\u00a0,\\.\r\n"},
		{[]string{""}, "\"\"\r\n"},
		{nil, "\r\n"},
		{[]string{"", ""}, ",\r\n"},
		{[]string{"quote \"", "a,b", "a\rb", "a\nb", "\ttab"}, "\"quote \"\"\",\"a,b\",\"a\rb\",\"a\nb\",\ttab\r\n"},
	} {
		var out bytes.Buffer
		writeExchangeCSVRecord(&out, tc.fields)
		if out.String() != tc.want {
			t.Fatalf("%q: got %q want %q", tc.fields, out.String(), tc.want)
		}
	}
}
