package equation

import (
	"github.com/luo-studio/go-tex/tex/layout"
	"github.com/luo-studio/go-tex/tex/mathstyle"
	"github.com/luo-studio/go-tex/tex/parser"
	"math"
	"testing"
)

// Exact accumulated wrapper scales captured from pinned MathJax 3.2.2.
// These are transform checks, not absolute-box or pixel equivalence.
func TestMathJaxFractionAbsoluteChild(t *testing.T) {
	cases := []struct {
		ID, Tex string
		Display bool
		Scales  []float64
	}{{"case00", "{\\scriptstyle \\small {\\textstyle p}_{\\Large a}}", false, []float64{0.85, 0.72}}, {"case01", "{\\scriptstyle \\small {\\textstyle p}_{\\Large a}}", true, []float64{0.85, 0.72}}, {"case02", "{\\scriptstyle \\small {\\textstyle q}_{\\Large b}}", false, []float64{0.85, 0.72}}, {"case03", "{\\scriptstyle \\small {\\textstyle q}_{\\Large b}}", true, []float64{0.85, 0.72}}, {"case04", "{\\scriptstyle \\small {\\textstyle r}_{\\Large c}}", false, []float64{0.85, 0.72}}, {"case05", "{\\scriptstyle \\small {\\textstyle r}_{\\Large c}}", true, []float64{0.85, 0.72}}, {"case06", "{\\scriptstyle \\small {\\textstyle s}_{\\Large d}}", false, []float64{0.85, 0.72}}, {"case07", "{\\scriptstyle \\small {\\textstyle s}_{\\Large d}}", true, []float64{0.85, 0.72}}, {"case08", "{\\scriptstyle \\small {\\textstyle t}_{\\Large e}}", false, []float64{0.85, 0.72}}, {"case09", "{\\scriptstyle \\small {\\textstyle t}_{\\Large e}}", true, []float64{0.85, 0.72}}, {"case10", "{\\Large \\frac{\\small p}{\\Huge a}}", false, []float64{0.6010407640085653, 1.7606958851545036}}, {"case11", "{\\Large \\frac{\\small p}{\\Huge a}}", true, []float64{0.85, 2.49}}, {"case12", "{\\Large \\frac{\\small q}{\\Huge b}}", false, []float64{0.6010407640085653, 1.7606958851545036}}, {"case13", "{\\Large \\frac{\\small q}{\\Huge b}}", true, []float64{0.85, 2.49}}, {"case14", "{\\Large \\frac{\\small r}{\\Huge c}}", false, []float64{0.6010407640085653, 1.7606958851545036}}, {"case15", "{\\Large \\frac{\\small r}{\\Huge c}}", true, []float64{0.85, 2.49}}, {"case16", "{\\Large \\frac{\\small s}{\\Huge d}}", false, []float64{0.6010407640085653, 1.7606958851545036}}, {"case17", "{\\Large \\frac{\\small s}{\\Huge d}}", true, []float64{0.85, 2.49}}, {"case18", "{\\Large \\frac{\\small t}{\\Huge e}}", false, []float64{0.6010407640085653, 1.7606958851545036}}, {"case19", "{\\Large \\frac{\\small t}{\\Huge e}}", true, []float64{0.85, 2.49}}, {"case20", "{\\Large {\\Huge p}^{\\small {\\scriptstyle a}}}", false, []float64{2.49, 0.6010407640085654}}, {"case21", "{\\Large {\\Huge p}^{\\small {\\scriptstyle a}}}", true, []float64{2.49, 0.6010407640085654}}, {"case22", "{\\Large {\\Huge q}^{\\small {\\scriptstyle b}}}", false, []float64{2.49, 0.6010407640085654}}, {"case23", "{\\Large {\\Huge q}^{\\small {\\scriptstyle b}}}", true, []float64{2.49, 0.6010407640085654}}, {"case24", "{\\Large {\\Huge r}^{\\small {\\scriptstyle c}}}", false, []float64{2.49, 0.6010407640085654}}, {"case25", "{\\Large {\\Huge r}^{\\small {\\scriptstyle c}}}", true, []float64{2.49, 0.6010407640085654}}, {"case26", "{\\Large {\\Huge s}^{\\small {\\scriptstyle d}}}", false, []float64{2.49, 0.6010407640085654}}, {"case27", "{\\Large {\\Huge s}^{\\small {\\scriptstyle d}}}", true, []float64{2.49, 0.6010407640085654}}, {"case28", "{\\Large {\\Huge t}^{\\small {\\scriptstyle e}}}", false, []float64{2.49, 0.6010407640085654}}, {"case29", "{\\Large {\\Huge t}^{\\small {\\scriptstyle e}}}", true, []float64{2.49, 0.6010407640085654}}, {"case30", "{\\scriptstyle {\\Large p}_{\\small {\\textstyle a}}}", false, []float64{1.0182337649086284, 0.85}}, {"case31", "{\\scriptstyle {\\Large p}_{\\small {\\textstyle a}}}", true, []float64{1.0182337649086284, 0.85}}, {"case32", "{\\scriptstyle {\\Large q}_{\\small {\\textstyle b}}}", false, []float64{1.0182337649086284, 0.85}}, {"case33", "{\\scriptstyle {\\Large q}_{\\small {\\textstyle b}}}", true, []float64{1.0182337649086284, 0.85}}, {"case34", "{\\scriptstyle {\\Large r}_{\\small {\\textstyle c}}}", false, []float64{1.0182337649086284, 0.85}}, {"case35", "{\\scriptstyle {\\Large r}_{\\small {\\textstyle c}}}", true, []float64{1.0182337649086284, 0.85}}, {"case36", "{\\scriptstyle {\\Large s}_{\\small {\\textstyle d}}}", false, []float64{1.0182337649086284, 0.85}}, {"case37", "{\\scriptstyle {\\Large s}_{\\small {\\textstyle d}}}", true, []float64{1.0182337649086284, 0.85}}, {"case38", "{\\scriptstyle {\\Large t}_{\\small {\\textstyle e}}}", false, []float64{1.0182337649086284, 0.85}}, {"case39", "{\\scriptstyle {\\Large t}_{\\small {\\textstyle e}}}", true, []float64{1.0182337649086284, 0.85}}, {"case40", "{\\Large \\frac{\\small {\\scriptstyle p}}{\\Huge a}}", false, []float64{0.6010407640085653, 1.7606958851545036}}, {"case41", "{\\Large \\frac{\\small {\\scriptstyle p}}{\\Huge a}}", true, []float64{0.6010407640085653, 2.49}}, {"case42", "{\\Large \\frac{\\small {\\scriptstyle q}}{\\Huge b}}", false, []float64{0.6010407640085653, 1.7606958851545036}}, {"case43", "{\\Large \\frac{\\small {\\scriptstyle q}}{\\Huge b}}", true, []float64{0.6010407640085653, 2.49}}, {"case44", "{\\Large \\frac{\\small {\\scriptstyle r}}{\\Huge c}}", false, []float64{0.6010407640085653, 1.7606958851545036}}, {"case45", "{\\Large \\frac{\\small {\\scriptstyle r}}{\\Huge c}}", true, []float64{0.6010407640085653, 2.49}}, {"case46", "{\\Large \\frac{\\small {\\scriptstyle s}}{\\Huge d}}", false, []float64{0.6010407640085653, 1.7606958851545036}}, {"case47", "{\\Large \\frac{\\small {\\scriptstyle s}}{\\Huge d}}", true, []float64{0.6010407640085653, 2.49}}, {"case48", "{\\Large \\frac{\\small {\\scriptstyle t}}{\\Huge e}}", false, []float64{0.6010407640085653, 1.7606958851545036}}, {"case49", "{\\Large \\frac{\\small {\\scriptstyle t}}{\\Huge e}}", true, []float64{0.6010407640085653, 2.49}}}
	for _, c := range cases {
		n, e := parser.Parse(c.Tex)
		if e != nil {
			t.Fatal(e)
		}
		o := layout.DefaultOptions()
		o.MathJaxSizes = true
		o.MathJaxSideScripts = true
		o.FractionScriptScale = 1 / math.Sqrt(2)
		o.MathJaxFractionMetrics = true
		o.FractionRulePadding = .1
		if !c.Display {
			o.Style = mathstyle.Text
		}
		leaves := []map[string]any{}
		var visit func(*layout.Box, float64)
		visit = func(b *layout.Box, s float64) {
			if b == nil {
				return
			}
			switch v := b.Content.(type) {
			case layout.Kern:
			case layout.Glyph:
				leaves = append(leaves, map[string]any{"text": string(v.CharCode), "scale": s})
			case layout.Scaled:
				visit(v.Body, s*v.ChildScale)
			case layout.HBox:
				for _, b := range v.Children {
					visit(b, s)
				}
			case layout.SupSub:
				visit(v.Base, s)
				visit(v.Sub, s*v.SubScale)
				visit(v.Sup, s*v.SupScale)
			case layout.Fraction:
				visit(v.Numer, s*v.NumerScale)
				visit(v.Denom, s*v.DenomScale)
			default:
				t.Fatalf("unsupported content %T", v)
			}
		}
		visit(layout.Layout(n, o), 1)
		if len(leaves) != len(c.Scales) {
			t.Fatalf("%s glyph count %v", c.ID, leaves)
		}
		for i, l := range leaves {
			if l["scale"] != c.Scales[i] {
				t.Errorf("%s glyph%d scale %.17g want %.17g", c.ID, i, l["scale"], c.Scales[i])
			}
		}
	}
}
