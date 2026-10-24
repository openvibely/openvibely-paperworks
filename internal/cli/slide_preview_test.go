package cli

import (
	"context"
	"encoding/json"
	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"github.com/openvibely/openvibely-paperworks/internal/pptx"
	"image"
	"os"
	"path/filepath"
	"testing"
)

func TestPairedSlidePreview(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "in.pptx")
	if err := pptx.Create(context.Background(), pptx.Spec{Slides: []pptx.Slide{{Elements: []pptx.Element{{Type: "text", Text: "Revenue", X: 1, Y: 2, W: 3, H: 1, FontSize: 24}}}}}, dir, file); err != nil {
		t.Fatal(err)
	}
	p, err := opc.Open(file)
	if err != nil {
		t.Fatal(err)
	}
	reports, err := office.PresentationLayouts(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(reports) != 1 {
		t.Fatal("slide count")
	}
	elements := reports[0]["elements"].([]map[string]any)
	found := false
	for _, e := range elements {
		if e["text"] == "Revenue" {
			found = true
			if e["editable"] != true || e["shape_id"] != "2" {
				t.Fatal(e)
			}
			pos := e["position"].(map[string]float64)
			if pos["left"] != 96 || pos["top"] != 192 {
				t.Fatal(pos)
			}
			paras := e["paragraphs"].([]map[string]any)
			runs := paras[0]["runs"].([]map[string]any)
			if runs[0]["style"].(map[string]any)["font_size_px"] != float64(32) {
				t.Fatal(runs)
			}
		} else if e["scope"] != "slide" && e["editable"] != false {
			t.Fatal("inherited editable")
		}
	}
	if !found {
		t.Fatal("missing native text")
	}
	stage := filepath.Join(dir, "stage")
	os.Mkdir(stage, 0755)
	pngPath := filepath.Join(stage, "page.png")
	if err = writePreview(pngPath, image.NewRGBA(image.Rect(0, 0, 1280, 720)), "png"); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "out")
	if _, err = publishSlidePreview(context.Background(), stage, out, []string{pngPath}, reports); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(out, "slide-1.layout.json"))
	if err != nil {
		t.Fatal(err)
	}
	var saved map[string]any
	if err = json.Unmarshal(data, &saved); err != nil {
		t.Fatal(err)
	}
	if saved["preview"].(map[string]any)["file"] != "slide-1.png" {
		t.Fatal(saved)
	}
	if _, err = os.Stat(filepath.Join(out, "slide-1.png")); err != nil {
		t.Fatal(err)
	}
	if _, err = publishSlidePreview(context.Background(), stage, filepath.Join(dir, "bad"), nil, reports); err == nil {
		t.Fatal("accepted mismatched counts")
	}
	if _, err = os.Stat(filepath.Join(dir, "bad")); !os.IsNotExist(err) {
		t.Fatal("published failed pairs")
	}
}

func TestSlideLayoutInheritedGeometryAndStyle(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "in.pptx")
	if err := pptx.Create(context.Background(), pptx.Spec{Slides: []pptx.Slide{{Elements: []pptx.Element{{Type: "text", Text: "Inherited", X: 1, Y: 2, W: 3, H: 1, FontSize: 24}}}}}, dir, file); err != nil {
		t.Fatal(err)
	}
	pkg, err := opc.Open(file)
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
	ph.CreateAttr("type", "body")
	ph.CreateAttr("idx", "1")
	inherited := shape.Copy()
	inherited.CreateAttr("xmlns:p", office.NSP)
	inherited.CreateAttr("xmlns:a", office.NSA)
	layout, err := pkg.XML("ppt/slideLayouts/slideLayout1.xml")
	if err != nil {
		t.Fatal(err)
	}
	office.Child(office.Child(layout.Root(), office.NSP, "cSld"), office.NSP, "spTree").AddChild(inherited)
	// Put defaults on the layout and remove all local text styling and geometry.
	bp := office.Child(inherited, office.NSP, "txBody")
	lst := office.Child(bp, office.NSA, "lstStyle")
	level := lst.CreateElement("a:lvl1pPr")
	rpr := level.CreateElement("a:defRPr")
	rpr.CreateAttr("sz", "3000")
	localBody := office.Child(shape, office.NSP, "txBody")
	for _, p := range office.Children(localBody, office.NSA, "p") {
		if pp := office.Child(p, office.NSA, "pPr"); pp != nil {
			p.RemoveChild(pp)
		}
		for _, r := range office.Children(p, office.NSA, "r") {
			if rp := office.Child(r, office.NSA, "rPr"); rp != nil {
				r.RemoveChild(rp)
			}
		}
	}
	sp := office.Child(shape, office.NSP, "spPr")
	sp.RemoveChild(office.Child(sp, office.NSA, "xfrm"))
	if err = pkg.SetXML("ppt/slides/slide1.xml", slide); err != nil {
		t.Fatal(err)
	}
	if err = pkg.SetXML("ppt/slideLayouts/slideLayout1.xml", layout); err != nil {
		t.Fatal(err)
	}
	reports, err := office.PresentationLayouts(pkg)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range reports[0]["elements"].([]map[string]any) {
		if e["scope"] != "slide" {
			continue
		}
		if e["position"].(map[string]float64)["left"] != 96 {
			t.Fatal(e)
		}
		runs := e["paragraphs"].([]map[string]any)[0]["runs"].([]map[string]any)
		if runs[0]["style"].(map[string]any)["font_size_px"] != float64(40) {
			t.Fatal(runs)
		}
	}
}
