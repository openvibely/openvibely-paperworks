package xlsx

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/beevik/etree"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"github.com/xuri/excelize/v2"
)

func TestInstalledWorksheetReferenceBytes(t *testing.T) {
	base := "../../testdata/document-examples/document_spreadsheets/examples"
	raw, e := os.ReadFile(filepath.Join(base, "report.json"))
	if e != nil {
		t.Fatal(e)
	}
	var spec Spec
	if e = json.Unmarshal(raw, &spec); e != nil {
		t.Fatal(e)
	}
	out := filepath.Join(t.TempDir(), "report.xlsx")
	if e = Create(context.Background(), spec, base, out); e != nil {
		t.Fatal(e)
	}
	p, e := opc.Open(out)
	if e != nil {
		t.Fatal(e)
	}
	// Unchanged local pinned openpyxl example, captured in the worksheet audit.
	if got := fmt.Sprintf("%x", sha256.Sum256(p.Get("xl/worksheets/sheet1.xml"))); got != "720d41cfcd5915abc6b5ea6b9f6a6fddad7362627b002f4fdae324dbd6d0a504" {
		t.Fatalf("worksheet hash %s", got)
	}
	g, e := excelize.OpenFile(out)
	if e != nil {
		t.Fatal(e)
	}
	defer g.Close()
	if got, e := g.GetCellValue("Revenue", "A2"); e != nil || got != "Subscriptions" {
		t.Fatalf("Revenue!A2 = %q: %v", got, e)
	}
	if got, e := g.GetCellFormula("Revenue", "D4"); e != nil || got != "SUM(D2:D3)" {
		t.Fatalf("Revenue!D4 formula = %q: %v", got, e)
	}
}

func TestNormalizeWorksheetXMLPreservesCellContents(t *testing.T) {
	raw := []byte(`<worksheet><!-- <c x="<v/>"/>; <c r="comment"/> --><![CDATA[<c x="<v/>"/>; <c r="cdata"/>]]><sheetData><row r="1"><c r="A1" s="2"/><c r="B1" s="3"><v/></c><c r="C1" t="str"><f>1+2</f><v>3</v></c></row></sheetData></worksheet>`)
	want := `<worksheet><!-- <c x="<v></v>"/>; <c r="comment"/> --><![CDATA[<c x="<v></v>"/>; <c r="cdata"/>]]><sheetData><row r="1"><c r="A1" s="2"></c><c r="B1" s="3"><v></v></c><c r="C1" t="str"><f>1+2</f><v>3</v></c></row></sheetData></worksheet>`
	if got := string(normalizeWorksheetXML(raw)); got != want {
		t.Fatalf("normalized worksheet XML mismatch\n got: %s\nwant: %s", got, want)
	}
}

