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

func TestWorkbookReferenceSerialization(t *testing.T) {
	raw, err := os.ReadFile("testdata/workbook-reference.json")
	if err != nil {
		t.Fatal(err)
	}
	var reference struct {
		Cases []struct {
			Name  string
			Parts map[string]string
		}
	}
	if err = json.Unmarshal(raw, &reference); err != nil {
		t.Fatal(err)
	}
	for _, c := range reference.Cases {
		t.Run(c.Name, func(t *testing.T) {
			base := "../../testdata/document-examples/document_spreadsheets/examples"
			var spec Spec
			if c.Name == "installed" {
				raw, err = os.ReadFile(filepath.Join(base, "report.json"))
				if err != nil {
					t.Fatal(err)
				}
				if err = json.Unmarshal(raw, &spec); err != nil {
					t.Fatal(err)
				}
			} else {
				spec = Spec{ThemeFile: "report-theme.xml", Sheets: []Sheet{{Name: "Hidden", Hidden: true}, {Name: "O'Brien & Sons", PrintArea: "B2:D5"}}, Names: []excelize.DefinedName{{Name: "RevenueTotal", RefersTo: "'O''Brien & Sons'!$B$2"}}}
			}
			out := filepath.Join(t.TempDir(), "test.xlsx")
			if err = Create(context.Background(), spec, base, out); err != nil {
				t.Fatal(err)
			}
			p, err := opc.Open(out)
			if err != nil {
				t.Fatal(err)
			}
			for name, want := range c.Parts {
				if got := fmt.Sprintf("%x", sha256.Sum256(p.Get(name))); got != want {
					t.Errorf("%s hash %s want %s\n%s", name, got, want, p.Get(name))
				}
			}
			f, err := excelize.OpenFile(out)
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			if c.Name == "supplemental" {
				if f.GetActiveSheetIndex() != 1 {
					t.Error("active sheet lost")
				}
				visible, err := f.GetSheetVisible("Hidden")
				if err != nil || visible {
					t.Errorf("hidden sheet: %v %v", visible, err)
				}
				if len(f.GetDefinedName()) != 2 {
					t.Error("defined names lost")
				}
			}
		})
	}
}
