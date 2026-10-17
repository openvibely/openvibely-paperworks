package equation

import (
	"bytes"
	"context"
	"testing"
)

// Exact raster equality verifies argument placement, font, geometry and ink,
// not merely successful parsing or presence of a PNG.
func TestOptionalMacroExpansionPixels(t *testing.T) {
	cases := []struct{ name, input, expanded string }{
		{"command default", `\newcommand{\pair}[2][x]{\frac{#1}{#2}}\pair{y}`, `\frac{x}{y}`},
		{"command override", `\newcommand{\pair}[2][x]{\frac{#1}{#2}}\pair[z]{y}`, `\frac{z}{y}`},
		{"empty default", `\newcommand{\pair}[2][]{#1+#2}\pair{y}`, `+y`},
		{"grouped bracket", `\newcommand{\pair}[2][{[x]}]{#1+#2}\pair{y}`, `{[x]}+y`},
		{"nested default", `\newcommand{\base}{z}\newcommand{\pair}[2][\base]{#1+#2}\pair{y}`, `z+y`},
		{"renewcommand", `\newcommand{\pair}[1]{#1}\renewcommand{\pair}[2][z]{#1+#2}\pair{y}`, `z+y`},
		{"providecommand", `\newcommand{\pair}[1]{#1}\providecommand{\pair}[2][z]{#1+#2}\pair{y}`, `y`},
		{"environment default", `\newenvironment{pair}[2][x]{\left( #1+#2+}{\right)}\begin{pair}{y}z\end{pair}`, `\left( x+y+z\right)`},
		{"environment override", `\newenvironment{pair}[2][x]{\left( #1+#2+}{\right)}\begin{pair}[w]{y}z\end{pair}`, `\left( w+y+z\right)`},
		{"environment redefine", `\newenvironment{pair}{x}{y}\renewenvironment{pair}[1][z]{#1+}{+w}\begin{pair}v\end{pair}`, `z+v+w`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := PNG(context.Background(), c.input, Defaults())
			if err != nil {
				t.Fatal(err)
			}
			want, err := PNG(context.Background(), c.expanded, Defaults())
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, want) {
				t.Fatal("macro rendering differs from explicit expansion")
			}
		})
	}
}

func TestMalformedMacroDefinitions(t *testing.T) {
	for _, s := range []string{
		`\newcommand{\foo}[10]{x}\foo`, `\newcommand{\foo}[1x]{x}\foo`,
		`\newcommand{\foo}[0][x]{x}\foo`, `\newcommand{\foo}[1]{#2}\foo{x}`,
		`\newcommand{\foo}[1]{#}\foo{x}`, `\newcommand{\foo}[1][x]{#1}\foo[y`,
		`\renewenvironment{missing}{x}{y}\begin{missing}z\end{missing}`,
		`\newenvironment{foo}[0][x]{x}{y}\begin{foo}z\end{foo}`,
		`\newenvironment{foo}[1]{#2}{y}\begin{foo}{x}z\end{foo}`,
	} {
		if _, err := PNG(context.Background(), s, Defaults()); err == nil {
			t.Errorf("accepted %s", s)
		}
	}
}
