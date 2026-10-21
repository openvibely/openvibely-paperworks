package xlsx

import "testing"

func TestTranslateFormulaFunctionWhitespace(t *testing.T) {
	for _, tc := range []struct{ formula, want string }{
		{"LOG10(A1)", "LOG10(B2)"},
		{"LOG10 (A1)", "LOG10 (B2)"},
		{"log10  ($A1+A$1)", "log10  ($A2+B$1)"},
		{"LOG10\t\r\n(A1)", "LOG10\t\r\n(B2)"},
		{"LOG10 (LOG10 (A1))+LOG10", "LOG10 (LOG10 (B2))+LOH11"},
		{`"LOG10 (A1)"&LOG10 (A1)`, `"LOG10 (A1)"&LOG10 (B2)`},
		// Whitespace can also be the reference intersection operator.
		{"A1 B1", "B2 C2"},
		{"A1 (A1:B2)", "B2 (B2:C3)"},
		{"$A1 ($A1:B2)", "$A2 ($A2:C3)"},
	} {
		t.Run(tc.formula, func(t *testing.T) {
			got, err := TranslateFormula(tc.formula, 1, 1)
			if err != nil || got != tc.want {
				t.Fatalf("got %q (%v), want %q", got, err, tc.want)
			}
		})
	}
}
