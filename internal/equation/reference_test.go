package equation

import (
	"context"
	"image"
	"math"
	"os"
	"testing"

	"github.com/luo-studio/go-tex/tex/layout"
	"github.com/luo-studio/go-tex/tex/parser"
)

func inkBounds(im image.Image) (image.Rectangle, int) {
	b := im.Bounds()
	minx, miny, maxx, maxy, n := b.Max.X, b.Max.Y, 0, 0, 0
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			_, _, _, a := im.At(x, y).RGBA()
			if a > 32768 {
				minx = min(minx, x)
				miny = min(miny, y)
				maxx = max(maxx, x+1)
				maxy = max(maxy, y+1)
				n++
			}
		}
	}
	return image.Rect(minx, miny, maxx, maxy), n
}
func coverage(a, b image.Image, ab, bb image.Rectangle) float64 {
	n, hit := 0, 0
	for y := 0; y < ab.Dy(); y++ {
		for x := 0; x < ab.Dx(); x++ {
			_, _, _, alpha := a.At(ab.Min.X+x, ab.Min.Y+y).RGBA()
			if alpha < 32768 {
				continue
			}
			n++
			found := false
			for dy := -2; dy <= 2 && !found; dy++ {
				for dx := -2; dx <= 2; dx++ {
					_, _, _, v := b.At(bb.Min.X+x+dx, bb.Min.Y+y+dy).RGBA()
					if v > 32768 {
						found = true
						break
					}
				}
			}
			if found {
				hit++
			}
		}
	}
	return float64(hit) / float64(max(n, 1))
}
func TestReferencePixels(t *testing.T) {
	cases := []struct {
		name, tex string
		minimum   float64
	}{
		{"label", `\text{net profit}`, .99}, {"unicode", `\unicode{x2605}`, .99},
		{"cancel", `\cancel{x}`, .98}, {"bcancel", `\bcancel{x}`, .98}, {"xcancel", `\xcancel{x}`, .98},
		{"enclose", `\enclose{circle}{x}`, .98},
		{"overbrace", `\overbrace{a+b}^{\text{two terms}}`, .97},
		{"xrightarrow", `A\xrightarrow[below]{above}B`, .95},
		{"xleftrightarrow", `A\xleftrightarrow{above}B`, .93},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			data, e := os.ReadFile("../../testdata/equations/reference/" + c.name + ".png")
			if e != nil {
				t.Fatal(e)
			}
			ref := decode(t, data)
			got := imageFor(t, c.tex, Defaults())
			rb, rn := inkBounds(ref)
			gb, gn := inkBounds(got)
			if math.Abs(float64(gb.Dx())/float64(rb.Dx())-1) > .04 || math.Abs(float64(gb.Dy())/float64(rb.Dy())-1) > .04 {
				t.Fatalf("ink geometry: got %v reference %v", gb, rb)
			}
			if ratio := float64(gn) / float64(rn); ratio < .94 || ratio > 1.08 {
				t.Fatalf("ink ratio %f", ratio)
			}
			if v := math.Min(coverage(got, ref, gb, rb), coverage(ref, got, rb, gb)); v < c.minimum {
				t.Fatalf("2px symmetric coverage %.4f < %.4f", v, c.minimum)
			}
		})
	}
}
func TestColorsBackgroundsAndNoClipping(t *testing.T) {
	for _, s := range []string{`\color{red}{x^2}`, `\textcolor{blue}{x}`, `\colorbox{yellow}{$x$}`, `\fcolorbox{red}{yellow}{$x$}`, `\bbox[5px,border:2px solid red]{x+1}`} {
		im := imageFor(t, s, Defaults())
		red, blue, yellow := 0, 0, 0
		for y := 0; y < im.Bounds().Dy(); y++ {
			for x := 0; x < im.Bounds().Dx(); x++ {
				r, g, b, a := im.At(x, y).RGBA()
				if a < 32768 {
					continue
				}
				if r > 50000 && g < 10000 && b < 10000 {
					red++
				}
				if b > 50000 && r < 10000 && g < 10000 {
					blue++
				}
				if r > 50000 && g > 50000 && b < 10000 {
					yellow++
				}
			}
		}
		if s == `\textcolor{blue}{x}` && blue < 100 {
			t.Fatal("blue glyph missing")
		}
		if (s == `\color{red}{x^2}` || s == `\fcolorbox{red}{yellow}{$x$}` || s == `\bbox[5px,border:2px solid red]{x+1}`) && red < 100 {
			t.Fatal("red glyph/border missing")
		}
		if (s == `\colorbox{yellow}{$x$}` || s == `\fcolorbox{red}{yellow}{$x$}`) && yellow < 500 {
			t.Fatal("background missing")
		}
	}
}
func TestCustomEnvironmentEquivalence(t *testing.T) {
	a, e := PNG(context.Background(), `\newenvironment{foo}{\begin{matrix}}{\end{matrix}}\begin{foo}a&b\end{foo}`, Defaults())
	if e != nil {
		t.Fatal(e)
	}
	b, e := PNG(context.Background(), `\begin{matrix}a&b\end{matrix}`, Defaults())
	if e != nil {
		t.Fatal(e)
	}
	if string(a) != string(b) {
		t.Fatal("environment changed content")
	}
}
func TestScriptsAndArrowsRetainContent(t *testing.T) {
	for _, c := range []struct{ tex, chars string }{{`\prescript{14}{6}{C}`, "146C"}, {`\sideset{_a^b}{_c^d}\sum`, "abcd∑"}, {`A\xrightarrow[below]{above}B`, "AbelowaboveB"}, {`\cancelto{0}{x}`, "0x"}} {
		nodes, e := parser.Parse(c.tex)
		if e != nil {
			t.Fatal(e)
		}
		dl := layout.ToDisplayList(layout.Layout(nodes, layout.DefaultOptions()))
		counts := map[rune]int{}
		for _, item := range dl.Items {
			if g, ok := item.(layout.GlyphPath); ok {
				counts[g.CharCode]++
			}
		}
		for _, ch := range c.chars {
			counts[ch]--
			if counts[ch] < 0 {
				t.Fatalf("%s missing glyph %c", c.tex, ch)
			}
		}
	}
}
func TestColorValidation(t *testing.T) {
	for _, s := range []string{`\textcolor{#123456}{x}`, `\textcolor{RebeccaPurple}{x}`, `\bbox[yellow,border:2px solid navy]{x}`} {
		imageFor(t, s, Defaults())
	}
	for _, s := range []string{`\textcolor{boguscolor}{x}`, `\colorbox{boguscolor}{x}`, `\bbox[border:2px solid boguscolor]{x}`} {
		if _, e := PNG(context.Background(), s, Defaults()); e == nil {
			t.Fatalf("invalid color accepted: %s", s)
		}
	}
}
