package equation

import (
	"github.com/luo-studio/go-tex/tex/layout"
	"github.com/luo-studio/go-tex/tex/mathstyle"
	"github.com/luo-studio/go-tex/tex/parser"
	"testing"
)

func TestPinnedSizeKeywords(t *testing.T) {
	for _, c := range []struct {
		name, tex string
		display   bool
		scale     float64
	}{
		{"case00", "{x^2}", false, 1},
		{"case01", "{x^2}", true, 1},
		{"case02", "{x_2}", false, 1},
		{"case03", "{x_2}", true, 1},
		{"case04", "{x_2^3}", false, 1},
		{"case05", "{x_2^3}", true, 1},
		{"case06", "{x^{y^2}}", false, 1},
		{"case07", "{x^{y^2}}", true, 1},
		{"case08", "{x_{y_2}}", false, 1},
		{"case09", "{x_{y_2}}", true, 1},
		{"case10", "{\\small x^2}", false, 0.85},
		{"case11", "{\\small x^2}", true, 0.85},
		{"case12", "{\\small x_2}", false, 0.85},
		{"case13", "{\\small x_2}", true, 0.85},
		{"case14", "{\\small x_2^3}", false, 0.85},
		{"case15", "{\\small x_2^3}", true, 0.85},
		{"case16", "{\\small x^{y^2}}", false, 0.85},
		{"case17", "{\\small x^{y^2}}", true, 0.85},
		{"case18", "{\\small x_{y_2}}", false, 0.85},
		{"case19", "{\\small x_{y_2}}", true, 0.85},
		{"case20", "{\\large x^2}", false, 1.2},
		{"case21", "{\\large x^2}", true, 1.2},
		{"case22", "{\\large x_2}", false, 1.2},
		{"case23", "{\\large x_2}", true, 1.2},
		{"case24", "{\\large x_2^3}", false, 1.2},
		{"case25", "{\\large x_2^3}", true, 1.2},
		{"case26", "{\\large x^{y^2}}", false, 1.2},
		{"case27", "{\\large x^{y^2}}", true, 1.2},
		{"case28", "{\\large x_{y_2}}", false, 1.2},
		{"case29", "{\\large x_{y_2}}", true, 1.2},
		{"case30", "{\\Large x^2}", false, 1.44},
		{"case31", "{\\Large x^2}", true, 1.44},
		{"case32", "{\\Large x_2}", false, 1.44},
		{"case33", "{\\Large x_2}", true, 1.44},
		{"case34", "{\\Large x_2^3}", false, 1.44},
		{"case35", "{\\Large x_2^3}", true, 1.44},
		{"case36", "{\\Large x^{y^2}}", false, 1.44},
		{"case37", "{\\Large x^{y^2}}", true, 1.44},
		{"case38", "{\\Large x_{y_2}}", false, 1.44},
		{"case39", "{\\Large x_{y_2}}", true, 1.44},
		{"case40", "{\\huge x^2}", false, 2.07},
		{"case41", "{\\huge x^2}", true, 2.07},
		{"case42", "{\\huge x_2}", false, 2.07},
		{"case43", "{\\huge x_2}", true, 2.07},
		{"case44", "{\\huge x_2^3}", false, 2.07},
		{"case45", "{\\huge x_2^3}", true, 2.07},
		{"case46", "{\\huge x^{y^2}}", false, 2.07},
		{"case47", "{\\huge x^{y^2}}", true, 2.07},
		{"case48", "{\\huge x_{y_2}}", false, 2.07},
		{"case49", "{\\huge x_{y_2}}", true, 2.07},
		{"LARGE", "{\\LARGE x}", false, 1.73},
		{"Huge", "{\\Huge x}", false, 2.49},
	} {
		t.Run(c.name, func(t *testing.T) {
			n, e := parser.Parse(c.tex)
			if e != nil {
				t.Fatal(e)
			}
			o := layout.DefaultOptions()
			o.MathJaxSizes = true
			if !c.display {
				o.Style = mathstyle.Text
			}
			b := layout.Layout(n, o)
			got := 1.0
			var visit func(*layout.Box)
			visit = func(b *layout.Box) {
				switch v := b.Content.(type) {
				case layout.Scaled:
					got = v.ChildScale
				case layout.HBox:
					for _, ch := range v.Children {
						visit(ch)
					}
				}
			}
			visit(b)
			if got != c.scale {
				t.Fatalf("size scale %.17g want %.17g", got, c.scale)
			}
		})
	}
}
