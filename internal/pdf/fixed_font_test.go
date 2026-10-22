package pdf

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
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
