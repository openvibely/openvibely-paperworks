package convert

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"github.com/openvibely/openvibely-paperworks/internal/pdf"
)

// Runtime 839-864: all four screen-view modes deliberately oppose the print
// options. Interior-edge assertion is written; geometry adapter review is open.
// Execution authorized; passing this port does not certify the geometry adapter.
func TestReferenceStandaloneRuntimeViewSettings(t *testing.T) {
	referenceRuntimeOffice(t)
	dir := t.TempDir()
	modes := []struct{ headers, grid bool }{{true, true}, {true, false}, {false, true}, {false, false}}
	src := referenceRuntimeOriginalWorkbookFixture(t, "runtime-workflow-fixtures.zip", "view-settings")
	must := func(e error) {
		t.Helper()
		if e != nil {
			t.Fatal(e)
		}
	}
	before := contents(t, src)
	out := filepath.Join(dir, "render")
	images, e := RenderFile(context.Background(), src, out, "", "", pdf.RenderOptions{})
	must(e)
	t.Run("images", func(t *testing.T) {
		if len(images) != 4 {
			t.Errorf("images=%d, want 4", len(images))
		}
	})
	pages := referenceRuntimePDFTexts(t, contents(t, filepath.Join(out, "view-settings.pdf")))
	t.Run("pages", func(t *testing.T) {
		if len(pages) != 4 {
			t.Errorf("pages=%d, want 4", len(pages))
		}
	})
	for i, m := range modes {
		t.Run(fmt.Sprintf("headers-%t-grid-%t", m.headers, m.grid), func(t *testing.T) {
			if i >= len(pages) {
				t.Fatal("missing page", i)
			}
			g := referencePDFPageGeometry(t, contents(t, filepath.Join(out, "view-settings.pdf")), i, true)

			words := map[string]bool{}
			for _, word := range g.words {
				words[word.text] = true
			}
			t.Run("cell-words", func(t *testing.T) {
				for _, s := range []string{"North", "South", "East", "West"} {
					if !words[s] {
						t.Errorf("missing %q in %v", s, words)
					}
				}
			})
			t.Run("only-expected-headers", func(t *testing.T) {
				var extra []string
				for s := range words {
					if s != "North" && s != "South" && s != "East" && s != "West" {
						extra = append(extra, s)
					}
				}
				sort.Strings(extra)
				var want []string
				if m.headers {
					want = []string{"1", "2", "A", "B"}
				}
				if !reflect.DeepEqual(extra, want) {
					t.Errorf("extra words=%v, want %v", extra, want)
				}
			})
			t.Run("interior-edges", func(t *testing.T) {
				north := referencePDFRequireWord(t, g, "North")
				south := referencePDFRequireWord(t, g, "South")
				east := referencePDFRequireWord(t, g, "East")
				horizontal, vertical := false, false
				edges := referencePDFEdges(g)
				for _, edge := range edges {
					horizontal = horizontal || (north.bottom < edge.top && edge.top < east.top && math.Abs(edge.bottom-edge.top) < .5 && edge.x0 <= north.x0 && edge.x1 >= south.x1)
					vertical = vertical || (north.x1 < edge.x0 && edge.x0 < south.x0 && math.Abs(edge.x1-edge.x0) < .5 && edge.top <= north.top && edge.bottom >= east.bottom)
				}
				if horizontal != m.grid || vertical != m.grid {
					t.Errorf("headers=%t grid=%t horizontal=%t vertical=%t edges=%+v", m.headers, m.grid, horizontal, vertical, edges)
				}
			})
		})
	}
	t.Run("source", func(t *testing.T) {
		if !bytes.Equal(before, contents(t, src)) {
			t.Error("source modified")
		}
	})
}
