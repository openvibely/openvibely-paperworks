package pdf

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

func builtinFontDict(base string) types.Dict {
	return types.Dict{"Type": types.Name("Font"), "Subtype": types.Name("Type1"), "BaseFont": types.Name(base), "Encoding": types.Name("WinAnsiEncoding")}
}

func createdPageFonts(t *testing.T, page CreatePage) []string {
	t.Helper()
	ctx := context.Background()
	out := filepath.Join(t.TempDir(), "fonts.pdf")
	if e := Create(ctx, CreateSpec{Pages: []CreatePage{page}}, "", out); e != nil {
		t.Fatal(e)
	}
	b, e := os.ReadFile(out)
	if e != nil {
		t.Fatal(e)
	}
	c, e := Load(ctx, b)
	if e != nil {
		t.Fatal(e)
	}
	pd, _, e := pageDict(ctx, c, 1)
	if e != nil {
		t.Fatal(e)
	}
	res := dictOf(c.XRefTable, pd["Resources"]).d
	return sortedDictKeys(dictOf(c.XRefTable, res["Font"]).d)
}

func TestFixedLayoutFontKeys(t *testing.T) {
	para := func(font string) FixedParagraph {
		return FixedParagraph{Position: [2]float64{20, 300}, Width: 200, Text: "hello", Font: font, FontSize: 12, Leading: 14}
	}
	size := &[2]float64{300, 400}
	for _, tc := range []struct {
		name string
		page CreatePage
		want []string
	}{
		{"table", CreatePage{Size: size, FixedTables: []FixedTable{sampleFixedTable()}}, []string{"FixedTableFont"}},
		{"paragraph", CreatePage{Size: size, FixedParagraphs: []FixedParagraph{para("Helvetica")}}, []string{"F1"}},
		{"both", CreatePage{Size: size, FixedTables: []FixedTable{sampleFixedTable()}, FixedParagraphs: []FixedParagraph{para("Helvetica"), para("Helvetica-Bold")}}, []string{"F1", "F2"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := createdPageFonts(t, tc.page); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("font keys %v, want %v", got, tc.want)
			}
		})
	}
}

func TestFindBuiltinFontReuseRule(t *testing.T) {
	c, e := Load(context.Background(), mustCreateBlank(t))
	if e != nil {
		t.Fatal(e)
	}
	t.Run("identical reused", func(t *testing.T) {
		fonts := types.Dict{"Existing": builtinFontDict("Helvetica")}
		if k, ok := findBuiltinFont(c, fonts, "Helvetica"); !ok || k != "Existing" {
			t.Fatal(k, ok)
		}
		if k := fixedFont(c, fonts, "Helvetica"); k != "Existing" || len(fonts) != 1 {
			t.Fatal(k, fonts)
		}
	})
	custom := builtinFontDict("Helvetica")
	custom["Encoding"] = types.Name("MacRomanEncoding")
	widths := builtinFontDict("Helvetica")
	widths["Widths"] = types.Array{types.Integer(500)}
	for name, d := range map[string]types.Dict{"custom encoding": custom, "widths": widths} {
		t.Run(name, func(t *testing.T) {
			fonts := types.Dict{"F1": d}
			if _, ok := findBuiltinFont(c, fonts, "Helvetica"); ok {
				t.Fatal("reused modified font")
			}
			if k := fixedFont(c, fonts, "Helvetica"); k != "F2" || !reflect.DeepEqual(fonts["F2"], builtinFontDict("Helvetica")) {
				t.Fatal(k, fonts)
			}
		})
	}
}

func mustCreateBlank(t *testing.T) []byte {
	t.Helper()
	out := filepath.Join(t.TempDir(), "blank.pdf")
	if e := Create(context.Background(), CreateSpec{Pages: []CreatePage{{}}}, "", out); e != nil {
		t.Fatal(e)
	}
	b, e := os.ReadFile(out)
	if e != nil {
		t.Fatal(e)
	}
	return b
}

