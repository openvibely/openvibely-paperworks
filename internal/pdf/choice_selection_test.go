package pdf

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

func TestFillScalarChoiceSelectionState(t *testing.T) {
	for _, combo := range []bool{false, true} {
		t.Run(fmt.Sprintf("combo=%v", combo), func(t *testing.T) {
			for _, value := range []string{"Alpha", "Beta", "Custom"} {
				t.Run(value, func(t *testing.T) {
					ctx := context.Background()
					c, err := Load(ctx, formPDF())
					if err != nil {
						t.Fatal(err)
					}
					fields, _, err := fieldTree(c.XRefTable)
					if err != nil {
						t.Fatal(err)
					}
					if !combo {
						fields["choice"].D["Ff"] = types.Integer(0)
					}
					fields["choice"].D["I"] = types.Array{types.Integer(0)}
					data, err := Serialize(ctx, c)
					if err != nil {
						t.Fatal(err)
					}
					source := writeTemp(t, "source.pdf", data)
					for _, edit := range []bool{false, true} {
						values := map[string]string{"name": "Ada"}
						if edit {
							values = map[string]string{"choice": value}
						}
						output := filepath.Join(t.TempDir(), "filled.pdf")
						if _, err := Fill(ctx, source, output, values, false); err != nil {
							t.Fatal(err)
						}
						saved, _, err := LoadFile(ctx, output)
						if err != nil {
							t.Fatal(err)
						}
						got, _, err := fieldTree(saved.XRefTable)
						if err != nil {
							t.Fatal(err)
						}
						choice := got["choice"].D
						if edit {
							if choice["I"] != nil {
								t.Fatalf("edited scalar choice retained indices: %v", choice["I"])
							}
							if v := display(saved.XRefTable, choice["V"]); v == nil || *v != value {
								t.Fatalf("saved value = %v", v)
							}
						} else {
							indices, ok := entryArray(saved.XRefTable, choice["I"])
							if !ok || len(indices) != 1 || indices[0] != types.Integer(0) {
								t.Fatalf("untouched selection changed: %v", choice["I"])
							}
						}
					}
					after, err := os.ReadFile(source)
					if err != nil {
						t.Fatal(err)
					}
					if !bytes.Equal(data, after) {
						t.Fatal("source changed")
					}
				})
			}
		})
	}
}
