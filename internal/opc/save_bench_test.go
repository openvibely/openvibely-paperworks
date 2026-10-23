package opc

import (
	"archive/zip"
	"bytes"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"testing"
)

const benchmarkDocumentName = "word/document.xml"

func saveBenchmarkPackage(tb testing.TB, mediaParts, mediaBytes int) *Package {
	tb.Helper()
	var archive bytes.Buffer
	zw := zip.NewWriter(&archive)
	add := func(name string, data []byte) {
		tb.Helper()
		w, err := zw.Create(name)
		if err != nil {
			tb.Fatal(err)
		}
		if _, err := w.Write(data); err != nil {
			tb.Fatal(err)
		}
	}
	add("[Content_Types].xml", []byte(testCT))
	add("_rels/.rels", []byte(`<Relationships xmlns="`+NSRel+`"/>`))
	add(benchmarkDocumentName, []byte(`<document><body>original</body></document>`))

	rng := rand.New(rand.NewSource(1))
	for i := 0; i < mediaParts; i++ {
		data := make([]byte, mediaBytes)
		if _, err := rng.Read(data); err != nil {
			tb.Fatal(err)
		}
		add(fmt.Sprintf("word/media/image%05d.bin", i), data)
	}
	if err := zw.Close(); err != nil {
		tb.Fatal(err)
	}
	pkg, err := Read(archive.Bytes())
	if err != nil {
		tb.Fatal(err)
	}
	pkg.Set(benchmarkDocumentName, []byte(`<document><body>edited</body></document>`))
	return pkg
}

func benchmarkSaveNew(b *testing.B, mediaParts, mediaBytes int) {
	pkg := saveBenchmarkPackage(b, mediaParts, mediaBytes)
	output := filepath.Join(b.TempDir(), "package.zip")
	b.ReportAllocs()
	b.SetBytes(int64(mediaParts * mediaBytes))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := pkg.SaveNew(output); err != nil {
			b.Fatal(err)
		}
		b.StopTimer()
		if err := os.Remove(output); err != nil {
			b.Fatal(err)
		}
		b.StartTimer()
	}
}

// This fixture has 200 MiB of unchanged, incompressible media across 20 parts.
func BenchmarkSaveNew200MiBMedia(b *testing.B) {
	benchmarkSaveNew(b, 20, 10<<20)
}

// This fixture exercises ZIP metadata overhead with 10,000 small unchanged media parts.
func BenchmarkSaveNewManyParts(b *testing.B) {
	benchmarkSaveNew(b, 10_000, 256)
}
