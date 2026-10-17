package office

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
)

func TestTraceSharedFormulaPreservesReferencesAndSource(t *testing.T) {
	p, file := traceFixture(t, func(f *excelize.File) {
		typ, area := "shared", "C5:D6"
		if err := f.SetCellFormula("Output", "C5", `Inputs!A1+$A$1+A$2+$B3+SUM(A1:B2)+IF(A1="A1",1,0)`, excelize.FormulaOpts{Type: &typ, Ref: &area}); err != nil {
			t.Fatal(err)
		}
	})
	before, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	parts := map[string][]byte{}
	for _, part := range p.Parts {
		parts[part.Name] = bytes.Clone(part.Data)
	}
	r := traceRun(t, p, "Output", "D6", 200, 20)
	node := r.Nodes["'Output'!D6"]
	want := `='Inputs'!B2+$A$1+B$2+$B4+SUM(B2:C3)+IF(B2="A1",1,0)`
	if node.Formula == nil || *node.Formula != want {
		t.Fatalf("formula %q; want %s; issues %+v", *node.Formula, want, r.Unresolved)
	}
	if !r.Complete {
		t.Fatalf("unresolved: %+v", r.Unresolved)
	}
	for _, label := range []string{"'Inputs'!B2", "'Output'!A1", "'Output'!B2", "'Output'!B4", "'Output'!C3"} {
		hasTraceNode(t, r, label)
	}
	after, _ := os.ReadFile(file)
	if !bytes.Equal(before, after) {
		t.Fatal("source file mutated")
	}
	for _, part := range p.Parts {
		if !bytes.Equal(parts[part.Name], part.Data) {
			t.Fatal("package part mutated", part.Name)
		}
	}
}

func TestTraceSharedMalformedGroupsAreUnresolved(t *testing.T) {
	for _, kind := range []string{"missing", "duplicate", "outside", "bad-index", "reversed"} {
		t.Run(kind, func(t *testing.T) {
			p, _ := traceFixture(t, func(f *excelize.File) {
				typ, area := "shared", "C5:D6"
				if err := f.SetCellFormula("Output", "C5", "Inputs!A1", excelize.FormulaOpts{Type: &typ, Ref: &area}); err != nil {
					t.Fatal(err)
				}
			})
			wb, err := OpenWorkbook(p)
			if err != nil {
				t.Fatal(err)
			}
			sheet := wb.Sheets["Output"]
			master := Child(sheet.Cells["C5"], NSS, "f")
			follower := Child(sheet.Cells["D6"], NSS, "f")
			switch kind {
			case "missing":
				master.SetText("")
				master.RemoveAttr("ref")
			case "duplicate":
				follower.SetText("Inputs!B2")
				follower.CreateAttr("ref", "C5:D6")
			case "outside":
				master.CreateAttr("ref", "C5:C6")
			case "bad-index":
				follower.CreateAttr("si", "-1")
			case "reversed":
				master.CreateAttr("ref", "D6:C5")
			}
			if err = p.SetXML(sheet.Part, sheet.Doc); err != nil {
				t.Fatal(err)
			}
			r := traceRun(t, p, "Output", "D6", 200, 20)
			if r.Complete || len(r.Unresolved) == 0 {
				t.Fatalf("malformed group accepted: %+v", r)
			}
			if len(r.Edges) != 0 {
				t.Fatalf("invented dependencies: %+v", r.Edges)
			}
			if !strings.Contains(strings.ToLower(r.Unresolved[0].Reason), "shared") {
				t.Fatal(r.Unresolved)
			}
		})
	}
}

func TestSharedFormulaTranslationEdges(t *testing.T) {
	for _, c := range []struct {
		input, want string
		dx, dy      int
	}{
		{`'O''Brien'!A1+"say ""A1"""`, `'O''Brien'!B2+"say ""A1"""`, 1, 1},
		{`SUM(A:C,$A:$C,1:3,$1:$3)`, `SUM(B:D,$A:$C,2:4,$1:$3)`, 1, 1},
		{`SUM('Inputs:Output'!A1:B2)+Driver`, `SUM('Inputs:Output'!B2:C3)+Driver`, 1, 1},
		{`B2+$B2+B$2+$B$2`, `A1+$B1+A$2+$B$2`, -1, -1},
	} {
		got, err := translateSharedFormula(c.input, c.dx, c.dy)
		if err != nil || got != c.want {
			t.Errorf("%s: got %q err %v; want %q", c.input, got, err, c.want)
		}
	}
	for _, s := range []string{`A1`, `A:A`, `1:1`} {
		if _, err := translateSharedFormula(s, -1, -1); err == nil {
			t.Errorf("accepted out-of-range shift %s", s)
		}
	}
}

