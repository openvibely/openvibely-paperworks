package convert

import (
	"context"
	"math"
	"regexp"
	"strconv"
	"strings"
	"testing"

	documentpdf "github.com/openvibely/openvibely-paperworks/internal/pdf"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// Bounded acquisition of the simple TrueType dictionaries emitted by the
// original LibreOffice consumers. This is NOT a general CMap interpreter.
// Only this complete one-byte, bfchar-only CMap grammar is accepted. Anything
// else retains the explicitly unverified reader path, never guessed widths.
var referencePDFSimpleCMap = regexp.MustCompile(`(?s)^\s*/CIDInit/ProcSet\s+findresource\s+begin\s+12\s+dict\s+begin\s+begincmap\s+/CIDSystemInfo\s*<<\s*/Registry\s+\(Adobe\)\s*/Ordering\s+\(UCS\)\s*/Supplement\s+0\s*>>\s*def\s*/CMapName/Adobe-Identity-UCS\s+def\s*/CMapType\s+2\s+def\s+1\s+begincodespacerange\s*<00>\s*<FF>\s*endcodespacerange\s+(\d+)\s+beginbfchar\s*((?:<[0-9A-Fa-f]{2}>\s*<[0-9A-Fa-f]{4}>\s*)+)endbfchar\s+endcmap\s+CMapName\s+currentdict\s+/CMap\s+defineresource\s+pop\s+end\s+end\s*$`)
var referencePDFSimplePair = regexp.MustCompile(`<([0-9A-Fa-f]{2})>\s*<([0-9A-Fa-f]{4})>`)

// Both installed PDFium font-name APIs omit valid six-letter subset tags.
// Key all dictionaries the same way and reject collisions before acquisition.
func referencePDFMetricFontKey(name string) string {
	if len(name) > 7 && name[6] == '+' {
		valid := true
		for _, r := range name[:6] {
			if r < 'A' || r > 'Z' {
				valid = false
			}
		}
		if valid {
			return name[7:]
		}
	}
	return name
}

type referencePDFEmbeddedMetrics struct {
	descent    float64
	widths     map[rune]float64
	codes      map[int]rune
	codeWidths map[int]float64
}

func referencePDFSimpleUnicode(raw []byte) (map[int]rune, bool) {
	match := referencePDFSimpleCMap.FindSubmatch(raw)
	if match == nil {
		return nil, false
	}
	count, err := strconv.Atoi(string(match[1]))
	if err != nil {
		return nil, false
	}
	pairs := referencePDFSimplePair.FindAllSubmatch(match[2], -1)
	if len(pairs) != count {
		return nil, false
	}
	out := map[int]rune{}
	for _, p := range pairs {
		code, _ := strconv.ParseUint(string(p[1]), 16, 8)
		r, _ := strconv.ParseUint(string(p[2]), 16, 16)
		if r >= 0xd800 && r <= 0xdfff {
			return nil, false
		}
		if _, exists := out[int(code)]; exists {
			return nil, false
		}
		out[int(code)] = rune(r)
	}
	return out, true
}

func referencePDFEmbeddedFonts(t *testing.T, data []byte) map[string]referencePDFEmbeddedMetrics {
	t.Helper()
	c, err := documentpdf.Load(context.Background(), data)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]referencePDFEmbeddedMetrics{}
	seen := map[string]bool{}
	var inspect func(types.Object, int)
	inspect = func(o types.Object, depth int) {
		if depth > 64 {
			t.Fatal("font dictionary nesting exceeds reader limit")
		}
		var d types.Dict
		switch v := o.(type) {
		case types.Dict:
			d = v
		case types.StreamDict:
			d = v.Dict
		case types.Array:
			for _, child := range v {
				inspect(child, depth+1)
			}
			return
		default:
			return
		}
		if n, ok := d["BaseFont"].(types.Name); ok {
			name := referencePDFMetricFontKey(string(n))
			// Multiple dictionaries with the same name cannot be identified uniquely
			// through PDFium's name API, even if one is otherwise eligible.
			duplicate := seen[name]
			seen[name] = true
			delete(out, name)
			if !duplicate {
				if metrics, ok := func() (referencePDFEmbeddedMetrics, bool) {
					_, standard := referencePDFStandardDescents[name]
					if standard || name == "Symbol" || name == "ZapfDingbats" || d["Subtype"] != types.Name("TrueType") {
						return referencePDFEmbeddedMetrics{}, false
					}
					desc, e := c.DereferenceDict(d["FontDescriptor"])
					if e != nil || desc == nil || desc["FontFile2"] == nil {
						return referencePDFEmbeddedMetrics{}, false
					}
					number := func(o types.Object) (float64, bool) {
						v, e := c.Dereference(o)
						if e != nil {
							return 0, false
						}
						switch n := v.(type) {
						case types.Integer:
							return float64(n), true
						case types.Float:
							f := float64(n)
							return f, !math.IsNaN(f) && !math.IsInf(f, 0)
						}
						return 0, false
					}
					descent, ok := number(desc["Descent"])
					if !ok {
						return referencePDFEmbeddedMetrics{}, false
					}
					first, ok := number(d["FirstChar"])
					if !ok || first < 0 || first > 255 || first != math.Trunc(first) {
						return referencePDFEmbeddedMetrics{}, false
					}
					widths, e := c.DereferenceArray(d["Widths"])
					if e != nil || len(widths) == 0 || int(first)+len(widths) > 256 {
						return referencePDFEmbeddedMetrics{}, false
					}
					object, e := c.Dereference(d["ToUnicode"])
					if e != nil {
						return referencePDFEmbeddedMetrics{}, false
					}
					stream, ok := object.(types.StreamDict)
					if !ok || stream.Decode() != nil {
						return referencePDFEmbeddedMetrics{}, false
					}
					codes, ok := referencePDFSimpleUnicode(stream.Content)
					if !ok {
						return referencePDFEmbeddedMetrics{}, false
					}
					m := referencePDFEmbeddedMetrics{descent: -math.Abs(descent) * .001, widths: map[rune]float64{}, codes: codes, codeWidths: map[int]float64{}}
					for code, r := range codes {
						index := code - int(first)
						if index < 0 || index >= len(widths) {
							return referencePDFEmbeddedMetrics{}, false
						}
						w, ok := number(widths[index])
						if !ok {
							return referencePDFEmbeddedMetrics{}, false
						}
						// Unicode alone cannot recover the original CID when different source
						// codes map to the same text. Reject the font instead of selecting one.
						if _, exists := m.widths[r]; exists {
							return referencePDFEmbeddedMetrics{}, false
						}
						m.widths[r] = w * .001
						m.codeWidths[code] = w * .001
					}
					return m, true
				}(); ok {
					out[name] = metrics
				}
			}
		}
		for _, child := range d {
			inspect(child, depth+1)
		}
	}
	for _, entry := range c.Table {
		if entry != nil {
			inspect(entry.Object, 0)
		}
	}
	return out
}

