package equation

import (
	"fmt"
	"math"
	"testing"

	"github.com/luo-studio/go-tex/tex/layout"
	"github.com/luo-studio/go-tex/tex/mathstyle"
	"github.com/luo-studio/go-tex/tex/parser"
)

// Expected shifts and rule thickness come from pinned MathJax fraction wrappers.
// These checks do not certify glyph metrics, nested scripts or rasterization.
func TestMathJaxFractionVerticalMetrics(t *testing.T) {
	for n := 1; n <= 25; n++ {
		for _, display := range []bool{false, true} {
			t.Run(fmt.Sprintf("%d-display-%t", n, display), func(t *testing.T) {
				nodes, err := parser.Parse(fmt.Sprintf(`\frac{%d}{%d}`, n, 26-n))
				if err != nil {
					t.Fatal(err)
				}
				o := layout.DefaultOptions()
				if !display {
					o.Style = mathstyle.Text
				}
				o.FractionRulePadding = .1
				o.FractionScriptScale = 1 / math.Sqrt(2)
				o.MathJaxFractionMetrics = true
				want := verticalOracle[(n-1)*2]
				if display {
					want = verticalOracle[(n-1)*2+1]
				}
				count := 0
				var visit func(*layout.Box)
				visit = func(b *layout.Box) {
					switch c := b.Content.(type) {
					case layout.Fraction:
						count++
						if c.BarThickness != want[0] || c.NumerShift != want[1] || c.DenomShift != want[2] {
							t.Fatalf("rule/shifts %g %g %g want %v", c.BarThickness, c.NumerShift, c.DenomShift, want)
						}
					case layout.HBox:
						for _, child := range c.Children {
							visit(child)
						}
					}
				}
				visit(layout.Layout(nodes, o))
				if count != 1 {
					t.Fatalf("fractions=%d", count)
				}
			})
		}
	}
}

var verticalOracle = [50][3]float64{
	{0.06, 0.394, 0.345},
	{0.06, 0.676, 0.686},
	{0.06, 0.394, 0.345},
	{0.06, 0.676, 0.686},
	{0.06, 0.394, 0.345},
	{0.06, 0.676, 0.686},
	{0.06, 0.394, 0.345},
	{0.06, 0.676, 0.686},
	{0.06, 0.394, 0.345},
	{0.06, 0.676, 0.686},
	{0.06, 0.394, 0.345},
	{0.06, 0.676, 0.686},
	{0.06, 0.394, 0.345},
	{0.06, 0.676, 0.686},
	{0.06, 0.394, 0.345},
	{0.06, 0.676, 0.686},
	{0.06, 0.394, 0.345},
	{0.06, 0.676, 0.686},
	{0.06, 0.394, 0.345},
	{0.06, 0.676, 0.686},
	{0.06, 0.394, 0.345},
	{0.06, 0.676, 0.686},
	{0.06, 0.394, 0.345},
	{0.06, 0.676, 0.686},
	{0.06, 0.394, 0.345},
	{0.06, 0.676, 0.686},
	{0.06, 0.394, 0.345},
	{0.06, 0.676, 0.686},
	{0.06, 0.394, 0.345},
	{0.06, 0.676, 0.686},
	{0.06, 0.394, 0.345},
	{0.06, 0.676, 0.686},
	{0.06, 0.394, 0.345},
	{0.06, 0.676, 0.686},
	{0.06, 0.394, 0.345},
	{0.06, 0.676, 0.686},
	{0.06, 0.394, 0.345},
	{0.06, 0.676, 0.686},
	{0.06, 0.394, 0.345},
	{0.06, 0.676, 0.686},
	{0.06, 0.394, 0.345},
	{0.06, 0.676, 0.686},
	{0.06, 0.394, 0.345},
	{0.06, 0.676, 0.686},
	{0.06, 0.394, 0.345},
	{0.06, 0.676, 0.686},
	{0.06, 0.394, 0.345},
	{0.06, 0.676, 0.686},
	{0.06, 0.394, 0.345},
	{0.06, 0.676, 0.686},
}
