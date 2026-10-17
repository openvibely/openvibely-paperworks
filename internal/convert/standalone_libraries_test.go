package convert

import (
	"bytes"
	"encoding/csv"
	"encoding/xml"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/beevik/etree"
	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"github.com/xuri/excelize/v2"
)

// test_document_libraries.py: typed exchange and read-only format workflows.
func TestReferenceStandaloneLibrariesTableExchange(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "exchange.xlsx")
	date := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	fixture, e := os.ReadFile("../../testdata/reference-mirror/library-exchange.xlsx")
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(src, fixture, 0600); e != nil {
		t.Fatal(e)
	}
	original := contents(t, src)
	dst := filepath.Join(dir, "exchange.docx")
	if _, e = TableExchange(src, dst, "Evidence", "A1:D2", 0); e != nil {
		t.Fatal(e)
	}
	p, e := opc.Open(dst)
	if e != nil {
		t.Fatal(e)
	}
	d, e := p.XML("word/document.xml")
	if e != nil {
		t.Fatal(e)
	}
	// reference DOCX library Document.tables exposes direct body tables only.
	tables := office.Children(office.Child(d.Root(), office.NSW, "body"), office.NSW, "tbl")
	if len(tables) != 1 {
		t.Fatalf("tables: %d", len(tables))
	}
	rows := office.Children(tables[0], office.NSW, "tr")
	if len(rows) != 2 {
		t.Fatalf("rows: %d", len(rows))
	}
	cells := office.Children(rows[1], office.NSW, "tc")
	if len(cells) != 4 {
		t.Fatalf("cells: %d", len(cells))
	}
	t.Run("typed-values", func(t *testing.T) {
		for _, c := range []struct {
			col  int
			want string
		}{{0, "001"}, {1, "2026-01-02T00:00:00"}, {3, "42.5"}} {
			if got := referenceExchangeCellText(cells[c.col]); got != c.want {
				t.Errorf("column %d: %q want %q", c.col, got, c.want)
			}
		}
		para := office.Child(cells[2], office.NSW, "p")
		jc := office.Child(office.Child(para, office.NSW, "pPr"), office.NSW, "jc")
		if val, ok := office.Attr(jc, office.NSW, "val"); !ok || val != "center" {
			t.Errorf("text alignment: %v", jc)
		}
	})
	csvPath := filepath.Join(dir, "exchange.csv")
	if _, e = TableExchange(dst, csvPath, "", "", 0); e != nil {
		t.Fatal(e)
	}
	records, e := csv.NewReader(bytes.NewReader(contents(t, csvPath))).ReadAll()
	if e != nil {
		t.Fatal(e)
	}
	t.Run("csv-and-source", func(t *testing.T) {
		if len(records) != 2 || len(records[1]) != 4 || records[1][2] != "comma, quote \" and\nnewline" {
			t.Errorf("CSV escaped text: %#v", records)
		}
		// Reference's quoted field retains LF; Go csv.Reader normalizes CRLF and
		// cannot alone detect a writer changing the original cell's newline.
		wantCSV := "Identifier,Date,Text,Amount\r\n001,2026-01-02T00:00:00,\"comma, quote \"\" and\nnewline\",42.5\r\n"
		if got := string(contents(t, csvPath)); got != wantCSV {
			t.Errorf("CSV bytes: %q want %q", got, wantCSV)
		}
		if !bytes.Equal(original, contents(t, src)) {
			t.Error("source workbook changed during exchange")
		}
	})
	t.Run("overwrite-error-identity", func(t *testing.T) {
		before := contents(t, csvPath)
		_, e := TableExchange(dst, csvPath, "", "", 0)
		if !errors.Is(e, os.ErrExist) {
			t.Errorf("want file-exists error, got %v", e)
		}
		if !bytes.Equal(before, contents(t, csvPath)) {
			t.Error("existing CSV changed")
		}
	})
	uncached := filepath.Join(dir, "uncached.xlsx")
	fixture, e = os.ReadFile("../../testdata/reference-mirror/library-uncached.xlsx")
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(uncached, fixture, 0600); e != nil {
		t.Fatal(e)
	}
	missing := filepath.Join(dir, "uncached.docx")
	_, e = TableExchange(uncached, missing, "Evidence", "A1:D2", 0)
	t.Run("missing-cache-error", func(t *testing.T) {
		if e == nil || !strings.Contains(e.Error(), "Missing saved formula") {
			t.Errorf("missing cache error: %v", e)
		}
	})
	t.Run("missing-cache-no-output", func(t *testing.T) {
		if _, e = os.Stat(missing); !errors.Is(e, os.ErrNotExist) {
			t.Errorf("failed exchange published output: %v", e)
		}
	})
	t.Run("read-only-values", func(t *testing.T) {
		saved := referenceRuntimeRead(t, src)
		id, e := saved.GetCellValue("Evidence", "A2")
		if e != nil || id != "001" {
			t.Errorf("identifier: %q %v", id, e)
		}
		kind, e := saved.GetCellType("Evidence", "A2")
		if e != nil || (kind != excelize.CellTypeSharedString && kind != excelize.CellTypeInlineString) {
			t.Errorf("identifier type: %v %v", kind, e)
		}
		styleID, e := saved.GetCellStyle("Evidence", "B2")
		if e != nil {
			t.Fatal(e)
		}
		style, e := saved.GetStyle(styleID)
		if e != nil {
			t.Fatal(e)
		}
		if style.CustomNumFmt == nil || *style.CustomNumFmt != "yyyy-mm-dd h:mm:ss" {
			t.Fatalf("date lost its original date format: %+v", style)
		}
		serial := referenceRuntimeValue(t, saved, "Evidence", "B2")
		gotDate, e := excelize.ExcelDateToTime(serial, false)
		if e != nil || !gotDate.Equal(date) {
			t.Errorf("date: %v %v", gotDate, e)
		}
		if got := referenceRuntimeValue(t, saved, "Evidence", "D2"); got != 42.5 {
			t.Errorf("amount: %v", got)
		}
		if !bytes.Equal(original, contents(t, src)) {
			t.Error("read-only workbook source changed")
		}
	})
}

