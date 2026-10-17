package convert

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/klippa-app/go-pdfium"
	"github.com/klippa-app/go-pdfium/references"
	"github.com/klippa-app/go-pdfium/requests"
	documentpdf "github.com/openvibely/openvibely-paperworks/internal/pdf"
	pdffont "github.com/pdfcpu/pdfcpu/pkg/font"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// Pinned PDFMiner 20260107 fontmetrics.py, not PDFium's substitute-font bbox.
// Symbol and ZapfDingbats require separate encoding review.
var referencePDFStandardDescents = map[string]float64{
	"Courier": -.194, "Courier-Bold": -.194, "Courier-Oblique": -.194, "Courier-BoldOblique": -.194,
	"Helvetica": -.207, "Helvetica-Bold": -.207, "Helvetica-Oblique": -.207, "Helvetica-BoldOblique": -.207,
	"Times-Roman": -.217, "Times-Bold": -.217, "Times-Italic": -.217, "Times-BoldItalic": -.217,
}

// Conservatively qualify fonts from the actual PDF dictionaries. A matching
// name alone does not establish standard metrics. Ambiguous names fail closed
// to the existing, explicitly unverified adapter; no metrics are guessed.
func referencePDFStandardFonts(t *testing.T, data []byte) map[string]bool {
	t.Helper()
	c, err := documentpdf.Load(context.Background(), data)
	if err != nil {
		t.Fatal(err)
	}
	eligible, rejected := map[string]bool{}, map[string]bool{}
	var inspect func(types.Object, int)
	inspect = func(object types.Object, depth int) {
		if depth > 64 {
			t.Fatal("font dictionary nesting exceeds reader limit")
		}
		var d types.Dict
		switch value := object.(type) {
		case types.Dict:
			d = value
		case types.StreamDict:
			d = value.Dict
		case types.Array:
			for _, child := range value {
				inspect(child, depth+1)
			}
			return
		default:
			return // Indirect references are visited through c.Table below.
		}
		if name, ok := d["BaseFont"].(types.Name); ok {
			base := referencePDFMetricFontKey(string(name))
			if _, standard := referencePDFStandardDescents[base]; standard {
				encoding, encodingOK := d["Encoding"].(types.Name)
				subtype, _ := d["Subtype"].(types.Name)
				allowed := string(name) == base && subtype == types.Name("Type1") && d["FontDescriptor"] == nil && d["ToUnicode"] == nil &&
					(d["Encoding"] == nil || (encodingOK && (encoding == "StandardEncoding" || encoding == "WinAnsiEncoding")))
				eligible[base] = true
				if !allowed {
					rejected[base] = true
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
	for name := range rejected {
		delete(eligible, name)
	}
	return eligible
}

// PDFium's per-character matrix includes horizontal scale and rise. Its
// character origin includes the advance of preceding characters. Apply font
// metrics in that coordinate system, rather than transform an axis-aligned
// loose box (which loses corners under skew/rotation).
func referencePDFStandardChar(inst pdfium.Pdfium, text references.FPDF_TEXTPAGE, index int, ch rune, page referencePDFMatrix, height float64, eligible map[string]bool, embedded map[string]referencePDFEmbeddedMetrics) (referencePDFDirectedChar, bool, error) {
	var zero referencePDFDirectedChar

	obj, err := inst.FPDFText_GetTextObject(&requests.FPDFText_GetTextObject{TextPage: text, Index: index})
	if err != nil {
		return zero, false, err
	}
	font, err := inst.FPDFTextObj_GetFont(&requests.FPDFTextObj_GetFont{PageObject: obj.TextObject})
	if err != nil {
		return zero, false, err
	}
	base, err := inst.FPDFFont_GetBaseFontName(&requests.FPDFFont_GetBaseFontName{Font: font.Font})
	if err != nil {
		return zero, false, err
	}
	var width, descent float64
	if eligible[base.BaseFontName] && ch >= 32 && ch <= 126 {
		w, e := pdffont.CharWidth(context.Background(), base.BaseFontName, ch)
		if e != nil {
			return zero, false, e
		}
		width, descent = float64(w)*.001, referencePDFStandardDescents[base.BaseFontName]
	} else if metrics, ok := embedded[base.BaseFontName]; ok {
		var exists bool
		width, exists = metrics.widths[ch]
		if !exists {
			return zero, false, nil
		}
		descent = metrics.descent
	} else {
		return zero, false, nil
	}
	size, err := inst.FPDFText_GetFontSize(&requests.FPDFText_GetFontSize{TextPage: text, Index: index})
	if err != nil {
		return zero, false, err
	}
	if size.FontSize <= 0 {
		return zero, false, nil
	}
	transform, err := inst.FPDFText_GetMatrix(&requests.FPDFText_GetMatrix{TextPage: text, Index: index})
	if err != nil {
		return zero, false, err
	}
	origin, err := inst.FPDFText_GetCharOrigin(&requests.FPDFText_GetCharOrigin{TextPage: text, Index: index})
	if err != nil {
		return zero, false, err
	}

	m := transform.Matrix
	effective := referencePDFComposeMatrix(page, referencePDFMatrix{float64(m.A), float64(m.B), float64(m.C), float64(m.D), origin.X, origin.Y})
	layout := referencePDFMinerCharLayout(referencePDFCharMetrics{matrix: effective, fontSize: size.FontSize, scale: 1, width: width, descent: descent})
	b := layout.box
	return referencePDFDirectedChar{referencePDFTableChar{string(ch), referencePDFCell{b[0], height - b[3], b[2], height - b[1]}}, layout.upright}, true, nil
}

func TestReferencePDFStandardMetricTables(t *testing.T) {
	// Full printable-ASCII widths are compared against pinned Reference by the
	// same-input audit; retain a few independent metric landmarks here.
	for _, tc := range []struct {
		font    string
		char    rune
		width   int
		descent float64
	}{
		{"Helvetica", 'A', 667, -.207}, {"Helvetica-Bold", 'A', 722, -.207},
		{"Times-Roman", 'A', 722, -.217}, {"Courier", 'A', 600, -.194},
	} {
		t.Run(fmt.Sprintf("%s-%c", tc.font, tc.char), func(t *testing.T) {
			w, err := pdffont.CharWidth(context.Background(), tc.font, tc.char)
			if err != nil || w != tc.width || referencePDFStandardDescents[tc.font] != tc.descent {
				t.Fatalf("width=%d descent=%g err=%v", w, referencePDFStandardDescents[tc.font], err)
			}
		})
	}
}

// These exact boxes were independently obtained from pinned PDFMiner 20260107
// on the same text operators. No PDFium-box adjustment or pixel tolerance.
func TestReferencePDFStandardMetricAcquisition(t *testing.T) {
	cases := []struct {
		ops     string
		box     referencePDFCell
		upright bool
	}{
		{"1 0 0 1 50 100 Tm", referencePDFCell{50, 92.07, 56.67, 102.07}, true},
		{"1 0 0 1 50 100 Tm 7 Ts", referencePDFCell{50, 85.07, 56.67, 95.07}, true},
		{"1 0 0 1 50 100 Tm 50 Tz", referencePDFCell{50, 92.07, 53.335, 102.07}, true},
		{"1 0 0 1 50 100 Tm -100 Tz", referencePDFCell{43.33, 92.07, 50, 102.07}, false},
		{"1 0 -1 1 50 100 Tm", referencePDFCell{42.07, 92.07, 58.74, 102.07}, true},
		{"0 1 -1 0 50 100 Tm", referencePDFCell{42.07, 93.33, 52.07, 100}, false},
		{"-1 0 0 1 50 100 Tm -100 Tz", referencePDFCell{50, 92.07, 56.67, 102.07}, true},
	}
	var streams []string
	for _, c := range cases {
		streams = append(streams, "BT /F1 10 Tf "+c.ops+" (A) Tj ET")
	}
	data := referencePDFMetricDocument(streams, "")
	for i, c := range cases {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			g := referencePDFPageGeometry(t, data, i, true)
			if len(g.chars) != 1 {
				t.Fatalf("chars=%v", g.chars)
			}
			got := g.chars[0]
			if got.char.text != "A" || got.char.box != c.box || got.upright != c.upright {
				t.Fatalf("got=%+v want box=%v upright=%t", got, c.box, c.upright)
			}
		})
	}
}

func referencePDFMetricDocument(streams []string, fontExtra string, extraFonts ...string) []byte {
	objects := []string{"<< /Type /Catalog /Pages 2 0 R >>", "", "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica " + fontExtra + " >>"}
	kids := ""
	for _, stream := range streams {
		n := len(objects) + 1
		kids += fmt.Sprintf("%d 0 R ", n)
		objects = append(objects, fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] /Resources << /Font << /F1 3 0 R %s >> >> /Contents %d 0 R >>", strings.Join(extraFonts, " "), n+1), fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(stream), stream))
	}
	objects[1] = fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", kids, len(streams))
	data := "%PDF-1.4\n"
	offsets := []int{0}
	for i, o := range objects {
		offsets = append(offsets, len(data))
		data += fmt.Sprintf("%d 0 obj\n%s\nendobj\n", i+1, o)
	}
	start := len(data)
	data += fmt.Sprintf("xref\n0 %d\n0000000000 65535 f \n", len(offsets))
	for _, o := range offsets[1:] {
		data += fmt.Sprintf("%010d 00000 n \n", o)
	}
	data += fmt.Sprintf("trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(offsets), start)
	return []byte(data)
}

func TestReferencePDFStandardMetricEligibility(t *testing.T) {
	for _, c := range []struct {
		extra string
		want  bool
	}{
		{"", true}, {"/Encoding /WinAnsiEncoding", true},
		{"/Encoding << /BaseEncoding /WinAnsiEncoding /Differences [65 /B] >>", false},
		{"/Encoding /MacRomanEncoding", false},
	} {
		t.Run(c.extra, func(t *testing.T) {
			got := referencePDFStandardFonts(t, referencePDFMetricDocument([]string{"BT /F1 10 Tf (A) Tj ET"}, c.extra))["Helvetica"]
			if got != c.want {
				t.Fatalf("eligible=%t want %t", got, c.want)
			}
		})
	}
}

func TestReferencePDFStandardMetricDirectFontConflict(t *testing.T) {
	data := referencePDFMetricDocument([]string{"BT /F1 10 Tf (A) Tj ET"}, "", "/F2 << /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding << /Differences [65 /B] >> >>")
	if referencePDFStandardFonts(t, data)["Helvetica"] {
		t.Fatal("conflicting direct font must disqualify the shared BaseFont name")
	}
}

func TestReferencePDFStandardMetricSubsetConflict(t *testing.T) {
	data := referencePDFMetricDocument([]string{"BT /F1 10 Tf (A) Tj ET"}, "", "/F2 << /Type /Font /Subtype /Type1 /BaseFont /ABCDEF+Helvetica /Encoding << /Differences [65 /B] >> >>")
	if referencePDFStandardFonts(t, data)["Helvetica"] {
		t.Fatal("subset name collision must disqualify standard metrics")
	}
}
