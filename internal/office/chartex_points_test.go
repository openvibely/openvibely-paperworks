package office

import (
	"bytes"
	"fmt"
	"strconv"
	"testing"

	"github.com/beevik/etree"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"github.com/xuri/excelize/v2"
)

func assertChartExLevels(t *testing.T, dim *etree.Element, want [][]string, prefix string, directNS bool) {
	t.Helper()
	levels := Children(dim, NSCX, "lvl")
	if len(levels) != len(want) {
		t.Fatalf("levels = %d, want %d", len(levels), len(want))
	}
	for li, texts := range want {
		lvl := levels[li]
		if lvl.Space != prefix || lvl.SelectAttrValue("ptCount", "") != strconv.Itoa(len(texts)) {
			t.Fatalf("level %d: namespace/count: %+v", li, lvl)
		}
		pts := lvl.ChildElements()
		if len(pts) != len(texts) {
			t.Fatalf("level %d: points = %d, want %d", li, len(pts), len(texts))
		}
		for i, text := range texts {
			pt := pts[i]
			if !Is(pt, NSCX, "pt") || pt.Space != prefix || pt.SelectAttrValue("idx", "") != strconv.Itoa(i) || pt.Text() != text {
				t.Fatalf("level %d point %d: got %+v text %q, want %q", li, i, pt, pt.Text(), text)
			}
			if (pt.SelectAttr("xmlns:cx") != nil) != directNS {
				t.Fatalf("level %d point %d: changed namespace declaration", li, i)
			}
		}
	}
}

func TestExtendedChartPointSerialization(t *testing.T) {
	for _, single := range []bool{false, true} {
		t.Run(fmt.Sprintf("single=%v", single), func(t *testing.T) {
			p := fixture(t, "reference-bar.pptx")
			values := []float64{1e20, 0.125, 1e-7}
			paths := [][]string{{"東京", ""}, {"", "北 & <南>"}, {"é", "🌲"}}
			want := [][]string{{"東京", "", "é"}, {"", "北 & <南>", "🌲"}}
			nums := []string{"1e+20", "0.125", "1e-07"}
			if single {
				values = []float64{7}
				paths = paths[:1]
				want = [][]string{{"東京"}, {""}}
				nums = []string{"7"}
			}
			reports, err := ExtendedCharts(p, ExtendedChartSpec{Action: "add", Type: "treemap", Slide: 1, Values: values, Paths: paths, Position: []int64{914400, 914400, 5486400, 3657600}})
			if err != nil {
				t.Fatal(err)
			}
			d, err := p.XML(reports[0].Chart)
			if err != nil {
				t.Fatal(err)
			}
			data := Child(Child(d.Root(), NSCX, "chartData"), NSCX, "data")
			assertChartExLevels(t, Child(data, NSCX, "strDim"), want, "cx", true)
			assertChartExLevels(t, Child(data, NSCX, "numDim"), [][]string{nums}, "cx", true)
		})
	}
}

func TestExtendedChartRefreshPointSerialization(t *testing.T) {
	p, _ := traceFixture(t, func(f *excelize.File) {
		for cell, value := range map[string]any{"A1": "東京", "A2": "", "A3": "é & <🌲>", "B1": true, "B2": 0.125, "B3": 7, "C1": 1e20, "C2": 0.125, "C3": 1e-7, "D1": 7} {
			if err := f.SetCellValue("Inputs", cell, value); err != nil {
				t.Fatal(err)
			}
		}
	})
	wb, err := OpenWorkbook(p)
	if err != nil {
		t.Fatal(err)
	}
	for _, prefix := range []string{"cx", "other", ""} {
		t.Run("prefix="+prefix, func(t *testing.T) {
			ns := "xmlns"
			if prefix != "" {
				ns += ":" + prefix
			}
			// Parse a serialized document to exercise namespace resolution and retained attributes.
			tag := func(s string) string {
				if prefix != "" {
					return prefix + ":" + s
				}
				return s
			}
			xml := fmt.Sprintf(`<%s %s="%s"><%s><%s id="0">`, tag("chartSpace"), ns, NSCX, tag("chartData"), tag("data"))
			for _, dim := range []struct{ kind, ref string }{{"strDim", "A1:B3"}, {"numDim", "C1:C3"}, {"numDim", "D1:D1"}} {
				xml += fmt.Sprintf(`<%s type="cat"><%s>Inputs!%s</%s><%s ptCount="99" formatCode="0.00" custom="keep"><%s idx="8">stale</%s></%s><%s><%s uri="keep"/></%s></%s>`, tag(dim.kind), tag("f"), dim.ref, tag("f"), tag("lvl"), tag("pt"), tag("pt"), tag("lvl"), tag("extLst"), tag("ext"), tag("extLst"), tag(dim.kind))
			}
			xml += fmt.Sprintf(`</%s></%s></%s>`, tag("data"), tag("chartData"), tag("chartSpace"))
			d, err := opc.ParseXML([]byte(xml))
			if err != nil {
				t.Fatal(err)
			}
			if n, err := ExtendedChartCache(d.Root(), wb, true); err != nil || n != 3 {
				t.Fatalf("refresh = %d, %v", n, err)
			}
			dims := Child(Child(d.Root(), NSCX, "chartData"), NSCX, "data").ChildElements()
			wants := [][][]string{{{"東京", "", "é & <🌲>"}, {"True", "0.125", "7"}}, {{"1e+20", "0.125", "1e-07"}}, {{"7.0"}}}
			for i, dim := range dims {
				assertChartExLevels(t, dim, wants[i], prefix, false)
				levels := Children(dim, NSCX, "lvl")
				if levels[0].SelectAttrValue("formatCode", "") != "0.00" || levels[0].SelectAttrValue("custom", "") != "keep" {
					t.Fatal("refresh lost level attributes")
				}
				if len(levels) > 1 && levels[1].SelectAttr("custom") != nil {
					t.Fatal("new level inherited unrelated attributes")
				}
				children := dim.ChildElements()
				if !Is(children[0], NSCX, "f") || !Is(children[len(children)-1], NSCX, "extLst") || Child(children[len(children)-1], NSCX, "ext").SelectAttrValue("uri", "") != "keep" {
					t.Fatal("formula/extension structure changed")
				}
			}
			before, err := d.WriteToBytes()
			if err != nil {
				t.Fatal(err)
			}
			// Verify the serialized refresh without changing it.
			d, err = opc.ParseXML(before)
			if err != nil {
				t.Fatal(err)
			}
			if n, err := ExtendedChartCache(d.Root(), wb, false); err != nil || n != 3 {
				t.Fatalf("verify = %d, %v", n, err)
			}
			after, err := d.WriteToBytes()
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(before, after) {
				t.Fatal("verification modified XML")
			}
		})
	}
}
