package equation

import (
	"context"
	"crypto/sha256"
	"fmt"
	"github.com/luo-studio/go-tex/tex/layout"
	"github.com/luo-studio/go-tex/tex/path"
	"github.com/luo-studio/go-tex/tex/render"
	"os"
	"testing"
)

func TestPinnedTimesAlphaLocalPath(t *testing.T) {
	font, err := os.ReadFile("/System/Library/Fonts/Times.ttc")
	if err != nil {
		t.Skip("pinned system font unavailable:", err)
	}
	if fmt.Sprintf("%x", sha256.Sum256(font)) != "20e3dc89912f4b37f2c29add764855164efafb4c66ca32551d9eb52c7411c7c7" {
		t.Skip("different system font; pinned comparison not applicable")
	}
	commands, _, err := systemOutline(font, 0, 'α')
	if err != nil {
		t.Fatal(err)
	}
	// Match CoreText implicit contour closure (diagnostic only).
	var clean []path.Command
	var sx, sy float64
	for _, c := range commands {
		if c.Kind == path.KindMoveTo {
			sx, sy = c.X, c.Y
		}
		if c.Kind == path.KindClose && len(clean) > 0 {
			last := clean[len(clean)-1]
			if last.Kind == path.KindLineTo && last.X == sx && last.Y == sy {
				clean = clean[:len(clean)-1]
			}
		}
		clean = append(clean, c)
	}
	commands = clean
	// SVG unknownText uses 884px in a 1000-unit coordinate system.
	for i := range commands {
		c := &commands[i]
		c.X *= .884
		c.Y *= .884
		c.X1 *= .884
		c.Y1 *= .884
		c.X2 *= .884
		c.Y2 *= .884
	}
	dl := layout.DisplayList{Width: .6, Height: .75, Depth: .2, Items: []layout.DisplayItem{layout.PathItem{Y: .75, Commands: commands, Fill: true, SystemOutline: true, Color: layout.Color{A: 255}}}}
	im, err := render.Image(dl, render.WithFontSize(24), render.WithPadding(3.6), render.WithScale(300.0/72), render.WithMathJaxViewport())
	if err != nil {
		t.Fatal(err)
	}
	raw, err := encodePNG(context.Background(), im, 300)
	if err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(raw)); got != "239a875f86bd823e40373a317dd251aa00bd1618fee42baa0f0b0ed16c97e5f5" {
		t.Fatalf("complete pinned alpha PNG differs: %s", got)
	}
}
