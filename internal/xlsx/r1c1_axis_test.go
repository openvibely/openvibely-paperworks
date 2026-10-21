package xlsx

import "testing"

func TestR1C1WholeRowsAndColumns(t *testing.T) {
	for _, tc := range []struct{ formula, want string }{
		{"SUM(C1)", "SUM($A:$A)"},
		{"SUM(R1)", "SUM($1:$1)"},
		{"SUM(C)", "SUM(D:D)"},
		{"SUM(R)", "SUM(5:5)"},
		{"SUM(C[-2]:C[-1])", "SUM(B:C)"},
		{"SUM(R[-2]:R[-1])", "SUM(3:4)"},
		{"SUM(C1:C[1])", "SUM($A:E)"},
		{"SUM(C1 : C[1])", "SUM($A:E)"},
		{"SUM(R : R[1])", "SUM(5:6)"},
		{"SUM(R1:R[1])", "SUM($1:6)"},
		{"SUM(c[+1])+SUM(r[+1])", "SUM(E:E)+SUM(6:6)"},
		{"SUM(Data!C1:C3)", "SUM(Data!$A:$C)"},
		{"SUM('O''Brien'!R1:R3)", "SUM('O''Brien'!$1:$3)"},
		{"SUM(C1!C1:C3)", "SUM(C1!$A:$C)"},
		{"SUM([1]Data!C1)", "SUM([1]Data!$A:$A)"},
		{"SUM(S1:S2!C1:C3)", "SUM(S1:S2!$A:$C)"},
		{"SUM(Data!C1:Data!C3)", "SUM(Data!$A:Data!$C)"},
		{"SUM('Data'!R1:'Data'!R3)", "SUM('Data'!$1:'Data'!$3)"},
		{"SUM([1]Data!C1:[1]Data!C3)", "SUM([1]Data!$A:[1]Data!$C)"},
		{`SUM(Table1[C1])+LEN("R1:C1")+Revenue+Column1`, `SUM(Table1[C1])+LEN("R1:C1")+Revenue+Column1`},
		{"SUM(Table1[x']C1])+R1C1", "SUM(Table1[x']C1])+$A$1"},
		{"R1C1+R[-1]C+RC[+1]", "$A$1+D4+E5"},
		{"SUM(C16384)+SUM(R1048576)", "SUM($XFD:$XFD)+SUM($1048576:$1048576)"},
	} {
		t.Run(tc.formula, func(t *testing.T) {
			got, err := r1c1Formula(tc.formula, "D5")
			if err != nil || got != tc.want {
				t.Fatalf("got %q (%v), want %q", got, err, tc.want)
			}
		})
	}
}

func TestR1C1WholeAxisRejectsInvalidBounds(t *testing.T) {
	for _, formula := range []string{"SUM(C0)", "SUM(R0)", "SUM(C16385)", "SUM(R1048577)", "SUM(C[-4])", "SUM(R[-5])", "SUM(C1:C16385)", "SUM(R1:R1048577)", "SUM(C1:R3)", "SUM(C[9223372036854775807])"} {
		if got, err := r1c1Formula(formula, "D5"); err == nil {
			t.Errorf("accepted %q as %q", formula, got)
		}
	}
}
