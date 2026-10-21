package xlsx

import "testing"

func TestTranslateFormulaPreservesNamesOutsideGrid(t *testing.T) {
	for _, tc := range []struct {
		formula, want string
		dx, dy        int
	}{
		{"A1048577+A2", "A1048577+A1", 0, -1},
		{"XFE1+A1", "XFE1+B2", 1, 1},
		{"ZZZ42+XFD1048576", "ZZZ42+XFC1048575", -1, -1},
		{"A999999999999999999999999+A1", "A999999999999999999999999+B2", 1, 1},
		{`Data!A1048577+'Other sheet'!XFE1+A2`, `Data!A1048577+'Other sheet'!XFE1+A1`, 0, -1},
		{`\A1+A1`, `\A1+B2`, 1, 1},
		{"SUM(XFE:XFF)+A1", "SUM(XFE:XFF)+B2", 1, 1},
		{"SUM(XFD:XFE)+A1", "SUM(XFD:XFE)+B2", 1, 1},
	} {
		t.Run(tc.formula, func(t *testing.T) {
			got, err := TranslateFormula(tc.formula, tc.dx, tc.dy)
			if err != nil || got != tc.want {
				t.Fatalf("got %q (%v), want %q", got, err, tc.want)
			}
		})
	}
}

func TestTranslateFormulaRejectsInvalidReferences(t *testing.T) {
	for _, tc := range []struct {
		formula string
		dx, dy  int
	}{
		{"A1", -1, 0}, {"A1", 0, -1}, {"XFD1", 1, 0}, {"A1048576", 0, 1},
		{"$XFE$1", 0, 0}, {"$A$1048577", 0, -1}, {"$XFE:$XFF", 0, 0},
	} {
		if got, err := TranslateFormula(tc.formula, tc.dx, tc.dy); err == nil {
			t.Errorf("accepted %s as %s", tc.formula, got)
		}
	}
}