func TestReferenceStandaloneLibrariesDelimitedReadOnly(t *testing.T) {
	for _, c := range []struct {
		suffix    string
		delimiter rune
	}{{"csv", ','}, {"tsv", '\t'}} {
		t.Run(c.suffix, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "read-only."+c.suffix)
			var buf bytes.Buffer
			w := csv.NewWriter(&buf)
			w.Comma = c.delimiter
			w.UseCRLF = true
			want := [][]string{{"ID", "Date", "Amount"}, {"001", "2026-01-02", "42.50"}}
			if e := w.WriteAll(want); e != nil {
				t.Fatal(e)
			}
			original := append([]byte(nil), buf.Bytes()...)
			if e := os.WriteFile(path, original, 0600); e != nil {
				t.Fatal(e)
			}
			r := csv.NewReader(bytes.NewReader(contents(t, path)))
			r.Comma = c.delimiter
			records, e := r.ReadAll()
			if e != nil {
				t.Fatal(e)
			}
			if len(records) != 2 || !reflect.DeepEqual(records[1], want[1]) {
				t.Errorf("records: %v", records)
			}
			if !bytes.Equal(original, contents(t, path)) {
				t.Error("source changed")
			}
		})
	}
}

func TestReferenceStandaloneLibrariesCorrectionsSeparate(t *testing.T) {
	original := [][]any{{"ID", "Value", "Unit"}, {"001", float64(8), "fixture-unit"}, {"002", float64(12), "fixture-unit"}}
	path := filepath.Join(t.TempDir(), "corrections.xlsx")
	if err := os.WriteFile(path, contents(t, "../../testdata/reference-mirror/library-corrections.xlsx"), 0600); err != nil {
		t.Fatal(err)
	}
	saved := referenceRuntimeRead(t, path)
	if got := referenceLibraryTypedRows(t, saved, "Observations"); !reflect.DeepEqual(got, original) {
		t.Errorf("observations: %#v, want %#v", got, original)
	}
	rows := referenceLibraryTypedRows(t, saved, "Corrections")
	want := []any{"002", float64(12), float64(11), "Supplied transcription correction", "Synthetic protocol fixture"}
	if len(rows) != 2 || !reflect.DeepEqual(rows[1], want) {
		t.Errorf("corrections: %#v", rows)
	}

}

