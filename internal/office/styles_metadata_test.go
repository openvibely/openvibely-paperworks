package office

import (
	"testing"

	"github.com/openvibely/openvibely-paperworks/internal/opc"
)

func setStylesXML(t *testing.T, p *opc.Package, body string) {
	t.Helper()
	p.Set("xl/styles.xml", []byte(`<styleSheet xmlns="`+NSS+`">`+body+`</styleSheet>`))
}

func TestSharedStyleNumberFormatMetadata(t *testing.T) {
	p, _ := traceFixture(t, nil)
	setStylesXML(t, p, `<numFmts count="2"><numFmt numFmtId="164" formatCode="yyyy-mm-dd"/><numFmt numFmtId="165" formatCode="[h]"/></numFmts><cellXfs count="4"><xf numFmtId="14"><alignment horizontal="center"/></xf><xf numFmtId="164"/><xf numFmtId="165"><alignment horizontal="right"/></xf><xf numFmtId="0"/></cellXfs>`)
	wb, err := OpenWorkbook(p)
	if err != nil {
		t.Fatal(err)
	}

	importStyles := wb.Styles()
	wantImport := []CellStyle{
		{Date: true, Horizontal: "center"},
		{Date: true},
		{Horizontal: "right"},
		{},
	}
	if len(importStyles) != len(wantImport) {
		t.Fatalf("import styles = %#v", importStyles)
	}
	for i := range wantImport {
		if importStyles[i] != wantImport[i] {
			t.Errorf("import style %d = %+v, want %+v", i, importStyles[i], wantImport[i])
		}
	}

	traceStyles, err := wb.traceDateStyles()
	if err != nil {
		t.Fatal(err)
	}
	wantTrace := []traceDateStyle{{date: true}, {date: true}, {date: true, elapsed: true}, {}}
	if len(traceStyles) != len(wantTrace) {
		t.Fatalf("trace styles = %#v", traceStyles)
	}
	for i := range wantTrace {
		if traceStyles[i] != wantTrace[i] {
			t.Errorf("trace style %d = %+v, want %+v", i, traceStyles[i], wantTrace[i])
		}
	}
}

func TestStyleReadersMissingStyles(t *testing.T) {
	p, _ := traceFixture(t, nil)
	wb, err := OpenWorkbook(p)
	if err != nil {
		t.Fatal(err)
	}
	p.Remove("xl/styles.xml")
	if got := wb.Styles(); got != nil {
		t.Fatalf("Styles() = %#v, want nil", got)
	}
	if got, err := wb.traceDateStyles(); err != nil || got != nil {
		t.Fatalf("traceDateStyles() = %#v, %v; want nil, nil", got, err)
	}
}

func TestStyleReadersMalformedNumberFormatIDs(t *testing.T) {
	t.Run("custom ID", func(t *testing.T) {
		p, _ := traceFixture(t, nil)
		setStylesXML(t, p, `<numFmts count="1"><numFmt numFmtId="invalid" formatCode="yyyy-mm-dd"/></numFmts><cellXfs count="1"><xf numFmtId="0"/></cellXfs>`)
		wb, err := OpenWorkbook(p)
		if err != nil {
			t.Fatal(err)
		}
		// Import has historically treated a malformed custom ID as zero and
		// connected that format to a zero-ID cell style.
		if got := wb.Styles(); len(got) != 1 || !got[0].Date {
			t.Fatalf("Styles() = %#v, want malformed custom ID treated as zero", got)
		}
		if got, err := wb.traceDateStyles(); err == nil || err.Error() != "invalid number format ID" || got != nil {
			t.Fatalf("traceDateStyles() = %#v, %v; want invalid number format ID", got, err)
		}
	})

	t.Run("cell format ID", func(t *testing.T) {
		p, _ := traceFixture(t, nil)
		setStylesXML(t, p, `<cellXfs count="1"><xf numFmtId="invalid"><alignment horizontal="center"/></xf></cellXfs>`)
		wb, err := OpenWorkbook(p)
		if err != nil {
			t.Fatal(err)
		}
		if got := wb.Styles(); len(got) != 1 || got[0].Date || got[0].Horizontal != "center" {
			t.Fatalf("Styles() = %#v, want zero-ID date flag and retained alignment", got)
		}
		if got, err := wb.traceDateStyles(); err == nil || err.Error() != `strconv.Atoi: parsing "invalid": invalid syntax` || got != nil {
			t.Fatalf("traceDateStyles() = %#v, %v; want the number-format parse error", got, err)
		}
	})
}

func TestStyleReadersMalformedStylesXML(t *testing.T) {
	p, _ := traceFixture(t, nil)
	p.Set("xl/styles.xml", []byte(`<styleSheet`))
	wb, err := OpenWorkbook(p)
	if err != nil {
		t.Fatal(err)
	}
	if got := wb.Styles(); got != nil {
		t.Fatalf("Styles() = %#v, want nil", got)
	}
	if got, err := wb.traceDateStyles(); err == nil || got != nil {
		t.Fatalf("traceDateStyles() = %#v, %v; want XML error", got, err)
	}
}
