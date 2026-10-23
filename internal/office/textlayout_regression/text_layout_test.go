package textlayout_regression

import (
	"math"
	"strings"
	"testing"
	"unicode"

	"github.com/openvibely/openvibely-paperworks/internal/office"
	"golang.org/x/image/font"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"
)

func TestTextHeightRegressionCases(t *testing.T) {
	regular := office.TextRun{Font: "Go", Size: 12}
	cases := []struct {
		name  string
		runs  []office.TextRun
		width float64
		wrap  bool
	}{
		{
			name:  "ASCII words exactly at wrap boundary",
			runs:  []office.TextRun{{Text: "one two", Font: "Go", Size: 12}},
			width: office.Advance(office.FindFont("Go", false, false), "one two", 12),
			wrap:  true,
		},
		{
			name:  "ASCII words just below wrap boundary",
			runs:  []office.TextRun{{Text: "one two", Font: "Go", Size: 12}},
			width: math.Nextafter(office.Advance(office.FindFont("Go", false, false), "one two", 12), 0),
			wrap:  true,
		},
		{
			name:  "mixed text runs split within a word",
			runs:  []office.TextRun{{Text: "hel", Font: "Go", Size: 12}, {Text: "lo world", Font: "Go", Size: 12}},
			width: 26,
			wrap:  true,
		},
		{
			name:  "Unicode",
			runs:  []office.TextRun{{Text: "café Ω", Font: "Go", Size: 12}},
			width: office.Advance(office.FindFont("Go", false, false), "café Ω", 12),
			wrap:  true,
		},
		{
			name:  "tab whitespace",
			runs:  []office.TextRun{{Text: "a\tb", Font: "Go", Size: 12}},
			width: office.Advance(office.FindFont("Go", false, false), "a", 12) + office.Advance(office.FindFont("Go", false, false), "    ", 12) + office.Advance(office.FindFont("Go", false, false), "b", 12),
			wrap:  true,
		},
		{
			name:  "explicit newlines",
			runs:  []office.TextRun{{Text: "first\nsecond\r\nthird", Font: "Go", Size: 12}},
			width: 200,
			wrap:  true,
		},
		{
			name:  "empty runs",
			runs:  []office.TextRun{regular, {Text: "", Font: "Go", Size: 12}},
			width: 200,
			wrap:  true,
		},
		{
			name:  "bold and italic faces",
			runs:  []office.TextRun{{Text: "bold ", Font: "Go", Size: 12, Bold: true}, {Text: "italic", Font: "Go", Size: 12, Italic: true}},
			width: 200,
			wrap:  true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			want, wantErr := referenceTextHeight(tc.runs, tc.width, tc.wrap)
			got, gotErr := office.TextHeight(tc.runs, tc.width, tc.wrap)
			if (gotErr != nil) != (wantErr != nil) {
				t.Fatalf("TextHeight error = %v; reference error = %v", gotErr, wantErr)
			}
			if gotErr == nil && got != want {
				t.Fatalf("TextHeight = %v; reference = %v", got, want)
			}
		})
	}

	t.Run("empty run list", func(t *testing.T) {
		want, wantErr := referenceTextHeight(nil, 200, true)
		got, gotErr := office.TextHeight(nil, 200, true)
		if gotErr != nil || wantErr != nil || got != want {
			t.Fatalf("TextHeight = (%v, %v); reference = (%v, %v)", got, gotErr, want, wantErr)
		}
	})
}

func TestTextHeightASCIIBoundary(t *testing.T) {
	runs := []office.TextRun{{Text: "one two", Font: "Go", Size: 12}}
	f := office.FindFont("Go", false, false)
	boundary := office.Advance(f, "one two", 12)
	atBoundary, err := office.TextHeight(runs, boundary, true)
	if err != nil {
		t.Fatal(err)
	}
	belowBoundary, err := office.TextHeight(runs, math.Nextafter(boundary, 0), true)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(atBoundary-14.4) > 1e-9 || math.Abs(belowBoundary-28.8) > 1e-9 {
		t.Fatalf("heights at and below wrap boundary = %v, %v; want 14.4, 28.8", atBoundary, belowBoundary)
	}
}

func TestTextHeightCombiningCharacter(t *testing.T) {
	family := ""
	for _, candidate := range []string{"Go", "Arial", "Helvetica Neue", "Menlo", "Times New Roman"} {
		f := office.FindFont(candidate, false, false)
		if f == nil {
			continue
		}
		var b sfnt.Buffer
		glyph, err := f.GlyphIndex(&b, '\u0301')
		if err == nil && glyph != 0 {
			family = candidate
			break
		}
	}
	if family == "" {
		t.Skip("no installed test font supports combining acute accent")
	}
	const text = "e\u0301 x"
	runs := []office.TextRun{{Text: text, Font: family, Size: 12}}
	width := office.Advance(office.FindFont(family, false, false), text, 12)
	want, wantErr := referenceTextHeight(runs, width, true)
	got, gotErr := office.TextHeight(runs, width, true)
	if gotErr != nil || wantErr != nil || got != want || math.Abs(got-14.4) > 1e-9 {
		t.Fatalf("TextHeight = (%v, %v); reference = (%v, %v)", got, gotErr, want, wantErr)
	}
	belowBoundary, err := office.TextHeight(runs, math.Nextafter(width, 0), true)
	if err != nil || math.Abs(belowBoundary-28.8) > 1e-9 {
		t.Fatalf("height below combining-character wrap boundary = (%v, %v); want 28.8", belowBoundary, err)
	}
}

