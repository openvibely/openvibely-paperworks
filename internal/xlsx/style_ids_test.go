package xlsx

import (
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"github.com/xuri/excelize/v2"
	"testing"
)

func TestStyleIDsIndependentOfRegistration(t *testing.T) {
	for _, reverse := range []bool{false, true} {
		f := excelize.NewFile()
		formats := []int{2, 4}
		if reverse {
			formats = []int{4, 2}
		}
		ids := map[int]int{}
		for _, n := range formats {
			id, err := f.NewStyle(&excelize.Style{NumFmt: n})
			if err != nil {
				t.Fatal(err)
			}
			ids[n] = id
		}
		for _, cell := range []struct {
			name   string
			format int
		}{{"A1", 2}, {"B1", 4}} {
			if err := f.SetCellValue("Sheet1", cell.name, 12.5); err != nil {
				t.Fatal(err)
			}
			if err := f.SetCellStyle("Sheet1", cell.name, cell.name, ids[cell.format]); err != nil {
				t.Fatal(err)
			}
		}
		b, err := f.WriteToBuffer()
		f.Close()
		if err != nil {
			t.Fatal(err)
		}
		p, err := opc.Read(b.Bytes())
		if err != nil {
			t.Fatal(err)
		}
		if err = stabilizeStyleIDs(p); err != nil {
			t.Fatal(err)
		}
		d, err := p.XML("xl/worksheets/sheet1.xml")
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range []struct{ name, id string }{{"A1", "1"}, {"B1", "2"}} {
			e := d.Root().FindElement("sheetData/row/c[@r='" + c.name + "']")
			if e == nil || e.SelectAttrValue("s", "") != c.id {
				t.Fatalf("registration order %v: style lost", reverse)
			}
		}
		d, err = p.XML("xl/styles.xml")
		if err != nil {
			t.Fatal(err)
		}
		x := d.Root().SelectElement("cellXfs").ChildElements()
		if x[1].SelectAttrValue("numFmtId", "") != "2" || x[2].SelectAttrValue("numFmtId", "") != "4" {
			t.Fatal("style referents changed")
		}
	}
}
