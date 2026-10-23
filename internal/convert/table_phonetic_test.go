package convert

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
)

func phoneticTableWorkbook(t *testing.T) string {
	t.Helper()
	parts := map[string]string{
		"xl/workbook.xml":            `<workbook xmlns="` + office.NSS + `" xmlns:r="` + office.NSR + `"><sheets><sheet name="Sheet1" sheetId="1" r:id="rId1"/></sheets></workbook>`,
		"xl/_rels/workbook.xml.rels": `<Relationships xmlns="` + opc.NSRel + `"><Relationship Id="rId1" Type="` + opc.NSOffR + `/worksheet" Target="worksheets/sheet1.xml"/></Relationships>`,
		"xl/sharedStrings.xml":       `<sst xmlns="` + office.NSS + `"><si><t>東京</t><rPh sb="0" eb="2"><t>とうきょう</t></rPh></si></sst>`,
		"xl/worksheets/sheet1.xml":   `<worksheet xmlns="` + office.NSS + `"><sheetData><row r="1"><c r="A1" t="s"><v>0</v></c></row></sheetData></worksheet>`,
	}
	var archive bytes.Buffer
	zw := zip.NewWriter(&archive)
	for name, data := range parts {
		entry, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write([]byte(data)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "phonetic.xlsx")
	if err := os.WriteFile(path, archive.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestXLSXTableExportExcludesPhoneticRuns(t *testing.T) {
	source := phoneticTableWorkbook(t)
	destination := filepath.Join(t.TempDir(), "table.docx")
	if _, err := TableExchange(source, destination, "Sheet1", "A1:A1", 0); err != nil {
		t.Fatal(err)
	}
	pkg, err := opc.Open(destination)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := pkg.XML("word/document.xml")
	if err != nil {
		t.Fatal(err)
	}
	cells := office.All(doc.Root(), office.NSW, "tc")
	if len(cells) != 1 {
		t.Fatalf("exported %d table cells; want 1", len(cells))
	}
	if got := cellText(cells[0]); got != "東京" {
		t.Fatalf("exported cell text %q; want base text only", got)
	}
}
