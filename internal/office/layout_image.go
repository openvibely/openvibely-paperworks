package office

import "github.com/beevik/etree"

// Crop values are fractions of the source image, including negative outsets.
// Fit describes the native DrawingML mode, not an inferred CSS object-fit.
func layoutImage(record map[string]any, shape, fill *etree.Element) {
	rect := func(e *etree.Element) map[string]float64 {
		r := map[string]float64{}
		for _, side := range []string{"l", "t", "r", "b"} {
			r[side] = emuAttr(e, side, 0) / 100000
		}
		return r
	}
	record["crop"] = rect(Child(fill, NSA, "srcRect"))
	if stretch := Child(fill, NSA, "stretch"); stretch != nil {
		record["fit"] = map[string]any{"mode": "stretch", "fill_rect": rect(Child(stretch, NSA, "fillRect"))}
	} else if tile := Child(fill, NSA, "tile"); tile != nil {
		settings := map[string]string{}
		for _, a := range tile.Attr {
			settings[a.Key] = a.Value
		}
		record["fit"] = map[string]any{"mode": "tile", "settings": settings}
	} else {
		record["fit"] = map[string]any{"mode": "unspecified"}
	}
	pr := Child(shape, NSP, "spPr")
	if g := Child(pr, NSA, "prstGeom"); g != nil {
		record["mask"] = map[string]any{"kind": "preset", "preset": AttrOr(g, "prst", ""), "adjustments_xml": layoutXML(Child(g, NSA, "avLst"))}
	} else if g := Child(pr, NSA, "custGeom"); g != nil {
		record["mask"] = map[string]any{"kind": "custom", "geometry_xml": xmlString(g)}
	}
}
