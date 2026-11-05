package docx

import (
	"bytes"
	"testing"

	"github.com/openvibely/openvibely-paperworks/internal/opc"
)

func TestRunXMLNormalizesNewlineSequences(t *testing.T) {
	for _, tc := range []struct {
		name    string
		newline string
	}{
		{name: "CRLF", newline: "\r\n"},
		{name: "LF", newline: "\n"},
		{name: "CR", newline: "\r"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := runXML(Run{Text: "A" + tc.newline + "B"})
			want := `<w:r><w:t>A</w:t><w:br/><w:t>B</w:t></w:r>`
			if got != want {
				t.Fatalf("runXML() = %q, want %q", got, want)
			}
		})
	}
}

func TestBuildNormalizesNewlinesAcrossDocumentTextPaths(t *testing.T) {
	spec := Spec{
		Header: "A\r\nB",
		Footer: "A\r\nB",
		Blocks: []Block{
			{Type: "paragraph", Text: "A\r\nB"},
			{Type: "paragraph", Runs: []Run{{Text: "A\r\nB"}}},
			{Type: "table", Rows: [][]string{{"A\r\nB"}}},
			{Type: "list", Items: []string{"A\r\nB"}},
		},
	}

	raw, err := Build(spec, "")
	if err != nil {
		t.Fatal(err)
	}
	pkg, err := opc.Read(raw)
	if err != nil {
		t.Fatal(err)
	}

	want := []byte(`<w:t>A</w:t><w:br/><w:t>B</w:t>`)
	for _, tc := range []struct {
		part  string
		count int
	}{
		{part: "word/document.xml", count: 4},
		{part: "word/header1.xml", count: 1},
		{part: "word/footer1.xml", count: 1},
	} {
		got := bytes.Count(pkg.Get(tc.part), want)
		if got != tc.count {
			t.Errorf("%s contains %d single-break text runs, want %d", tc.part, got, tc.count)
		}
	}
}
