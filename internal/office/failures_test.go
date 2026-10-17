package office

import (
	"bytes"
	"errors"
	"github.com/openvibely/openvibely-paperworks/internal/testsupport"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/beevik/etree"

	"github.com/openvibely/openvibely-paperworks/internal/opc"
)

func TestReferenceHeadingMetricsAndSubtitleCollision(t *testing.T) {
	// Mirror the reference's presentation_font_file mock with the identical font.
	fonts := loadFonts("../../testdata/reference-mirror/Vera.ttf")
	if len(fonts) != 1 {
		t.Fatal("original reference font unavailable")
	}
	key := faceKey{"Bitstream Vera Sans", false, false}
	faceMu.Lock()
	previous, exists := faceCache[key]
	faceCache[key] = fonts[0]
	faceMu.Unlock()
	t.Cleanup(func() {
		faceMu.Lock()
		defer faceMu.Unlock()
		if exists {
			faceCache[key] = previous
		} else {
			delete(faceCache, key)
		}
	})
	src := "../../testdata/reference-mirror/wrapped-heading.pptx"
	original, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	p, err := opc.Open(src)
	if err != nil {
		t.Fatal(err)
	}
	verify := func(want string) {
		t.Helper()
		report, e := PackageReportOf(p)
		if e != nil {
			t.Fatal(e)
		}
		checked, e := VerifyPresentation(p, report, SlideRequirements{})
		if want == "" {
			if e != nil || checked == nil {
				t.Fatal(e)
			}
		} else if e == nil || !strings.Contains(e.Error(), want) {
			t.Fatal(want, e)
		}
	}
	verify("heading")
	doc, err := p.XML("ppt/slides/slide1.xml")
	if err != nil {
		t.Fatal(err)
	}
	sp := All(doc.Root(), NSP, "sp")[0]
	Child(Child(Child(sp, NSP, "spPr"), NSA, "xfrm"), NSA, "ext").CreateAttr("cy", "3657600")
	// Exact default add_textbox shape from pinned reference presentation library 1.0.2: no
	// inherited title font, paragraph properties or noAutofit setting.
	subdoc := etree.NewDocument()
	if err := subdoc.ReadFromString(`<p:sp xmlns:p="` + NSP + `" xmlns:a="` + NSA + `"><p:nvSpPr><p:cNvPr id="3" name="TextBox 2"/><p:cNvSpPr txBox="1"/><p:nvPr/></p:nvSpPr><p:spPr><a:xfrm><a:off x="914400" y="1371600"/><a:ext cx="1828800" cy="365760"/></a:xfrm><a:prstGeom prst="rect"><a:avLst/></a:prstGeom><a:noFill/></p:spPr><p:txBody><a:bodyPr wrap="none"><a:spAutoFit/></a:bodyPr><a:lstStyle/><a:p><a:r><a:t>Subtitle</a:t></a:r></a:p></p:txBody></p:sp>`); err != nil {
		t.Fatal(err)
	}
	subtitle := subdoc.Root().Copy()
	sp.Parent().AddChild(subtitle)
	xfrm := Child(Child(subtitle, NSP, "spPr"), NSA, "xfrm")
	if err = p.SetXML("ppt/slides/slide1.xml", doc); err != nil {
		t.Fatal(err)
	}
	verify("overlaps")
	Child(xfrm, NSA, "off").CreateAttr("y", "5029200")
	if err = p.SetXML("ppt/slides/slide1.xml", doc); err != nil {
		t.Fatal(err)
	}
	verify("")
	after, _ := os.ReadFile(src)
	if !bytes.Equal(original, after) {
		t.Fatal("source changed")
	}
}

func TestReferenceLiteralChartMaterialization(t *testing.T) {
	src := testsupport.LiteralChartFixture(t)
	original, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	p, err := opc.Open(src)
	if err != nil {
		t.Fatal(err)
	}
	report, err := PackageReportOf(p)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = VerifyPresentation(p, report, SlideRequirements{}); err == nil || !strings.Contains(err.Error(), "materialization") {
		t.Fatal(err)
	}
	target := "powerpoint"
	result, err := VerifyPresentation(p, report, SlideRequirements{TargetApplication: &target})
	if err != nil || len(result.ChartData) == 0 {
		t.Fatal(result, err)
	}
	if result.ChartData[0].(map[string]any)["workbook_verified"] != false {
		t.Fatal(result.ChartData)
	}
	if _, err = MaterializeLiteralCharts(p); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "materialized.pptx")
	if err = p.SaveNew(out); err != nil {
		t.Fatal(err)
	}
	saved, err := opc.Open(out)
	if err != nil {
		t.Fatal(err)
	}
	report, err = PackageReportOf(saved)
	if err != nil {
		t.Fatal(err)
	}
	result, err = VerifyPresentation(saved, report, SlideRequirements{})
	if err != nil {
		t.Fatal(err)
	}
	chart := result.ChartData[0].(map[string]any)
	if chart["references"].(int) <= 0 || chart["literal_series"] != 0 {
		t.Fatal(chart)
	}
	after, readErr := os.ReadFile(src)
	if readErr != nil || !bytes.Equal(original, after) {
		t.Fatal("source changed", readErr)
	}
	if err = p.SaveNew(out); !errors.Is(err, os.ErrExist) {
		t.Fatal(err)
	}
	// Independently follow the first slide's first graphic-frame chart.
	z := referencePreviewZIP(t, out)
	main := referencePackageTarget(t, z, "", "", NSR+"/officeDocument")
	ids := Children(Child(referencePackageXML(t, z, main), NSP, "sldIdLst"), NSP, "sldId")
	if len(ids) == 0 {
		t.Fatal("missing slide")
	}
	slide := referencePackageTarget(t, z, main, AttrV(ids[0], NSR, "id"), NSR+"/slide")
	tree := Child(Child(referencePackageXML(t, z, slide), NSP, "cSld"), NSP, "spTree")
	var first *etree.Element
	for _, e := range tree.ChildElements() {
		if e.NamespaceURI() != NSP {
			continue
		}
		switch e.Tag {
		case "sp", "grpSp", "graphicFrame", "cxnSp", "pic", "contentPart":
			first = e
		}
		if first != nil {
			break
		}
	}
	data := Child(Child(first, NSA, "graphic"), NSA, "graphicData")
	if data == nil || data.SelectAttrValue("uri", "") != NSC {
		t.Fatal("first shape is not chart")
	}
	part := referencePackageTarget(t, z, slide, AttrV(Child(data, NSC, "chart"), NSR, "id"), NSR+"/chart")
	series := All(referencePackageXML(t, z, part), NSC, "ser")
	if len(series) == 0 {
		t.Fatal("missing series")
	}
	value := Child(series[0], NSC, "val")
	counts := All(value, NSC, "ptCount")
	if len(counts) == 0 || counts[0].SelectAttrValue("val", "") != "2" {
		t.Fatal("expected two points")
	}
	for idx, want := range []float64{2, 3} {
		var point *etree.Element
		for _, candidate := range All(value, NSC, "pt") {
			if candidate.SelectAttrValue("idx", "") == strconv.Itoa(idx) {
				point = candidate
				break
			}
		}
		v := Child(point, NSC, "v")
		if v == nil {
			t.Fatal("missing indexed value", idx)
		}
		got, e := strconv.ParseFloat(v.Text(), 64)
		if e != nil || got != want {
			t.Fatal(idx, got, e)
		}
	}
}
