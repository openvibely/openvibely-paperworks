package opc

import (
	"archive/zip"
	"bytes"
	"fmt"
	"math/rand"
	"testing"
)

func largeMediaPackage(tb testing.TB) []byte {
	tb.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	add := func(name string, method uint16, data []byte) {
		w, err := zw.CreateHeader(&zip.FileHeader{Name: name, Method: method})
		if err != nil {
			tb.Fatal(err)
		}
		if _, err := w.Write(data); err != nil {
			tb.Fatal(err)
		}
	}
	add(contentTypesName, zip.Deflate, []byte(`<?xml version="1.0"?><Types xmlns="`+NSCT+`"><Default Extension="xml" ContentType="application/xml"/></Types>`))
	add("word/document.xml", zip.Deflate, []byte(`<?xml version="1.0"?><document><body>hello</body></document>`))
	rng := rand.New(rand.NewSource(1))
	for i := 0; i < 20; i++ {
		media := make([]byte, 2<<20)
		rng.Read(media)
		add(fmt.Sprintf("word/media/image%d.png", i), zip.Deflate, media)
	}
	if err := zw.Close(); err != nil {
		tb.Fatal(err)
	}
	return buf.Bytes()
}

func BenchmarkSaveSmallEditLargeMedia(b *testing.B) {
	p, err := Read(largeMediaPackage(b))
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		p.Set("word/document.xml", []byte(fmt.Sprintf(`<?xml version="1.0"?><document><body>edit %d</body></document>`, i)))
		if _, err := p.Bytes(); err != nil {
			b.Fatal(err)
		}
	}
}
