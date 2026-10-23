package pptx

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/beevik/etree"
	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
)

func paintFixture(t *testing.T, element Element) (string, *opc.Package) {
	t.Helper()
	input := filepath.Join(t.TempDir(), "source.pptx")
	if err := Create(context.Background(), Spec{Slides: []Slide{{Elements: []Element{element}}}}, filepath.Dir(input), input); err != nil {
		t.Fatal(err)
	}
	p, err := opc.Open(input)
	if err != nil {
		t.Fatal(err)
	}
	return input, p
}
func savePaintFixture(t *testing.T, p *opc.Package, part string, d *etree.Document) string {
	t.Helper()
	if err := p.SetXML(part, d); err != nil {
		t.Fatal(err)
	}
	name := filepath.Join(t.TempDir(), "imported.pptx")
	if err := p.SaveNew(name); err != nil {
		t.Fatal(err)
	}
	return name
}
func editedPaintPart(t *testing.T, input, part string, op Edit) *etree.Document {
	t.Helper()
	output := filepath.Join(t.TempDir(), "edited.pptx")
	if _, err := EditFile(context.Background(), input, output, EditSpec{Edits: []Edit{op}}); err != nil {
		t.Fatal(err)
	}
	p, err := opc.Open(output)
	if err != nil {
		t.Fatal(err)
	}
	d, err := p.XML(part)
	if err != nil {
		t.Fatal(err)
	}
	return d
}
func TestChartRetitleWithImportedPrefix(t *testing.T) {
	input, p := paintFixture(t, Element{Type: "chart", W: 8, H: 5, Chart: &Chart{Type: "bar", Categories: []string{"A", "B"}, Series: []Series{{Name: "Sales", Values: []float64{10, 20}}}}})
	part := "ppt/charts/chart1.xml"
	d, err := p.XML(part)
	if err != nil {
		t.Fatal(err)
	}
	id := office.Child(office.All(d.Root(), cns, "valAx")[0], cns, "axId").SelectAttrValue("val", "")
	title := "Original"
	op := Edit{Slide: 1, ShapeID: "2", Operation: "chart-format", ChartFormat: &ChartFormat{Axes: []ChartAxisFormat{{ID: id, Title: &title}}}}
	d = editedPaintPart(t, input, part, op)
	raw, err := d.WriteToString()
	if err != nil {
		t.Fatal(err)
	}
	raw = strings.NewReplacer("xmlns:c=", "xmlns:chart=", "<c:", "<chart:", "</c:", "</chart:").Replace(raw)
	d, err = opc.ParseXML([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	input = savePaintFixture(t, p, part, d)
	title = "Updated revenue"
	d = editedPaintPart(t, input, part, op)
	axis := office.All(d.Root(), cns, "valAx")[0]
	rich := office.Child(office.Child(office.Child(axis, cns, "title"), cns, "tx"), cns, "rich")
	if rich == nil {
		t.Fatal("title rich text has an unbound chart namespace")
	}
	texts := office.All(rich, ans, "t")
	if len(texts) != 1 || texts[0].Text() != title {
		t.Fatal("title was not updated")
	}
}
func TestTableTextPaintWithImportedPrefix(t *testing.T) {
	_, p := paintFixture(t, Element{Type: "table", W: 5, H: 2, Rows: [][]string{{"Keep me", "Unchanged"}}})
	part := "ppt/slides/slide1.xml"
	raw := strings.NewReplacer("xmlns:a=", "xmlns:draw=", "<a:", "<draw:", "</a:", "</draw:").Replace(string(p.Get(part)))
	d, err := opc.ParseXML([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	input := savePaintFixture(t, p, part, d)
	beforeCells := office.All(d.Root(), ans, "tc")
	untouched := beforeCells[1].Copy()
	d = editedPaintPart(t, input, part, Edit{Slide: 1, ShapeID: "2", Operation: "table-format", TableFormat: &TableFormat{Cells: []TableCellFormat{{Row: 0, Column: 0, TextPaint: &TextPaint{Fill: &DrawingStyle{Fill: "FF0000"}}}}}})
	cells := office.All(d.Root(), ans, "tc")
	pr := office.Child(office.All(cells[0], ans, "r")[0], ans, "rPr")
	color := office.Child(office.Child(pr, ans, "solidFill"), ans, "srgbClr")
	if color == nil || color.SelectAttrValue("val", "") != "FF0000" {
		t.Fatal("requested cell text color was not saved")
	}
	if office.All(cells[0], ans, "t")[0].Text() != "Keep me" {
		t.Fatal("cell text changed")
	}
	a, b := etree.NewDocument(), etree.NewDocument()
	a.SetRoot(untouched)
	b.SetRoot(cells[1].Copy())
	x, _ := a.WriteToString()
	y, _ := b.WriteToString()
	if x != y {
		t.Fatal("untargeted cell changed")
	}
}
func TestGlyphFillPreservesUnderlinePaint(t *testing.T) {
	for _, operation := range []string{"text-paint", "run-format"} {
		t.Run(operation, func(t *testing.T) {
			_, p := paintFixture(t, Element{Type: "text", Text: "Underlined", W: 5, H: 2})
			part := "ppt/slides/slide1.xml"
			d, err := p.XML(part)
			if err != nil {
				t.Fatal(err)
			}
			pr := office.Child(office.All(d.Root(), ans, "r")[0], ans, "rPr")
			pr.CreateAttr("u", "sng")
			underline := etree.NewElement("a:uFill")
			underline.CreateElement("a:solidFill").CreateElement("a:srgbClr").CreateAttr("val", "00FF00")
			// Underline paint precedes font declarations in CT_TextCharacterProperties.
			latin := office.Child(pr, ans, "latin")
			if latin == nil {
				t.Fatal("fixture font missing")
			}
			pr.InsertChildAt(latin.Index(), underline)
			input := savePaintFixture(t, p, part, d)
			d = editedPaintPart(t, input, part, Edit{Slide: 1, ShapeID: "2", Operation: operation, TextPaint: &TextPaint{Fill: &DrawingStyle{Fill: "FF0000"}}})
			pr = office.Child(office.All(d.Root(), ans, "r")[0], ans, "rPr")
			fill := office.Child(office.Child(pr, ans, "solidFill"), ans, "srgbClr")
			underlineColor := office.Child(office.Child(office.Child(pr, ans, "uFill"), ans, "solidFill"), ans, "srgbClr")
			if fill == nil || fill.SelectAttrValue("val", "") != "FF0000" {
				t.Fatal("glyph fill not saved")
			}
			if underlineColor == nil || underlineColor.SelectAttrValue("val", "") != "00FF00" || pr.SelectAttrValue("u", "") != "sng" {
				t.Fatal("underline formatting lost")
			}
		})
	}
}

func TestRunPaintCreatesNamespacedProperties(t *testing.T) {
	_, p := paintFixture(t, Element{Type: "text", Text: "Keep me", W: 4, H: 2})
	part := "ppt/slides/slide1.xml"
	raw := strings.NewReplacer("xmlns:a=", "xmlns:draw=", "<a:", "<draw:", "</a:", "</draw:").Replace(string(p.Get(part)))
	d, err := opc.ParseXML([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	run := office.All(d.Root(), ans, "r")[0]
	if pr := office.Child(run, ans, "rPr"); pr != nil {
		run.RemoveChild(pr)
	}
	input := savePaintFixture(t, p, part, d)
	d = editedPaintPart(t, input, part, Edit{Slide: 1, ShapeID: "2", Operation: "run-format", TextPaint: &TextPaint{Fill: &DrawingStyle{Fill: "FF0000"}}})
	run = office.All(d.Root(), ans, "r")[0]
	pr := office.Child(run, ans, "rPr")
	if pr == nil {
		t.Fatal("new run properties have an unbound drawing namespace")
	}
	color := office.Child(office.Child(pr, ans, "solidFill"), ans, "srgbClr")
	if color == nil || color.SelectAttrValue("val", "") != "FF0000" {
		t.Fatal("requested run color was not saved")
	}
	if office.Child(run, ans, "t").Text() != "Keep me" {
		t.Fatal("run text changed")
	}
}
