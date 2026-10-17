package convert

import (
	"path/filepath"
	"strconv"
	"testing"

	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
)

// Pinned reference DOCX library Document()/Table Grid defaults, observed through the
// original exchange_table implementation: six-inch block width, zero vertical
// padding and 108-twip horizontal padding inherited from Normal Table.
func TestTableExchangeDefaultGeometry(t *testing.T) {
	src := referenceRuntimeOriginalWorkbookFixture(t, "runtime-table-fixtures.zip", "table-source")
	for _, tc := range []struct {
		area           string
		width, columns int
	}{{"A1:A3", 8640, 1}, {"A1:B3", 4320, 2}, {"A1:C3", 2880, 3}} {
		t.Run(tc.area, func(t *testing.T) {
			dst := filepath.Join(t.TempDir(), "table.docx")
			if _, err := TableExchange(src, dst, "Source", tc.area, 0); err != nil {
				t.Fatal(err)
			}
			p, err := opc.Open(dst)
			if err != nil {
				t.Fatal(err)
			}
			d, err := p.XML("word/document.xml")
			if err != nil {
				t.Fatal(err)
			}
			styles, err := p.XML("word/styles.xml")
			if err != nil {
				t.Fatal(err)
			}
			defaults := office.Child(styles.Root(), office.NSW, "docDefaults")
			fonts := office.All(defaults, office.NSW, "rFonts")[0]
			if office.AttrV(fonts, office.NSW, "asciiTheme") != "minorHAnsi" {
				t.Fatal("default Latin theme reference differs from pinned template")
			}
			settings, err := p.XML("word/settings.xml")
			if err != nil {
				t.Fatal(err)
			}
			mode := ""
			for _, e := range office.All(settings.Root(), office.NSW, "compatSetting") {
				if office.AttrV(e, office.NSW, "name") == "compatibilityMode" {
					mode = office.AttrV(e, office.NSW, "val")
				}
			}
			if mode != "14" {
				t.Fatal("table compatibility mode", mode)
			}
			margins := office.All(d.Root(), office.NSW, "pgMar")[0]
			for _, side := range []string{"left", "right"} {
				if office.AttrV(margins, office.NSW, side) != "1800" {
					t.Fatalf("wrong %s margin", side)
				}
			}
			for _, side := range []string{"top", "bottom"} {
				if office.AttrV(margins, office.NSW, side) != "1440" {
					t.Fatalf("wrong %s margin", side)
				}
			}
			cols := office.All(d.Root(), office.NSW, "gridCol")
			if len(cols) != tc.columns {
				t.Fatal("column count", len(cols))
			}
			for _, col := range cols {
				if office.AttrV(col, office.NSW, "w") != strconv.Itoa(tc.width) {
					t.Fatal("wrong column width", col.Attr)
				}
			}
			if len(office.All(d.Root(), office.NSW, "tcMar")) != 0 {
				t.Fatal("direct cell margins override original table-style padding")
			}
			width := office.All(d.Root(), office.NSW, "tblW")[0]
			if office.AttrV(width, office.NSW, "type") != "auto" || office.AttrV(width, office.NSW, "w") != "0" {
				t.Fatal("table width", width.Attr)
			}
			if len(office.All(d.Root(), office.NSW, "tblHeader")) != 1 {
				t.Fatal("header repeat lost")
			}
		})
	}
}
