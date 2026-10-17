package office

import (
	"bytes"
	"encoding/json"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"os"
	"testing"
)

func TestPinnedNotesRepairSerialization(t *testing.T) {
	raw, err := os.ReadFile("testdata/notes-repair-serialization.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Cases []struct{ Name, XML, Rels, Expected string }
	}
	if err = json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	for _, c := range fixture.Cases {
		t.Run(c.Name, func(t *testing.T) {
			p, err := opc.Open(referenceNotesFixture(t))
			if err != nil {
				t.Fatal(err)
			}
			p.Set("ppt/presentation.xml", []byte(c.XML))
			p.Set("ppt/_rels/presentation.xml.rels", []byte(c.Rels))
			before := map[string][]byte{}
			for _, part := range p.Parts {
				before[part.Name] = append([]byte(nil), part.Data...)
			}
			if err = RepairNotesPackage(p); err != nil {
				t.Fatal(err)
			}
			if got := string(p.Get("ppt/presentation.xml")); got != c.Expected {
				t.Fatalf("repaired XML differs from pinned helper\ngot %s\nwant %s", got, c.Expected)
			}
			for n, b := range before {
				if n != "ppt/presentation.xml" && !bytes.Equal(b, p.Get(n)) {
					t.Fatalf("unrelated part changed: %s", n)
				}
			}
			if err = RepairNotesPackage(p); err != nil {
				t.Fatal(err)
			}
			if string(p.Get("ppt/presentation.xml")) != c.Expected {
				t.Fatal("repeat repair changed payload")
			}
		})
	}
}
