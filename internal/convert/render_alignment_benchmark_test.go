package convert

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/openvibely/openvibely-paperworks/internal/pdf"
)

var alignmentBenchmarkResult []pageAlignment

func benchmarkAlignmentPage(id int) image.Image {
	page := image.NewRGBA(image.Rect(0, 0, 48, 48))
	for y := 0; y < 48; y++ {
		for x := 0; x < 48; x++ {
			page.SetRGBA(x, y, color.RGBA{
				R: uint8((id*37 + x*19 + y*7 + x*y*3) & 255),
				G: uint8((id*71 + x*5 + y*29 + x*y*11) & 255),
				B: uint8((id*113 + x*23 + y*13 + x*y*5) & 255),
				A: 255,
			})
		}
	}
	return page
}

func benchmarkAlignmentSequences(count int) map[string][2][]image.Image {
	before := make([]image.Image, count)
	for i := range before {
		before[i] = benchmarkAlignmentPage(i + 1)
	}
	unchanged := append([]image.Image(nil), before...)

	inserted := make([]image.Image, 0, count+1)
	inserted = append(inserted, before[:count/2]...)
	inserted = append(inserted, benchmarkAlignmentPage(-count-1))
	inserted = append(inserted, before[count/2:]...)

	changed := append([]image.Image(nil), before...)
	for i := 7; i < count; i += 17 {
		mutated := image.NewRGBA(before[i].Bounds())
		draw.Draw(mutated, mutated.Bounds(), before[i], before[i].Bounds().Min, draw.Src)
		draw.Draw(mutated, image.Rect(4, 4, 12, 12), image.NewUniform(color.RGBA{R: 245, G: 18, B: 204, A: 255}), image.Point{}, draw.Src)
		changed[i] = mutated
	}
	return map[string][2][]image.Image{
		"unchanged": {before, unchanged},
		"inserted":  {before, inserted},
		"changed":   {before, changed},
	}
}

func BenchmarkAlignPages(b *testing.B) {
	for _, count := range []int{50, 100, 250, 500} {
		sequences := benchmarkAlignmentSequences(count)
		for _, scenario := range []string{"unchanged", "inserted", "changed"} {
			pair := sequences[scenario]
			b.Run(fmt.Sprintf("before=%d/after=%d/%s", len(pair[0]), len(pair[1]), scenario), func(b *testing.B) {
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					alignmentBenchmarkResult = alignPages(pair[0], pair[1])
				}
			})
		}
	}
}

func writeBenchmarkPages(b *testing.B, dir, prefix string, pages []image.Image) []string {
	b.Helper()
	paths := make([]string, len(pages))
	for i, page := range pages {
		path := filepath.Join(dir, fmt.Sprintf("%s-%04d.png", prefix, i))
		f, err := os.Create(path)
		if err != nil {
			b.Fatal(err)
		}
		if err := png.Encode(f, page); err != nil {
			f.Close()
			b.Fatal(err)
		}
		if err := f.Close(); err != nil {
			b.Fatal(err)
		}
		paths[i] = path
	}
	return paths
}

func BenchmarkCompareAlignment(b *testing.B) {
	const count = 50
	root := b.TempDir()
	sequences := benchmarkAlignmentSequences(count)
	for _, scenario := range []string{"unchanged", "inserted", "changed"} {
		pair := sequences[scenario]
		fixtureDir := filepath.Join(root, scenario)
		if err := os.MkdirAll(fixtureDir, 0o755); err != nil {
			b.Fatal(err)
		}
		beforePaths := writeBenchmarkPages(b, fixtureDir, "before", pair[0])
		afterPaths := writeBenchmarkPages(b, fixtureDir, "after", pair[1])
		ctx := withDeps(dependencies{render: func(_ context.Context, input, _ string, _ pdf.RenderOptions) ([]string, error) {
			switch input {
			case "before.pdf":
				return beforePaths, nil
			case "after.pdf":
				return afterPaths, nil
			default:
				return nil, fmt.Errorf("unexpected render input %q", input)
			}
		}})
		b.Run(scenario, func(b *testing.B) {
			output := filepath.Join(fixtureDir, "comparison")
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				b.StopTimer()
				if err := os.RemoveAll(output); err != nil {
					b.Fatal(err)
				}
				b.StartTimer()
				if _, err := Compare(ctx, "before.pdf", "after.pdf", output, pdf.RenderOptions{}); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
