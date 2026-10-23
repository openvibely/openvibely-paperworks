package pptx

import (
	"context"
	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"path/filepath"
	"testing"
)

func TestNativeTextPaint(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "in.pptx")
	style := &TextPaint{Fill: &DrawingStyle{Gradient: &Gradient{Stops: []GradientStop{{Position: 0, Color: "FF0000"}, {Position: 1, Color: "0000FF"}}}}, Outline: &TextOutline{Width: 1, Fill: &DrawingStyle{Pattern: &PatternFill{Preset: "cross", Foreground: "000000", Background: "FFFFFF"}}}}
	if err := Create(context.Background(), Spec{Slides: []Slide{{Elements: []Element{{Type: "text", Text: "Keep me", W: 4, H: 2, TextPaint: style}}}}}, dir, in); err != nil {
		t.Fatal(err)
	}
	check := func(path, fill string) {
		t.Helper()
		p, err := opc.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		d, err := p.XML("ppt/slides/slide1.xml")
		if err != nil {
			t.Fatal(err)
		}
		r := office.All(d.Root(), ans, "r")[0]
		pr := office.Child(r, ans, "rPr")
		if office.Child(pr, ans, fill) == nil || office.Child(office.Child(pr, ans, "ln"), ans, "pattFill") == nil {
			t.Fatal("missing glyph paint")
		}
		if office.Child(r, ans, "t").Text() != "Keep me" {
			t.Fatal("text changed")
		}
	}
	check(in, "gradFill")
	out := filepath.Join(dir, "out.pptx")
	if _, err := EditFile(context.Background(), in, out, EditSpec{Edits: []Edit{{Operation: "run-format", Slide: 1, ShapeID: "2", TextPaint: &TextPaint{Fill: &DrawingStyle{Fill: "00FF00"}}}}}); err != nil {
		t.Fatal(err)
	}
	check(out, "solidFill")
	if _, err := paintFill(&DrawingStyle{Image: &ImageFill{Path: "bad"}}); err == nil {
		t.Fatal("unsupported fill accepted")
	}
}

func TestNativePathGradientText(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "gradient.pptx")
	g := &Gradient{Path: "circle", Stops: []GradientStop{{Position: 0, Color: "FFFFFF"}, {Position: 1, Color: "0000FF"}}}
	spec := Spec{Slides: []Slide{{Elements: []Element{{Type: "text", Text: "Gradient", W: 4, H: 2, TextPaint: &TextPaint{Fill: &DrawingStyle{Gradient: g}}}}}}}
	if err := Create(context.Background(), spec, dir, out); err != nil {
		t.Fatal(err)
	}
	p, err := opc.Open(out)
	if err != nil {
		t.Fatal(err)
	}
	d, err := p.XML("ppt/slides/slide1.xml")
	if err != nil {
		t.Fatal(err)
	}
	paths := office.All(d.Root(), ans, "path")
	if len(paths) == 0 || paths[0].SelectAttrValue("path", "") != "circle" {
		t.Fatal("missing path gradient")
	}
	if office.Child(paths[0], ans, "fillToRect").SelectAttrValue("l", "") != "50000" {
		t.Fatal("missing gradient focus")
	}
	g.Focus = &GradientFocus{Left: .8, Right: .8}
	if err := Create(context.Background(), spec, dir, filepath.Join(dir, "bad.pptx")); err == nil {
		t.Fatal("inverted gradient accepted")
	}
}
