package convert

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"github.com/xuri/excelize/v2"
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

func TestTableExchangeDateStylesAndAlignment(t *testing.T) {
	f := excelize.NewFile()
	defer f.Close()
	sheet := f.GetSheetName(0)
	for cell, value := range map[string]float64{"A1": 45292.5, "B1": 45292.5, "C1": 42} {
		if err := f.SetCellValue(sheet, cell, value); err != nil {
			t.Fatal(err)
		}
	}
	builtin, err := f.NewStyle(&excelize.Style{NumFmt: 14, Alignment: &excelize.Alignment{Horizontal: "center"}})
	if err != nil {
		t.Fatal(err)
	}
	customCode := "yyyy-mm-dd"
	custom, err := f.NewStyle(&excelize.Style{CustomNumFmt: &customCode})
	if err != nil {
		t.Fatal(err)
	}
	if err = f.SetCellStyle(sheet, "A1", "A1", builtin); err != nil {
		t.Fatal(err)
	}
	if err = f.SetCellStyle(sheet, "B1", "B1", custom); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	source, destination := filepath.Join(dir, "styled.xlsx"), filepath.Join(dir, "styled.docx")
	if err = f.SaveAs(source); err != nil {
		t.Fatal(err)
	}
	if _, err = TableExchange(source, destination, sheet, "A1:C1", 0); err != nil {
		t.Fatal(err)
	}
	p, err := opc.Open(destination)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := p.XML("word/document.xml")
	if err != nil {
		t.Fatal(err)
	}
	tables := bodyTables(doc.Root())
	if len(tables) != 1 {
		t.Fatalf("document tables = %d, want 1", len(tables))
	}
	rows := office.Children(tables[0], office.NSW, "tr")
	if len(rows) != 1 {
		t.Fatalf("table rows = %d, want 1", len(rows))
	}
	cells := office.Children(rows[0], office.NSW, "tc")
	if len(cells) != 3 {
		t.Fatalf("table cells = %d, want 3", len(cells))
	}
	wantText := []string{"2024-01-01T12:00:00", "2024-01-01T12:00:00", "42"}
	wantAlignment := []string{"center", "left", "right"}
	for i, cell := range cells {
		if got := cellText(cell); got != wantText[i] {
			t.Errorf("cell %d text = %q, want %q", i, got, wantText[i])
		}
		paragraph := office.Child(cell, office.NSW, "p")
		jc := office.Child(office.Child(paragraph, office.NSW, "pPr"), office.NSW, "jc")
		if got := office.AttrV(jc, office.NSW, "val"); got != wantAlignment[i] {
			t.Errorf("cell %d alignment = %q, want %q", i, got, wantAlignment[i])
		}
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