func TestWrappedBenchmarkInputsKeepBaselineHeights(t *testing.T) {
	for _, tc := range []struct {
		name       string
		text       string
		wantHeight float64
		wantLines  int
	}{
		{name: "200_chars", text: strings.Repeat("word ", 40), wantHeight: 144, wantLines: 10},
		{name: "1800_chars", text: strings.Repeat("word ", 360), wantHeight: 1296, wantLines: 90}} {
		t.Run(tc.name, func(t *testing.T) {
			runs := []office.TextRun{{Text: tc.text, Font: "Go", Size: 12}}
			want, err := referenceTextHeight(runs, 120, true)
			if err != nil {
				t.Fatal(err)
			}
			if math.Abs(want-tc.wantHeight) > 1e-9 {
				t.Fatalf("reference height = %v; want %v", want, tc.wantHeight)
			}
			if int(math.Round(want/14.4)) != tc.wantLines {
				t.Fatalf("reference wrap count = %v; want %d lines", want/14.4, tc.wantLines)
			}
			got, err := office.TextHeight(runs, 120, true)
			if err != nil {
				t.Fatal(err)
			}
			if got != want {
				t.Fatalf("TextHeight = %v; baseline = %v", got, want)
			}
		})
	}
}

func TestTextHeightUnavailableGlyph(t *testing.T) {
	runs := []office.TextRun{{Text: "\U0010ffff", Font: "Go", Size: 12}}
	_, err := office.TextHeight(runs, 100, true)
	if err == nil || !strings.Contains(err.Error(), "lacks glyph U+10FFFF") {
		t.Fatalf("expected missing glyph error, got %v", err)
	}
}

func BenchmarkTextHeightWrappedParagraph(b *testing.B) {
	for _, tc := range []struct {
		name string
		text string
	}{
		{name: "200_chars", text: strings.Repeat("word ", 40)},
		{name: "1800_chars", text: strings.Repeat("word ", 360)},
	} {
		b.Run(tc.name, func(b *testing.B) {
			runs := []office.TextRun{{Text: tc.text, Font: "Go", Size: 12}}
			const width = 120.0
			height, err := office.TextHeight(runs, width, true)
			if err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := office.TextHeight(runs, width, true); err != nil {
					b.Fatal(err)
				}
			}
			b.ReportMetric(height, "height_pt")
		})
	}
}

// referenceTextHeight retains the previous per-rune Advance call so the tests
// compare behavior against the implementation that preceded glyph reuse.
func referenceTextHeight(runs []office.TextRun, width float64, wrap bool) (float64, error) {
	if width <= 0 || math.IsNaN(width) || math.IsInf(width, 0) {
		return 0, nil
	}
	type glyph struct {
		advance, height float64
		r               rune
	}
	var glyphs []glyph
	for _, r := range runs {
		f := office.FindFont(r.Font, r.Bold, r.Italic)
		if f == nil {
			return 0, nil
		}
		var b sfnt.Buffer
		m, err := f.Metrics(&b, fixed.Int26_6(r.Size*64), font.HintingNone)
		if err != nil {
			return 0, err
		}
		h := math.Max(float64(m.Height)/64, r.Size*1.2)
		for _, ch := range strings.ReplaceAll(strings.ReplaceAll(r.Text, "\r\n", "\n"), "\r", "\n") {
			if ch == '\n' {
				glyphs = append(glyphs, glyph{height: h, r: ch})
				continue
			}
			if ch == '\t' {
				glyphs = append(glyphs, glyph{office.Advance(f, "    ", r.Size), h, ch})
				continue
			}
			idx, err := f.GlyphIndex(&b, ch)
			if err != nil || idx == 0 {
				return 0, nil
			}
			glyphs = append(glyphs, glyph{office.Advance(f, string(ch), r.Size), h, ch})
		}
		if r.Text == "" && len(glyphs) == 0 {
			glyphs = append(glyphs, glyph{height: h})
		}
	}
	lineWidth, lineHeight, total := 0.0, 0.0, 0.0
	flush := func() { total += lineHeight; lineWidth = 0; lineHeight = 0 }
	for i := 0; i < len(glyphs); {
		g := glyphs[i]
		if g.r == '\n' {
			lineHeight = math.Max(lineHeight, g.height)
			flush()
			lineHeight = g.height
			i++
			continue
		}
		end := i + 1
		if !unicode.IsSpace(g.r) {
			for end < len(glyphs) && !unicode.IsSpace(glyphs[end].r) {
				end++
			}
		}
		wordWidth := 0.0
		for _, v := range glyphs[i:end] {
			wordWidth += v.advance
		}
		if wrap && !unicode.IsSpace(g.r) && lineWidth > 0 && lineWidth+wordWidth > width {
			flush()
		}
		for _, v := range glyphs[i:end] {
			if wrap && lineWidth > 0 && lineWidth+v.advance > width {
				flush()
				if unicode.IsSpace(v.r) {
					continue
				}
			}
			lineWidth += v.advance
			lineHeight = math.Max(lineHeight, v.height)
		}
		i = end
	}
	return total + lineHeight, nil
}
