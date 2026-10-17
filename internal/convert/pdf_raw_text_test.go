package convert

// Bounded content-stream acquisition for the embedded simple-font spreadsheet
// consumers. Operands retain PDF decimal precision and string bytes. Unsupported
// text state, XObjects, inline images and operators reject the WHOLE page; no
// partial result is substituted for PDFMiner. This is not a general interpreter.
import (
	"context"
	"encoding/hex"
	"fmt"
	pdffont "github.com/pdfcpu/pdfcpu/pkg/font"
	"math"
	"strconv"
	"strings"
	"testing"

	documentpdf "github.com/openvibely/openvibely-paperworks/internal/pdf"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

type referencePDFRawGlyph struct {
	font string
	code int
	char referencePDFDirectedChar
}

func referencePDFRawEmbeddedPage(t *testing.T, data []byte, index int, fonts map[string]referencePDFEmbeddedMetrics) ([]referencePDFRawGlyph, error) {
	t.Helper()
	c, err := documentpdf.Load(context.Background(), data)
	if err != nil {
		return nil, err
	}
	page, _, inherited, err := c.PageDict(context.Background(), index+1, false)
	if err != nil {
		return nil, err
	}
	raw, err := c.PageContent(page, index+1)
	if err != nil {
		return nil, err
	}
	resources, err := c.DereferenceDict(inherited.Resources["Font"])
	if err != nil {
		return nil, err
	}
	// Keep caller's embedded-font registry immutable.
	allFonts := make(map[string]referencePDFEmbeddedMetrics, len(fonts))
	for name, m := range fonts {
		allFonts[name] = m
	}
	for name := range referencePDFStandardFonts(t, data) {
		m := referencePDFEmbeddedMetrics{descent: referencePDFStandardDescents[name], codes: map[int]rune{}, codeWidths: map[int]float64{}}
		for code := 32; code <= 126; code++ {
			// StandardEncoding and WinAnsi disagree on these two glyph names.
			// Retain whole-page fallback until their encoding-specific mapping is read.
			if code == 39 || code == 96 {
				continue
			}
			width, e := pdffont.CharWidth(context.Background(), name, rune(code))
			if e != nil {
				return nil, e
			}
			m.codes[code] = rune(code)
			m.codeWidths[code] = float64(width) * .001
		}
		allFonts[name] = m
	}
	names := map[string]string{}
	displayNames := map[string]string{}
	for key, ref := range resources {
		d, err := c.DereferenceDict(ref)
		if err != nil {
			return nil, err
		}
		name, ok := d["BaseFont"].(types.Name)
		if ok {
			base := referencePDFMetricFontKey(string(name))
			names[key] = base
			// Add the observed WinAnsi em dash only when the resource explicitly
			// declares that encoding. Preserve rejection for every other high code.
			if d["Encoding"] == types.Name("WinAnsiEncoding") && allFonts[base].codes != nil && referencePDFStandardDescents[base] != 0 {
				m := allFonts[base]
				codes, widths := map[int]rune{}, map[int]float64{}
				for k, v := range m.codes {
					codes[k] = v
				}
				for k, v := range m.codeWidths {
					widths[k] = v
				}
				w, e := pdffont.CharWidth(context.Background(), base, rune(151))
				if e != nil {
					return nil, e
				}
				codes[151], widths[151] = '—', float64(w)*.001
				m.codes, m.codeWidths = codes, widths
				names[key] = "resource:" + key
				allFonts[names[key]] = m
				displayNames[names[key]] = base
			}
		}
	}
	b := inherited.MediaBox
	if b == nil {
		return nil, fmt.Errorf("missing media box")
	}
	matrix, _, height := referencePDFMinerPageCoordinates([4]float64{b.LL.X, b.LL.Y, b.UR.X, b.UR.Y}, ((inherited.Rotate%360)+360)%360)
	glyphs, err := referencePDFRawEmbeddedText(string(raw), names, allFonts, matrix, height)
	for i := range glyphs {
		if name, ok := displayNames[glyphs[i].font]; ok {
			glyphs[i].font = name
		}
	}
	return glyphs, err
}

func referencePDFRawEmbeddedText(input string, names map[string]string, fonts map[string]referencePDFEmbeddedMetrics, page referencePDFMatrix, height float64) ([]referencePDFRawGlyph, error) {
	type state struct {
		font    string
		size    float64
		leading float64
		ctm     referencePDFMatrix
	}
	st := state{ctm: page}
	var stack []state
	var args []types.Object
	var glyphs []referencePDFRawGlyph
	inText := false
	x, y, advance := 0.0, 0.0, 0.0
	fail := func(op string) ([]referencePDFRawGlyph, error) {
		return nil, fmt.Errorf("raw text reader: unsupported or malformed %s", op)
	}
	number := func(o types.Object) (float64, bool) {
		switch v := o.(type) {
		case types.Integer:
			return float64(v), true
		case types.Float:
			return float64(v), true
		}
		return 0, false
	}
	for {
		input = strings.TrimLeft(input, "\x00\t\n\f\r ")
		if input == "" {
			break
		}
		if input[0] == '%' {
			if n := strings.IndexAny(input, "\r\n"); n >= 0 {
				input = input[n:]
				continue
			}
			break
		}
		// A PDF object parser may consume `0 0 R` as a reference inside
		// `0 0 0 RG`. Content operands need standalone numeric tokenization.
		if strings.ContainsRune("+-.0123456789", rune(input[0])) {
			n := strings.IndexAny(input, "\x00\t\n\f\r /()<>[]%")
			if n < 0 {
				n = len(input)
			}
			token := input[:n]
			if strings.IndexFunc(token, func(r rune) bool { return !strings.ContainsRune("+-.0123456789", r) }) >= 0 {
				return fail("number")
			}
			v, err := strconv.ParseFloat(token, 64)
			if err != nil || math.IsInf(v, 0) || math.IsNaN(v) {
				return fail("number")
			}
			args = append(args, types.Float(v))
			input = input[n:]
			continue
		}
		if strings.ContainsRune("/([<", rune(input[0])) {
			before := len(input)
			result, err := model.ParseObjectWithPolicy(context.Background(), &input, 0, model.ValidationStrict)
			if err != nil || len(input) >= before {
				return fail("operand")
			}
			args = append(args, result.Object)
			continue
		}
		n := strings.IndexAny(input, "\x00\t\n\f\r /()<>[]%")
		if n < 0 {
			n = len(input)
		}
		if n == 0 {
			return fail("delimiter")
		}
		op := input[:n]
		input = input[n:]
		switch op {
		case "q":
			if inText || len(args) != 0 {
				return fail(op)
			}
			stack = append(stack, st)
		case "Q":
			if inText || len(args) != 0 || len(stack) == 0 {
				return fail(op)
			}
			st = stack[len(stack)-1]
			stack = stack[:len(stack)-1]
		case "cm":
			if inText || len(args) != 6 {
				return fail(op)
			}
			var m referencePDFMatrix
			for i, arg := range args {
				var ok bool
				m[i], ok = number(arg)
				if !ok {
					return fail(op)
				}
			}
			st.ctm = referencePDFComposeMatrix(st.ctm, m)
		case "Tw", "Tr":
			// Only default state is needed for the table consumer. Nonzero spacing
			// and rendering modes are explicitly unsupported, not silently ignored.
			if !inText || len(args) != 1 {
				return fail(op)
			}
			v, ok := number(args[0])
			if !ok || v != 0 {
				return fail(op)
			}
		case "BT":
			if inText || len(args) != 0 {
				return fail(op)
			}
			inText = true
			x, y, advance = 0, 0, 0
		case "ET":
			if !inText || len(args) != 0 {
				return fail(op)
			}
			inText = false
		case "Tf":
			if !inText || len(args) != 2 {
				return fail(op)
			}
			name, ok := args[0].(types.Name)
			size, valid := number(args[1])
			if !ok || !valid || size <= 0 {
				return fail(op)
			}
			st.font, st.size = names[string(name)], size
			if _, ok := fonts[st.font]; !ok {
				return fail("font " + string(name))
			}
		case "TL":
			if !inText || len(args) != 1 {
				return fail(op)
			}
			v, ok := number(args[0])
			if !ok {
				return fail(op)
			}
			st.leading = -v
		case "Tm":
			// Translation-only text matrices for the ReportLab column consumer.
			// Reject all other matrices instead of approximating their semantics.
			if !inText || len(args) != 6 {
				return fail(op)
			}
			var m [6]float64
			for i, arg := range args {
				v, ok := number(arg)
				if !ok {
					return fail(op)
				}
				m[i] = v
			}
			if m[0] != 1 || m[1] != 0 || m[2] != 0 || m[3] != 1 {
				return fail(op)
			}
			x, y, advance = m[4], m[5], 0
		case "T*":
			if !inText || len(args) != 0 {
				return fail(op)
			}
			y += st.leading
			advance = 0
		case "Td":
			if !inText || len(args) != 2 {
				return fail(op)
			}
			dx, ok := number(args[0])
			dy, valid := number(args[1])
			if !ok || !valid {
				return fail(op)
			}
			x += dx
			y += dy
			advance = 0
		case "Tj", "TJ":
			if !inText || len(args) != 1 || st.size <= 0 {
				return fail(op)
			}
			sequence := types.Array{args[0]}
			if op == "TJ" {
				var ok bool
				sequence, ok = args[0].(types.Array)
				if !ok {
					return fail(op)
				}
			}
			// PDFMiner PDFTextDevice.render_string_horizontal: TJ numbers
			// subtract n * (.001 * fontsize * scaling) from the line position.
			// Scaling and character/word spacing remain at their supported defaults.
			for _, item := range sequence {
				if adjustment, ok := number(item); ok {
					if op != "TJ" || math.IsNaN(adjustment) || math.IsInf(adjustment, 0) {
						return fail(op)
					}
					advance -= adjustment * (.001 * st.size)
					continue
				}
				var codes []byte
				var err error
				switch v := item.(type) {
				case types.HexLiteral:
					codes, err = hex.DecodeString(string(v))
				case types.StringLiteral:
					codes, err = types.Unescape(string(v))
				default:
					return fail(op)
				}
				if err != nil {
					return fail(op)
				}
				font, ok := fonts[st.font]
				if !ok {
					return fail("font")
				}
				for _, code := range codes {
					text, ok := font.codes[int(code)]
					width, valid := font.codeWidths[int(code)]
					if !ok || !valid {
						return fail("unmapped raw code")
					}
					// PDFMiner translates the text matrix by its accumulated line position,
					// then combines with the page CTM. No PDFium float32 values are involved.
					m := referencePDFComposeMatrix(st.ctm, referencePDFMatrix{1, 0, 0, 1, x, y})
					m[4] = advance*m[0] + m[4]
					m[5] = advance*m[1] + m[5]
					layout := referencePDFMinerCharLayout(referencePDFCharMetrics{matrix: m, fontSize: st.size, scale: 1, width: width, descent: font.descent})
					b := layout.box
					glyphs = append(glyphs, referencePDFRawGlyph{st.font, int(code), referencePDFDirectedChar{referencePDFTableChar{string(text), referencePDFCell{b[0], height - b[3], b[2], height - b[1]}}, layout.upright}})
					advance += layout.advance
				}
			}
		// Only the observed non-text operators. Reject rather than ignore an
		// operator such as gs/Do that could alter coordinates or introduce text.
		case "RG", "rg":
			if len(args) != 3 {
				return fail(op)
			}
			for _, arg := range args {
				if _, ok := number(arg); !ok {
					return fail(op)
				}
			}
		case "w", "J", "j", "B", "s", "m", "l", "re", "h", "S", "n", "W*", "B*", "f*", "BDC", "EMC":
			if inText {
				return fail(op)
			}
		default:
			return fail(op)
		}
		args = nil
	}
	if inText || len(stack) != 0 || len(args) != 0 {
		return fail("unfinished stream")
	}
	return glyphs, nil
}

func TestReferencePDFRawEmbeddedTextBoundaries(t *testing.T) {
	fonts := map[string]referencePDFEmbeddedMetrics{"Fixture": {descent: -.2, codes: map[int]rune{1: 'A', 2: 'A'}, codeWidths: map[int]float64{1: .4, 2: .7}}}
	names := map[string]string{"F1": "Fixture"}
	matrix := referencePDFMatrix{1, 0, 0, 1, 0, 0}
	// Distinct source codes with identical Unicode MUST select different widths.
	got, err := referencePDFRawEmbeddedText("0 0 0 RG q BT 50.123456789 100 Td /F1 10 Tf <01> Tj (\\002) Tj ET Q", names, fonts, matrix, 200)
	if err != nil || len(got) != 2 {
		t.Fatal(got, err)
	}
	if got[0].code != 1 || got[1].code != 2 || got[0].char.char.box != (referencePDFCell{50.123456789, 92, 54.123456789, 102}) || got[1].char.char.box != (referencePDFCell{54.123456789, 92, 61.123456789, 102}) {
		t.Fatalf("raw metrics=%+v", got)
	}
	for _, raw := range []string{"BT /F1 10 Tf <03> Tj ET", "BT /F1 10 Tf <01> Tj 1 Ts ET", "q BT /F1 10 Tf <01> Tj ET", "BT /F1 10 Tf <01> Tj ET /X1 Do", "BT /F1 10 Tf <01> Tj ET /GS1 gs", "BT /F1 10 Tf <01> Tj"} {
		if chars, err := referencePDFRawEmbeddedText(raw, names, fonts, matrix, 200); err == nil || chars != nil {
			t.Fatalf("accepted unsupported page %q: %+v %v", raw, chars, err)
		}
	}
}

func TestReferencePDFRawCoordinateStack(t *testing.T) {
	fonts := map[string]referencePDFEmbeddedMetrics{"Fixture": {descent: -.2, codes: map[int]rune{1: 'A'}, codeWidths: map[int]float64{1: .4}}}
	names := map[string]string{"F1": "Fixture"}
	raw := "q BT /F1 10 Tf ET 1 0 0 1 12.123456789 30 cm q 2 0 0 3 5 7 cm BT 0 Tw 0 Tr 0 0 0 rg 4 6 Td <01> Tj ET Q BT 4 6 Td <01> Tj ET Q"
	got, err := referencePDFRawEmbeddedText(raw, names, fonts, referencePDFMatrix{1, 0, 0, 1, 0, 0}, 200)
	// Exact pinned PDFMiner results, including binary64 addition rounding.
	want := []referencePDFCell{{25.123456789000002, 121, 33.123456789, 151}, {16.123456789000002, 156, 20.123456789000002, 166}}
	if err != nil || len(got) != len(want) {
		t.Fatal(got, err)
	}
	for i := range want {
		if got[i].char.char.box != want[i] {
			t.Fatalf("char %d=%v want %v", i, got[i].char.char.box, want[i])
		}
	}
	for _, suffix := range []string{"BT 1 Tw ET", "BT 1 Tr ET", "BT 1 0 0 1 0 0 cm ET", "/X1 Do", "1 0 0 cm"} {
		if chars, err := referencePDFRawEmbeddedText(raw+suffix, names, fonts, referencePDFMatrix{1, 0, 0, 1, 0, 0}, 200); err == nil || chars != nil {
			t.Fatalf("accepted %q: %v %v", suffix, chars, err)
		}
	}
}

func TestReferencePDFRawStandardWidthRounding(t *testing.T) {
	// Pinned PDFMiner: Helvetica '@' width 1015 * .001, not 1015 / 1000.
	data := referencePDFMetricDocument([]string{"BT /F1 1 Tf (@) Tj ET"}, "")
	got, err := referencePDFRawEmbeddedPage(t, data, 0, nil)
	if err != nil || len(got) != 1 {
		t.Fatal(got, err)
	}
	if got[0].code != 64 || got[0].char.char.box[2] != 1.0150000000000001 {
		t.Fatalf("width rounding=%+v", got)
	}
}

// Exact same-input PDFMiner 20260107 results; no tolerance or PDFium boxes.
func TestReferencePDFRawTJAcquisition(t *testing.T) {
	for _, tc := range []struct {
		stream string
		text   string
		boxes  []referencePDFCell
	}{
		{"BT /F1 10 Tf 50 100 Td [(A) 120 (B) -250 <43>] TJ (D) Tj ET", "ABCD", []referencePDFCell{{50.0, 92.07, 56.67, 102.07}, {55.47, 92.07, 62.14, 102.07}, {64.64, 92.07, 71.86, 102.07}, {71.86, 92.07, 79.08, 102.07}}},
		{"BT /F1 10 Tf 50 100 Td [100 () -50 (A) 200] TJ (B) Tj ET", "AB", []referencePDFCell{{49.5, 92.07, 56.17, 102.07}, {54.17, 92.07, 60.84, 102.07}}},
		{"BT /F1 10 Tf 50 100 Td [] TJ (A) Tj ET", "A", []referencePDFCell{{50.0, 92.07, 56.67, 102.07}}},
		{"q 1 0 -1 1 12.123456789 30 cm BT /F1 10.998 Tf 4 6 Td [(A) 123.456 (B) -98.765 (C)] TJ ET Q", "ABC", []referencePDFCell{{1.4020427890000011, 155.27858600000002, 19.735708789, 166.276586}, {7.379939701000001, 155.27858600000002, 25.713605701, 166.276586}, {15.801823170999999, 155.27858600000002, 34.740379171, 166.276586}}},
	} {
		data := referencePDFMetricDocument([]string{tc.stream}, "")
		chars, err := referencePDFRawEmbeddedPage(t, data, 0, nil)
		if err != nil || len(chars) != len(tc.boxes) {
			t.Fatal(chars, err)
		}
		for i, g := range chars {
			if g.code != int(tc.text[i]) || g.char.char.text != string(tc.text[i]) || g.char.char.box != tc.boxes[i] || !g.char.upright {
				t.Fatalf("%q char %d: %+v want %v", tc.stream, i, g, tc.boxes[i])
			}
		}
	}
	for _, bad := range []string{"12 TJ", "[(A) /Bad] TJ", "[(A) [(B)]] TJ", "[(A) << /Bad 1 >>] TJ", "12 Tj"} {
		data := referencePDFMetricDocument([]string{"BT /F1 10 Tf (A) Tj " + bad + " ET"}, "")
		if chars, err := referencePDFRawEmbeddedPage(t, data, 0, nil); err == nil || chars != nil {
			t.Fatalf("accepted %q: %v %v", bad, chars, err)
		}
	}
}

func TestReferencePDFRawReportLabLeading(t *testing.T) {
	fonts := map[string]referencePDFEmbeddedMetrics{"Helvetica": {descent: -.207, codes: map[int]rune{65: 'A'}, codeWidths: map[int]float64{65: .667}}}
	names := map[string]string{"F1": "Helvetica"}
	page := referencePDFMatrix{1, 0, 0, 1, 0, 0}
	raw := "BT /F1 10 Tf 12 TL ET BT 1 0 0 1 30 100 Tm (A) Tj T* (A) Tj ET BT 1 0 0 1 50 70 Tm (A) Tj ET"
	got, err := referencePDFRawEmbeddedText(raw, names, fonts, page, 200)
	if err != nil || len(got) != 3 {
		t.Fatal(got, err)
	}
	want := []referencePDFCell{{30, 92.07, 36.67, 102.07}, {30, 104.07, 36.67, 114.07}, {50, 122.07, 56.67, 132.07}}
	for i, g := range got {
		if g.char.char.box != want[i] {
			t.Errorf("char %d: %v want %v", i, g.char.char.box, want[i])
		}
	}
	for _, bad := range []string{"BT /F1 10 Tf 12 TL 2 0 0 1 0 0 Tm (A) Tj ET", "BT /F1 10 Tf (A) Tj /bad TL ET", "BT /F1 10 Tf (A) Tj 1 T* ET"} {
		chars, e := referencePDFRawEmbeddedText(bad, names, fonts, page, 200)
		if e == nil || chars != nil {
			t.Fatalf("partial/unsupported accepted %q: %v %v", bad, chars, e)
		}
	}
}
