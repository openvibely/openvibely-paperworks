package pdf

// Bounded source port of ReportLab 5.0.1 Canvas.drawImage(ImageReader(...),
// mask=None), PDFImageXObject.loadImageFromSRC and the default one-page canvas.
// Only the RGB scan used by the original runtime is supported. Compression,
// timestamps, document IDs and general ReportLab serialization are not equivalence.
import (
	"bytes"
	"compress/zlib"
	"crypto/md5"
	"encoding/ascii85"
	"fmt"
	"image"
	"image/color"
	"testing"
)

func referenceScanPDF(t *testing.T, im image.Image) []byte {
	t.Helper()
	raw := make([]byte, 0, im.Bounds().Dx()*im.Bounds().Dy()*3)
	for y := im.Bounds().Min.Y; y < im.Bounds().Max.Y; y++ {
		for x := im.Bounds().Min.X; x < im.Bounds().Max.X; x++ {
			// ImageReader drops alpha with mask=None; it does not composite on black.
			c := color.NRGBAModel.Convert(im.At(x, y)).(color.NRGBA)
			raw = append(raw, c.R, c.G, c.B)
		}
	}
	signature := md5.New() // ReportLab's resource identifier, not a security hash.
	signature.Write(raw)
	signature.Write([]byte("None"))
	name := fmt.Sprintf("FormXob.%x", signature.Sum(nil))
	encode := func(data []byte) string {
		var b bytes.Buffer
		z := zlib.NewWriter(&b)
		if _, err := z.Write(data); err != nil {
			t.Fatal(err)
		}
		if err := z.Close(); err != nil {
			t.Fatal(err)
		}
		dst := make([]byte, ascii85.MaxEncodedLen(b.Len()))
		n := ascii85.Encode(dst, b.Bytes())
		return string(dst[:n]) + "~>"
	}
	content := []byte("1 0 0 1 0 0 cm  BT /F1 12 Tf 14.4 TL ET\nq\n600 0 0 400 0 0 cm\n/" + name + " Do\nQ\n \n")
	objs := []string{
		"<< /F1 2 0 R >>",
		"<< /BaseFont /Helvetica /Encoding /WinAnsiEncoding /Name /F1 /Subtype /Type1 /Type /Font >>",
		stream(fmt.Sprintf("/BitsPerComponent 8 /ColorSpace /DeviceRGB /Filter [/ASCII85Decode /FlateDecode] /Height %d /Subtype /Image /Type /XObject /Width %d", im.Bounds().Dy(), im.Bounds().Dx()), encode(raw)),
		fmt.Sprintf("<< /Contents 8 0 R /MediaBox [0 0 600 400] /Parent 7 0 R /Resources << /Font 1 0 R /ProcSet [/PDF /Text /ImageB /ImageC /ImageI] /XObject << /%s 3 0 R >> >> /Rotate 0 /Trans << >> /Type /Page >>", name),
		"<< /PageMode /UseNone /Pages 7 0 R /Type /Catalog >>",
		// Time/document identity intentionally absent: this is a test fixture builder,
		// not a byte-identical implementation of ReportLab's document metadata.
		"<< /Author (anonymous) /Creator (anonymous) /Keywords () /Subject (unspecified) /Title (untitled) /Trapped /False >>",
		"<< /Count 1 /Kids [4 0 R] /Type /Pages >>",
		stream("/Filter [/ASCII85Decode /FlateDecode]", encode(content)),
	}
	var b bytes.Buffer
	b.WriteString("%PDF-1.3\n%\x93\x8c\x8b\x9e\n")
	offsets := make([]int, len(objs))
	for i, o := range objs {
		offsets[i] = b.Len()
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", i+1, o)
	}
	xref := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f \n", len(objs)+1)
	for _, o := range offsets {
		fmt.Fprintf(&b, "%010d 00000 n \n", o)
	}
	fmt.Fprintf(&b, "trailer\n<< /Info 6 0 R /Root 5 0 R /Size %d >>\nstartxref\n%d\n%%%%EOF\n", len(objs)+1, xref)
	return b.Bytes()
}
