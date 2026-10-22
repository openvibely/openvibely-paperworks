package pptx

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/beevik/etree"
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
func TestPresentationResourcePathsAndDefaultTextSerialization(t *testing.T) {
	root := t.TempDir()
	theme := []byte(`<a:theme xmlns:a="` + ans + `" name="Original bytes"><a:themeElements/></a:theme>`)
	master := []byte(`<p:sldMaster xmlns:p="` + pns + `" xmlns:r="` + rns + `"><p:sldLayoutIdLst><p:sldLayoutId id="2147483649" r:id="rId1"/></p:sldLayoutIdLst></p:sldMaster>`)
	defaultText := []byte(`<p:defaultTextStyle xmlns:p="` + pns + `"><p:defPPr marL="123"><p:defRPr sz="1800"/></p:defPPr></p:defaultTextStyle>`)
	for name, data := range map[string][]byte{"theme.xml": theme, "master.xml": master, "default-text.xml": defaultText} {
		if err := os.WriteFile(filepath.Join(root, name), data, 0600); err != nil {
			t.Fatal(err)
		}
	}

	out := filepath.Join(root, "resources.pptx")
	spec := Spec{StyleResources: &StyleResources{
		Theme:       "theme.xml",
		Master:      "master.xml",
		DefaultText: filepath.Join(root, "default-text.xml"),
	}, Slides: []Slide{{}}}
	if err := Create(context.Background(), spec, root, out); err != nil {
		t.Fatal(err)
	}
	pkg, err := opc.Open(out)
	if err != nil {
		t.Fatal(err)
	}
	if got := pkg.Get("ppt/theme/theme1.xml"); !bytes.Equal(got, theme) {
		t.Fatal("theme resource bytes changed")
	}
	if got := pkg.Get("ppt/slideMasters/slideMaster1.xml"); !bytes.Equal(got, master) {
		t.Fatal("master resource bytes changed")
	}
	presentation, err := pkg.XML("ppt/presentation.xml")
	if err != nil {
		t.Fatal(err)
	}
	fragment := presentation.Root().SelectElement("p:defaultTextStyle")
	if fragment == nil {
		t.Fatal("defaultTextStyle fragment missing")
	}
	paragraph := fragment.SelectElement("p:defPPr")
	if paragraph == nil || paragraph.SelectAttrValue("marL", "") != "123" {
		t.Fatal("defaultTextStyle fragment was not serialized")
	}
}

func TestPresentationResourceValidation(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
	}{
		{name: "missing"},
		{name: "malformed", body: "<"},
		{name: "wrong-root", body: `<a:clrScheme xmlns:a="` + ans + `"/>`},
		{name: "wrong-namespace", body: `<theme/>`},
		{name: "relationship", body: `<a:theme xmlns:a="` + ans + `" xmlns:r="` + rns + `"><a:blip r:embed="missing"/></a:theme>`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			if tc.body != "" {
				if err := os.WriteFile(filepath.Join(root, "style.xml"), []byte(tc.body), 0600); err != nil {
					t.Fatal(err)
				}
			}
			out := filepath.Join(root, "bad.pptx")
			err := Create(context.Background(), Spec{StyleResources: &StyleResources{Theme: "style.xml"}, Slides: []Slide{{}}}, root, out)
			if err == nil {
				t.Fatal("invalid resource accepted")
			}
			if _, err = os.Stat(out); !os.IsNotExist(err) {
				t.Fatal("published failure", err)
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

func TestInheritedTableCellMatchesInheritedShapeText(t *testing.T) {
	text := "First paragraph\nSecond paragraph"
	shape := Element{Type: "text", X: 1, Y: 1, W: 3, H: 1, Text: text, FontSize: 22, Color: "123456", Bold: true, InheritFormatting: true}
	table := Element{Type: "table", X: 1, Y: 3, W: 3, H: 1, Rows: [][]string{{text}}, FontSize: 22, Color: "123456", Bold: true, InheritFormatting: true}
	out := filepath.Join(t.TempDir(), "inherited-table.pptx")
	if err := Create(context.Background(), Spec{Slides: []Slide{{Elements: []Element{shape, table}}}}, ".", out); err != nil {
		t.Fatal(err)
	}
	pkg, err := opc.Open(out)
	if err != nil {
		t.Fatal(err)
	}
	slide, err := pkg.XML("ppt/slides/slide1.xml")
	if err != nil {
		t.Fatal(err)
	}
	shapeBodies := office.All(slide.Root(), pns, "txBody")
	cellBodies := office.All(slide.Root(), ans, "txBody")
	if len(shapeBodies) != 1 || len(cellBodies) != 1 {
		t.Fatalf("shape text bodies=%d table cell text bodies=%d", len(shapeBodies), len(cellBodies))
	}
	for _, body := range []*etree.Element{shapeBodies[0], cellBodies[0]} {
		bodyPr := office.Child(body, ans, "bodyPr")
		paragraphs := office.Children(body, ans, "p")
		if bodyPr == nil || len(bodyPr.Attr) != 0 || len(paragraphs) != 2 || len(office.All(body, ans, "endParaRPr")) != 0 {
			t.Fatal("inherited body acquired authored body defaults")
		}
		for _, paragraph := range paragraphs {
			if office.Child(paragraph, ans, "pPr") != nil {
				t.Fatal("unexpected paragraph defaults")
			}
			run := office.Child(paragraph, ans, "r")
			rPr := office.Child(run, ans, "rPr")
			if run == nil || rPr == nil || rPr.SelectAttrValue("sz", "") != "2200" || rPr.SelectAttrValue("b", "") != "1" || rPr.SelectAttrValue("lang", "") != "" || office.Child(rPr, ans, "latin") != nil {
				t.Fatal("inherited run styling differs from inherited shape policy")
			}
			if office.Child(rPr, ans, "solidFill").SelectElement("a:srgbClr").SelectAttrValue("val", "") != "123456" {
				t.Fatal("inherited color lost")
			}
		}
	}
	for _, body := range []*etree.Element{shapeBodies[0], cellBodies[0]} {
		texts := office.All(body, ans, "t")
		if len(texts) != 2 || texts[0].Text() != "First paragraph" || texts[1].Text() != "Second paragraph" {
			t.Fatal("inherited paragraph text differs between shape and table cell")
		}
	}
}
