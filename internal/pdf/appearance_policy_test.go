package pdf

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

func TestFillDisablesViewerAppearanceRegeneration(t *testing.T) {
	for _, policy := range []string{"absent", "true", "false"} {
		for _, flatten := range []bool{false, true} {
			t.Run(policy+map[bool]string{true: "-flat", false: "-interactive"}[flatten], func(t *testing.T) {
				ctx := context.Background()
				c, err := Load(ctx, formPDF())
				if err != nil {
					t.Fatal(err)
				}
				cat, err := c.XRefTable.Catalog()
				if err != nil {
					t.Fatal(err)
				}
				form := dictOf(c.XRefTable, cat["AcroForm"]).d
				delete(form, "NeedAppearances")
				if policy != "absent" {
					form["NeedAppearances"] = types.Boolean(policy == "true")
				}
				data, err := Serialize(ctx, c)
				if err != nil {
					t.Fatal(err)
				}
				source := writeTemp(t, "source.pdf", data)
				output := filepath.Join(t.TempDir(), "filled.pdf")
				if _, err := Fill(ctx, source, output, map[string]string{"name": "Ada"}, flatten); err != nil {
					t.Fatal(err)
				}
				saved, _, err := LoadFile(ctx, output)
				if err != nil {
					t.Fatal(err)
				}
				root, err := saved.XRefTable.Catalog()
				if err != nil {
					t.Fatal(err)
				}
				if flatten {
					if root["AcroForm"] != nil {
						t.Fatal("flattened output retains form")
					}
				} else {
					got := dictOf(saved.XRefTable, root["AcroForm"]).d["NeedAppearances"]
					if got != types.Boolean(false) {
						t.Fatalf("saved regeneration request = %v", got)
					}
					report, err := Report(ctx, output)
					if err != nil {
						t.Fatal(err)
					}
					if report.Fields["name"] == nil || *report.Fields["name"] != "Ada" {
						t.Fatal("filled value lost")
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
	}
}
