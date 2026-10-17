// Package equation renders local TeX input with the maintained, embedded Go engine.
package equation

import (
	"context"
	"fmt"
	"math"
	"os"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/luo-studio/go-tex/tex/layout"
	"github.com/luo-studio/go-tex/tex/mathstyle"
	"github.com/luo-studio/go-tex/tex/parser"
	"github.com/luo-studio/go-tex/tex/path"
	"github.com/luo-studio/go-tex/tex/render"
)

type Options struct {
	Inline                        bool
	FontSize, DPI, Scale, Padding float64
}

// Defaults match equation.cjs: 24 CSS pixels/em, 300 dpi, 0.15em padding.
func Defaults() Options { return Options{FontSize: 24, DPI: 300, Scale: 1, Padding: 3.6} }

type Result struct {
	Output            string `json:"output"`
	Width             int    `json:"width"`
	Height            int    `json:"height"`
	Inline            bool   `json:"inline"`
	Engine            string `json:"engine"`
	ReferenceVerified bool   `json:"reference_verified"`
}

func PNG(ctx context.Context, expression string, o Options) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !utf8.ValidString(expression) || strings.TrimSpace(expression) == "" || len(utf16.Encode([]rune(expression))) > 20000 {
		return nil, fmt.Errorf("provide a nonempty UTF-8 TeX expression of at most 20000 UTF-16 units")
	}
	for _, v := range []float64{o.FontSize, o.DPI, o.Scale, o.Padding} {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return nil, fmt.Errorf("nonfinite equation option")
		}
	}
	if o.FontSize <= 0 || o.FontSize > 1000 || o.DPI <= 0 || o.DPI > 2400 || o.Scale <= 0 || o.Scale > 16 || o.Padding < 0 || o.Padding > 1000 {
		return nil, fmt.Errorf("equation options out of range")
	}
	// These commands must remain forbidden even if the underlying engine grows.
	prohibited := map[string]bool{"csname": true, "require": true, "autoload": true, "includegraphics": true, "input": true, "include": true, "href": true, "url": true, "htmlClass": true, "htmlId": true, "htmlStyle": true, "htmlData": true, "class": true, "cssId": true, "style": true, "write": true, "openout": true, "read": true}
	for i := 0; i < len(expression); i++ {
		if expression[i] != '\\' {
			continue
		}
		j := i + 1
		for j < len(expression) && ((expression[j] >= 'a' && expression[j] <= 'z') || (expression[j] >= 'A' && expression[j] <= 'Z')) {
			j++
		}
		if prohibited[expression[i+1:j]] {
			return nil, fmt.Errorf("prohibited command %s", expression[i:j])
		}
		if j == i+1 {
			i++
		} else {
			i = j - 1
		}
	}
	nodes, err := parser.ParseContext(ctx, expression)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	opts := layout.DefaultOptions()
	opts.SystemUnicode = make(map[rune][]path.Command)
	resolve := newSystemEmojiResolver(ctx)
	var systemErr error
	opts.MathJaxGlyphs = layout.SupportsMathJaxGlyphsWithSystem(nodes, func(cp rune) bool {
		if _, ok := opts.SystemUnicode[cp]; ok {
			return true
		}
		outline, e := resolve(cp)
		if e != nil {
			systemErr = e
			return false
		}
		opts.SystemUnicode[cp] = outline
		return true
	})
	if systemErr != nil {
		return nil, systemErr
	}
	opts.MathJaxSizes = true
	opts.MathJaxSideScripts = true
	// Pinned MathJax SVGmfrac.makeFraction extends the rule by .1em
	// on each side of the wider child, independently of glyph metrics.
	opts.FractionRulePadding = 0.1
	opts.MathJaxFractionMetrics = true
	// Pinned MathJax uses 1/sqrt(2) per script level, capped at two levels.
	// This option is limited to fraction children; general scripts remain unverified.
	opts.FractionScriptScale = 1 / math.Sqrt(2)
	if o.Inline {
		opts.Style = mathstyle.Text
	}
	box := layout.Layout(nodes, opts)
	dl := layout.ToNominalDisplayList(box)
	if opts.MathJaxGlyphs {
		dl = layout.ToLibrsvgGlyphDisplayList(box)
	}
	if len(dl.Items) == 0 {
		return nil, fmt.Errorf("equation has no visible content")
	}
	w, h := dl.Width*o.FontSize+2*o.Padding, (dl.Height+dl.Depth)*o.FontSize+2*o.Padding
	if w <= 0 || h <= 0 || w*h > 4000000 {
		return nil, fmt.Errorf("equation dimensions exceed render limit")
	}
	im, err := render.Image(dl, render.WithFontSize(o.FontSize), render.WithPadding(o.Padding), render.WithScale(o.DPI/72*o.Scale), render.WithContext(ctx), render.WithMathJaxViewport())
	if err != nil {
		return nil, err
	}
	return encodePNG(ctx, im, o.DPI*o.Scale)
}

// Write uses exclusive creation. Invalid input never creates the destination;
// write/close failures remove the incomplete file created by this call.
func Write(ctx context.Context, expression, destination string, o Options) (Result, error) {
	result := Result{Output: destination, Inline: o.Inline, Engine: "go-tex-maintained"}
	if _, err := os.Lstat(destination); err == nil {
		return result, fmt.Errorf("output exists: %s: %w", destination, os.ErrExist)
	} else if !os.IsNotExist(err) {
		return result, err
	}
	data, err := PNG(ctx, expression, o)
	if err != nil {
		return result, err
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	f, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return result, err
	}
	_, err = f.Write(data)
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		os.Remove(destination)
		return result, err
	}
	// PNG IHDR width and height are always bytes 16..23.
	result.Width = int(uint32(data[16])<<24 | uint32(data[17])<<16 | uint32(data[18])<<8 | uint32(data[19]))
	result.Height = int(uint32(data[20])<<24 | uint32(data[21])<<16 | uint32(data[22])<<8 | uint32(data[23]))
	return result, nil
}
