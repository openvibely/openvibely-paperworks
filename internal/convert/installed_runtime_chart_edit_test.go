package convert

import (
	"context"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/beevik/etree"
	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"github.com/openvibely/openvibely-paperworks/internal/pdf"
	"github.com/openvibely/openvibely-paperworks/internal/pptx"
)

// Runtime 123-146: edit the actual installed example, never a replacement
// fixture. Reader scope and execution evidence are tracked in Batch42.
func referenceRuntimeChartEdit(t *testing.T, ctx context.Context, source string) {
	t.Helper()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	p, err := opc.Open(source)
	must(err)
	slides := referenceSkillSlides(t, p)
	if len(slides) == 0 {
		t.Fatal("example has no slides")
	}
	d, err := p.XML(slides[0])
	must(err)
	tree := office.Child(office.Child(d.Root(), office.NSP, "cSld"), office.NSP, "spTree")
	shapeCount := func(tree *etree.Element) int {
		n := 0
		for _, e := range tree.ChildElements() {
			if e.NamespaceURI() != office.NSP {
				continue
			}
			switch e.Tag {
			case "sp", "grpSp", "graphicFrame", "cxnSp", "pic", "contentPart":
				n++
			}
		}
		return n
	}
	originalCount := shapeCount(tree)
	title := func(root *etree.Element) *etree.Element {
		// SlideShapes.title selects the first top-level placeholder with idx=0,
		// not any descendant whose placeholder type happens to be title.
		tree := office.Child(office.Child(root, office.NSP, "cSld"), office.NSP, "spTree")
		for _, sp := range tree.ChildElements() {
			if sp.NamespaceURI() != office.NSP {
				continue
			}
			children := sp.ChildElements()
			if len(children) == 0 {
				continue
			}
			ph := office.Child(office.Child(children[0], office.NSP, "nvPr"), office.NSP, "ph")
			if ph != nil {
				idx, e := strconv.Atoi(ph.SelectAttrValue("idx", "0"))
				must(e)
				if idx == 0 {
					return sp
				}
			}
		}
		t.Fatal("original first slide must have a title placeholder")
		return nil
	}
	firstTitle := title(d.Root())
	firstRun := func(sp *etree.Element) *etree.Element {
		body := office.Child(sp, office.NSP, "txBody")
		paras := office.Children(body, office.NSA, "p")
		if len(paras) == 0 {
			t.Fatal("title paragraph missing")
		}
		runs := office.Children(paras[0], office.NSA, "r")
		if len(runs) == 0 {
			t.Fatal("title run missing")
		}
		return runs[0]
	}
	fontSize := func(sp *etree.Element) string {
		props := office.Child(firstRun(sp), office.NSA, "rPr")
		if props == nil {
			return ""
		}
		return props.SelectAttrValue("sz", "")
	}
	originalFont := fontSize(firstTitle)
	shapeID := func(sp *etree.Element) string {
		props := office.All(sp, office.NSP, "cNvPr")
		if len(props) != 1 {
			t.Fatal("ambiguous shape identity")
		}
		return props[0].SelectAttrValue("id", "")
	}
	notesText := func(pkg *opc.Package, slide string) string {
		return referenceSkillNotes(t, pkg, slide)
	}

	chartPart := func(pkg *opc.Package, slide string) string {
		doc, e := pkg.XML(slide)
		must(e)
		tree := office.Child(office.Child(doc.Root(), office.NSP, "cSld"), office.NSP, "spTree")
		var selected *etree.Element
		for _, frame := range office.Children(tree, office.NSP, "graphicFrame") {
			data := office.Child(office.Child(frame, office.NSA, "graphic"), office.NSA, "graphicData")
			if data != nil && data.SelectAttrValue("uri", "") == office.NSC {
				selected = office.Child(data, office.NSC, "chart")
				break
			}
		}
		if selected == nil {
			t.Fatal("first slide has no top-level chart")
		}
		id := office.AttrV(selected, office.NSR, "id")
		rels, e := pkg.Rels(slide)
		must(e)
		for _, r := range rels {
			if r.ID == id {
				return opc.Resolve(slide, r.Target)
			}
		}
		t.Fatal("chart relationship missing")
		return ""
	}
	chartType := func(pkg *opc.Package, part string) []string {
		doc, e := pkg.XML(part)
		must(e)
		plots := office.All(doc.Root(), office.NSC, "plotArea")
		if len(plots) != 1 {
			t.Fatal("chart plot missing")
		}
		for _, el := range plots[0].ChildElements() {
			if strings.HasSuffix(el.Tag, "Chart") {
				signature := []string{el.Tag}
				for _, name := range []string{"barDir", "grouping", "scatterStyle", "radarStyle"} {
					if v := office.Child(el, office.NSC, name); v != nil {
						signature = append(signature, name, v.SelectAttrValue("val", ""))
					}
				}
				return signature
			}
		}
		t.Fatal("chart type missing")
		return nil
	}
	originalNotes := notesText(p, slides[0])
	originalPart := chartPart(p, slides[0])
	originalType := chartType(p, originalPart)
	edits := []pptx.Edit{{Slide: 1, ShapeID: shapeID(firstTitle), Operation: "run-text", Text: "Subscriptions generated 67% of revenue"}, {Slide: 1, Operation: "notes", Text: strings.ReplaceAll(originalNotes, "Total: $200.", "Total: $270.")}}
	for _, frame := range office.Children(tree, office.NSP, "graphicFrame") {
		data := office.Child(office.Child(frame, office.NSA, "graphic"), office.NSA, "graphicData")
		if data != nil && data.SelectAttrValue("uri", "") == "http://schemas.openxmlformats.org/drawingml/2006/table" {
			for i, value := range []string{"$180", "$90"} {
				edits = append(edits, pptx.Edit{Slide: 1, ShapeID: shapeID(frame), Operation: "table-cell", Row: i + 1, Column: 1, Text: value})
			}
		}
	}
	dir := t.TempDir()
	textEdited := filepath.Join(dir, "text-edited.pptx")
	_, err = pptx.EditFile(ctx, source, textEdited, pptx.EditSpec{Edits: edits})
	must(err)
	// Reference chart.replace_data replaces the entire embedded workbook. The
	// literal-input editor intentionally preserves it and is not equivalent here.
	replacement, err := opc.Open(textEdited)
	must(err)
	must(office.ReplaceCategorySeries(replacement, originalPart, "Revenue (USD)", []string{"Subscriptions", "Services"}, []float64{180, 90}))
	output := filepath.Join(dir, "chart-edited.pptx")
	must(replacement.SaveNew(output))
	saved, err := opc.Open(output)
	must(err)
	savedSlides := referenceSkillSlides(t, saved)
	if len(savedSlides) == 0 {
		t.Fatal("edited deck has no slides")
	}
	edited, err := saved.XML(savedSlides[0])
	must(err)
	t.Run("shape-count", func(t *testing.T) {
		count := shapeCount(office.Child(office.Child(edited.Root(), office.NSP, "cSld"), office.NSP, "spTree"))
		if len(savedSlides) != 1 || count != originalCount {
			t.Errorf("slides=%d shapes=%d; want 1/%d", len(savedSlides), count, originalCount)
		}
	})
	t.Run("title", func(t *testing.T) {
		text := referenceRuntimeFrameText(office.Child(title(edited.Root()), office.NSP, "txBody"))
		if text != "Subscriptions generated 67% of revenue" {
			t.Errorf("title=%q", text)
		}
	})
	t.Run("title-font", func(t *testing.T) {
		if got := fontSize(title(edited.Root())); got != originalFont {
			t.Errorf("font=%q want %q", got, originalFont)
		}
	})
	t.Run("chart", func(t *testing.T) {
		part := chartPart(saved, savedSlides[0])
		if got := chartType(saved, part); !reflect.DeepEqual(got, originalType) {
			t.Errorf("type=%v want %v", got, originalType)
		}
		doc, e := saved.XML(part)
		must(e)
		series := office.All(doc.Root(), office.NSC, "ser")
		if len(series) == 0 {
			t.Fatal("series missing")
		}
		value := office.Child(series[0], office.NSC, "val")
		if value == nil {
			t.Fatal("values missing")
		}
		values, e := referenceSkillSeriesValues(value)
		must(e)
		if len(values) != 2 || values[0] == nil || values[1] == nil || *values[0] != 180 || *values[1] != 90 {
			t.Errorf("indexed values=%v; want (180,90)", values)
		}
	})
	t.Run("notes", func(t *testing.T) {
		if got, want := notesText(saved, savedSlides[0]), strings.ReplaceAll(originalNotes, "Total: $200.", "Total: $270."); got != want {
			t.Errorf("notes=%q want %q", got, want)
		}
	})
	t.Run("render", func(t *testing.T) {
		images, e := RenderFile(ctx, output, filepath.Join(dir, "render"), "", "", pdf.RenderOptions{})
		if e != nil || len(images) == 0 {
			t.Errorf("render=%v: %v", images, e)
		}
	})
}

// reference presentation library text frames join direct paragraphs, runs/fields and soft breaks.
func referenceRuntimeFrameText(body *etree.Element) string {
	var paragraphs []string
	for _, p := range office.Children(body, office.NSA, "p") {
		var text strings.Builder
		for _, e := range p.ChildElements() {
			if e.NamespaceURI() != office.NSA {
				continue
			}
			switch e.Tag {
			case "br":
				text.WriteByte('\v')
			case "r", "fld":
				if t := office.Child(e, office.NSA, "t"); t != nil {
					text.WriteString(t.Text())
				}
			}
		}
		paragraphs = append(paragraphs, text.String())
	}
	return strings.Join(paragraphs, "\n")
}
