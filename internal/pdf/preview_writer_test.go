package pdf

import (
	"bytes"
	"context"
	"os"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// Reference output is captured from the unchanged pinned select_preview_sheets
// on this exact input. This checks the reachable selected graph, not strict bytes or orphan objects.
func TestPreviewWriterReferenceGraph(t *testing.T) {
	for _, tc := range []struct {
		name, prefix string
		pages        []int
	}{{"installed", "", []int{1}}, {"inherited-repeated", "inherited-", []int{3, 1, 3}}} {
		t.Run(tc.name, func(t *testing.T) {
			input, e := os.ReadFile("testdata/preview-writer/" + tc.prefix + "input.pdf")
			if e != nil {
				t.Fatal(e)
			}
			expected, e := os.ReadFile("testdata/preview-writer/" + tc.prefix + "expected.pdf")
			if e != nil {
				t.Fatal(e)
			}
			got, e := SelectPreviewPages(context.Background(), input, tc.pages)
			if e != nil {
				t.Fatal(e)
			}
			canon := func(data []byte) []byte {
				c, e := Load(context.Background(), data)
				if e != nil {
					t.Fatal(e)
				}
				seen := map[types.IndirectRef]int{}
				var b bytes.Buffer
				var walk func(types.Object)
				walk = func(o types.Object) {
					switch v := o.(type) {
					case types.IndirectRef:
						if n, ok := seen[v]; ok {
							writePreviewObject(&b, types.Integer(n))
							return
						}
						seen[v] = len(seen) + 1
						b.WriteString("object{")
						x, e := c.Dereference(v)
						if e != nil {
							t.Fatal(e)
						}
						walk(x)
						b.WriteByte('}')
					case types.Dict:
						b.WriteString("<<")
						for _, k := range previewKeys(v) {
							b.WriteString("/" + k + " ")
							walk(v[k])
						}
						b.WriteString(">>")
					case types.Array:
						b.WriteByte('[')
						for _, x := range v {
							walk(x)
							b.WriteByte(' ')
						}
						b.WriteByte(']')
					case types.StreamDict:
						walk(v.Dict)
						b.Write(v.Raw)
					default:
						if e := writePreviewObject(&b, o); e != nil {
							t.Fatal(e)
						}
					}
				}
				walk(*c.Root)
				walk(*c.Info)
				return b.Bytes()
			}
			if !bytes.Equal(canon(got), canon(expected)) {
				t.Fatal("reachable graph differs from pinned page-copy reference")
			}
			again, e := SelectPreviewPages(context.Background(), input, tc.pages)
			if e != nil || !bytes.Equal(got, again) {
				t.Fatal("selection not deterministic", e)
			}
			// The existing collector is the before-implementation control.
			old, e := SelectPages(context.Background(), input, tc.pages)
			if e != nil {
				t.Fatal(e)
			}
			if bytes.Equal(canon(old), canon(expected)) {
				t.Fatal("before control unexpectedly matches")
			}
		})
	}
}

func TestPreviewWriterCompleteFiles(t *testing.T) {
	for _, tc := range []struct {
		prefix string
		pages  []int
	}{{"", []int{1}}, {"inherited-", []int{3, 1, 3}}} {
		t.Run(tc.prefix, func(t *testing.T) {
			input, e := os.ReadFile("testdata/preview-writer/" + tc.prefix + "input.pdf")
			if e != nil {
				t.Fatal(e)
			}
			want, e := os.ReadFile("testdata/preview-writer/" + tc.prefix + "expected.pdf")
			if e != nil {
				t.Fatal(e)
			}
			got, e := SelectPreviewPages(context.Background(), input, tc.pages)
			if e != nil {
				t.Fatal(e)
			}
			if !bytes.Equal(got, want) {
				t.Fatalf("complete file differs: %d vs %d", len(got), len(want))
			}
		})
	}
}
