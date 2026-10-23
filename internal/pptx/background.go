package pptx

import (
	"fmt"
	"github.com/beevik/etree"
	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"strconv"
	"strings"
)

func styleBackground(pkg *opc.Package, part string, root *etree.Element, style *DrawingStyle, base string) error {
	if style == nil || style.Outline != nil || style.Shadow != nil {
		return fmt.Errorf("background_fill requires a fill without outline or shadow")
	}
	if style.Fill == "" && style.Gradient == nil && style.Pattern == nil && style.Image == nil {
		return fmt.Errorf("background_fill requires solid, gradient, pattern or image")
	}
	main, err := presentationPart(pkg)
	if err != nil {
		return err
	}
	doc, err := pkg.XML(main)
	if err != nil {
		return err
	}
	size := office.Child(doc.Root(), pns, "sldSz")
	if size == nil {
		return fmt.Errorf("missing slide dimensions")
	}
	w, err := strconv.ParseInt(size.SelectAttrValue("cx", ""), 10, 64)
	if err != nil || w <= 0 {
		return fmt.Errorf("invalid slide width")
	}
	h, err := strconv.ParseInt(size.SelectAttrValue("cy", ""), 10, 64)
	if err != nil || h <= 0 {
		return fmt.Errorf("invalid slide height")
	}
	// Reuse the shape fill implementation with the slide frame for cover cropping.
	temp := etree.NewDocument()
	if err = temp.ReadFromString(fmt.Sprintf(`<p:sp%s><p:spPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="%d" cy="%d"/></a:xfrm></p:spPr></p:sp>`, ns, w, h)); err != nil {
		return err
	}
	if err = styleDrawingInPackage(pkg, part, temp.Root(), style, base); err != nil {
		return err
	}
	cs := office.Child(root, pns, "cSld")
	if cs == nil {
		return fmt.Errorf("missing slide content")
	}
	bg := office.Child(cs, pns, "bg")
	if bg == nil {
		bg = etree.NewElement("p:bg")
		cs.InsertChildAt(0, bg)
	}
	for _, ref := range office.Children(bg, pns, "bgRef") {
		bg.RemoveChild(ref)
	}
	pr := office.Child(bg, pns, "bgPr")
	if pr == nil {
		pr = etree.NewElement("p:bgPr")
		bg.InsertChildAt(0, pr)
	}
	for _, c := range pr.ChildElements() {
		if strings.HasSuffix(c.Tag, "Fill") {
			pr.RemoveChild(c)
		}
	}
	for _, c := range office.Child(temp.Root(), pns, "spPr").ChildElements() {
		if strings.HasSuffix(c.Tag, "Fill") {
			pr.InsertChildAt(0, c.Copy())
		}
	}
	return nil
}
