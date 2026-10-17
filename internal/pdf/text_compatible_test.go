package pdf

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// Same minimal simple-font contract as the independently generated pypdf cases.
func compatibleTextFixture(contents ...string) []byte {
	cmap := `/CIDInit/ProcSet findresource begin 12 dict begin begincmap /CIDSystemInfo<< /Registry (Adobe) /Ordering (UCS) /Supplement 0 >> def /CMapName/Adobe-Identity-UCS def /CMapType 2 def 1 begincodespacerange <00> <FF> endcodespacerange 4 beginbfchar <01> <0041> <02> <0042> <03> <0066006C> <20> <0020> endbfchar endcmap CMapName currentdict /CMap defineresource pop end end`
	widths := make([]string, 33)
	for i := range widths {
		widths[i] = "0"
	}
	for _, i := range []int{1, 2, 3} {
		widths[i] = "600"
	}
	widths[32] = "200"
	objs := []string{"<< /Type /Catalog /Pages 2 0 R >>", "", "<< /Type /Font /Subtype /TrueType /BaseFont /Example /FirstChar 0 /LastChar 32 /Widths [" + strings.Join(widths, " ") + "] /ToUnicode 4 0 R /FontDescriptor << /Flags 4 >> >>", stream("", cmap)}
	var kids []string
	for _, content := range contents {
		n := len(objs) + 1
		kids = append(kids, fmt.Sprintf("%d 0 R", n))
		objs = append(objs, fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] /Resources << /Font << /F 3 0 R >> >> /Contents %d 0 R >>", n+1), stream("", content))
	}
	objs[1] = fmt.Sprintf("<< /Type /Pages /Count %d /Kids [%s] >>", len(contents), strings.Join(kids, " "))
	return rawPDF(objs)
}

func TestPypdfPageTextDocumentBoundary(t *testing.T) {
	good := "BT /F 12 Tf 0 20 Td [<01> 120 <02> -120 <03>] TJ ET"
	raw := compatibleTextFixture(good, "BT /F 12 Tf <03> Tj ET")
	got, err := PypdfPageText(context.Background(), raw)
	if err != nil || !reflect.DeepEqual(got, []string{"A B fl", "fl"}) {
		t.Fatalf("%q: %v", got, err)
	}
	for _, suffix := range []string{"/Unknown Do", "BT /F 12 Tf 2 Tc <01> Tj ET", "BT /F 12 Tf <ff> Tj ET"} {
		t.Run(suffix, func(t *testing.T) {
			got, err := PypdfPageText(context.Background(), compatibleTextFixture(good, good+" "+suffix))
			if err == nil || got != nil || !strings.Contains(err.Error(), "page 2") {
				t.Fatalf("partial result or hidden rejection: %q %v", got, err)
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if got, err := PypdfPageText(ctx, raw); err != context.Canceled || got != nil {
		t.Fatalf("cancellation: %q %v", got, err)
	}
}