func TestSharedFormulaStructuredHeadersStayLiteral(t *testing.T) {
	for _, c := range []struct{ input, want string }{
		{`SUM(Sales[Q1:Q2:Q3])+A1`, `SUM(Sales[Q1:Q2:Q3])+B2`},
		{`SUM(Sales[Revenue!A1])+A1`, `SUM(Sales[Revenue!A1])+B2`},
		{`SUM(Sales[[#Data],[Q1:Q2:Q3]])+$A1`, `SUM(Sales[[#Data],[Q1:Q2:Q3]])+$A2`},
		{`SUM(Sales[[A1]:[B2]])+A$1`, `SUM(Sales[[A1]:[B2]])+B$1`},
		{`[@[Q1:Q2:Q3]]+$A$1`, `[@[Q1:Q2:Q3]]+$A$1`},
	} {
		got, err := translateSharedFormula(c.input, 1, 1)
		if err != nil || got != c.want {
			t.Errorf("%s: got %q (%v); want %q", c.input, got, err, c.want)
		}
	}
}

func TestTraceSharedFormulaStructuredHeaderWorkbook(t *testing.T) {
	for _, tc := range []struct{ header, selector string }{
		{"Q1:Q2:Q3", "[Q1:Q2:Q3]"}, {"Revenue!A1", "[Revenue!A1]"},
		{"Cost]2024", "[[Cost']2024]]"}, {"Cost[2024", "[[Cost'[2024]]"},
		{"#Data", "[['#Data]]"}, {"@Revenue", "[['@Revenue]]"},
		{"O'Brien", "[[O''Brien]]"}, {"[A1]#@'!", "[['[A1']'#'@''!]]"},
		{"Cost]2024", "[[#Data],[Cost']2024]]"},
	} {
		header, selector := tc.header, tc.selector
		t.Run(header+selector, func(t *testing.T) {
			p, file := traceFixture(t, func(f *excelize.File) {
				for cell, value := range map[string]any{"E1": header, "E2": 11, "E3": 13} {
					if err := f.SetCellValue("Inputs", cell, value); err != nil {
						t.Fatal(err)
					}
				}
				if err := f.AddTable("Inputs", &excelize.Table{Range: "E1:E3", Name: "Sales"}); err != nil {
					t.Fatal(err)
				}
				typ, area := "shared", "C5:D6"
				if err := f.SetCellFormula("Output", "C5", `SUM(Sales`+selector+`)+Inputs!A1`, excelize.FormulaOpts{Type: &typ, Ref: &area}); err != nil {
					t.Fatal(err)
				}
			})
			before, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			parts := map[string][]byte{}
			for _, part := range p.Parts {
				parts[part.Name] = bytes.Clone(part.Data)
			}
			r := traceRun(t, p, "Output", "D6", 200, 20)
			want := `=SUM(Sales` + selector + `)+'Inputs'!B2`
			n := r.Nodes["'Output'!D6"]
			if !r.Complete || n.Formula == nil || *n.Formula != want {
				t.Fatalf("unexpected trace %+v", r)
			}
			if len(r.Nodes) != 4 || len(r.Edges) != 3 {
				t.Fatalf("unexpected dependencies %+v", r)
			}
			for _, label := range []string{"'Inputs'!E2", "'Inputs'!E3", "'Inputs'!B2"} {
				hasTraceNode(t, r, label)
			}
			if r.Nodes["'Inputs'!E2"].CachedValue != float64(11) || r.Nodes["'Inputs'!E3"].CachedValue != float64(13) {
				t.Fatal(r.Nodes)
			}
			after, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(before, after) {
				t.Fatal("source changed")
			}
			for _, part := range p.Parts {
				if !bytes.Equal(parts[part.Name], part.Data) {
					t.Fatal("part changed", part.Name)
				}
			}
		})
	}
}

