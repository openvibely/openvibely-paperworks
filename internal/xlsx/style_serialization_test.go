package xlsx

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"github.com/xuri/excelize/v2"
)

func TestStyleReferenceSerialization(t *testing.T) {
	raw, err := os.ReadFile("testdata/style-reference.json")
	if err != nil {
		t.Fatal(err)
	}
	var refs struct {
		Installed    string `json:"installed_sha256"`
		Supplemental string `json:"supplemental_sha256"`
	}
	if err = json.Unmarshal(raw, &refs); err != nil {
		t.Fatal(err)
	}
	t.Run("installed", func(t *testing.T) {
		base := "../../testdata/document-examples/document_spreadsheets/examples"
		raw, err := os.ReadFile(filepath.Join(base, "report.json"))
		if err != nil {
			t.Fatal(err)
		}
		var s Spec
		if err = json.Unmarshal(raw, &s); err != nil {
			t.Fatal(err)
		}
		name := filepath.Join(t.TempDir(), "book.xlsx")
		if err = Create(context.Background(), s, base, name); err != nil {
			t.Fatal(err)
		}
		p, err := opc.Open(name)
		if err != nil {
			t.Fatal(err)
		}
		if got := fmt.Sprintf("%x", sha256.Sum256(p.Get("xl/styles.xml"))); got != refs.Installed {
			t.Fatalf("%s want %s", got, refs.Installed)
		}
	})
	t.Run("unused-formats-and-conditional-font", func(t *testing.T) {
		f := excelize.NewFile()
		defer f.Close()
		f.SetSheetName("Sheet1", "Probe")
		// Deliberately register the unused format first; conditional formatting must
		// retain its own font even after ordinary unused resources are discarded.
		f.NewStyle(&excelize.Style{Font: &excelize.Font{Color: "C00000"}})
		money := `$#,##0.00;[Red]($#,##0.00)`
		num, e := f.NewStyle(&excelize.Style{CustomNumFmt: &money})
		if e != nil {
			t.Fatal(e)
		}
		head, e := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true, Color: "FFFFFF"}, Fill: excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"17324D"}}})
		if e != nil {
			t.Fatal(e)
		}
		f.SetCellValue("Probe", "A1", "heading")
		f.SetCellStyle("Probe", "A1", "A1", head)
		f.SetCellValue("Probe", "B2", 12.5)
		f.SetCellStyle("Probe", "B2", "B2", num)
		cond, e := f.NewConditionalStyle(&excelize.Style{Font: &excelize.Font{Color: "C00000"}})
		if e != nil {
			t.Fatal(e)
		}
		if e = f.SetConditionalFormat("Probe", "B2", []excelize.ConditionalFormatOptions{{Type: "cell", Criteria: "<", Value: "0", Format: &cond}}); e != nil {
			t.Fatal(e)
		}
		b, e := f.WriteToBuffer()
		if e != nil {
			t.Fatal(e)
		}
		p, e := opc.Read(b.Bytes())
		if e != nil {
			t.Fatal(e)
		}
		if e = stabilizeStyleIDs(p); e != nil {
			t.Fatal(e)
		}
		if e = serializeStyles(p, Spec{}); e != nil {
			t.Fatal(e)
		}
		if got := fmt.Sprintf("%x", sha256.Sum256(p.Get("xl/styles.xml"))); got != refs.Supplemental {
			t.Fatalf("%s want %s\n%s", got, refs.Supplemental, p.Get("xl/styles.xml"))
		}
	})
}
