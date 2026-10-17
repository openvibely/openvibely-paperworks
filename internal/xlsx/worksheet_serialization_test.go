package xlsx

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"github.com/xuri/excelize/v2"
	"os"
	"path/filepath"
	"testing"
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
}

func TestInlineWorksheetValuesAndFormulas(t *testing.T) {
	f := excelize.NewFile()
	defer f.Close()
	if _, e := f.NewSheet("Second"); e != nil {
		t.Fatal(e)
	}
	values := []string{"  spaced  ", "α & <β>", "=literal", "duplicate", "duplicate", "line\nbreak"}
	for _, sheet := range []string{"Sheet1", "Second"} {
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
	if e = serializeWorksheets(p); e != nil {
		t.Fatal(e)
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
	}
}
