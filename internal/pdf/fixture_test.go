package pdf

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// rawPDF assembles numbered objects into a PDF with a valid xref table.
func rawPDF(objs []string) []byte {
	var b bytes.Buffer
	b.WriteString("%PDF-1.7\n%\xe2\xe3\xcf\xd3\n")
	offs := make([]int, len(objs))
	for i, o := range objs {
		offs[i] = b.Len()
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", i+1, o)
	}
	x := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f \n", len(objs)+1)
	for _, o := range offs {
		fmt.Fprintf(&b, "%010d 00000 n \n", o)
	}
	fmt.Fprintf(&b, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objs)+1, x)
	return b.Bytes()
}

func stream(dict, body string) string {
	return fmt.Sprintf("<< %s /Length %d >>\nstream\n%s\nendstream", dict, len(body), body)
}

const (
	radioOn  = "0 g 10 10 m 10 15.5 14.5 20 10 20 c 4.5 20 0 15.5 0 10 c 0 4.5 4.5 0 10 0 c 15.5 0 20 4.5 20 10 c f"
	radioOff = "0 G 1 w 10 19.5 m 4.75 19.5 .5 15.25 .5 10 c .5 4.75 4.75 .5 10 .5 c 15.25 .5 19.5 4.75 19.5 10 c 19.5 15.25 15.25 19.5 10 19.5 c s"
)

// formPDF mirrors the reportlab fixture: page 1 holds a three-way radio
// group and a checkbox; page 2 a text field and a combo box.
func formPDF() []byte { return rawPDF(formObjs()) }

func formObjs() []string {
	form := "/Type /XObject /Subtype /Form /BBox [0 0 20 20] /Resources << >>"
	text := "/Type /XObject /Subtype /Form /BBox [0 0 180 20] /Resources << /Font << /Helv 5 0 R >> >>"
	objs := []string{
		/*1*/ "<< /Type /Catalog /Pages 2 0 R /AcroForm << /Fields [6 0 R 10 0 R 13 0 R 15 0 R] /DR << /Font << /Helv 5 0 R >> >> /DA (/Helv 0 Tf 0 g) >> >>",
		/*2*/ "<< /Type /Pages /Kids [3 0 R 4 0 R] /Count 2 >>",
		/*3*/ "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 400 400] /Resources << /Font << /Helv 5 0 R >> >> /Contents 17 0 R /Annots [7 0 R 8 0 R 9 0 R 11 0 R] >>",
		/*4*/ "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 400 400] /Resources << /Font << /Helv 5 0 R >> >> /Contents 18 0 R /Annots [14 0 R 16 0 R] >>",
		/*5*/ "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>",
		/*6*/ "<< /FT /Btn /Ff 49152 /T (answer) /V /A /Kids [7 0 R 8 0 R 9 0 R] >>",
		/*7*/ "<< /Type /Annot /Subtype /Widget /Parent 6 0 R /Rect [40 300 60 320] /F 4 /P 3 0 R /AS /A /AP << /N << /A 19 0 R /Off 20 0 R >> >> >>",
		/*8*/ "<< /Type /Annot /Subtype /Widget /Parent 6 0 R /Rect [40 250 60 270] /F 4 /P 3 0 R /AS /Off /AP << /N << /B 21 0 R /Off 22 0 R >> >> >>",
		/*9*/ "<< /Type /Annot /Subtype /Widget /Parent 6 0 R /Rect [40 200 60 220] /F 4 /P 3 0 R /AS /Off /AP << /N << /C 23 0 R /Off 24 0 R >> >> >>",
		/*10*/ "<< /FT /Btn /T (approved) /V /Yes /Kids [11 0 R] >>",
		/*11*/ "<< /Type /Annot /Subtype /Widget /Parent 10 0 R /Rect [150 300 170 320] /F 4 /P 3 0 R /AS /Yes /AP << /N << /Yes 25 0 R /Off 26 0 R >> >> >>",
		/*12*/ "null",
		/*13*/ "<< /FT /Tx /T (name) /V (Original) /DA (/Helv 12 Tf 0 g) /Kids [14 0 R] >>",
		/*14*/ "<< /Type /Annot /Subtype /Widget /Parent 13 0 R /Rect [40 300 220 320] /F 4 /P 4 0 R /AP << /N 27 0 R >> >>",
		/*15*/ "<< /FT /Ch /Ff 131072 /T (choice) /V (Alpha) /Opt [(Alpha) (Beta)] /DA (/Helv 12 Tf 0 g) /Kids [16 0 R] >>",
		/*16*/ "<< /Type /Annot /Subtype /Widget /Parent 15 0 R /Rect [40 200 220 220] /F 4 /P 4 0 R /AP << /N 28 0 R >> >>",
		/*17*/ stream("", "BT /Helv 12 Tf 20 370 Td (Form page 1) Tj ET"),
		/*18*/ stream("", "BT /Helv 12 Tf 20 370 Td (Form page 2) Tj ET"),
		/*19*/ stream(form, radioOff+" "+radioOn),
		/*20*/ stream(form, radioOff),
		/*21*/ stream(form, radioOff+" "+radioOn),
		/*22*/ stream(form, radioOff),
		/*23*/ stream(form, radioOff+" "+radioOn),
		/*24*/ stream(form, radioOff),
		/*25*/ stream(form, "0 G 1 w .5 .5 19 19 re s 0 g 4 4 12 12 re f"),
		/*26*/ stream(form, "0 G 1 w .5 .5 19 19 re s"),
		/*27*/ stream(text, "/Tx BMC q BT /Helv 12 Tf 0 g 2 5 Td (Original) Tj ET Q EMC"),
		/*28*/ stream(text, "/Tx BMC q BT /Helv 12 Tf 0 g 2 5 Td (Alpha) Tj ET Q EMC"),
	}
	return objs
}

func writeTemp(t *testing.T, name string, data []byte) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}
