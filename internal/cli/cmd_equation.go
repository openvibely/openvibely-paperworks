package cli

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/luo-studio/go-tex/tex/fonts"
	"github.com/openvibely/openvibely-paperworks/internal/equation"
)

func init() {
	register(&Command{Name: "equation-fonts", ValueOpts: []string{}, Summary: "List embedded equation fonts and their complete redistribution license.", Usage: "paperworks equation-fonts", Run: func(ctx context.Context, a Args) (any, error) {
		if len(a.Pos) != 0 {
			return nil, usagef("equation-fonts takes no arguments")
		}
		return map[string]any{"fonts": fonts.Names(), "license": fonts.License, "provenance": "KaTeX v0.16.22 (unmodified)"}, nil
	}})
	register(&Command{Name: "equation", ValueOpts: []string{"font-size", "dpi", "scale", "padding"}, Summary: "Render local TeX to a transparent PNG using embedded fonts (compatibility work in progress).", Usage: "paperworks equation EXPRESSION.txt NEW.png [--inline] [--font-size 24] [--dpi 300] [--scale 1] [--padding 3.6]", BoolOpts: []string{"inline"}, Run: func(ctx context.Context, a Args) (any, error) {
		in, err := input(a, 2)
		if err != nil {
			return nil, err
		}
		if err := newFile(in[1], in[0]); err != nil {
			return nil, err
		}
		f, err := os.Open(in[0])
		if err != nil {
			return nil, err
		}
		defer f.Close()
		data, err := io.ReadAll(io.LimitReader(f, 80001))
		if err != nil {
			return nil, err
		}
		if len(data) > 80000 {
			return nil, fmt.Errorf("equation input exceeds limit")
		}
		o := equation.Defaults()
		o.Inline = a.Bools["inline"]
		for name, dest := range map[string]*float64{"font-size": &o.FontSize, "dpi": &o.DPI, "scale": &o.Scale, "padding": &o.Padding} {
			v, ok, err := a.Float(name)
			if err != nil {
				return nil, err
			}
			if ok {
				*dest = v
			}
		}
		return equation.Write(ctx, string(data), in[1], o)
	}})
}
