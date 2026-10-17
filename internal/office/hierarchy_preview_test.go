package office

import (
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/beevik/etree"
)

// The original styled_hierarchy helper reverses the reference levels to
// leaf-first order and sets explicit title, legend, parent labels and colors.
func referenceStyledHierarchy(t *testing.T, kind, position, mode string, overlay ...bool) string {
	t.Helper()
	p := previewOpen(t, "../../testdata/fixtures/reference-hierarchy-"+kind+".pptx")
	var parts []string
	zip := referencePreviewZIP(t, "../../testdata/fixtures/reference-hierarchy-"+kind+".pptx")
	for _, name := range p.Names() {
		if !strings.HasSuffix(name, ".xml") {
			continue
		}
		root := referencePackageXML(t, zip, name)
		if Is(root, NSCX, "chartSpace") {
			parts = append(parts, name)
			break
		}
	}
	if len(parts) == 0 {
		t.Fatal("missing extended chartSpace")
	}
	d, e := p.XML(parts[0])
	if e != nil {
		t.Fatal(e)
	}
	chart := Child(d.Root(), NSCX, "chart")
	appendXML := func(parent *etree.Element, raw string) *etree.Element {
		doc := etree.NewDocument()
		if e := doc.ReadFromString(raw); e != nil {
			t.Fatal(e)
		}
		n := doc.Root()
		parent.AddChild(n)
		return n
	}
	title := appendXML(chart, `<cx:title xmlns:cx="`+NSCX+`" xmlns:a="`+NSA+`" overlay="0"><cx:tx><cx:txData><cx:v>Quarterly regional sales</cx:v></cx:txData></cx:tx><cx:txPr><a:p><a:pPr><a:defRPr sz="1600" b="1"><a:latin typeface="Liberation Sans"/><a:solidFill><a:srgbClr val="123456"/></a:solidFill></a:defRPr></a:pPr></a:p></cx:txPr></cx:title>`)
	chart.RemoveChild(title)
	chart.InsertChildAt(0, title)
	for _, l := range Children(chart, NSCX, "legend") {
		chart.RemoveChild(l)
	}
	if position != "" {
		legendOverlay := "0"
		if len(overlay) > 0 && overlay[0] {
			legendOverlay = "1"
		}
		appendXML(chart, `<cx:legend xmlns:cx="`+NSCX+`" pos="`+position+`" overlay="`+legendOverlay+`"/>`)
	}
	series := All(chart, NSCX, "series")[0]
	if mode != "" {
		appendXML(series, `<cx:layoutPr xmlns:cx="`+NSCX+`"><cx:parentLabelLayout val="`+mode+`"/></cx:layoutPr>`)
	}
	dim := All(d.Root(), NSCX, "strDim")[0]
	levels := Children(dim, NSCX, "lvl")
	for _, l := range levels {
		dim.RemoveChild(l)
	}
	for i := len(levels) - 1; i >= 0; i-- {
		dim.AddChild(levels[i])
	}
	for i, color := range []string{"C01020", "20A030", "3040D0"} {
		appendXML(series, fmt.Sprintf(`<cx:dataPt xmlns:cx="%s" xmlns:a="%s" idx="%d"><cx:spPr><a:solidFill><a:srgbClr val="%s"/></a:solidFill></cx:spPr></cx:dataPt>`, NSCX, NSA, i, color))
	}
	if e := p.SetXML(parts[0], d); e != nil {
		t.Fatal(e)
	}
	return referenceWriteMutation(t, p)
}
func referenceHierarchyPreview(t *testing.T, src string) ([]*etree.Element, []string) {
	t.Helper()
	out := referenceNextPresentation(t)
	if _, e := OfficePreviewCopy(src, out); e != nil {
		t.Fatal(e)
	}
	return referencePreviewShapes(t, out)
}
func referenceShapeText(t *testing.T, s *etree.Element) string {
	t.Helper()
	if !Is(s, NSP, "sp") {
		return ""
	}
	// Reuse direct paragraph/run/field/break acquisition, not descendant text.
	slide := etree.NewElement("p:sld")
	slide.CreateAttr("xmlns:p", NSP)
	slide.CreateAttr("xmlns:a", NSA)
	tree := slide.CreateElement("p:cSld").CreateElement("p:spTree")
	tree.AddChild(s.Copy())
	return referenceHistogramShapeTexts(t, slide)[0]
}
func TestReferenceHierarchyTitleLegendStyles(t *testing.T) {
	referencePresentationRoot(t)
	for _, kind := range []string{"treemap", "sunburst"} {
		for _, position := range []string{"b", "t", "l", "r"} {
			t.Run(kind+"/"+position, func(t *testing.T) {
				src := referenceStyledHierarchy(t, kind, position, "")
				before := previewRead(t, src)
				shapes, labels := referenceHierarchyPreview(t, src)
				var title *etree.Element
				for _, s := range shapes {
					if referenceShapeText(t, s) == "Quarterly regional sales" {
						title = s
						break
					}
				}
				if title == nil {
					t.Fatal("missing title")
				}
				props := Children(Child(Child(Child(title, NSP, "txBody"), NSA, "p"), NSA, "pPr"), NSA, "defRPr")
				if len(props) == 0 {
					t.Fatal("missing title font")
				}
				if props[0].SelectAttrValue("sz", "") != "1600" {
					t.Error("title size", props[0].Attr)
				}
				colors := Children(Child(props[0], NSA, "solidFill"), NSA, "srgbClr")
				if len(colors) == 0 || colors[0].SelectAttrValue("val", "") != "123456" {
					t.Error("title color")
				}
				if props[0].SelectAttrValue("b", "") != "1" {
					t.Error("title bold", props[0].Attr)
				}
				want := []string{"A", "B", "C", "North", "South"}
				if kind == "sunburst" {
					want = []string{"A", "B", "C", "Values"}
				}
				for _, label := range want {
					if !slices.Contains(labels, label) {
						t.Error("missing label", label, labels)
					}
				}
				for _, color := range []string{"C01020", "20A030", "3040D0"} {
					count := 0
					for _, s := range shapes {
						for _, c := range All(s, NSA, "srgbClr") {
							if c.SelectAttrValue("val", "") == color {
								count++
							}
						}
					}
					if count < 1 {
						t.Error("missing color", color)
					}
				}
				slide := previewXML(t, previewOpen(t, src), "ppt/slides/slide1.xml")
				frame := All(slide, NSP, "graphicFrame")[0]
				xf := Child(frame, NSP, "xfrm")
				metric := func(element, attr string) float64 {
					n := Child(xf, NSA, element)
					v, e := strconv.ParseFloat(n.SelectAttrValue(attr, ""), 64)
					if e != nil {
						t.Fatal(e)
					}
					return v
				}
				x, y, w, h := metric("off", "x"), metric("off", "y"), metric("ext", "cx"), metric("ext", "cy")
				for _, s := range shapes {
					sx, sy, sw, sh := referenceShapeMetric(t, s, "off", "x"), referenceShapeMetric(t, s, "off", "y"), referenceShapeMetric(t, s, "ext", "cx"), referenceShapeMetric(t, s, "ext", "cy")
					if sx < x || sy < y || sx+sw > x+w+1 || sy+sh > y+h+1 {
						t.Error("shape outside chart", referenceShapeName(t, s), sx, sy, sw, sh)
					}
				}
				previewUnchanged(t, src, before)
			})
		}
	}
}
func TestReferenceHierarchyHiddenLegend(t *testing.T) {
	referencePresentationRoot(t)
	_, labels := referenceHierarchyPreview(t, referenceStyledHierarchy(t, "treemap", "", ""))
	count := 0
	for _, s := range labels {
		if s == "Quarterly regional sales" {
			count++
		}
	}
	if count != 1 {
		t.Error("title count", count)
	}
	for _, label := range []string{"A", "B", "C"} {
		count := 0
		for _, s := range labels {
			if s == label || strings.HasPrefix(s, label+" ") {
				count++
			}
		}
		if count != 1 {
			t.Error(label, count, labels)
		}
	}
}
func TestReferenceTreemapParentLabelModes(t *testing.T) {
	referencePresentationRoot(t)
	tops := map[string]float64{}
	for _, mode := range []string{"none", "banner", "overlapping"} {
		shapes, labels := referenceHierarchyPreview(t, referenceStyledHierarchy(t, "treemap", "b", mode))
		count := 0
		for _, label := range labels {
			if label == "North" {
				count++
			}
		}
		want := 2
		if mode == "none" {
			want = 1
		}
		if count != want {
			t.Error(mode, count, want)
		}
		for _, s := range shapes {
			if referenceShapeText(t, s) == "B" {
				tops[mode] = referenceShapeMetric(t, s, "off", "y")
				break
			}
		}
		if _, ok := tops[mode]; !ok {
			t.Fatal("missing B", mode)
		}
	}
	for mode, want := range map[string]float64{"banner": 22, "overlapping": 44} {
		if got := (tops[mode] - tops["none"]) / 9525; math.Abs(got-want) > .01 {
			t.Error(mode, got, want)
		}
	}
}
func TestReferenceSunburstHoleAndLeafFirstGroups(t *testing.T) {
	referencePresentationRoot(t)
	shapes, labels := referenceHierarchyPreview(t, referenceStyledHierarchy(t, "sunburst", "b", ""))
	for _, label := range []string{"North", "South", "A", "B", "C", "30", "5", "10", "20"} {
		if !slices.Contains(labels, label) {
			t.Error("missing label", label, labels)
		}
	}
	var vertices [][2]float64
	number := func(n *etree.Element, key string) float64 {
		v, e := strconv.ParseFloat(n.SelectAttrValue(key, ""), 64)
		if e != nil {
			t.Fatal(e)
		}
		return v
	}
	for _, s := range shapes {
		paths := All(s, NSA, "path")
		if len(paths) == 0 {
			continue
		}
		pts := All(paths[0], NSA, "pt")
		if len(pts) <= 10 {
			continue
		}
		w, h := number(paths[0], "w"), number(paths[0], "h")
		x, y, sw, sh := referenceShapeMetric(t, s, "off", "x"), referenceShapeMetric(t, s, "off", "y"), referenceShapeMetric(t, s, "ext", "cx"), referenceShapeMetric(t, s, "ext", "cy")
		for _, pt := range pts {
			vertices = append(vertices, [2]float64{x + number(pt, "x")*sw/w, y + number(pt, "y")*sh/h})
		}
	}
	if len(vertices) == 0 {
		t.Fatal("no saved vector vertices")
	}
	xmin, xmax, ymin, ymax := math.Inf(1), math.Inf(-1), math.Inf(1), math.Inf(-1)
	for _, v := range vertices {
		xmin = math.Min(xmin, v[0])
		xmax = math.Max(xmax, v[0])
		ymin = math.Min(ymin, v[1])
		ymax = math.Max(ymax, v[1])
	}
	cx, cy := (xmin+xmax)/2, (ymin+ymax)/2
	rmin, rmax := math.Inf(1), 0.0
	for _, v := range vertices {
		r := math.Hypot(v[0]-cx, v[1]-cy)
		rmin = math.Min(rmin, r)
		rmax = math.Max(rmax, r)
	}
	if math.Abs(rmin/rmax-1.0/3) > .002 {
		t.Error("hole radius ratio", rmin/rmax)
	}
	count := 0
	for _, label := range labels {
		if label == "Values" {
			count++
		}
	}
	if count != 1 {
		t.Error("series legend count", count)
	}
}

// Exercise the saved-input helper only: no preview or renderer is invoked.
func TestReferenceStyledHierarchyLegendOverlay(t *testing.T) {
	referencePresentationRoot(t)
	for _, position := range []string{"", "b", "t", "l", "r"} {
		for _, overlay := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/%t", position, overlay), func(t *testing.T) {
				p := previewOpen(t, referenceStyledHierarchy(t, "treemap", position, "banner", overlay))
				parts, err := ChartParts(p)
				if err != nil || len(parts) != 1 {
					t.Fatalf("chart parts: %v %v", parts, err)
				}
				d, err := p.XML(parts[0])
				if err != nil {
					t.Fatal(err)
				}
				legend := Child(Child(d.Root(), NSCX, "chart"), NSCX, "legend")
				if position == "" {
					if legend != nil {
						t.Fatal("unexpected legend")
					}
					return
				}
				want := "0"
				if overlay {
					want = "1"
				}
				if legend == nil || AttrV(legend, "", "overlay") != want || AttrV(legend, "", "pos") != position {
					t.Fatal("legend options not preserved")
				}
			})
		}
	}
}
