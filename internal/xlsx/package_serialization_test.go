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
)

func TestPackageReferenceSerialization(t *testing.T) {
	raw, err := os.ReadFile("testdata/package-reference.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Cases []struct {
			Name  string
			Parts map[string]string
		}
	}
	if err = json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	for _, c := range fixture.Cases {
		t.Run(c.Name, func(t *testing.T) {
			var s Spec
			base := "../../testdata/document-examples/document_spreadsheets/examples"
			if c.Name == "installed" {
				raw, err = os.ReadFile(filepath.Join(base, "report.json"))
				if err != nil {
					t.Fatal(err)
				}
				if err = json.Unmarshal(raw, &s); err != nil {
					t.Fatal(err)
				}
			} else {
				for _, name := range []string{"First", "Second"} {
					s.Sheets = append(s.Sheets, Sheet{Name: name, Rows: [][]any{{"Name", "Value"}, {"A", 2}, {"B", 3}}, ColumnCharts: []ColumnChart{{Cell: "D2", WidthEMU: 5400000, HeightEMU: 2700000, Series: []ColumnSeries{{Name: "'" + name + "'!B1", Categories: "'" + name + "'!$A$2:$A$3", Values: "'" + name + "'!$B$2:$B$3"}}}}})
				}
			}
			out := filepath.Join(t.TempDir(), "new.xlsx")
			if err = Create(context.Background(), s, base, out); err != nil {
				t.Fatal(err)
			}
			p, err := opc.Open(out)
			if err != nil {
				t.Fatal(err)
			}
			for n, want := range c.Parts {
				if got := fmt.Sprintf("%x", sha256.Sum256(p.Get(n))); got != want {
					t.Errorf("%s got %s want %s\n%s", n, got, want, p.Get(n))
				}
			}
			// External links and unrelated content types survive a second serialization.
			rd, err := p.XML("xl/worksheets/_rels/sheet1.xml.rels")
			if err != nil {
				t.Fatal(err)
			}
			e := rd.Root().CreateElement("Relationship")
			e.CreateAttr("Id", "rId99")
			e.CreateAttr("Type", opc.NSOffR+"/hyperlink")
			e.CreateAttr("Target", "https://example.org/a?x=1&y=2")
			e.CreateAttr("TargetMode", "External")
			if err = p.SetXML("xl/worksheets/_rels/sheet1.xml.rels", rd); err != nil {
				t.Fatal(err)
			}
			ct, err := p.XML("[Content_Types].xml")
			if err != nil {
				t.Fatal(err)
			}
			e = ct.Root().CreateElement("Override")
			e.CreateAttr("PartName", "/custom/data.xml")
			e.CreateAttr("ContentType", "application/x-test")
			if err = p.SetXML("[Content_Types].xml", ct); err != nil {
				t.Fatal(err)
			}
			if err = serializePackageGraph(p); err != nil {
				t.Fatal(err)
			}
			rd, err = p.XML("xl/worksheets/_rels/sheet1.xml.rels")
			if err != nil {
				t.Fatal(err)
			}
			e = rd.Root().FindElement("Relationship[@Id='rId99']")
			if e == nil || e.SelectAttrValue("Target", "") != "https://example.org/a?x=1&y=2" || e.SelectAttrValue("TargetMode", "") != "External" {
				t.Fatal("external link changed")
			}
			ct, err = p.XML("[Content_Types].xml")
			if err != nil {
				t.Fatal(err)
			}
			if ct.Root().FindElement("Override[@PartName='/custom/data.xml']") == nil {
				t.Fatal("unknown content type lost")
			}
		})
	}
}
