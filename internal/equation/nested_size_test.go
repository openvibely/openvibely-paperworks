package equation

import (
	"github.com/luo-studio/go-tex/tex/layout"
	"github.com/luo-studio/go-tex/tex/mathstyle"
	"github.com/luo-studio/go-tex/tex/parser"
	"math"
	"testing"
)

// Pinned MathJax 3.2.2 absolute glyph scales for 46 cases. Cases04/05/08/09
// assert the exact accumulated products of its relative size/style ratios;
// these differ from MathJax absolute boxes by one ULP. No tolerance or pixel
// equivalence is implied. See Batch42 nested-size evidence.
func TestMathJaxNestedSizeBoundaries(t *testing.T) {
	cases := []struct {
		ID, Tex string
		Display bool
		Scales  []float64
	}{{"case00", "{\\small {\\small x}}", false, []float64{0.85}}, {"case01", "{\\small {\\small x}}", true, []float64{0.85}}, {"case02", "{\\small {\\Huge x}}", false, []float64{2.49}}, {"case03", "{\\small {\\Huge x}}", true, []float64{2.49}}, {"case04", "{\\small x^{\\large y}}", false, []float64{0.85, 0.8485281374238568}}, {"case05", "{\\small x^{\\large y}}", true, []float64{0.85, 0.8485281374238568}}, {"case06", "{\\small \\frac{\\small x}{y}}", false, []float64{0.6010407640085653, 0.6010407640085653}}, {"case07", "{\\small \\frac{\\small x}{y}}", true, []float64{0.85, 0.85}}, {"case08", "{\\small {\\scriptstyle x^y}}", false, []float64{0.6010407640085653, 0.42499999999999993}}, {"case09", "{\\small {\\scriptstyle x^y}}", true, []float64{0.6010407640085653, 0.42499999999999993}}, {"case10", "{\\large {\\small x}}", false, []float64{0.85}}, {"case11", "{\\large {\\small x}}", true, []float64{0.85}}, {"case12", "{\\large {\\Huge x}}", false, []float64{2.49}}, {"case13", "{\\large {\\Huge x}}", true, []float64{2.49}}, {"case14", "{\\large x^{\\large y}}", false, []float64{1.2, 0.8485281374238569}}, {"case15", "{\\large x^{\\large y}}", true, []float64{1.2, 0.8485281374238569}}, {"case16", "{\\large \\frac{\\small x}{y}}", false, []float64{0.6010407640085653, 0.8485281374238569}}, {"case17", "{\\large \\frac{\\small x}{y}}", true, []float64{0.85, 1.2}}, {"case18", "{\\large {\\scriptstyle x^y}}", false, []float64{0.8485281374238569, 0.5999999999999999}}, {"case19", "{\\large {\\scriptstyle x^y}}", true, []float64{0.8485281374238569, 0.5999999999999999}}, {"case20", "{\\Huge {\\small x}}", false, []float64{0.85}}, {"case21", "{\\Huge {\\small x}}", true, []float64{0.85}}, {"case22", "{\\Huge {\\Huge x}}", false, []float64{2.49}}, {"case23", "{\\Huge {\\Huge x}}", true, []float64{2.49}}, {"case24", "{\\Huge x^{\\large y}}", false, []float64{2.49, 0.8485281374238569}}, {"case25", "{\\Huge x^{\\large y}}", true, []float64{2.49, 0.8485281374238569}}, {"case26", "{\\Huge \\frac{\\small x}{y}}", false, []float64{0.6010407640085653, 1.7606958851545034}}, {"case27", "{\\Huge \\frac{\\small x}{y}}", true, []float64{0.85, 2.49}}, {"case28", "{\\Huge {\\scriptstyle x^y}}", false, []float64{1.7606958851545034, 1.2449999999999999}}, {"case29", "{\\Huge {\\scriptstyle x^y}}", true, []float64{1.7606958851545034, 1.2449999999999999}}, {"case30", "{\\scriptstyle {\\small x}}", false, []float64{0.6010407640085653}}, {"case31", "{\\scriptstyle {\\small x}}", true, []float64{0.6010407640085653}}, {"case32", "{\\scriptstyle {\\Huge x}}", false, []float64{1.7606958851545034}}, {"case33", "{\\scriptstyle {\\Huge x}}", true, []float64{1.7606958851545034}}, {"case34", "{\\scriptstyle x^{\\large y}}", false, []float64{0.7071067811865475, 0.5999999999999999}}, {"case35", "{\\scriptstyle x^{\\large y}}", true, []float64{0.7071067811865475, 0.5999999999999999}}, {"case36", "{\\scriptstyle \\frac{\\small x}{y}}", false, []float64{0.4249999999999999, 0.4999999999999999}}, {"case37", "{\\scriptstyle \\frac{\\small x}{y}}", true, []float64{0.4249999999999999, 0.4999999999999999}}, {"case38", "{\\scriptstyle {\\scriptstyle x^y}}", false, []float64{0.7071067811865475, 0.4999999999999999}}, {"case39", "{\\scriptstyle {\\scriptstyle x^y}}", true, []float64{0.7071067811865475, 0.4999999999999999}}, {"case40", "{\\displaystyle {\\small x}}", false, []float64{0.85}}, {"case41", "{\\displaystyle {\\small x}}", true, []float64{0.85}}, {"case42", "{\\displaystyle {\\Huge x}}", false, []float64{2.49}}, {"case43", "{\\displaystyle {\\Huge x}}", true, []float64{2.49}}, {"case44", "{\\displaystyle x^{\\large y}}", false, []float64{1, 0.8485281374238569}}, {"case45", "{\\displaystyle x^{\\large y}}", true, []float64{1, 0.8485281374238569}}, {"case46", "{\\displaystyle \\frac{\\small x}{y}}", false, []float64{0.85, 1}}, {"case47", "{\\displaystyle \\frac{\\small x}{y}}", true, []float64{0.85, 1}}, {"case48", "{\\displaystyle {\\scriptstyle x^y}}", false, []float64{0.7071067811865475, 0.4999999999999999}}, {"case49", "{\\displaystyle {\\scriptstyle x^y}}", true, []float64{0.7071067811865475, 0.4999999999999999}}}
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
