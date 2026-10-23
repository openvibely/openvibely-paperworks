package pptx

import (
	"fmt"
	"github.com/beevik/etree"
	"github.com/openvibely/openvibely-paperworks/internal/office"
)

func setPictureLock(shape *etree.Element, value *bool) error {
	if !office.Is(shape, pns, "pic") || value == nil {
		return fmt.Errorf("image-lock requires a picture and lock_aspect_ratio")
	}
	pr := office.Child(office.Child(shape, pns, "nvPicPr"), pns, "cNvPicPr")
	if pr == nil {
		return fmt.Errorf("picture nonvisual properties missing")
	}
	locks := office.Child(pr, ans, "picLocks")
	if locks == nil {
		locks = etree.NewElement("a:picLocks")
		locks.CreateAttr("xmlns:a", ans)
		pr.InsertChildAt(0, locks)
	}
	v := "0"
	if *value {
		v = "1"
	}
	locks.CreateAttr("noChangeAspect", v)
	return nil
}
