package pptx

import (
	"fmt"
	"math"
	"strconv"

	"github.com/beevik/etree"
	"github.com/openvibely/openvibely-paperworks/internal/office"
)

func setPictureMask(shape *etree.Element, mask string, radius *float64) error {
	if !office.Is(shape, pns, "pic") {
		return fmt.Errorf("image-mask requires a picture")
	}
	pr := office.Child(shape, pns, "spPr")
	if pr == nil {
		return fmt.Errorf("picture properties missing")
	}
	old := office.Child(pr, ans, "prstGeom")
	if mask == "" && radius != nil && old != nil {
		mask = old.SelectAttrValue("prst", "")
	}
	if !validPictureMask(mask) {
		return fmt.Errorf("mask must be a DrawingML preset shape name")
	}
	adj := -1
	if radius != nil {
		if mask != "rect" && mask != "roundRect" {
			return fmt.Errorf("corner radius requires rect or roundRect")
		}
		ext := office.Child(office.Child(pr, ans, "xfrm"), ans, "ext")
		if ext == nil {
			return fmt.Errorf("corner radius requires explicit picture dimensions")
		}
		w, e1 := strconv.ParseFloat(ext.SelectAttrValue("cx", ""), 64)
		h, e2 := strconv.ParseFloat(ext.SelectAttrValue("cy", ""), 64)
		side := math.Min(w, h) / 12700
		if e1 != nil || e2 != nil || !finite(side) || side <= 0 {
			return fmt.Errorf("invalid picture dimensions")
		}
		if !finite(*radius) || *radius < 0 || *radius > side/2 {
			return fmt.Errorf("corner radius must be 0..half the shorter picture side in points")
		}
		adj = int(math.Round(*radius / side * 100000))
		mask = "roundRect"
	}
	geom := etree.NewElement("a:prstGeom")
	geom.CreateAttr("xmlns:a", ans)
	geom.CreateAttr("prst", mask)
	if old != nil && old.SelectAttrValue("prst", "") == mask {
		if av := office.Child(old, ans, "avLst"); av != nil {
			geom.AddChild(av.Copy())
		}
	}
	av := office.Child(geom, ans, "avLst")
	if av == nil {
		av = geom.CreateElement("a:avLst")
	}
	if adj >= 0 {
		for _, g := range av.ChildElements() {
			if g.Tag == "gd" && g.SelectAttrValue("name", "") == "adj" {
				av.RemoveChild(g)
			}
		}
		g := av.CreateElement("a:gd")
		g.CreateAttr("name", "adj")
		g.CreateAttr("fmla", fmt.Sprintf("val %d", adj))
	}
	for _, tag := range []string{"prstGeom", "custGeom"} {
		if n := office.Child(pr, ans, tag); n != nil {
			pr.RemoveChild(n)
		}
	}
	insertDrawingProperty(pr, geom)
	return nil
}
