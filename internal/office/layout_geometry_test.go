package office

import (
	"math"
	"testing"
)

func TestLayoutGroupCoordinates(t *testing.T) {
	root := paintFixture(t, `<p:grpSp xmlns:p="`+NSP+`" xmlns:a="`+NSA+`"><p:grpSpPr><a:xfrm flipH="1"><a:off x="952500" y="1905000"/><a:ext cx="1905000" cy="952500"/><a:chOff x="0" y="0"/><a:chExt cx="952500" cy="952500"/></a:xfrm></p:grpSpPr><p:sp><p:spPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="476250" cy="476250"/></a:xfrm></p:spPr></p:sp></p:grpSp>`)
	r := map[string]any{}
	layoutGroupGeometry(r, Child(root, NSP, "sp"))
	p := r["position"].(map[string]float64)
	if math.Abs(p["left"]-200) > 1e-9 || p["top"] != 200 || p["width"] != 100 || p["height"] != 50 {
		t.Fatal(r)
	}
	if r["local_position"].(map[string]float64)["left"] != 0 {
		t.Fatal(r)
	}
	group := map[string]any{}
	layoutGroupGeometry(group, root)
	if group["position"].(map[string]float64)["left"] != 100 {
		t.Fatal(group)
	}
	Child(Child(root, NSP, "grpSpPr"), NSA, "xfrm").CreateAttr("rot", "5400000")
	rotated := map[string]any{}
	layoutGroupGeometry(rotated, Child(root, NSP, "sp"))
	bounds := rotated["position"].(map[string]float64)
	if math.Abs(bounds["left"]-200) > 1e-8 || math.Abs(bounds["top"]-250) > 1e-8 || math.Abs(bounds["width"]-50) > 1e-8 || math.Abs(bounds["height"]-100) > 1e-8 {
		t.Fatal(rotated)
	}

}
