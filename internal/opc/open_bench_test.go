package opc

import (
	"archive/zip"
	"hash/crc32"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func BenchmarkOpen(b *testing.B) {
	fixtures := []struct {
		name string
		path string
	}{
		{name: "DOCX", path: "../../testdata/inspection-reports/call-0344.docx"},
		{name: "XLSX", path: "../../testdata/fixtures/reference-pie.xlsx"},
		{name: "PPTX", path: "../../testdata/fixtures/reference-waterfall.pptx"},
	}
	for _, fixture := range fixtures {
		b.Run(fixture.name, func(b *testing.B) {
			benchmarkOpenFile(b, fixture.path)
		})
	}
	b.Run("LargeMedia", func(b *testing.B) {
		path := filepath.Join(b.TempDir(), "large-media.docx")
		writeOpenBenchmarkLargeMedia(b, path)
		benchmarkOpenFile(b, path)
	})
}

func benchmarkOpenFile(b *testing.B, file string) {
	info, err := os.Stat(file)
	if err != nil {
		b.Fatal(err)
	}
	b.SetBytes(info.Size())
	b.ReportAllocs()
	b.ResetTimer()
	b.ReportMetric(float64(info.Size()), "input-B")
	for i := 0; i < b.N; i++ {
		p, err := Open(file)
		if err != nil {
			b.Fatal(err)
		}
		if len(p.Parts) == 0 {
			b.Fatal("opened package has no parts")
		}
		runtime.KeepAlive(p)
	}
}

// writeOpenBenchmarkLargeMedia streams a valid package with 32 MiB of
// incompressible stored media to disk, keeping fixture generation out of the
// Open benchmark's allocation and resident-memory measurements.
func writeOpenBenchmarkLargeMedia(tb testing.TB, file string) {
	tb.Helper()
	out, err := os.Create(file)
	if err != nil {
		tb.Fatal(err)
	}
	zw := zip.NewWriter(out)
	add := func(name string, method uint16, data []byte) {
		tb.Helper()
		if method == zip.Store {
			h := &zip.FileHeader{
				Name:               name,
				Method:             zip.Store,
				CRC32:              crc32.ChecksumIEEE(data),
				CompressedSize64:   uint64(len(data)),
				UncompressedSize64: uint64(len(data)),
			}
			w, err := zw.CreateRaw(h)
			if err != nil {
				tb.Fatal(err)
			}
			if _, err := w.Write(data); err != nil {
				tb.Fatal(err)
			}
			return
		}
		w, err := zw.CreateHeader(&zip.FileHeader{Name: name, Method: method})
		if err != nil {
			tb.Fatal(err)
		}
		if _, err := w.Write(data); err != nil {
			tb.Fatal(err)
		}
	}

	add(contentTypesName, zip.Deflate, []byte(testCT))
	add("word/document.xml", zip.Deflate, []byte("<?xml version=\"1.0\"?><document><body/></document>"))
	media := make([]byte, 2<<20)
	rng := rand.New(rand.NewSource(1))
	for i := 0; i < 16; i++ {
		if _, err := rng.Read(media); err != nil {
			tb.Fatal(err)
		}
		add(filepath.ToSlash(filepath.Join("word/media", "image"+string(rune('a'+i))+".bin")), zip.Store, media)
	}
	if err := zw.Close(); err != nil {
		tb.Fatal(err)
	}
	if err := out.Close(); err != nil {
		tb.Fatal(err)
	}
}
