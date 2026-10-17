package office

import (
	"bytes"
	"os"
	"testing"

	"github.com/xuri/excelize/v2"
)

func TestTraceArrayErrorOperands(t *testing.T) {
	for _, tc := range []struct {
		name, literal string
		errors        []string
	}{
		{"reference", `{#REF!,1}`, []string{"#REF!"}},
		{"repeated", `{#REF!,#REF!}`, []string{"#REF!", "#REF!"}},
		{"other errors", `{#NULL!,#NAME?}`, []string{"#NULL!", "#NAME?"}},
		{"multiple", `{#DIV/0!,#N/A;#VALUE!,#NUM!}`, []string{"#DIV/0!", "#N/A", "#VALUE!", "#NUM!"}},
		{"quoted", `{"#REF!","say ""#N/A"""}`, nil},
		{"mixed", `{"#REF!",#REF!;"#VALUE!",#VALUE!}`, []string{"#REF!", "#VALUE!"}},
	} {
		for _, shared := range []bool{false, true} {
			t.Run(tc.name+map[bool]string{true: "/shared", false: "/ordinary"}[shared], func(t *testing.T) {
				formula := `SUM(` + tc.literal + `)+Inputs!A1`
				p, file := traceFixture(t, func(f *excelize.File) {
					var opts []excelize.FormulaOpts
					if shared {
						typ, area := "shared", "C5:D6"
						opts = append(opts, excelize.FormulaOpts{Type: &typ, Ref: &area})
					}
					if err := f.SetCellFormula("Output", "C5", formula, opts...); err != nil {
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
				cell, target, want := "C5", "A1", "="+formula
				if shared {
					cell, target, want = "D6", "B2", `=SUM(`+tc.literal+`)+'Inputs'!B2`
				}
				r := traceRun(t, p, "Output", cell, 200, 20)
				if r.Complete != (len(tc.errors) == 0) || r.Truncated || len(r.Unresolved) != len(tc.errors) {
					t.Fatalf("unexpected report %+v", r)
				}
				if len(r.Nodes) != 2 || len(r.Edges) != 1 || r.Edges[0] != [2]string{traceLabel("Output", cell), traceLabel("Inputs", target)} {
					t.Fatalf("dependencies %+v", r)
				}
				n := r.Nodes[traceLabel("Output", cell)]
				if n.Formula == nil || *n.Formula != want {
					t.Fatalf("formula %+v; want %s", n, want)
				}
				for i, value := range tc.errors {
					issue := r.Unresolved[i]
					if issue.Reference != value || issue.Cell != r.Root || issue.Reason != "Formula contains an error reference/value" {
						t.Fatalf("issue %+v", issue)
					}
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
}
