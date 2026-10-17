package office

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"github.com/xuri/excelize/v2"
)

// Reference: openpyxl.styles.numbers BUILTIN_FORMATS, builtin_format_code,
// is_date_format and is_timedelta_format. Reserved IDs without a format entry
// are not inferred as dates by that reader (even if Excel localizes them).
func TestTraceReservedNumberFormatsBothEpochs(t *testing.T) {
	for _, epoch := range []bool{false, true} {
		t.Run(fmt.Sprint(epoch), func(t *testing.T) {
			p, _ := traceFixture(t, func(f *excelize.File) {
				for id := 0; id < 164; id++ {
					if err := f.SetCellValue("Inputs", fmt.Sprintf("A%d", id+1), 1.5); err != nil {
						t.Fatal(err)
					}
				}
				if err := f.SetCellFormula("Output", "C1", "SUM(Inputs!A1:A164)"); err != nil {
					t.Fatal(err)
				}
			})
			styles, err := p.XML("xl/styles.xml")
			if err != nil {
				t.Fatal(err)
			}
			xfs := Child(styles.Root(), NSS, "cellXfs")
			template := Children(xfs, NSS, "xf")[0].Copy()
			for _, xf := range Children(xfs, NSS, "xf") {
				xfs.RemoveChild(xf)
			}
			xfs.CreateAttr("count", "164")
			for id := 0; id < 164; id++ {
				xf := template.Copy()
				xf.CreateAttr("numFmtId", strconv.Itoa(id))
				xfs.AddChild(xf)
			}
			if err = p.SetXML("xl/styles.xml", styles); err != nil {
				t.Fatal(err)
			}
			wb, err := OpenWorkbook(p)
			if err != nil {
				t.Fatal(err)
			}
			for id := 0; id < 164; id++ {
				wb.Sheets["Inputs"].Cells[fmt.Sprintf("A%d", id+1)].CreateAttr("s", strconv.Itoa(id))
			}
			if err = p.SetXML(wb.Sheets["Inputs"].Part, wb.Sheets["Inputs"].Doc); err != nil {
				t.Fatal(err)
			}
			doc, err := p.XML("xl/workbook.xml")
			if err != nil {
				t.Fatal(err)
			}
			pr := Child(doc.Root(), NSS, "workbookPr")
			if pr == nil {
				pr = doc.Root().CreateElement("workbookPr")
			}
			pr.CreateAttr("date1904", strconv.FormatBool(epoch))
			if err = p.SetXML("xl/workbook.xml", doc); err != nil {
				t.Fatal(err)
			}
			// Save and reopen the actual package, rather than tracing only an in-memory edit.
			file := filepath.Join(t.TempDir(), "reserved-formats.xlsx")
			if err = p.SaveNew(file); err != nil {
				t.Fatal(err)
			}
			original, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			p, err = opc.Open(file)
			if err != nil {
				t.Fatal(err)
			}
			parts := map[string][]byte{}
			for _, part := range p.Parts {
				parts[part.Name] = bytes.Clone(part.Data)
			}
			r := traceRun(t, p, "Output", "C1", 200, 20)
			if !r.Complete || len(r.Nodes) != 165 || len(r.Edges) != 164 {
				t.Fatalf("unexpected graph: %+v", r)
			}
			for id := 0; id < 164; id++ {
				var want any = float64(1.5)
				// Explicit reference expectations, independent of the implementation map.
				switch id {
				case 14, 15, 16, 17, 18, 19, 20, 21, 22, 45, 47:
					want = "1900-01-01T12:00:00"
					if epoch {
						want = "1904-01-02T12:00:00"
					}
				case 46:
					want = "1 day, 12:00:00" // documented JSON extension for timedelta
				}
				label := fmt.Sprintf("'Inputs'!A%d", id+1)
				if got := r.Nodes[label].CachedValue; got != want {
					t.Errorf("numFmtId %d: got %#v want %#v", id, got, want)
				}
			}
			if got := r.Nodes["'Output'!C1"].Formula; got == nil || *got != "=SUM(Inputs!A1:A164)" {
				t.Fatal("formula changed", got)
			}
			after, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(original, after) {
				t.Fatal("source file changed")
			}
			for _, part := range p.Parts {
				if !bytes.Equal(parts[part.Name], part.Data) {
					t.Fatal("source part changed", part.Name)
				}
			}
		})
	}
}