func TestReferencePDFSimpleUnicodeBoundary(t *testing.T) {
	raw := `/CIDInit/ProcSet findresource begin
12 dict begin begincmap
/CIDSystemInfo<< /Registry (Adobe) /Ordering (UCS) /Supplement 0 >> def
/CMapName/Adobe-Identity-UCS def /CMapType 2 def
1 begincodespacerange <00> <FF> endcodespacerange
2 beginbfchar <01> <0041> <02> <0062> endbfchar
endcmap CMapName currentdict /CMap defineresource pop end end`
	got, ok := referencePDFSimpleUnicode([]byte(raw))
	if !ok || got[1] != 'A' || got[2] != 'b' {
		t.Fatal(got, ok)
	}
	for _, bad := range []string{strings.Replace(raw, "2 beginbfchar", "3 beginbfchar", 1), strings.Replace(raw, "<02>", "<01>", 1), strings.Replace(raw, "<0062>", "<D800>", 1), raw + " /Other usecmap", strings.Replace(raw, "<0062>", "<00620063>", 1)} {
		if _, ok := referencePDFSimpleUnicode([]byte(bad)); ok {
			t.Fatal("unsupported CMap accepted", bad)
		}
	}
}

// Font-program validity is deliberately outside this dictionary-acquisition
// test: PDFMiner takes these metrics from the PDF, not the TrueType program.
func TestReferencePDFEmbeddedDictionaryMetrics(t *testing.T) {
	cmap := `/CIDInit/ProcSet findresource begin
12 dict begin begincmap
/CIDSystemInfo<< /Registry (Adobe) /Ordering (UCS) /Supplement 0 >> def
/CMapName/Adobe-Identity-UCS def /CMapType 2 def
1 begincodespacerange <00> <FF> endcodespacerange
2 beginbfchar <01> <0041> <02> <0062> endbfchar
endcmap CMapName currentdict /CMap defineresource pop end end`
	for _, tc := range []struct {
		name      string
		descent   int
		duplicate bool
		unicode   string
		want      bool
	}{
		{"negative", -268, false, cmap, true}, {"positive-descent", 268, false, cmap, true},
		{"duplicate-name", -268, true, cmap, false}, {"subset-collision", -268, true, cmap, false}, {"ambiguous-unicode", -268, false, strings.Replace(cmap, "<0062>", "<0041>", 1), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, err := documentpdf.Load(context.Background(), referencePDFMetricDocument([]string{"BT /F1 10 Tf (A) Tj ET"}, ""))
			if err != nil {
				t.Fatal(err)
			}
			stream := func(raw string) types.Object {
				s, e := c.NewStreamDictForBuf([]byte(raw))
				if e != nil {
					t.Fatal(e)
				}
				s.Dict["Length1"] = types.Integer(len(raw))
				if e = s.Encode(); e != nil {
					t.Fatal(e)
				}
				ref, e := c.IndRefForNewObject(*s)
				if e != nil {
					t.Fatal(e)
				}
				return *ref
			}
			d := types.Dict{"Type": types.Name("Font"), "Subtype": types.Name("TrueType"), "BaseFont": types.Name("BAAAAA+MetricFixture"), "FirstChar": types.Integer(0), "LastChar": types.Integer(2), "Widths": types.Array{types.Integer(0), types.Integer(487), types.Integer(525)}, "ToUnicode": stream(tc.unicode), "FontDescriptor": types.Dict{"Type": types.Name("FontDescriptor"), "FontName": types.Name("BAAAAA+MetricFixture"), "Flags": types.Integer(4), "FontBBox": types.Array{types.Integer(0), types.Integer(-300), types.Integer(1000), types.Integer(1000)}, "ItalicAngle": types.Integer(0), "Ascent": types.Integer(952), "Descent": types.Integer(tc.descent), "CapHeight": types.Integer(900), "StemV": types.Integer(80), "FontFile2": stream("dictionary-only")}}
			c.Table[3].Object = d
			if tc.duplicate {
				page, _, _, e := c.PageDict(context.Background(), 1, false)
				if e != nil {
					t.Fatal(e)
				}
				resources, e := c.DereferenceDict(page["Resources"])
				if e != nil {
					t.Fatal(e)
				}
				fonts, e := c.DereferenceDict(resources["Font"])
				if e != nil {
					t.Fatal(e)
				}
				other := d.Clone().(types.Dict)
				if tc.name == "subset-collision" {
					other["BaseFont"] = types.Name("CAAAAA+MetricFixture")
				}
				fonts["F2"] = other
			}
			raw, err := documentpdf.Serialize(context.Background(), c)
			if err != nil {
				t.Fatal(err)
			}
			m, ok := referencePDFEmbeddedFonts(t, raw)["MetricFixture"]
			if ok != tc.want {
				t.Fatalf("eligible=%t want %t", ok, tc.want)
			}
			if ok && (m.descent != -.268 || m.widths['A'] != .487 || m.widths['b'] != .525) {
				t.Fatalf("metrics=%+v", m)
			}
		})
	}
}