func TestInlineWorksheetValuesAndFormulas(t *testing.T) {
	f := excelize.NewFile()
	defer f.Close()
	if _, e := f.NewSheet("Second"); e != nil {
		t.Fatal(e)
	}
	style, e := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true}})
	if e != nil {
		t.Fatal(e)
	}
	values := []string{"  spaced  ", "α & <β>", "=literal", "duplicate", "duplicate", "line\nbreak"}
	for _, sheet := range []string{"Sheet1", "Second"} {
		if e := f.SetCellStyle(sheet, "C2", "C2", style); e != nil {
			t.Fatal(e)
		}
		for i, v := range values {
			cell, _ := excelize.CoordinatesToCellName(1, i+1)
			if e := f.SetCellStr(sheet, cell, v); e != nil {
				t.Fatal(e)
			}
		}
		if e := f.SetCellFormula(sheet, "B1", "1+2"); e != nil {
			t.Fatal(e)
		}
	}
	data, e := f.WriteToBuffer()
	if e != nil {
		t.Fatal(e)
	}
	p, e := opc.Read(data.Bytes())
	if e != nil {
		t.Fatal(e)
	}
	sheet, e := p.XML("xl/worksheets/sheet1.xml")
	if e != nil {
		t.Fatal(e)
	}
	sheet.Root().CreateComment(`<c x="<v/>"/>;<c r="comment"/>`)
	var emptyCell *etree.Element
	for _, cell := range sheet.Root().FindElements("sheetData/row/c") {
		if cell.SelectAttrValue("r", "") == "C2" {
			emptyCell = cell
			break
		}
	}
	if emptyCell == nil {
		t.Fatal("styled empty cell C2 is missing")
	}
	emptyCell.CreateElement("v")
	if e = p.SetXML("xl/worksheets/sheet1.xml", sheet); e != nil {
		t.Fatal(e)
	}
	if e = serializeWorksheets(p); e != nil {
		t.Fatal(e)
	}
	worksheetXML := string(p.Get("xl/worksheets/sheet1.xml"))
	if !strings.Contains(worksheetXML, "<v></v>") || strings.Contains(worksheetXML, "<v/>") {
		t.Fatalf("empty value element was not normalized: %s", worksheetXML)
	}
	if !strings.Contains(worksheetXML, `r="C2"`) || !strings.Contains(worksheetXML, "</c>") {
		t.Fatalf("styled empty cell was not serialized with a closing tag: %s", worksheetXML)
	}
	if !strings.Contains(worksheetXML, `<!--<c x="<v></v>"/>;<c r="comment"/>-->`) {
		t.Fatalf("comment markup was normalized as worksheet cells: %s", worksheetXML)
	}
	out := filepath.Join(t.TempDir(), "inline.xlsx")
	if e = p.SaveNew(out); e != nil {
		t.Fatal(e)
	}
	g, e := excelize.OpenFile(out)
	if e != nil {
		t.Fatal(e)
	}
	defer g.Close()
	for _, sheet := range []string{"Sheet1", "Second"} {
		for i, want := range values {
			cell, _ := excelize.CoordinatesToCellName(1, i+1)
			got, e := g.GetCellValue(sheet, cell)
			if e != nil || got != want {
				t.Fatalf("%s!%s %q: %v", sheet, cell, got, e)
			}
		}
		if got, e := g.GetCellFormula(sheet, "B1"); e != nil || got != "1+2" {
			t.Fatalf("formula %q %v", got, e)
		}
		if got, e := g.GetCellValue(sheet, "C2"); e != nil || got != "" {
			t.Fatalf("styled empty cell %q: %v", got, e)
		}
	}
}

var worksheetXMLBenchmarkSink []byte

func BenchmarkNormalizeWorksheetXML(b *testing.B) {
	for _, count := range []int{1000, 5000, 10000} {
		b.Run(fmt.Sprintf("styled_empty/%d", count), func(b *testing.B) {
			raw := benchmarkWorksheetXML(count, false)
			b.SetBytes(int64(len(raw)))
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				worksheetXMLBenchmarkSink = normalizeWorksheetXML(raw)
			}
		})
	}
	b.Run("populated/10000", func(b *testing.B) {
		raw := benchmarkWorksheetXML(10000, true)
		b.SetBytes(int64(len(raw)))
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			worksheetXMLBenchmarkSink = normalizeWorksheetXML(raw)
		}
	})
}

func benchmarkWorksheetXML(cells int, populated bool) []byte {
	var xml strings.Builder
	xml.Grow(cells * 56)
	xml.WriteString(`<worksheet><sheetData>`)
	for row := 1; row <= cells; row++ {
		rowNumber := strconv.Itoa(row)
		xml.WriteString(`<row r="`)
		xml.WriteString(rowNumber)
		xml.WriteString(`"><c r="A`)
		xml.WriteString(rowNumber)
		xml.WriteString(`" s="1"`)
		if populated {
			xml.WriteString(`><v>123</v></c>`)
		} else {
			xml.WriteString(`/>`)
		}
		xml.WriteString(`</row>`)
	}
	xml.WriteString(`</sheetData></worksheet>`)
	return []byte(xml.String())
}
