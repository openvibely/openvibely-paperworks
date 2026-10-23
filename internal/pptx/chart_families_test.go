package pptx

import (
	"context"
	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"path/filepath"
	"testing"
)

func TestAdditionalStandardCharts(t *testing.T) {
	for kind, tag := range map[string]string{"surface": "surfaceChart", "surface3D": "surface3DChart", "ofPie": "ofPieChart", "barOfPie": "ofPieChart"} {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			out := filepath.Join(dir, "deck.pptx")
			if err := Create(context.Background(), Spec{Slides: []Slide{{Elements: []Element{{Type: "chart", W: 8, H: 5, Chart: &Chart{Type: kind, Categories: []string{"A", "B", "C"}, Series: []Series{{Name: "Values", Values: []float64{10, 20, 5}}}}}}}}}, dir, out); err != nil {
				t.Fatal(err)
			}
			p, err := opc.Open(out)
			if err != nil {
				t.Fatal(err)
			}
			d, err := p.XML("ppt/charts/chart1.xml")
			if err != nil {
				t.Fatal(err)
			}
			if len(office.All(d.Root(), cns, tag)) != 1 {
				t.Fatal("wrong native chart type")
			}
			if len(office.All(d.Root(), cns, "numCache")) == 0 {
				t.Fatal("missing chart cache")
			}
		})
	}
}
