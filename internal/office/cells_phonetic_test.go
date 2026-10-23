package office_test

import (
	"archive/zip"
	"bytes"
	"testing"

	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
)

func phoneticTestWorkbook(t *testing.T) *opc.Package {
	t.Helper()
	parts := map[string]string{
		"xl/workbook.xml":            `<workbook xmlns="` + office.NSS + `" xmlns:r="` + office.NSR + `"><sheets><sheet name="Sheet1" sheetId="1" r:id="rId1"/></sheets></workbook>`,
		"xl/_rels/workbook.xml.rels": `<Relationships xmlns="` + opc.NSRel + `"><Relationship Id="rId1" Type="` + opc.NSOffR + `/worksheet" Target="worksheets/sheet1.xml"/></Relationships>`,
		"xl/sharedStrings.xml":       `<sst xmlns="` + office.NSS + `" count="3" uniqueCount="3"><si><t>東京</t><rPh sb="0" eb="2"><t>とうきょう</t></rPh></si><si><r><t xml:space="preserve">shared </t></r><r><t>runs</t></r></si><si><t>plain shared</t></si></sst>`,
		"xl/worksheets/sheet1.xml":   `<worksheet xmlns="` + office.NSS + `"><sheetData><row r="1"><c r="A1" t="s"><v>0</v></c><c r="B1" t="inlineStr"><is><r><t xml:space="preserve">東京 </t></r><rPh sb="0" eb="2"><t>とうきょう</t></rPh><r><t>駅</t></r></is></c><c r="C1" t="s"><v>1</v></c><c r="D1" t="inlineStr"><is><r><t xml:space="preserve">inline </t></r><r><t>runs</t></r></is></c><c r="E1" t="s"><v>2</v></c><c r="F1" t="inlineStr"><is><t>plain inline</t></is></c></row></sheetData></worksheet>`,
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
	pkg, err := opc.Read(archive.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	return pkg
}

func TestWorkbookValuesExcludePhoneticRuns(t *testing.T) {
	wb, err := office.OpenWorkbook(phoneticTestWorkbook(t))
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		cell string
		want string
	}{
		{"A1", "東京"},
		{"B1", "東京 駅"},
		{"C1", "shared runs"},
		{"D1", "inline runs"},
		{"E1", "plain shared"},
		{"F1", "plain inline"},
	} {
		t.Run(tc.cell, func(t *testing.T) {
			got, err := wb.Value("Sheet1", tc.cell)
			if err != nil {
				t.Fatal(err)
			}
			if got.Kind != "str" || got.Text != tc.want {
				t.Fatalf("Value(%s) = %#v; want string %q", tc.cell, got, tc.want)
			}
		})
	}
}
