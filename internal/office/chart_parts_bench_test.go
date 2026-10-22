package office

import (
	"fmt"
	"strings"
	"testing"

	"github.com/openvibely/openvibely-paperworks/internal/opc"
)

func BenchmarkChartPartsManyParts(b *testing.B) {
	p, err := opc.Open("../../testdata/fixtures/reference-bar.pptx")
	if err != nil {
		b.Fatal(err)
	}
	ct := string(p.Get("[Content_Types].xml"))
	var extra strings.Builder
	for i := 0; i < 600; i++ {
		name := fmt.Sprintf("ppt/media/bench%d.xml", i)
		fmt.Fprintf(&extra, `<Override PartName="/%s" ContentType="application/xml"/>`, name)
		p.Set(name, []byte("<x/>"))
	}
	p.Set("[Content_Types].xml", []byte(strings.Replace(ct, "</Types>", extra.String()+"</Types>", 1)))
	raw, err := p.Bytes()
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		q, err := opc.Read(raw)
		if err != nil {
			b.Fatal(err)
		}
		b.StartTimer()
		parts, err := ChartParts(q)
		if err != nil || len(parts) == 0 {
			b.Fatal(parts, err)
		}
	}
}