func TestReferenceStandaloneLibrariesLiteralInputs(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "inputs.xlsx")
	if err := os.WriteFile(src, contents(t, "../../testdata/reference-mirror/library-inputs.xlsx"), 0600); err != nil {
		t.Fatal(err)
	}
	original := contents(t, src)
	dst := filepath.Join(dir, "input-edited.xlsx")
	update, e := WorkbookInputs(src, dst, "Inputs", map[string]any{"A1": float64(3), "C2": " 001 ", "D3": true, "G4": nil})
	if e != nil {
		t.Fatal(e)
	}
	changed := update["changed_parts"].([]string)
	if len(changed) != 1 {
		t.Fatal(changed)
	}
	part := changed[0]
	if _, e = office.VerifyPreservation(src, dst, changed); e != nil {
		t.Fatal(e)
	}
	saved := referenceRuntimeRead(t, dst)
	if got := referenceRuntimeValue(t, saved, "Inputs", "A1"); got != 3 {
		t.Error("A1", got)
	}
	id, e := saved.GetCellStyle("Inputs", "A1")
	if e != nil {
		t.Fatal(e)
	}
	format, e := saved.GetStyle(id)
	if e != nil {
		t.Fatal(e)
	}
	if format.NumFmt != 2 {
		t.Errorf("number format not 0.00: %+v", format)
	}
	formula, e := saved.GetCellFormula("Inputs", "B1")
	if e != nil || formula != "A1*2" {
		t.Errorf("formula: %q %v", formula, e)
	}
	kindC2, err := saved.GetCellType("Inputs", "C2")
	if err != nil || (kindC2 != excelize.CellTypeInlineString && kindC2 != excelize.CellTypeSharedString) {
		t.Fatalf("C2 must be string: %v %v", kindC2, err)
	}
	text, e := saved.GetCellValue("Inputs", "C2")
	if e != nil || text != " 001 " {
		t.Errorf("literal text: %q %v", text, e)
	}
	kind, e := saved.GetCellType("Inputs", "D3")
	if e != nil || kind != excelize.CellTypeBool {
		t.Errorf("boolean type: %v %v", kind, e)
	}
	val, e := saved.GetCellValue("Inputs", "D3", excelize.Options{RawCellValue: true})
	if e != nil || val != "1" {
		t.Errorf("boolean value: %q %v", val, e)
	}
	pkg, e := opc.Open(dst)
	if e != nil {
		t.Fatal(e)
	}
	var sheet struct {
		Rows []struct {
			Cells []struct {
				Ref string `xml:"r,attr"`
			} `xml:"c"`
		} `xml:"sheetData>row"`
	}
	e = xml.Unmarshal(pkg.Get(part), &sheet)
	if e != nil {
		t.Fatal(e)
	}
	maxRow, maxCol := 0, 0
	for _, rowEntry := range sheet.Rows {
		for _, cell := range rowEntry.Cells {
			col, row, e := excelize.CellNameToCoordinates(cell.Ref)
			if e != nil {
				t.Fatal(e)
			}
			maxRow = max(maxRow, row)
			maxCol = max(maxCol, col)
		}
	}
	if maxRow != 4 || maxCol != 7 {
		t.Errorf("dimensions: rows=%d cols=%d", maxRow, maxCol)
	}
	for i, c := range []struct {
		cell  string
		value any
	}{{"B1", float64(4)}, {"A1", "=2+2"}, {"A1", math.Inf(1)}, {"F1", float64(3)}, {"XFE1", float64(3)}, {"A1048577", float64(3)}, {"A1", map[string]any{}}} {
		t.Run(fmt.Sprintf("invalid-%d", i), func(t *testing.T) {
			bad := filepath.Join(dir, "bad-input.xlsx")
			_, err := WorkbookInputs(src, bad, "Inputs", map[string]any{c.cell: c.value})
			if err == nil {
				t.Errorf("unsafe input accepted %s=%v", c.cell, c.value)
			}

			if _, err = os.Stat(bad); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("invalid output published: %v", err)
			}
		})
	}
	if !bytes.Equal(original, contents(t, src)) {
		t.Error("original inputs changed")
	}
}

// Independent reference DOCX library 1.2.0 Cell.text -> Paragraph.text -> Run.text.
// Do not use the production exchange reader to validate its own output.
func referenceExchangeCellText(cell *etree.Element) string {
	var paragraphs []string
	for _, p := range office.Children(cell, office.NSW, "p") {
		var text strings.Builder
		var run func(*etree.Element)
		run = func(r *etree.Element) {
			for _, e := range r.ChildElements() {
				if e.NamespaceURI() != office.NSW {
					continue
				}
				switch e.Tag {
				case "t":
					text.WriteString(e.Text())
				case "tab", "ptab":
					text.WriteByte('\t')
				case "cr":
					text.WriteByte('\n')
				case "br":
					if office.AttrV(e, office.NSW, "type") == "" || office.AttrV(e, office.NSW, "type") == "textWrapping" {
						text.WriteByte('\n')
					}
				case "noBreakHyphen":
					text.WriteByte('-')
				}
			}
		}
		for _, child := range p.ChildElements() {
			if office.Is(child, office.NSW, "r") {
				run(child)
			}
			if office.Is(child, office.NSW, "hyperlink") {
				for _, r := range office.Children(child, office.NSW, "r") {
					run(r)
				}
			}
		}
		paragraphs = append(paragraphs, text.String())
	}
	return strings.Join(paragraphs, "\n")
}

// Typed row acquisition for the pinned library fixtures. Excelize display strings
// alone cannot distinguish numeric 1 from the string "1". Reject formulas/errors
// outside this literal fixture domain rather than silently coercing them.
func referenceLibraryTypedRows(t *testing.T, f *excelize.File, sheet string) [][]any {
	t.Helper()
	rows, err := f.GetRows(sheet, excelize.Options{RawCellValue: true})
	if err != nil {
		t.Fatal(err)
	}
	result := make([][]any, len(rows))
	for i, row := range rows {
		result[i] = make([]any, len(row))
		for j, text := range row {
			cell, err := excelize.CoordinatesToCellName(j+1, i+1)
			if err != nil {
				t.Fatal(err)
			}
			typ, err := f.GetCellType(sheet, cell)
			if err != nil {
				t.Fatal(err)
			}
			switch typ {
			case excelize.CellTypeInlineString, excelize.CellTypeSharedString:
				result[i][j] = text
			case excelize.CellTypeNumber, excelize.CellTypeUnset:
				if text != "" {
					result[i][j] = referenceRuntimeValue(t, f, sheet, cell)
				}
			case excelize.CellTypeBool:
				if text != "0" && text != "1" {
					t.Fatalf("invalid bool %s", text)
				}
				result[i][j] = text == "1"
			default:
				t.Fatalf("unsupported literal type %v at %s", typ, cell)
			}
		}
	}
	return result
}
