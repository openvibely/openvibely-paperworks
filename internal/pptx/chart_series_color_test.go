package pptx

import (
	"context"
	"encoding/json"
	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"os"
	"path/filepath"
	"testing"
)

func TestAuthoredBarChartsRetainSeriesLegend(t *testing.T) {
	data, err := os.ReadFile("../../testdata/document-examples/document_presentations/examples/revenue-slide.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"column", "bar"} {
		t.Run(kind, func(t *testing.T) {
			var spec Spec
			if err := json.Unmarshal(data, &spec); err != nil {
				t.Fatal(err)
			}
			for i := range spec.Slides[0].Elements {
				if c := spec.Slides[0].Elements[i].Chart; c != nil {
					c.Type = kind
					c.InheritFormatting = false // This regression covers the existing Excelize authoring path.
				}
			}
			dst := filepath.Join(t.TempDir(), "chart.pptx")
			if err := Create(context.Background(), spec, "../../testdata/document-examples/document_presentations/examples", dst); err != nil {
				t.Fatal(err)
			}
			p, err := opc.Open(dst)
			if err != nil {
				t.Fatal(err)
			}
			slide, err := p.XML("ppt/slides/slide1.xml")
			if err != nil {
				t.Fatal(err)
			}
			styles := office.All(slide.Root(), ans, "tableStyleId")
			if len(styles) != 1 || styles[0].Text() != "{5C22544A-7EE6-4342-B048-85BDC9FD1C3A}" {
				t.Fatal("missing pinned default table style")
			}
			d, err := p.XML("ppt/charts/chart1.xml")
			if err != nil {
				t.Fatal(err)
			}
			text := office.Child(d.Root(), office.NSC, "txPr")
			paragraph := office.Child(text, ans, "p")
			defaults := office.Child(office.Child(paragraph, ans, "pPr"), ans, "defRPr")
			if defaults == nil || defaults.SelectAttrValue("sz", "") != "1800" {
				t.Fatal("missing pinned 18-point chart text default")
			}
			plots := office.All(d.Root(), office.NSC, "barChart")
			if len(plots) != 1 {
				t.Fatalf("bar plots %d, want 1", len(plots))
			}
			for _, plot := range plots {
				if office.Child(plot, office.NSC, "varyColors") != nil {
					t.Fatal("unexpected category-varying colors")
				}
			}
			names := office.All(d.Root(), office.NSC, "strCache")
			if len(names) == 0 || office.Child(office.Child(names[0], office.NSC, "pt"), office.NSC, "v").Text() != "Revenue (USD)" {
				t.Fatal("series name lost")
			}
		})
	}
}