func TestSharedEscapedSelectorTranslationAndRejection(t *testing.T) {
	for _, tc := range []struct{ input, want string }{
		{`SUM(Sales[[Cost']2024]:['#Data]])+A1`, `SUM(Sales[[Cost']2024]:['#Data]])+B2`},
		{`[@['@Revenue]]+$A1`, `[@['@Revenue]]+$A2`},
		{`SUM(Sales[[O''Brien]])+"Sales[Cost']2024]"+A$1`, `SUM(Sales[[O''Brien]])+"Sales[Cost']2024]"+B$1`},
	} {
		got, err := translateSharedFormula(tc.input, 1, 1)
		if err != nil || got != tc.want {
			t.Errorf("%s: %q, %v; want %q", tc.input, got, err, tc.want)
		}
	}
	for _, formula := range []string{`SUM(Sales[Cost'])+A1`, `SUM(Sales[[O''Brien])+A1`, `SUM(Sales[['[A1']])+A1`} {
		if _, err := translateSharedFormula(formula, 1, 1); err == nil {
			t.Errorf("accepted unbalanced selector %s", formula)
		}
	}
}

func TestSharedFormulaColonChains(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{`SUM(A1:B2:C3)+D1`, `SUM(B2:C3:D4)+E2`},
		{`SUM($A1:B$2:$C$3)+D1`, `SUM($A2:C$2:$C$3)+E2`},
		{`SUM(First:A1:Last)+D1`, `SUM(First:B2:Last)+E2`},
		{`SUM('Inputs'!A1:B2:C3)+D1`, `SUM('Inputs'!B2:C3:D4)+E2`},
		{`SUM(A1:B2:C3)+"A1:B2:C3"`, `SUM(B2:C3:D4)+"A1:B2:C3"`},
	} {
		got, err := translateSharedFormula(tc.in, 1, 1)
		if err != nil || got != tc.want {
			t.Errorf("%s: got %q (%v), want %s", tc.in, got, err, tc.want)
		}
	}
	if _, err := translateSharedFormula(`SUM(B2:C3:A1)`, -1, -1); err == nil {
		t.Fatal("out-of-bounds endpoint accepted")
	}
}

func TestTraceSharedColonChainRetainsIndependentDependencies(t *testing.T) {
	p, file := traceFixture(t, func(f *excelize.File) {
		typ, area := "shared", "C5:D6"
		if err := f.SetCellFormula("Output", "C5", `SUM(Inputs!A1:B2:C3)+Inputs!C1`, excelize.FormulaOpts{Type: &typ, Ref: &area}); err != nil {
			t.Fatal(err)
		}
		if err := f.SetCellValue("Inputs", "D2", 17); err != nil {
			t.Fatal(err)
		}
	})
	before, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	parts := map[string][]byte{}
	for _, part := range p.Parts {
		parts[part.Name] = bytes.Clone(part.Data)
	}
	r := traceRun(t, p, "Output", "D6", 200, 20)
	want := `=SUM('Inputs'!B2:C3:D4)+'Inputs'!D2`
	if f := r.Nodes["'Output'!D6"].Formula; f == nil || *f != want {
		t.Fatalf("formula: %v; issues: %+v", f, r.Unresolved)
	}
	if r.Complete || r.Truncated || len(r.Unresolved) != 1 || r.Unresolved[0].Reference != "'Inputs'!B2:C3:D4" {
		t.Fatalf("issues: %+v", r)
	}
	if len(r.Nodes) != 2 || len(r.Edges) != 1 || r.Edges[0] != [2]string{"'Output'!D6", "'Inputs'!D2"} || r.Nodes["'Inputs'!D2"].CachedValue != float64(17) {
		t.Fatalf("dependencies: %+v", r)
	}
	after, err := os.ReadFile(file)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("source changed", err)
	}
	for _, part := range p.Parts {
		if !bytes.Equal(parts[part.Name], part.Data) {
			t.Fatal("package changed", part.Name)
		}
	}
}
