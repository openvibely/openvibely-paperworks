package pptx

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"github.com/xuri/excelize/v2"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInheritedPresentationAuthoring(t *testing.T) {
	base := "../../testdata/document-examples/document_presentations/examples"
	raw, err := os.ReadFile(filepath.Join(base, "revenue-slide.json"))
	if err != nil {
		t.Fatal(err)
	}
	var s Spec
	if err = json.Unmarshal(raw, &s); err != nil {
		t.Fatal(err)
	}
	s.Slides[0].Elements[0].Text = "Title < & >"
	s.Slides[0].Elements[1].Chart.Series = append(s.Slides[0].Elements[1].Chart.Series, Series{Name: "Second & series", Values: []float64{45.5, -10}})
	out := filepath.Join(t.TempDir(), "deck.pptx")
	if err = Create(context.Background(), s, base, out); err != nil {
		t.Fatal(err)
	}
	p, err := opc.Open(out)
	if err != nil {
		t.Fatal(err)
	}
	for _, part := range p.Parts {
		if filepath.Ext(part.Name) == ".rels" {
			rs, err := p.Rels(opc.SourceOf(part.Name))
			if err != nil {
				t.Fatal(err)
			}
			for _, r := range rs {
				if r.TargetMode != "External" && !p.Has(opc.Resolve(opc.SourceOf(part.Name), r.Target)) {
					t.Fatalf("dangling relation %s %s", part.Name, r.Target)
				}
			}
		}
	}
	slide, err := p.XML("ppt/slides/slide1.xml")
	if err != nil {
		t.Fatal(err)
	}
	if office.All(slide.Root(), ans, "t")[0].Text() != "Title < & >" {
		t.Fatal("text escaping")
	}
	for _, r := range office.All(slide.Root(), ans, "rPr") {
		if r.SelectAttrValue("sz", "") == "" || len(r.ChildElements()) != 0 {
			t.Fatal("explicit font/color defeats inheritance")
		}
	}
	chart, err := p.XML("ppt/charts/chart1.xml")
	if err != nil {
		t.Fatal(err)
	}
	series := office.All(chart.Root(), cns, "ser")
	if len(series) != 2 {
		t.Fatal("series lost")
	}
	vals := office.All(office.Child(series[1], cns, "val"), cns, "v")
	if len(vals) != 2 || vals[0].Text() != "45.5" || vals[1].Text() != "-10" {
		t.Fatal("cache differs")
	}
	workbook, err := opc.Read(p.Get("ppt/embeddings/Microsoft_Excel_Sheet1.xlsx"))
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range workbook.Names() {
		if strings.Contains(name, "drawings/") || strings.Contains(name, "charts/") {
			t.Fatalf("embedded data workbook contains chart resource %s", name)
		}
	}
	f, err := excelize.OpenReader(bytes.NewReader(p.Get("ppt/embeddings/Microsoft_Excel_Sheet1.xlsx")))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	for cell, want := range map[string]string{"C1": "Second & series", "C2": "45.5", "C3": "-10"} {
		got, err := f.GetCellValue("Sheet1", cell)
		if err != nil || got != want {
			t.Fatalf("%s: %q %v", cell, got, err)
		}
	}
	if len(office.All(chart.Root(), cns, "legend")) != 0 {
		t.Fatal("unexpected legend")
	}
}
func TestPresentationResourceValidation(t *testing.T) {
	for _, body := range []string{"<bad/>", "<", `<a:theme xmlns:a="` + ans + `" xmlns:r="` + rns + `"><a:blip r:embed="missing"/></a:theme>`} {
		t.Run(body[:1]+string(rune(len(body))), func(t *testing.T) {
			root := t.TempDir()
			if err := os.WriteFile(filepath.Join(root, "style.xml"), []byte(body), 0600); err != nil {
				t.Fatal(err)
			}
			out := filepath.Join(root, "bad.pptx")
			err := Create(context.Background(), Spec{StyleResources: &StyleResources{Theme: "style.xml"}, Slides: []Slide{{}}}, root, out)
			if err == nil {
				t.Fatal("invalid resource accepted")
			}
			if _, err = os.Stat(out); !os.IsNotExist(err) {
				t.Fatal("published failure")
			}
		})
	}
}
func TestInheritedChartRejectsUnsupportedInputs(t *testing.T) {
	for _, c := range []Chart{
		{InheritFormatting: true, Type: "bar", Categories: []string{"A"}, Series: []Series{{Name: "X", Values: []float64{1}}}},
		{InheritFormatting: true, Type: "column", Title: "not supported", Categories: []string{"A"}, Series: []Series{{Name: "X", Values: []float64{1}}}},
		{InheritFormatting: true, Type: "column", Categories: []string{"A"}, Series: []Series{{Name: "X", Values: []float64{math.NaN()}}}},
	} {
		out := filepath.Join(t.TempDir(), "bad.pptx")
		err := Create(context.Background(), Spec{Slides: []Slide{{Elements: []Element{{Type: "chart", W: 5, H: 3, Chart: &c}}}}}, "", out)
		if err == nil {
			t.Fatal("bad chart accepted")
		}
		if _, err = os.Stat(out); !os.IsNotExist(err) {
			t.Fatal("published failure")
		}
	}
}