// Exercise resource lookup, allocation, and the emitted Tf operators together.
// Different font sizes distinguish the table, paragraph, and initial setup Tf.
func TestFixedLayoutExistingFontResources(t *testing.T) {
	blank := mustCreateBlank(t)
	for _, layout := range []string{"table", "paragraph", "both"} {
		for _, resource := range []string{"identical", "custom-encoding", "widths"} {
			for _, occupied := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/occupied=%t", layout, resource, occupied), func(t *testing.T) {
					ctx := context.Background()
					c, err := Load(ctx, blank)
					if err != nil {
						t.Fatal(err)
					}
					pd, _, err := pageDict(ctx, c, 1)
					if err != nil {
						t.Fatal(err)
					}
					existing := builtinFontDict("Helvetica")
					switch resource {
					case "custom-encoding":
						existing["Encoding"] = types.Name("MacRomanEncoding")
					case "widths":
						existing["Widths"] = types.Array{types.Integer(500)}
					}
					// Two matching resources also verify deterministic first-key selection.
					fonts := types.Dict{"Existing": existing, "ZExisting": existing.Clone()}
					if occupied {
						for _, key := range []string{"F1", "F2", "FixedTableFont", "FixedTableFont1"} {
							fonts[key] = builtinFontDict("Courier")
						}
					}
					original := fonts.Clone().(types.Dict)
					pd["Resources"] = types.Dict{"Font": fonts}
					page := CreatePage{}
					if layout != "paragraph" {
						page.FixedTables = []FixedTable{sampleFixedTable()}
					}
					if layout != "table" {
						page.FixedParagraphs = []FixedParagraph{
							{Position: [2]float64{20, 300}, Width: 200, Text: "plain paragraph", Font: "Helvetica", FontSize: 13, Leading: 15},
							{Position: [2]float64{20, 330}, Width: 200, Text: "bold paragraph", Font: "Helvetica-Bold", FontSize: 14, Leading: 16},
						}
					}
					changed, err := applyFixedLayout(ctx, c, []CreatePage{page})
					if err != nil || !changed {
						t.Fatalf("applyFixedLayout: changed=%t err=%v", changed, err)
					}
					res := dictOf(c.XRefTable, pd["Resources"]).d
					gotFonts := dictOf(c.XRefTable, res["Font"]).d
					plainKey := "Existing"
					if resource != "identical" {
						plainKey = "FixedTableFont"
						if layout != "table" {
							plainKey = "F1"
						}
						if occupied {
							plainKey = "FixedTableFont2"
						}
					}
					expected := original.Clone().(types.Dict)
					expected[plainKey] = builtinFontDict("Helvetica")
					boldKey := ""
					if layout != "table" {
						boldKey = "F1"
						if occupied {
							boldKey = "F3"
						} else if resource != "identical" {
							boldKey = "F2"
						}
						expected[boldKey] = builtinFontDict("Helvetica-Bold")
					}
					if got, want := sortedDictKeys(gotFonts), sortedDictKeys(expected); !reflect.DeepEqual(got, want) {
						t.Fatalf("font keys: got %v want %v", got, want)
					}
					for key, want := range expected {
						got := dictOf(c.XRefTable, gotFonts[key]).d.Clone().(types.Dict)
						// Standalone paragraph pages add /Name while publishing resources.
						delete(got, "Name")
						if !reflect.DeepEqual(got, want) {
							t.Fatalf("font %s: got %v want %v", key, got, want)
						}
					}
					raw, err := c.PageContent(pd, 1)
					if err != nil {
						t.Fatal(err)
					}
					stream := string(raw)
					if layout != "paragraph" {
						want := fmt.Sprintf("BT /%s 10 Tf 12 TL ET", plainKey)
						if !strings.Contains(stream, want) || !strings.Contains(stream, "(Label) Tj") {
							t.Fatalf("table did not select %s and paint text: %s", plainKey, stream)
						}
					}
					if layout != "table" {
						for _, text := range []struct{ key, suffix string }{
							{plainKey, "13 Tf 15 TL (plain paragraph) Tj"},
							{boldKey, "14 Tf 16 TL (bold paragraph) Tj"},
						} {
							if !strings.Contains(stream, "/"+text.key+" "+text.suffix) {
								t.Fatalf("paragraph did not select %s: %s", text.key, stream)
							}
						}
					}
				})
			}
		}
	}
}
