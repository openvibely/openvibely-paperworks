package equation

import (
	"github.com/luo-studio/go-tex/tex/layout"
	"github.com/luo-studio/go-tex/tex/mathstyle"
	"github.com/luo-studio/go-tex/tex/parser"
	"testing"
)

// Expected child ratios captured from pinned MathJax CommonWrapper.getScale.
// These check ordinary side scripts, not operator limits or rendered equivalence.
func TestMathJaxSideScriptScale(t *testing.T) {
	cases := []struct {
		name, tex string
		display   bool
		ratios    [][]float64
	}{
		{"case00", "{x^2}", false, [][]float64{{0.7071067811865475}}},
		{"case01", "{x^2}", true, [][]float64{{0.7071067811865475}}},
		{"case02", "{x_2}", false, [][]float64{{0.7071067811865475}}},
		{"case03", "{x_2}", true, [][]float64{{0.7071067811865475}}},
		{"case04", "{x_2^3}", false, [][]float64{{0.7071067811865475, 0.7071067811865475}}},
		{"case05", "{x_2^3}", true, [][]float64{{0.7071067811865475, 0.7071067811865475}}},
		{"case06", "{x^{y^2}}", false, [][]float64{{0.7071067811865475}, {0.7071067811865475}}},
		{"case07", "{x^{y^2}}", true, [][]float64{{0.7071067811865475}, {0.7071067811865475}}},
		{"case08", "{x_{y_2}}", false, [][]float64{{0.7071067811865475}, {0.7071067811865475}}},
		{"case09", "{x_{y_2}}", true, [][]float64{{0.7071067811865475}, {0.7071067811865475}}},
		{"case10", "{\\small x^2}", false, [][]float64{{0.7071067811865475}}},
		{"case11", "{\\small x^2}", true, [][]float64{{0.7071067811865475}}},
		{"case12", "{\\small x_2}", false, [][]float64{{0.7071067811865475}}},
		{"case13", "{\\small x_2}", true, [][]float64{{0.7071067811865475}}},
		{"case14", "{\\small x_2^3}", false, [][]float64{{0.7071067811865475, 0.7071067811865475}}},
		{"case15", "{\\small x_2^3}", true, [][]float64{{0.7071067811865475, 0.7071067811865475}}},
		{"case16", "{\\small x^{y^2}}", false, [][]float64{{0.7071067811865475}, {0.7071067811865475}}},
		{"case17", "{\\small x^{y^2}}", true, [][]float64{{0.7071067811865475}, {0.7071067811865475}}},
		{"case18", "{\\small x_{y_2}}", false, [][]float64{{0.7071067811865475}, {0.7071067811865475}}},
		{"case19", "{\\small x_{y_2}}", true, [][]float64{{0.7071067811865475}, {0.7071067811865475}}},
		{"case20", "{\\large x^2}", false, [][]float64{{0.7071067811865475}}},
		{"case21", "{\\large x^2}", true, [][]float64{{0.7071067811865475}}},
		{"case22", "{\\large x_2}", false, [][]float64{{0.7071067811865475}}},
		{"case23", "{\\large x_2}", true, [][]float64{{0.7071067811865475}}},
		{"case24", "{\\large x_2^3}", false, [][]float64{{0.7071067811865475, 0.7071067811865475}}},
		{"case25", "{\\large x_2^3}", true, [][]float64{{0.7071067811865475, 0.7071067811865475}}},
		{"case26", "{\\large x^{y^2}}", false, [][]float64{{0.7071067811865475}, {0.7071067811865475}}},
		{"case27", "{\\large x^{y^2}}", true, [][]float64{{0.7071067811865475}, {0.7071067811865475}}},
		{"case28", "{\\large x_{y_2}}", false, [][]float64{{0.7071067811865475}, {0.7071067811865475}}},
		{"case29", "{\\large x_{y_2}}", true, [][]float64{{0.7071067811865475}, {0.7071067811865475}}},
		{"case30", "{\\Large x^2}", false, [][]float64{{0.7071067811865475}}},
		{"case31", "{\\Large x^2}", true, [][]float64{{0.7071067811865475}}},
		{"case32", "{\\Large x_2}", false, [][]float64{{0.7071067811865475}}},
		{"case33", "{\\Large x_2}", true, [][]float64{{0.7071067811865475}}},
		{"case34", "{\\Large x_2^3}", false, [][]float64{{0.7071067811865475, 0.7071067811865475}}},
		{"case35", "{\\Large x_2^3}", true, [][]float64{{0.7071067811865475, 0.7071067811865475}}},
		{"case36", "{\\Large x^{y^2}}", false, [][]float64{{0.7071067811865475}, {0.7071067811865475}}},
		{"case37", "{\\Large x^{y^2}}", true, [][]float64{{0.7071067811865475}, {0.7071067811865475}}},
		{"case38", "{\\Large x_{y_2}}", false, [][]float64{{0.7071067811865475}, {0.7071067811865475}}},
		{"case39", "{\\Large x_{y_2}}", true, [][]float64{{0.7071067811865475}, {0.7071067811865475}}},
		{"case40", "{\\huge x^2}", false, [][]float64{{0.7071067811865475}}},
		{"case41", "{\\huge x^2}", true, [][]float64{{0.7071067811865475}}},
		{"case42", "{\\huge x_2}", false, [][]float64{{0.7071067811865475}}},
		{"case43", "{\\huge x_2}", true, [][]float64{{0.7071067811865475}}},
		{"case44", "{\\huge x_2^3}", false, [][]float64{{0.7071067811865475, 0.7071067811865475}}},
		{"case45", "{\\huge x_2^3}", true, [][]float64{{0.7071067811865475, 0.7071067811865475}}},
		{"case46", "{\\huge x^{y^2}}", false, [][]float64{{0.7071067811865475}, {0.7071067811865474}}},
		{"case47", "{\\huge x^{y^2}}", true, [][]float64{{0.7071067811865475}, {0.7071067811865474}}},
		{"case48", "{\\huge x_{y_2}}", false, [][]float64{{0.7071067811865475}, {0.7071067811865474}}},
		{"case49", "{\\huge x_{y_2}}", true, [][]float64{{0.7071067811865475}, {0.7071067811865474}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			n, err := parser.Parse(tc.tex)
			if err != nil {
				t.Fatal(err)
			}
			o := layout.DefaultOptions()
			o.MathJaxSizes = true
			o.MathJaxSideScripts = true
			if !tc.display {
				o.Style = mathstyle.Text
			}
			var got [][]float64
			var visit func(*layout.Box)
			visit = func(b *layout.Box) {
				if b == nil {
					return
				}
				switch v := b.Content.(type) {
				case layout.SupSub:
					r := []float64{}
					if v.Sub != nil {
						r = append(r, v.SubScale)
					}
					if v.Sup != nil {
						r = append(r, v.SupScale)
					}
					got = append(got, r)
					visit(v.Base)
					visit(v.Sub)
					visit(v.Sup)
				case layout.Scaled:
					visit(v.Body)
				case layout.HBox:
					for _, c := range v.Children {
						visit(c)
					}
				}
			}
			visit(layout.Layout(n, o))
			if len(got) != len(tc.ratios) {
				t.Fatalf("nodes %v want %v", got, tc.ratios)
			}
			for i, r := range got {
				if len(r) != len(tc.ratios[i]) {
					t.Fatal("child count", got)
				}
				for j, v := range r {
					if v != tc.ratios[i][j] {
						t.Errorf("node%d child%d %.17g want %.17g", i, j, v, tc.ratios[i][j])
					}
				}
			}
		})
	}
}
