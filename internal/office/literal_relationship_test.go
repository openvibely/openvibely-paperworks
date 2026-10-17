package office

import (
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"testing"
)

func TestLiteralWorkbookRelationshipCollisions(t *testing.T) {
	for _, prefix := range []string{"", "r"} {
		t.Run("prefix-"+prefix, func(t *testing.T) {
			p, err := opc.Open("../../testdata/reference-mirror/literal-chart.pptx")
			if err != nil {
				t.Fatal(err)
			}
			chart := "ppt/charts/chart1.xml"
			tag, ns := "", "xmlns"
			if prefix != "" {
				tag = prefix + ":"
				ns += ":" + prefix
			}
			p.Set(opc.RelsName(chart), []byte(`<`+tag+`Relationships `+ns+`="`+opc.NSRel+`"><`+tag+`Relationship Id="literalWorkbook" Type="keep" Target="a"/><`+tag+`Relationship Id="literalWorkbook1" Type="keep" Target="b"/></`+tag+`Relationships>`))
			id, err := literalWorkbookRelationship(p, chart, "../embeddings/data.xlsx")
			if err != nil || id != "literalWorkbook11" {
				t.Fatalf("%q: %v", id, err)
			}
			rels, err := p.Rels(chart)
			if err != nil || len(rels) != 3 {
				t.Fatalf("%v: %v", rels, err)
			}
			if rels[0].ID != "literalWorkbook" || rels[0].Target != "a" || rels[1].ID != "literalWorkbook1" || rels[1].Target != "b" || rels[2].ID != id || rels[2].Type != opc.NSOffR+"/package" || rels[2].Target != "../embeddings/data.xlsx" {
				t.Fatalf("relationships changed: %+v", rels)
			}
		})
	}
}
