package pdf

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
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

// Compare export-coded choices with equivalent identity-mapped choices. Both
// saved streams and rendered pages must agree, including selection highlights.
func TestFillPairedChoiceAppearances(t *testing.T) {
	for _, tc := range []struct {
		name   string
		flags  int
		value  any
		label  any
		stored string
	}{
		{"combo", ffCombo, "CA", "California", "CA"},
		{"editable-custom", ffCombo | 1<<18, "Custom", "Custom", "Custom"},
		{"single-list", 0, "CA", "California", "CA"},
		{"multi-list-scalar", 1 << 21, "CA", "California", "CA"},
		{"multi-list", 1 << 21, []string{"CA", "NY"}, []string{"California", "New York"}, "[CA, NY]"},
		{"mixed-string-option", 0, "Other", "Other", "Other"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			var sources [2]string
			for i, options := range []string{
				"[[(NY) (New York)] [(CA) (California)] (Other)]",
				"[[(New York) (New York)] [(California) (California)] (Other)]",
			} {
				objs := formObjs()
				objs[14] = fmt.Sprintf("<< /FT /Ch /Ff %d /T (choice) /V (NY) /Opt %s /DA (/Helv 12 Tf 0 g) /Kids [16 0 R] >>", tc.flags, options)
				objs[15] = strings.Replace(objs[15], "220 220", "220 280", 1)
				objs[27] = strings.Replace(objs[27], "180 20", "180 80", 1)
				sources[i] = writeTemp(t, "source.pdf", rawPDF(objs))
			}
			outputDir := t.TempDir()
			for _, mode := range []string{"editable", "flatten", "flatten-existing"} {
				t.Run(mode, func(t *testing.T) {
					var streams [2][]byte
					var documents [2][]byte
					for i, value := range []any{tc.value, tc.label} {
						source := sources[i]
						values := map[string]any{"choice": value}
						if mode == "flatten-existing" {
							existing := filepath.Join(t.TempDir(), "existing.pdf")
							if _, err := FillValues(ctx, source, existing, values, false); err != nil {
								t.Fatal(err)
							}
							source, values = existing, nil
						}
						output := filepath.Join(outputDir, fmt.Sprintf("%s-%d.pdf", mode, i))
						if _, err := FillValues(ctx, source, output, values, mode != "editable"); err != nil {
							t.Fatal(err)
						}
						c, data, err := LoadFile(ctx, output)
						if err != nil {
							t.Fatal(err)
						}
						documents[i] = data
						var object types.Object
						if mode == "editable" {
							fields, _, err := fieldTree(c.XRefTable)
							if err != nil {
								t.Fatal(err)
							}
							f := fields["choice"]
							if i == 0 {
								if got := display(c.XRefTable, f.D["V"]); got == nil || *got != tc.stored {
									t.Fatalf("stored value = %v, want %s", got, tc.stored)
								}
							}
							object = dictOf(c.XRefTable, f.Widgets[0]["AP"]).d["N"]
						} else {
							page, inh, err := pageDict(ctx, c, 2)
							if err != nil {
								t.Fatal(err)
							}
							if page["Annots"] != nil {
								t.Fatal("flatten left annotations")
							}
							object = dictOf(c.XRefTable, inh.Resources["XObject"]).d["Fm_widget_1"]
						}
						sd, _ := streamOf(c.XRefTable, object)
						if sd == nil {
							t.Fatal("missing saved choice appearance")
						}
						if err := sd.Decode(); err != nil {
							t.Fatal(err)
						}
						streams[i] = sd.Content
					}
					if !bytes.Equal(streams[0], streams[1]) {
						t.Fatalf("export/display appearance mismatch:\n%s\nwant:\n%s", streams[0], streams[1])
					}
					want := "California"
					if tc.name == "editable-custom" {
						want = "Custom"
					}
					if !bytes.Contains(streams[0], []byte("("+want+")")) {
						t.Fatalf("missing display label %q: %s", want, streams[0])
					}
					if !bytes.Equal(images(t, documents[0])[1].Pix, images(t, documents[1])[1].Pix) {
						t.Fatal("export/display rendered pages differ")
					}
				})
			}
		})
	}
}

func inheritedPairedChoicePDF(localOptions, value, appearance string) []byte {
	objs := formObjs()
	objs[0] = strings.Replace(objs[0], "13 0 R 15 0 R", "13 0 R 29 0 R", 1)
	objs[14] = strings.Replace(objs[14], "/Ff 131072", "/Ff 0", 1)
	objs[14] = strings.Replace(objs[14], "/V (Alpha) /Opt [(Alpha) (Beta)]", "/V ("+value+") /Parent 29 0 R", 1)
	objs[14] = strings.Replace(objs[14], "/Parent 29 0 R", "/Parent 29 0 R"+localOptions, 1)
	objs[27] = strings.Replace(objs[27], "(Alpha)", "("+appearance+")", 1)
	objs = append(objs, "<< /T (group) /Opt [[(A) (Alpha)] [(B) (Beta)]] /Kids [15 0 R] >>")
	return rawPDF(objs)
}

func savedChoiceAppearance(t *testing.T, c *model.Context, flattened bool) []byte {
	t.Helper()
	var object types.Object
	if flattened {
		page, inh, err := pageDict(context.Background(), c, 2)
		if err != nil {
			t.Fatal(err)
		}
		if page["Annots"] != nil {
			t.Fatal("flattened output retains page annotations")
		}
		object = dictOf(c.XRefTable, inh.Resources["XObject"]).d["Fm_widget_1"]
	} else {
		fields, _, err := fieldTree(c.XRefTable)
		if err != nil {
			t.Fatal(err)
		}
		object = dictOf(c.XRefTable, fields["group.choice"].Widgets[0]["AP"]).d["N"]
	}
	sd, _ := streamOf(c.XRefTable, object)
	if sd == nil {
		t.Fatal("missing saved choice appearance")
	}
	if err := sd.Decode(); err != nil {
		t.Fatal(err)
	}
	return append([]byte(nil), sd.Content...)
}

