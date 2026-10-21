package office

import "github.com/beevik/etree"

// RemoveSVGImageSource drops the alternate SVG source when a blip is replaced
// with a raster image. Keep unrelated extensions and package media: other
// pictures may still reference the original SVG.
func RemoveSVGImageSource(blip *etree.Element) {
	const svgNS = "http://schemas.microsoft.com/office/drawing/2016/SVG/main"
	for _, list := range Children(blip, NSA, "extLst") {
		for _, ext := range Children(list, NSA, "ext") {
			sources := Children(ext, svgNS, "svgBlip")
			if len(sources) == 0 {
				continue
			}
			for _, source := range sources {
				ext.RemoveChild(source)
			}
			if len(ext.ChildElements()) == 0 {
				list.RemoveChild(ext)
			}
		}
		if len(list.ChildElements()) == 0 {
			blip.RemoveChild(list)
		}
	}
}
