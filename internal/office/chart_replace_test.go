package office

import (
	"bytes"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"github.com/xuri/excelize/v2"
	"math"
	"testing"
)

func TestReplaceCategorySeriesDiscardsWorkbook(t *testing.T) {
	f := excelize.NewFile()
	defer f.Close()
	if err := f.SetCellValue("Sheet1", "A1", "Category"); err != nil {
		t.Fatal(err)
	}
	if err := f.SetCellValue("Sheet1", "Z99", "stale"); err != nil {
		t.Fatal(err)
	}
	raw, err := f.WriteToBuffer()
	if err != nil {
		t.Fatal(err)
	}
	// Start with a real package to preserve package invariants.
	p, err := opc.Read(raw.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	p.Set("ppt/charts/chart1.xml", []byte(`<c:chartSpace xmlns:c="`+NSC+`" xmlns:r="`+NSR+`"><c:chart><c:plotArea><c:barChart><c:barDir val="col"/><c:ser><c:idx val="0"/><c:order val="0"/><c:dLbls/><c:tx/><c:cat/><c:val/></c:ser></c:barChart></c:plotArea></c:chart><c:externalData r:id="rId1"/></c:chartSpace>`))
	p.Set("ppt/charts/_rels/chart1.xml.rels", []byte(`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="`+NSR+`/package" Target="../embeddings/book.xlsx"/></Relationships>`))
	p.Set("ppt/embeddings/book.xlsx", raw.Bytes())
	before := append([]byte(nil), p.Get("ppt/charts/chart1.xml")...)
	if err = ReplaceCategorySeries(p, "ppt/charts/chart1.xml", "Revenue", []string{"A"}, []float64{math.NaN()}); err == nil {
		t.Fatal("NaN accepted")
	}
	if !bytes.Equal(before, p.Get("ppt/charts/chart1.xml")) {
		t.Fatal("invalid edit mutated chart")
	}
	if err = ReplaceCategorySeries(p, "ppt/charts/chart1.xml", "Revenue", []string{"A", "B"}, []float64{180, 90}); err != nil {
		t.Fatal(err)
	}
	got, err := excelize.OpenReader(bytes.NewReader(p.Get("ppt/embeddings/book.xlsx")))
	if err != nil {
		t.Fatal(err)
	}
	defer got.Close()
	for cell, want := range map[string]string{"A1": "", "Z99": "", "B1": "Revenue", "A2": "A", "A3": "B", "B2": "180", "B3": "90"} {
		v, e := got.GetCellValue("Sheet1", cell)
		if e != nil || v != want {
			t.Fatalf("%s=%q want %q: %v", cell, v, want, e)
		}
	}
	doc, err := p.XML("ppt/charts/chart1.xml")
	if err != nil {
		t.Fatal(err)
	}
	s := All(doc.Root(), NSC, "ser")[0]
	children := s.ChildElements()
	if len(children) != 6 || children[2].Tag != "tx" || children[3].Tag != "dLbls" || children[4].Tag != "cat" || children[5].Tag != "val" {
		t.Fatal("replacement child order differs from pinned insertion semantics")
	}
}
