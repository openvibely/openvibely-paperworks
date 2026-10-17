package equation

import (
	"fmt"
	"math"
	"testing"

	"github.com/luo-studio/go-tex/tex/layout"
	"github.com/luo-studio/go-tex/tex/mathstyle"
	"github.com/luo-studio/go-tex/tex/parser"
)

// Pinned MathJax SVGmfrac.makeFraction adds d=.1 on each side of
// the wider child. This checks that local contract, not glyph/raster equivalence.
func TestMathJaxFractionRuleOverhang(t *testing.T) {
	for n := 1; n <= 25; n++ {
		for _, inline := range []bool{false, true} {
			t.Run(fmt.Sprintf("%d-inline-%t", n, inline), func(t *testing.T) {
				nodes, err := parser.Parse(fmt.Sprintf(`\frac{%d}{%d}`, n, 26-n))
				if err != nil {
					t.Fatal(err)
				}
				o := layout.DefaultOptions()
				if inline {
					o.Style = mathstyle.Text
				}
				before := layout.Layout(nodes, o)
				o.FractionRulePadding = .1
				after := layout.Layout(nodes, o)
				if math.Abs(after.Width-before.Width-.2) > 1e-14 {
					t.Fatalf("overhang: %.17g", after.Width-before.Width)
				}
				if after.Height != before.Height || after.Depth != before.Depth {
					t.Fatal("horizontal padding changed vertical layout")
				}
				beforeRule := 0.0
				for _, item := range layout.ToDisplayList(before).Items {
					if line, ok := item.(layout.Line); ok {
						beforeRule = line.Width
					}
				}
				dl := layout.ToDisplayList(after)
				rules := 0
				for _, item := range dl.Items {
					if line, ok := item.(layout.Line); ok {
						rules++
						if math.Abs(line.Width-beforeRule-.2) > 1e-14 {
							t.Fatalf("rule increment %g", line.Width-beforeRule)
						}
					}
				}
				if rules != 1 {
					t.Fatalf("rules=%d", rules)
				}
			})
		}
	}
}
