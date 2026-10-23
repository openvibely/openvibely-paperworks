package pptx

import (
	"bytes"
	"context"
	"github.com/beevik/etree"
	"path/filepath"
	"testing"

	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
)

func TestChartFormatPreservesDataAndWorkbook(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "in.pptx")
	out := filepath.Join(dir, "out.pptx")
	if err := Create(context.Background(), Spec{Slides: []Slide{{Elements: []Element{{Type: "chart", W: 8, H: 5, Chart: &Chart{Type: "bar", Categories: []string{"A", "B"}, Series: []Series{{Name: "Sales", Values: []float64{10, 20}}}}}}}}}, dir, in); err != nil {
		t.Fatal(err)
	}
	p, err := opc.Open(in)
	if err != nil {
		t.Fatal(err)
	}
	d, err := p.XML("ppt/charts/chart1.xml")
	if err != nil {
		t.Fatal(err)
	}
	axis := office.All(d.Root(), cns, "valAx")[0]
	id := office.Child(axis, cns, "axId").SelectAttrValue("val", "")
	yes := true
	lo := 0.0
	hi := 100.0
	title := "Revenue"
	fmtCode := "$0"
	change := ChartFormat{Axes: []ChartAxisFormat{{ID: id, Minimum: &lo, Maximum: &hi, NumberFormat: &fmtCode, Title: &title}}, Legend: &ChartLegendFormat{Position: "r"}, Series: []ChartSeriesFormat{{Index: 0, Fill: &DrawingStyle{Fill: "FF0000"}, Labels: &ChartLabelsFormat{ShowValue: &yes}}}}
	if _, err = EditFile(context.Background(), in, out, EditSpec{Edits: []Edit{{Slide: 1, ShapeID: "2", Operation: "chart-format", ChartFormat: &change}}}); err != nil {
		t.Fatal(err)
	}
	q, err := opc.Open(out)
	if err != nil {
		t.Fatal(err)
	}
	for _, part := range p.Parts {
		if part.Name != "ppt/charts/chart1.xml" && !bytes.Equal(part.Data, q.Get(part.Name)) {
			t.Fatal("unrelated part changed", part.Name)
		}
	}
	after, err := q.XML("ppt/charts/chart1.xml")
	if err != nil {
		t.Fatal(err)
	}
	for _, tag := range []string{"cat", "val", "externalData"} {
		old := office.All(d.Root(), cns, tag)
		now := office.All(after.Root(), cns, tag)
		if len(old) != len(now) {
			t.Fatal("lost data", tag)
		}
		for i := range old {
			a := etree.NewDocument()
			a.SetRoot(old[i].Copy())
			b := etree.NewDocument()
			b.SetRoot(now[i].Copy())
			aa, _ := a.WriteToBytes()
			bb, _ := b.WriteToBytes()
			if !bytes.Equal(aa, bb) {
				t.Fatal("data changed", tag)
			}
		}
	}
	axis = office.All(after.Root(), cns, "valAx")[0]
	if office.Child(office.Child(axis, cns, "scaling"), cns, "max").SelectAttrValue("val", "") != "100" {
		t.Fatal("maximum not saved")
	}
	if office.All(after.Root(), cns, "legendPos")[0].SelectAttrValue("val", "") != "r" {
		t.Fatal("legend not saved")
	}
	if office.All(after.Root(), cns, "showVal")[0].SelectAttrValue("val", "") != "1" {
		t.Fatal("labels not saved")
	}
	if _, err = office.VerifyChartData(q, "powerpoint"); err != nil {
		t.Fatal(err)
	}
	bad := filepath.Join(dir, "bad.pptx")
	change.Axes[0].Minimum = &hi
	if _, err = EditFile(context.Background(), in, bad, EditSpec{Edits: []Edit{{Slide: 1, ShapeID: "2", Operation: "chart-format", ChartFormat: &change}}}); err == nil {
		t.Fatal("invalid bounds accepted")
	}
}