func TestFillInheritedPairedChoiceAppearances(t *testing.T) {
	ctx := context.Background()
	inheritedSource := writeTemp(t, "inherited.pdf", inheritedPairedChoicePDF("", "A", "stale"))
	directSource := writeTemp(t, "direct.pdf", inheritedPairedChoicePDF(" /Opt [[(A) (Alpha)] [(B) (Beta)]]", "A", "stale"))

	for _, flatten := range []bool{false, true} {
		for _, strict := range []bool{false, true} {
			t.Run(fmt.Sprintf("flatten=%v/strict=%v", flatten, strict), func(t *testing.T) {
				var docs [2][]byte
				var appearances [2][]byte
				for i, source := range []string{inheritedSource, directSource} {
					output := filepath.Join(t.TempDir(), "filled.pdf")
					result, err := Fill(ctx, source, output, map[string]string{"group.choice": "A"}, flatten, FillOptions{StrictChoices: strict})
					if err != nil || len(result.Warnings) != 0 {
						t.Fatalf("fill result=%+v error=%v", result, err)
					}
					c, data, err := LoadFile(ctx, output)
					if err != nil {
						t.Fatal(err)
					}
					docs[i] = data
					appearances[i] = savedChoiceAppearance(t, c, flatten)
					if !flatten {
						fields, _, err := fieldTree(c.XRefTable)
						if err != nil {
							t.Fatal(err)
						}
						if got := display(c.XRefTable, fields["group.choice"].D["V"]); got == nil || *got != "A" {
							t.Fatalf("saved export value = %v, want A", got)
						}
					}
					if !bytes.Contains(appearances[i], []byte("(Alpha)")) {
						t.Fatalf("appearance does not show Alpha: %s", appearances[i])
					}
				}
				if !bytes.Equal(appearances[0], appearances[1]) {
					t.Fatalf("inherited and direct appearances differ:\n%s\nwant:\n%s", appearances[0], appearances[1])
				}
				if !bytes.Equal(images(t, docs[0])[1].Pix, images(t, docs[1])[1].Pix) {
					t.Fatal("inherited and direct rendered pages differ")
				}
			})
		}
	}
}

func TestFillInheritedPairedChoicePrecedence(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name        string
		local       string
		value       string
		strict      bool
		wantWarning bool
		wantLabel   string
	}{
		{"local overrides parent", " /Opt [[(L) (Local)]]", "L", true, false, "Local"},
		{"empty local shadows parent", " /Opt []", "A", false, false, ""},
		{"empty local rejects inherited value", " /Opt []", "A", true, true, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := writeTemp(t, "source.pdf", inheritedPairedChoicePDF(tc.local, "A", "stale"))
			output := filepath.Join(t.TempDir(), "filled.pdf")
			_, err := Fill(ctx, source, output, map[string]string{"group.choice": tc.value}, false, FillOptions{StrictChoices: tc.strict})
			if tc.wantWarning {
				var warning ChoiceWarning
				if !errors.As(err, &warning) || len(warning.AllowedChoices) != 0 {
					t.Fatalf("error = %v, want no allowed choices", err)
				}
				if _, statErr := os.Stat(output); !os.IsNotExist(statErr) {
					t.Fatalf("rejected fill created output: %v", statErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			c, _, err := LoadFile(ctx, output)
			if err != nil {
				t.Fatal(err)
			}
			fields, _, err := fieldTree(c.XRefTable)
			if err != nil {
				t.Fatal(err)
			}
			if got := display(c.XRefTable, fields["group.choice"].D["V"]); got == nil || *got != tc.value {
				t.Fatalf("saved value = %v, want %q", got, tc.value)
			}
			appearance := savedChoiceAppearance(t, c, false)
			if tc.wantLabel != "" && !bytes.Contains(appearance, []byte("("+tc.wantLabel+")")) {
				t.Fatalf("local label %q missing: %s", tc.wantLabel, appearance)
			}
			if tc.wantLabel == "" && bytes.Contains(appearance, []byte("(Alpha)")) {
				t.Fatalf("inherited label leaked through explicit local /Opt: %s", appearance)
			}
		})
	}
}

func TestFlattenInheritedPairedChoiceExistingSelection(t *testing.T) {
	ctx := context.Background()
	source := writeTemp(t, "source.pdf", inheritedPairedChoicePDF("", "A", "stale"))
	output := filepath.Join(t.TempDir(), "flattened.pdf")
	if _, err := Fill(ctx, source, output, map[string]string{"name": "Ada"}, true, FillOptions{StrictChoices: true}); err != nil {
		t.Fatal(err)
	}
	c, data, err := LoadFile(ctx, output)
	if err != nil {
		t.Fatal(err)
	}
	appearance := savedChoiceAppearance(t, c, true)
	if !bytes.Contains(appearance, []byte("(Alpha)")) || bytes.Contains(appearance, []byte("(stale)")) {
		t.Fatalf("flattened appearance did not refresh inherited selection: %s", appearance)
	}
	if len(images(t, data)) < 2 {
		t.Fatal("flattened document is missing the choice page")
	}
}
