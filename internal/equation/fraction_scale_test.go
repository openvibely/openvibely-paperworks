package equation

import (
	"fmt"
	"math"
	"testing"

	"github.com/luo-studio/go-tex/tex/layout"
	"github.com/luo-studio/go-tex/tex/mathstyle"
	"github.com/luo-studio/go-tex/tex/parser"
)

// Expected scales were captured from pinned MathJax's actual fraction wrappers.
// These checks do not certify glyph metrics, nested scripts or rasterization.
func TestMathJaxFractionChildScale(t *testing.T) {
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
				want := 0.7071067811865475
				if display {
					want = 1
				}
				count := 0
				var visit func(*layout.Box)
				visit = func(b *layout.Box) {
					switch c := b.Content.(type) {
					case layout.Fraction:
						count++
						if c.NumerScale != want || c.DenomScale != want {
							t.Fatalf("scales %g %g want %.17g", c.NumerScale, c.DenomScale, want)
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
