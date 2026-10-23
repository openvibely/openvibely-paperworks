package pptx

import "github.com/beevik/etree"

func setSlideHidden(root *etree.Element, hidden bool) {
	value := "1"
	if hidden {
		value = "0"
	}
	root.CreateAttr("show", value)
}
