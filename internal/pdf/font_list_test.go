package pdf

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestListFontsNestedAndRepeated(t *testing.T) {
	data := rawPDF([]string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R 4 0 R] /Count 2 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 300 300] /Resources << /XObject << /A 7 0 R >> >> /Contents 5 0 R >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 300 300] /Resources << /XObject << /A 7 0 R >> >> /Contents 5 0 R >>",
		stream("", "/A Do /A Do"),
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		stream("/Type /XObject /Subtype /Form /BBox [0 0 300 300] /Resources << /XObject << /B 8 0 R >> >>", "/B Do"),
		stream("/Type /XObject /Subtype /Form /BBox [0 0 300 300] /Resources << /Font << /F 6 0 R >> >>", "BT /F 12 Tf 20 20 Td (hello) Tj ET"),
	})
	got, err := ListFonts(context.Background(), data)
	if err != nil {
		t.Fatal(err)
	}
	want := []PDFFont{{Name: "Helvetica", Embedded: false, Pages: []int{1, 2}}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v want %+v", got, want)
	}
}
func TestListFontsEmbeddedAndEmpty(t *testing.T) {
	for _, text := range []string{"Hello", ""} {
		out := filepath.Join(t.TempDir(), "font.pdf")
		elements := []CanvasElement{}
		if text != "" {
			elements = append(elements, CanvasElement{Type: "text", X: 20, Y: 40, Text: text, FontSize: 14})
		}
		err := Create(context.Background(), CreateSpec{Canvas: &Canvas{Pages: []CanvasPage{{Width: 300, Height: 200, Elements: elements}}}}, ".", out)
		if err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(out)
		if err != nil {
			t.Fatal(err)
		}
		got, err := ListFonts(context.Background(), data)
		if err != nil {
			t.Fatal(err)
		}
		if text == "" {
			if got == nil || len(got) != 0 {
				t.Fatal(got)
			}
		} else if len(got) != 1 || !got[0].Embedded || got[0].Name == "" || !reflect.DeepEqual(got[0].Pages, []int{1}) {
			t.Fatal(got)
		}
	}
	if _, err := ListFonts(context.Background(), []byte("not a pdf")); err == nil {
		t.Fatal("accepted invalid PDF")
	}
}
