package office

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/beevik/etree"
)

// reference presentation library 1.0.2 SlideShapes iteration does not descend into groups.
// GraphicFrame.has_chart compares graphicData.uri, not chart descendants.
func referenceHistogramHasChart(t *testing.T, slide *etree.Element) bool {
	t.Helper()
	tree := Child(Child(slide, NSP, "cSld"), NSP, "spTree")
	if tree == nil {
		t.Fatal("missing slide shape tree")
	}
	for _, frame := range Children(tree, NSP, "graphicFrame") {
		data := Child(Child(frame, NSA, "graphic"), NSA, "graphicData")
		if data == nil || data.SelectAttr("uri") == nil {
			t.Fatal("graphic frame missing required graphicData URI")
		}
		if data.SelectAttrValue("uri", "") == NSC {
			return true
		}
	}
	return false
}

func TestReferenceHistogramChartDetection(t *testing.T) {
	frame := func(uri, content string) string {
		return `<p:graphicFrame><a:graphic><a:graphicData uri="` + uri + `">` + content + `</a:graphicData></a:graphic></p:graphicFrame>`
	}
	for _, c := range []struct {
		name, shapes string
		want         bool
	}{
		{"empty", "", false},
		{"chart-uri-without-chart", frame(NSC, ""), true},
		{"ordinary-chart", frame(NSC, `<c:chart/>`), true},
		{"chart-element-wrong-uri", frame("urn:other", `<c:chart/>`), false},
		{"extended-chart", frame(NSCX, `<cx:chart/>`), false},
		{"grouped-chart", `<p:grpSp>` + frame(NSC, `<c:chart/>`) + `</p:grpSp>`, false},
		{"shape-descendant-chart", `<p:sp><c:chart/></p:sp>`, false},
		{"later-top-level-chart", frame("urn:other", "") + frame(NSC, ""), true},
	} {
		t.Run(c.name, func(t *testing.T) {
			doc := etree.NewDocument()
			raw := fmt.Sprintf(`<p:sld xmlns:p="%s" xmlns:a="%s" xmlns:c="%s" xmlns:cx="%s"><p:cSld><p:spTree>%s</p:spTree></p:cSld></p:sld>`, NSP, NSA, NSC, NSCX, c.shapes)
			if err := doc.ReadFromString(raw); err != nil {
				t.Fatal(err)
			}
			if got := referenceHistogramHasChart(t, doc.Root()); got != c.want {
				t.Fatalf("has_chart=%v, want %v", got, c.want)
			}
		})
	}
}

// reference presentation library 1.0.2 BaseShape.name reads _element.shape_name, whose
// _nvXxPr is the first child element, not a tag-specific search. The shape
// factory leaves contentPart as a generic XML element without shape_name.
func referenceHistogramShapeName(shape *etree.Element) (name string, supported bool, err error) {
	if shape.NamespaceURI() != NSP {
		return "", false, nil
	}
	switch shape.Tag {
	case "sp", "grpSp", "graphicFrame", "cxnSp", "pic":
	case "contentPart":
		return "", true, fmt.Errorf("contentPart has no shape_name property")
	default:
		return "", false, nil
	}
	children := shape.ChildElements()
	if len(children) == 0 {
		return "", true, fmt.Errorf("shape has no first child")
	}
	first := children[0]
	if first.NamespaceURI() != NSP {
		return "", true, fmt.Errorf("first child has no cNvPr property")
	}
	switch first.Tag {
	case "nvSpPr", "nvGrpSpPr", "nvGraphicFramePr", "nvCxnSpPr", "nvPicPr":
	default:
		return "", true, fmt.Errorf("first child has no cNvPr property")
	}
	properties := Child(first, NSP, "cNvPr")
	if properties == nil || properties.SelectAttr("name") == nil {
		return "", true, fmt.Errorf("shape name missing")
	}
	return properties.SelectAttrValue("name", ""), true, nil
}

func TestReferenceHistogramShapeNameAcquisition(t *testing.T) {
	for _, c := range []struct {
		name, xml string
		names     []string
		wantError bool
	}{
		{"sp", `<p:sp><p:nvSpPr><p:cNvPr id="1" name="Histogram bin 1"/></p:nvSpPr></p:sp>`, []string{"Histogram bin 1"}, false},
		{"grpSp", `<p:grpSp><p:nvGrpSpPr><p:cNvPr id="1" name="Histogram bin 1"/></p:nvGrpSpPr></p:grpSp>`, []string{"Histogram bin 1"}, false},
		{"graphicFrame", `<p:graphicFrame><p:nvGraphicFramePr><p:cNvPr id="1" name="Histogram bin 1"/></p:nvGraphicFramePr></p:graphicFrame>`, []string{"Histogram bin 1"}, false},
		{"cxnSp", `<p:cxnSp><p:nvCxnSpPr><p:cNvPr id="1" name="Histogram bin 1"/></p:nvCxnSpPr></p:cxnSp>`, []string{"Histogram bin 1"}, false},
		{"pic", `<p:pic><p:nvPicPr><p:cNvPr id="1" name="Histogram bin 1"/></p:nvPicPr></p:pic>`, []string{"Histogram bin 1"}, false},
		{"empty-name", `<p:sp><p:nvSpPr><p:cNvPr id="1" name=""/></p:nvSpPr></p:sp>`, []string{""}, false},
		{"missing-name", `<p:sp><p:nvSpPr><p:cNvPr id="1"/></p:nvSpPr></p:sp>`, nil, true},
		{"wrong-first-child", `<p:sp><p:spPr/><p:nvSpPr><p:cNvPr id="1" name="late"/></p:nvSpPr></p:sp>`, nil, true},
		{"different-nonvisual-type", `<p:sp><p:nvPicPr><p:cNvPr id="1" name="first"/></p:nvPicPr></p:sp>`, []string{"first"}, false},
		{"content-part", `<p:contentPart><p:nvSpPr><p:cNvPr id="1" name="ignored"/></p:nvSpPr></p:contentPart>`, nil, true},
		{"unknown-shape", `<p:unknown/>`, nil, false},
		{"missing-child", `<p:sp/>`, nil, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			d := etree.NewDocument()
			if err := d.ReadFromString(`<p:spTree xmlns:p="` + NSP + `" xmlns:a="` + NSA + `">` + c.xml + `</p:spTree>`); err != nil {
				t.Fatal(err)
			}
			var names []string
			var readErr error
			for _, shape := range d.Root().ChildElements() {
				name, supported, err := referenceHistogramShapeName(shape)
				if err != nil {
					readErr = err
					break
				}
				if supported {
					names = append(names, name)
				}
			}
			if (readErr != nil) != c.wantError {
				t.Fatalf("error=%v, want error %v", readErr, c.wantError)
			}
			if !c.wantError && !reflect.DeepEqual(names, c.names) {
				t.Fatalf("names=%q, want %q", names, c.names)
			}
		})
	}
}
