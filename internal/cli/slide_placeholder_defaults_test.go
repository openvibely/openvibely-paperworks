package cli

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"github.com/openvibely/openvibely-paperworks/internal/pptx"
)

func TestSlideLayoutPlaceholderDefaultType(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "input.pptx")
	if err := pptx.Create(context.Background(), pptx.Spec{Slides: []pptx.Slide{{Elements: []pptx.Element{{Type: "text", Text: "Sample", X: 1, Y: 1, W: 3, H: 1}}}}}, dir, input); err != nil {
		t.Fatal(err)
	}
	pkg, err := opc.Open(input)
	if err != nil {
		t.Fatal(err)
	}
	slide, err := pkg.XML("ppt/slides/slide1.xml")
	if err != nil {
		t.Fatal(err)
	}
	shape := office.All(slide.Root(), office.NSP, "sp")[0]
	nv := office.Child(office.Child(shape, office.NSP, "nvSpPr"), office.NSP, "nvPr")
	ph := nv.CreateElement("p:ph")
	ph.CreateAttr("idx", "1")
	// Remove direct font defaults so the master style determines the reported size.
	for _, tag := range []string{"rPr", "pPr", "lstStyle"} {
		for _, el := range office.All(shape, office.NSA, tag) {
			el.Parent().RemoveChild(el)
		}
	}
	master, err := pkg.XML("ppt/slideMasters/slideMaster1.xml")
	if err != nil {
		t.Fatal(err)
	}
	if styles := office.Child(master.Root(), office.NSP, "txStyles"); styles != nil {
		master.Root().RemoveChild(styles)
	}
	styles := master.Root().CreateElement("p:txStyles")
	for name, size := range map[string]string{"bodyStyle": "3200", "otherStyle": "1800", "titleStyle": "4000"} {
		style := styles.CreateElement("p:" + name)
		level := style.CreateElement("a:lvl1pPr")
		level.CreateElement("a:defRPr").CreateAttr("sz", size)
	}
	if err = pkg.SetXML("ppt/slideMasters/slideMaster1.xml", master); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, typ string
		want      float64
	}{{"omitted", "", 24}, {"explicit-object", "obj", 24}, {"body", "body", 32.0 / 72 * 96}, {"title", "title", 40.0 / 72 * 96}} {
		t.Run(tc.name, func(t *testing.T) {
			ph.RemoveAttr("type")
			if tc.typ != "" {
				ph.CreateAttr("type", tc.typ)
			}
			if err := pkg.SetXML("ppt/slides/slide1.xml", slide); err != nil {
				t.Fatal(err)
			}
			reports, err := office.PresentationLayouts(pkg)
			if err != nil {
				t.Fatal(err)
			}
			for _, element := range reports[0]["elements"].([]map[string]any) {
				if element["scope"] != "slide" {
					continue
				}
				runs := element["paragraphs"].([]map[string]any)[0]["runs"].([]map[string]any)
				got := runs[0]["style"].(map[string]any)["font_size_px"].(float64)
				if got-tc.want > 1e-9 || tc.want-got > 1e-9 {
					t.Fatalf("font size %v, want %v", got, tc.want)
				}
				return
			}
			t.Fatal("missing slide text")
		})
	}
}
