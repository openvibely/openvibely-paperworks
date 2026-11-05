package office

import (
	"bytes"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"github.com/xuri/excelize/v2"
	"math"
	"strings"
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

func TestReplaceCategorySeriesIsolatesSharedWorkbook(t *testing.T) {
	p, chart1, chart2, sharedWorkbook := sharedWorkbookChartFixture(t)

	originalChart2 := bytes.Clone(p.Get(chart2))
	originalWorkbook := bytes.Clone(p.Get(sharedWorkbook))
	if err := ReplaceCategorySeries(p, chart1, "Revenue", []string{"Subscriptions", "Services"}, []float64{180, 90}); err != nil {
		t.Fatal(err)
	}

	archive, err := p.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := opc.Read(archive)
	if err != nil {
		t.Fatalf("reopen package: %v", err)
	}
	chart1Doc, err := reopened.XML(chart1)
	if err != nil {
		t.Fatal(err)
	}
	chart2Doc, err := reopened.XML(chart2)
	if err != nil {
		t.Fatal(err)
	}
	newTarget, err := ChartWorkbookTarget(reopened, chart1, chart1Doc.Root())
	if err != nil {
		t.Fatal(err)
	}
	chart2Target, err := ChartWorkbookTarget(reopened, chart2, chart2Doc.Root())
	if err != nil {
		t.Fatal(err)
	}
	if newTarget == sharedWorkbook || chart2Target != sharedWorkbook {
		t.Fatalf("edited chart target=%q, second chart target=%q", newTarget, chart2Target)
	}
	if !bytes.Equal(reopened.Get(chart2), originalChart2) {
		t.Fatal("second chart XML changed")
	}
	if !bytes.Equal(reopened.Get(sharedWorkbook), originalWorkbook) {
		t.Fatal("shared workbook changed")
	}

	series := All(chart1Doc.Root(), NSC, "ser")[0]
	nameRef := Child(Child(series, NSC, "tx"), NSC, "strRef")
	if nameRef == nil || Child(nameRef, NSC, "f").Text() != "Sheet1!$B$1" {
		t.Fatal("edited chart has no replacement series name reference")
	}
	newBook, err := excelize.OpenReader(bytes.NewReader(reopened.Get(newTarget)))
	if err != nil {
		t.Fatalf("open replacement workbook: %v", err)
	}
	defer newBook.Close()
	for cell, want := range map[string]string{"B1": "Revenue", "A2": "Subscriptions", "A3": "Services", "B2": "180", "B3": "90"} {
		got, err := newBook.GetCellValue("Sheet1", cell)
		if err != nil || got != want {
			t.Fatalf("replacement workbook %s=%q want %q: %v", cell, got, want, err)
		}
	}
	oldBook, err := excelize.OpenReader(bytes.NewReader(reopened.Get(chart2Target)))
	if err != nil {
		t.Fatalf("open preserved workbook: %v", err)
	}
	defer oldBook.Close()
	for cell, want := range map[string]string{"B1": "Shared source", "A2": "Legacy category", "B2": "45"} {
		got, err := oldBook.GetCellValue("Sheet1", cell)
		if err != nil || got != want {
			t.Fatalf("preserved workbook %s=%q want %q: %v", cell, got, want, err)
		}
	}
}

func TestReplaceCategorySeriesSharedWorkbookIsolationFailureIsAtomic(t *testing.T) {
	p, chart1, _, _ := sharedWorkbookChartFixture(t)
	p.Set(opc.RelsName(chart1), []byte(`<Relationships xmlns="`+opc.NSRel+`"><Relationship Id="rId1" Type="`+NSR+`/package" Target="../embeddings/shared.xlsx"/><Relationship Id="rId1" Type="`+NSR+`/package" Target="../embeddings/shared.xlsx"/></Relationships>`))
	before, err := p.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	if err := ReplaceCategorySeries(p, chart1, "Revenue", []string{"Subscriptions"}, []float64{180}); err == nil || !strings.Contains(err.Error(), "cannot isolate shared embedded workbook") {
		t.Fatalf("expected clear isolation error, got %v", err)
	}
	after, err := p.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("package bytes changed after isolation failed")
	}
}

func sharedWorkbookChartFixture(t *testing.T) (*opc.Package, string, string, string) {
	t.Helper()
	f := excelize.NewFile()
	defer f.Close()
	for cell, value := range map[string]string{"B1": "Shared source", "A2": "Legacy category", "B2": "45"} {
		if err := f.SetCellValue("Sheet1", cell, value); err != nil {
			t.Fatal(err)
		}
	}
	raw, err := f.WriteToBuffer()
	if err != nil {
		t.Fatal(err)
	}
	p, err := opc.Read(raw.Bytes())
	if err != nil {
		t.Fatal(err)
	}

	chart1 := "ppt/charts/chart1.xml"
	chart2 := "ppt/charts/chart2.xml"
	sharedWorkbook := "ppt/embeddings/shared.xlsx"
	chartXML := func(series string) []byte {
		return []byte(`<c:chartSpace xmlns:c="` + NSC + `" xmlns:r="` + NSR + `"><c:chart><c:plotArea><c:barChart><c:barDir val="col"/><c:ser>` + series + `</c:ser></c:barChart></c:plotArea></c:chart><c:externalData r:id="rId1"/></c:chartSpace>`)
	}
	p.Set(chart1, chartXML(`<c:idx val="0"/><c:order val="0"/><c:tx/><c:cat/><c:val/>`))
	p.Set(chart2, chartXML(`<c:idx val="0"/><c:order val="0"/><c:tx><c:strRef><c:f>Sheet1!$B$1</c:f></c:strRef></c:tx><c:cat/><c:val/>`))
	for _, chart := range []string{chart1, chart2} {
		if err := p.EnsureContentType(chart, "application/vnd.openxmlformats-officedocument.drawingml.chart+xml"); err != nil {
			t.Fatal(err)
		}
		p.Set(opc.RelsName(chart), []byte(`<Relationships xmlns="`+opc.NSRel+`"><Relationship Id="rId1" Type="`+NSR+`/package" Target="../embeddings/shared.xlsx"/></Relationships>`))
	}
	p.Set(sharedWorkbook, raw.Bytes())
	return p, chart1, chart2, sharedWorkbook
}
