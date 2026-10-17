package pdf

import (
	"context"
	"fmt"
	"reflect"
	"testing"
)

func compatibleCoreFixture(font, extra, contents string) []byte {
	return rawPDF([]string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] /Resources << /Font << /F 4 0 R >> >> /Contents 5 0 R >>",
		fmt.Sprintf("<< /Type /Font /Subtype /Type1 /BaseFont /%s /Encoding /WinAnsiEncoding %s >>", font, extra),
		stream("", contents),
	})
}
func TestPypdfCoreWinAnsi(t *testing.T) {
	// Independently compared with pypdf on these exact stream operands for all12 fonts.
	for name := range compatibleCoreWidths {
		t.Run(name, func(t *testing.T) {
			input := compatibleCoreFixture(name, "", "BT /F 12 Tf 14 TL 1 0 0 1 20 150 Tm (A'`) Tj T* [(B) -1000 (C)] TJ T* <97> Tj ET")
			got, err := PypdfPageText(context.Background(), input)
			want := []string{"A'`\nB C\n—"}
			if err != nil || !reflect.DeepEqual(got, want) {
				t.Fatalf("%q %v; want %q", got, err, want)
			}
		})
	}
}
func TestPypdfCoreRejectsOverrides(t *testing.T) {
	for _, extra := range []string{
		"/Encoding /StandardEncoding", "/Encoding << /BaseEncoding /WinAnsiEncoding /Differences [65 /B] >>",
		"/ToUnicode 9 0 R", "/Widths [600] /FirstChar 65 /LastChar 65",
		"/FontDescriptor << /MissingWidth 600 >>",
	} {
		t.Run(extra, func(t *testing.T) {
			got, err := PypdfPageText(context.Background(), compatibleCoreFixture("Helvetica", extra, "BT /F 12 Tf (A) Tj ET"))
			if err == nil || got != nil {
				t.Fatalf("accepted custom font: %q %v", got, err)
			}
		})
	}
	for _, tc := range []struct{ name, content string }{
		{"Symbol", "BT /F 12 Tf (A) Tj ET"},
		{"Unknown", "BT /F 12 Tf (A) Tj ET"},
		{"Arial", "BT /F 12 Tf (A) Tj ET"},
		{"Helvetica", "BT /F 12 Tf (A) Tj <80> Tj ET"},
		{"Helvetica", "BT /F 12 Tf (A) Tj ET /X Do"},
	} {
		got, err := PypdfPageText(context.Background(), compatibleCoreFixture(tc.name, "", tc.content))
		if err == nil || got != nil {
			t.Fatalf("partial/unsupported result: %q %v", got, err)
		}
	}
}

func compatibleImageFixture(subtype, content string) []byte {
	return rawPDF([]string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] /Resources << /Font << /F 4 0 R >> /XObject << /Im 6 0 R >> >> /Contents 5 0 R >>",
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>",
		stream("", content),
		stream("/Type /XObject /Subtype /"+subtype+" /Width 1 /Height 1 /ColorSpace /DeviceRGB /BitsPerComponent 8", "\x00\x00\x00"),
	})
}
func TestPypdfImageTextBoundary(t *testing.T) {
	for _, tc := range []struct{ content, want string }{
		{"/Im Do", ""},
		{"BT /F 12 Tf 1 0 0 1 20 100 Tm (A) Tj ET /Im Do", "A\n"},
		{"BT /F 12 Tf 1 0 0 1 20 100 Tm (A) Tj ET /Im Do BT /F 12 Tf 1 0 0 1 20 80 Tm (B) Tj ET", "A\nB"},
	} {
		got, err := PypdfPageText(context.Background(), compatibleImageFixture("Image", tc.content))
		if err != nil || !reflect.DeepEqual(got, []string{tc.want}) {
			t.Fatalf("%q: %q %v; want %q", tc.content, got, err, tc.want)
		}
	}
	for _, tc := range []struct{ subtype, content string }{
		{"Form", "BT /F 12 Tf (A) Tj ET /Im Do"},
		{"Image", "BT /F 12 Tf (A) Tj ET /Missing Do"},
		{"Image", "BT /F 12 Tf (A) Tj /Im Do ET"},
	} {
		got, err := PypdfPageText(context.Background(), compatibleImageFixture(tc.subtype, tc.content))
		if err == nil || got != nil {
			t.Fatalf("partial/unsupported image result: %q %v", got, err)
		}
	}
}
