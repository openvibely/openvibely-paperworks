package pdf

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

func TestFillStrictChoices(t *testing.T) {
	for _, flatten := range []bool{false, true} {
		for _, tc := range []struct {
			name  string
			flags int
			value any
			bad   string
		}{
			{"list-invalid", 0, "Custom", "Custom"},
			{"list-valid", 0, "Beta", ""},
			{"list-clear", 0, "", ""},
			{"multi-invalid", 1 << 21, []string{"Alpha", "Typo"}, "Typo"},
			{"multi-valid", 1 << 21, []string{"Alpha", "Beta"}, ""},
			{"multi-clear", 1 << 21, []string{}, ""},
			{"combo-invalid", ffCombo, "Custom", "Custom"},
			{"combo-valid", ffCombo, "Alpha", ""},
			{"combo-clear", ffCombo, "", ""},
			{"editable-custom", ffCombo | 1<<18, "Custom", ""},
		} {
			t.Run(fmt.Sprintf("%s/flatten=%v", tc.name, flatten), func(t *testing.T) {
				ctx := context.Background()
				objs := formObjs()
				objs[14] = strings.Replace(objs[14], "/Ff 131072", fmt.Sprintf("/Ff %d", tc.flags), 1)
				original := rawPDF(objs)
				source := writeTemp(t, "source.pdf", original)
				for _, strict := range []bool{true, false} {
					output := filepath.Join(t.TempDir(), "filled.pdf")
					result, err := FillValues(ctx, source, output, map[string]any{"choice": tc.value}, flatten, FillOptions{StrictChoices: strict})
					if strict && tc.bad != "" {
						var warning ChoiceWarning
						if !errors.As(err, &warning) || warning.Field != "choice" || warning.Value != tc.bad || !reflect.DeepEqual(warning.AllowedChoices, []string{"Alpha", "Beta"}) {
							t.Fatalf("error = %#v", err)
						}
						if _, err := os.Stat(output); !os.IsNotExist(err) {
							t.Fatalf("rejection created output: %v", err)
						}
					} else {
						if err != nil {
							t.Fatal(err)
						}
						if tc.bad != "" {
							if len(result.Warnings) != 1 || result.Warnings[0].Value != tc.bad || result.Warnings[0].Code != "unmatched_choice" {
								t.Fatalf("warnings = %+v", result.Warnings)
							}
						} else if len(result.Warnings) != 0 {
							t.Fatalf("unexpected warnings: %+v", result.Warnings)
						}
					}
					after, err := os.ReadFile(source)
					if err != nil || !bytes.Equal(original, after) {
						t.Fatal("source changed", err)
					}
				}
			})
		}
	}
}

func TestChoiceValidationInheritedExportValues(t *testing.T) {
	ctx := context.Background()
	c, err := Load(ctx, formPDF())
	if err != nil {
		t.Fatal(err)
	}
	x := c.XRefTable
	export, err := x.IndRefForNewObject(types.StringLiteral("export"))
	if err != nil {
		t.Fatal(err)
	}
	pair, err := x.IndRefForNewObject(types.Array{*export, types.StringLiteral("Display")})
	if err != nil {
		t.Fatal(err)
	}
	opts, err := x.IndRefForNewObject(types.Array{*pair, types.StringLiteral("Beta")})
	if err != nil {
		t.Fatal(err)
	}
	parent, err := x.IndRefForNewObject(types.Dict{"Opt": *opts})
	if err != nil {
		t.Fatal(err)
	}
	d := types.Dict{"Parent": *parent}
	fields := map[string]*field{"choice": {FT: "Ch", D: d}}
	for _, value := range []string{"export", "Beta", "", "Display", "Typo"} {
		warnings := validateChoiceValues(x, fields, map[string]string{"choice": value}, nil)
		wantBad := value == "Display" || value == "Typo"
		if (len(warnings) > 0) != wantBad {
			t.Fatalf("%q: %+v", value, warnings)
		}
		if wantBad && !reflect.DeepEqual(warnings[0].AllowedChoices, []string{"export", "Beta"}) {
			t.Fatal(warnings)
		}
	}
	// An empty local /Opt shadows inherited options. Empty array items are values,
	// not the empty-array clearing operation, unless explicitly allowed by /Opt.
	d["Opt"] = types.Array{}
	warnings := validateChoiceValues(x, fields, map[string]string{"choice": "[]"}, map[string][]string{"choice": {""}})
	if len(warnings) != 1 || len(warnings[0].AllowedChoices) != 0 {
		t.Fatal(warnings)
	}
	d["Opt"] = types.Array{types.StringLiteral("")}
	if warnings := validateChoiceValues(x, fields, map[string]string{"choice": "[]"}, map[string][]string{"choice": {""}}); len(warnings) != 0 {
		t.Fatal(warnings)
	}
}

func TestFillStrictChoicesInheritedOptions(t *testing.T) {
	for _, flatten := range []bool{false, true} {
		for _, value := range []string{"A", "Beta", "Alpha", ""} {
			t.Run(fmt.Sprintf("%q/flatten=%v", value, flatten), func(t *testing.T) {
				objs := formObjs()
				objs[0] = strings.Replace(objs[0], "13 0 R 15 0 R", "13 0 R 29 0 R", 1)
				objs[14] = strings.Replace(objs[14], "/Opt [(Alpha) (Beta)]", "/Parent 29 0 R", 1)
				objs = append(objs, "<< /T (group) /Opt [[(A) (Alpha)] (Beta)] /Kids [15 0 R] >>")
				source := writeTemp(t, "source.pdf", rawPDF(objs))
				output := filepath.Join(t.TempDir(), "filled.pdf")
				result, err := Fill(context.Background(), source, output, map[string]string{"group.choice": value}, flatten, FillOptions{StrictChoices: true})
				if value == "Alpha" {
					var warning ChoiceWarning
					if !errors.As(err, &warning) || warning.Field != "group.choice" || !reflect.DeepEqual(warning.AllowedChoices, []string{"A", "Beta"}) {
						t.Fatalf("error: %v", err)
					}
					if _, err := os.Stat(output); !os.IsNotExist(err) {
						t.Fatal("output exists", err)
					}
				} else if err != nil || len(result.Warnings) != 0 {
					t.Fatalf("result: %+v, error: %v", result, err)
				}
			})
		}
	}
}

func TestFillStrictChoicesFlattenExistingValue(t *testing.T) {
	objs := formObjs()
	objs[14] = strings.Replace(objs[14], "/V (Alpha)", "/V (Custom)", 1)
	source := writeTemp(t, "source.pdf", rawPDF(objs))
	for _, flatten := range []bool{false, true} {
		output := filepath.Join(t.TempDir(), "filled.pdf")
		_, err := Fill(context.Background(), source, output, map[string]string{"name": "Ada"}, flatten, FillOptions{StrictChoices: true})
		if !flatten {
			if err != nil {
				t.Fatal(err)
			}
		} else {
			var warning ChoiceWarning
			if !errors.As(err, &warning) || warning.Value != "Custom" {
				t.Fatal(err)
			}
			if _, err := os.Stat(output); !os.IsNotExist(err) {
				t.Fatal("output exists", err)
			}
		}
	}
}
