package office

import (
	"github.com/beevik/etree"
	"math"
)

// Reports both local edit geometry and the transformed slide-space quadrilateral.
// A bounding box alone cannot represent a rotated or skewed grouped object.
func layoutGroupGeometry(record map[string]any, shape *etree.Element) {
	xf := Child(Child(shape, NSP, "spPr"), NSA, "xfrm")
	if shape.Tag == "grpSp" {
		xf = Child(Child(shape, NSP, "grpSpPr"), NSA, "xfrm")
	}
	if xf == nil {
		xf = Child(shape, NSP, "xfrm")
	}
	if xf == nil {
		record["position_unresolved"] = "missing object transform"
		return
	}
	off, ext := Child(xf, NSA, "off"), Child(xf, NSA, "ext")
	if off == nil || ext == nil {
		record["position_unresolved"] = "incomplete object transform"
		return
	}
	l, t, w, h := emuAttr(off, "x", 0), emuAttr(off, "y", 0), emuAttr(ext, "cx", 0), emuAttr(ext, "cy", 0)
	points := rotateShape([]pt{{l, t}, {l + w, t}, {l + w, t + h}, {l, t + h}}, xf, l, t, w, h)
	for a := shape.Parent(); a != nil; a = a.Parent() {
		if !Is(a, NSP, "grpSp") {
			continue
		}
		tr := Child(Child(a, NSP, "grpSpPr"), NSA, "xfrm")
		o, e, co, ce := Child(tr, NSA, "off"), Child(tr, NSA, "ext"), Child(tr, NSA, "chOff"), Child(tr, NSA, "chExt")
		if o == nil || e == nil || co == nil || ce == nil || emuAttr(ce, "cx", 0) <= 0 || emuAttr(ce, "cy", 0) <= 0 {
			record["position_unresolved"] = "incomplete or zero-sized ancestor group transform"
			return
		}
		gx, gy, gw, gh := emuAttr(o, "x", 0), emuAttr(o, "y", 0), emuAttr(e, "cx", 0), emuAttr(e, "cy", 0)
		for i, c := range points {
			points[i] = pt{gx + (c.x-emuAttr(co, "x", 0))*gw/emuAttr(ce, "cx", 0), gy + (c.y-emuAttr(co, "y", 0))*gh/emuAttr(ce, "cy", 0)}
		}
		points = rotateShape(points, tr, gx, gy, gw, gh)
	}
	minX, minY, maxX, maxY := math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)
	corners := [][2]float64{}
	for _, p := range points {
		minX = math.Min(minX, p.x)
		minY = math.Min(minY, p.y)
		maxX = math.Max(maxX, p.x)
		maxY = math.Max(maxY, p.y)
		corners = append(corners, [2]float64{p.x / 9525, p.y / 9525})
	}
	record["local_position"] = map[string]float64{"left": l / 9525, "top": t / 9525, "width": w / 9525, "height": h / 9525}
	record["position"] = map[string]float64{"left": minX / 9525, "top": minY / 9525, "width": (maxX - minX) / 9525, "height": (maxY - minY) / 9525}
	record["corners"] = corners
	record["position_space"] = "slide bounds; edit coordinates remain local to the parent group"
}
