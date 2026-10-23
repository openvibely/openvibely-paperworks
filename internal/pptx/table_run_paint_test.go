package pptx

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
)

func TestStructuredTableRunPaint(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	input := filepath.Join(dir, "in.pptx")
	paint := &TextPaint{Fill: &DrawingStyle{Gradient: &Gradient{Stops: []GradientStop{{Position: 0, Color: "FF0000"}, {Position: 1, Color: "0000FF"}}}}}
	paragraphs := []Paragraph{{Runs: []Run{{Text: "Gradient", TextPaint: paint, Hyperlink: "https://example.com"}, {Text: " plain", Color: "000000"}}}}
	format := &TableFormat{Cells: []TableCellFormat{{Paragraphs: paragraphs}}}
	spec := Spec{Slides: []Slide{{Elements: []Element{{Type: "table", W: 4, H: 2, Rows: [][]string{{"old", "Keep"}}, TableFormat: format}}}}}
	if err := Create(ctx, spec, "", input); err != nil {
		t.Fatal(err)
	}
	check := func(path string) {
		t.Helper()
		pkg, err := opc.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		doc, err := pkg.XML("ppt/slides/slide1.xml")
		if err != nil {
			t.Fatal(err)
		}
		cells := office.All(doc.Root(), ans, "tc")
		runs := office.All(cells[0], ans, "r")
		if len(runs) != 2 {
			t.Fatal("runs changed")
		}
		first := office.Child(runs[0], ans, "rPr")
		gradient := office.Child(first, ans, "gradFill")
		if gradient == nil || office.Child(first, ans, "solidFill") != nil {
			t.Fatal("run gradient missing or conflicting solid fill")
		}
		stops := office.All(gradient, ans, "srgbClr")
		if len(stops) != 2 || stops[0].SelectAttrValue("val", "") != "FF0000" || stops[1].SelectAttrValue("val", "") != "0000FF" {
			t.Fatal("gradient stops changed")
		}
		if office.Child(first, ans, "hlinkClick") == nil {
			t.Fatal("hyperlink lost")
		}
		plain := office.Child(runs[1], ans, "rPr")
		if office.Child(plain, ans, "gradFill") != nil || office.All(plain, ans, "srgbClr")[0].SelectAttrValue("val", "") != "000000" {
			t.Fatal("paint leaked to plain run")
		}
		if office.All(cells[1], ans, "t")[0].Text() != "Keep" {
			t.Fatal("neighbor changed")
		}
	}
	check(input)
	for _, edit := range []Edit{{Operation: "table-cell", Slide: 1, ShapeID: "2", Paragraphs: paragraphs}, {Operation: "table-format", Slide: 1, ShapeID: "2", TableFormat: format}} {
		output := filepath.Join(dir, edit.Operation+".pptx")
		if _, err := EditFile(ctx, input, output, EditSpec{Edits: []Edit{edit}}); err != nil {
			t.Fatal(err)
		}
		check(output)
	}
	paint.Fill = &DrawingStyle{Image: &ImageFill{Path: "unsupported.png"}}
	if err := Create(ctx, spec, "", filepath.Join(dir, "bad.pptx")); err == nil {
		t.Fatal("unsupported run paint accepted")
	}
}

func TestStructuredRunPaintWithoutHyperlink(t *testing.T) {
	for _, inherited := range []bool{false, true} {
		out := filepath.Join(t.TempDir(), "out.pptx")
		paragraphs := []Paragraph{{Runs: []Run{{Text: "Painted", TextPaint: &TextPaint{Fill: &DrawingStyle{Fill: "FF0000"}}}, {Text: "Plain"}}}}
		spec := Spec{Slides: []Slide{{NotesParagraphs: paragraphs, Elements: []Element{{Type: "text", W: 4, H: 2, InheritFormatting: inherited, Paragraphs: paragraphs}}}}}
		if err := Create(context.Background(), spec, "", out); err != nil {
			t.Fatal(err)
		}
		pkg, err := opc.Open(out)
		if err != nil {
			t.Fatal(err)
		}
		for _, part := range []string{"ppt/slides/slide1.xml", "ppt/notesSlides/notesSlide1.xml"} {
			doc, err := pkg.XML(part)
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, run := range office.All(doc.Root(), ans, "r") {
				if text := office.Child(run, ans, "t"); text != nil && text.Text() == "Painted" {
					colors := office.All(office.Child(run, ans, "rPr"), ans, "srgbClr")
					if len(colors) != 1 || colors[0].SelectAttrValue("val", "") != "FF0000" {
						t.Fatal("run paint was not saved")
					}
					found = true
				}
			}
			if !found {
				t.Fatal("run missing")
			}
		}
	}
}
