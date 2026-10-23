package pptx

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/beevik/etree"
	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
)

func drawingChild(parent *etree.Element, local string) *etree.Element {
	e := office.Child(parent, ans, local)
	if e == nil {
		e = etree.NewElement("a:" + local)
		parent.AddChild(e)
	}
	return e
}

// detachDeletedShapeConnections preserves connector geometry while removing
// attachments to the shape and, for groups, all of its descendants. IDs are
// scoped to the drawing; replacements deliberately do not use this path.
func detachDeletedShapeConnections(shape *etree.Element) {
	ids := map[string]bool{}
	for _, pr := range office.All(shape, pns, "cNvPr") {
		if id := pr.SelectAttrValue("id", ""); id != "" {
			ids[id] = true
		}
	}
	drawing := shape
	for drawing.Parent() != nil && drawing.Parent().Tag != "" {
		drawing = drawing.Parent()
	}
	for _, tag := range []string{"stCxn", "endCxn"} {
		for _, endpoint := range office.All(drawing, ans, tag) {
			if ids[endpoint.SelectAttrValue("id", "")] {
				endpoint.Parent().RemoveChild(endpoint)
			}
		}
	}
}

func editObject(pkg *opc.Package, part string, shape *etree.Element, e Edit) error {
	switch e.Operation {
	case "delete":
		detachDeletedShapeConnections(shape)
		shape.Parent().RemoveChild(shape)
	case "forward", "backward":
		parent := shape.Parent()
		if parent == nil || (parent.Tag != "spTree" && parent.Tag != "grpSp") {
			return fmt.Errorf("stacking requires a drawing object")
		}
		var siblings []*etree.Element
		at := -1
		for _, c := range parent.ChildElements() {
			switch c.Tag {
			case "sp", "pic", "cxnSp", "graphicFrame", "grpSp", "contentPart":
				if c == shape {
					at = len(siblings)
				}
				siblings = append(siblings, c)
			}
		}
		if at < 0 {
			return fmt.Errorf("unsupported stacking object")
		}
		next := at - 1
		if e.Operation == "forward" {
			next = at + 1
		}
		if next < 0 || next >= len(siblings) {
			return nil
		}
		target := siblings[next]
		parent.RemoveChild(shape)
		index := target.Index()
		if e.Operation == "forward" {
			index++
		}
		parent.InsertChildAt(index, shape)
	case "front", "back":
		parent := shape.Parent()
		if parent == nil || (parent.Tag != "spTree" && parent.Tag != "grpSp") {
			return fmt.Errorf("stacking requires a drawing object")
		}
		parent.RemoveChild(shape)
		if e.Operation == "front" {
			if ext := office.Child(parent, pns, "extLst"); ext != nil {
				parent.InsertChildAt(ext.Index(), shape)
			} else {
				parent.AddChild(shape)
			}
		} else {
			index := len(parent.Child)
			for _, c := range parent.ChildElements() {
				if c.Tag != "nvGrpSpPr" && c.Tag != "grpSpPr" {
					index = c.Index()
					break
				}
			}
			parent.InsertChildAt(index, shape)
		}
	case "geometry":
		pr := office.Child(shape, pns, "spPr")
		if pr == nil {
			pr = office.Child(shape, pns, "grpSpPr")
		}
		var x *etree.Element
		if pr != nil {
			x = office.Child(pr, ans, "xfrm")
			if x == nil {
				x = etree.NewElement("a:xfrm")
				pr.InsertChildAt(0, x)
			}
		} else {
			x = office.Child(shape, pns, "xfrm")
		}
		if x == nil {
			return fmt.Errorf("shape has no supported transform")
		}
		// A placeholder may inherit its whole transform. Do not write a partial
		// override that loses the inherited position or dimensions.
		for _, component := range []struct {
			child, first, second string
			a, b                 *float64
		}{{"off", "x", "y", e.X, e.Y}, {"ext", "cx", "cy", e.W, e.H}} {
			part := office.Child(x, ans, component.child)
			if (part == nil || part.SelectAttrValue(component.first, "") == "") && component.a == nil || (part == nil || part.SelectAttrValue(component.second, "") == "") && component.b == nil {
				return fmt.Errorf("inherited/incomplete geometry requires explicit x, y, w and h")
			}
		}
		if shape.Tag == "graphicFrame" {
			if err := resizeTableGrid(shape, e.W, e.H); err != nil {
				return err
			}
		}
		for _, v := range []struct {
			n     string
			p     *float64
			child string
		}{{"x", e.X, "off"}, {"y", e.Y, "off"}, {"cx", e.W, "ext"}, {"cy", e.H, "ext"}} {
			if v.p == nil {
				continue
			}
			if !finite(*v.p) || (v.child == "ext" && *v.p <= 0) {
				return fmt.Errorf("invalid geometry")
			}
			drawingChild(x, v.child).CreateAttr(v.n, strconv.FormatInt(emu(*v.p), 10))
		}
		if e.Rotation != nil {
			if !finite(*e.Rotation) {
				return fmt.Errorf("invalid rotation")
			}
			x.CreateAttr("rot", strconv.FormatInt(int64(*e.Rotation*60000), 10))
		}
		for _, v := range []struct {
			n string
			p *bool
		}{{"flipH", e.FlipH}, {"flipV", e.FlipV}} {
			if v.p != nil {
				s := "0"
				if *v.p {
					s = "1"
				}
				x.CreateAttr(v.n, s)
			}
		}
	case "fill":
		pr := office.Child(shape, pns, "spPr")
		if pr == nil {
			return fmt.Errorf("shape has no fill properties")
		}
		if _, err := color(e.Fill, ""); err != nil {
			return err
		}
		for _, c := range pr.ChildElements() {
			switch c.Tag {
			case "noFill", "solidFill", "gradFill", "blipFill", "pattFill", "grpFill":
				pr.RemoveChild(c)
			}
		}
		fill := etree.NewElement("a:solidFill")
		if e.Fill == "" {
			fill = etree.NewElement("a:noFill")
		} else {
			fill.CreateElement("a:srgbClr").CreateAttr("val", e.Fill)
		}
		idx := len(pr.Child)
		for _, c := range pr.ChildElements() {
			switch c.Tag {
			case "ln", "effectLst", "effectDag", "scene3d", "sp3d", "extLst":
				idx = c.Index()
				goto insertFill
			}
		}
	insertFill:
		pr.InsertChildAt(idx, fill)
	case "run-format", "paragraph-format":
		body := office.Child(shape, pns, "txBody")
		if body == nil {
			return fmt.Errorf("shape has no text")
		}
		paras := office.Children(body, ans, "p")
		if e.Paragraph < 0 || e.Paragraph >= len(paras) {
			return fmt.Errorf("paragraph out of range")
		}
		p := paras[e.Paragraph]
		if e.Operation == "paragraph-format" {
			style := e.ParagraphStyle
			if style == nil {
				style = &ParagraphStyle{Align: e.Align}
			}
			return styleParagraph(p, style)
		}
		runs := office.Children(p, ans, "r")
		if e.Run < 0 || e.Run >= len(runs) {
			return fmt.Errorf("run out of range")
		}
		pr := office.Child(runs[e.Run], ans, "rPr")
		if pr == nil {
			pr = etree.NewElement("a:rPr")
			runs[e.Run].InsertChildAt(0, pr)
		}
		if e.FontSize != nil {
			if !finite(*e.FontSize) || *e.FontSize <= 0 || *e.FontSize > 400 {
				return fmt.Errorf("invalid font_size")
			}
			pr.CreateAttr("sz", strconv.Itoa(int(*e.FontSize*100)))
		}
		for _, v := range []struct {
			n string
			p *bool
		}{{"b", e.Bold}, {"i", e.Italic}} {
			if v.p != nil {
				s := "0"
				if *v.p {
					s = "1"
				}
				pr.CreateAttr(v.n, s)
			}
		}
		if e.Font != "" {
			drawingChild(pr, "latin").CreateAttr("typeface", e.Font)
		}
		if e.Color != "" {
			if _, err := color(e.Color, ""); err != nil {
				return err
			}
			for _, c := range pr.ChildElements() {
				if strings.HasSuffix(c.Tag, "Fill") {
					pr.RemoveChild(c)
				}
			}
			f := etree.NewElement("a:solidFill")
			f.CreateElement("a:srgbClr").CreateAttr("val", e.Color)
			pr.InsertChildAt(0, f)
		}

		if e.Underline != nil {
			v := "none"
			if *e.Underline {
				v = "sng"
			}
			pr.CreateAttr("u", v)
		}
		if e.Hyperlink != nil {
			if err := setHyperlink(pkg, part, pr, *e.Hyperlink); err != nil {
				return err
			}
		}
	case "image-mask":
		if !office.Is(shape, pns, "pic") {
			return fmt.Errorf("image-mask requires a picture")
		}
		switch e.Mask {
		case "rect", "roundRect", "ellipse":
		default:
			return fmt.Errorf("mask must be rect, roundRect or ellipse")
		}
		pr := office.Child(shape, pns, "spPr")
		if pr == nil {
			return fmt.Errorf("picture properties missing")
		}
		for _, tag := range []string{"prstGeom", "custGeom"} {
			if old := office.Child(pr, ans, tag); old != nil {
				pr.RemoveChild(old)
			}
		}
		geom := etree.NewElement("a:prstGeom")
		geom.CreateAttr("prst", e.Mask)
		geom.CreateElement("a:avLst")
		insertDrawingProperty(pr, geom)
	case "replace-image":
		blips := office.All(shape, ans, "blip")
		if len(blips) != 1 {
			return fmt.Errorf("shape must contain one image")
		}
		data, _, format, err := loadSlideImage(e.Path, e.SVG)
		if err != nil {
			return err
		}
		id, err := attachImage(pkg, part, "ppt/media/replacement", format, data)
		if err != nil {
			return err
		}
		blips[0].RemoveAttr("r:link")
		blips[0].CreateAttr("r:embed", id)
		office.RemoveSVGImageSource(blips[0])
	case "crop":
		if e.Crop == nil {
			return fmt.Errorf("crop required")
		}
		fill := office.Child(shape, pns, "blipFill")
		if fill == nil {
			return fmt.Errorf("shape is not a picture")
		}
		v := e.Crop
		if !finite(v.Left) || !finite(v.Right) || !finite(v.Top) || !finite(v.Bottom) || v.Left < 0 || v.Right < 0 || v.Top < 0 || v.Bottom < 0 || v.Left+v.Right >= 1 || v.Top+v.Bottom >= 1 {
			return fmt.Errorf("invalid crop fractions")
		}
		r := office.Child(fill, ans, "srcRect")
		if r == nil {
			r = etree.NewElement("a:srcRect")
			idx := 0
			if b := office.Child(fill, ans, "blip"); b != nil {
				idx = b.Index() + 1
			}
			fill.InsertChildAt(idx, r)
		}
		for _, q := range []struct {
			k string
			v float64
		}{{"l", v.Left}, {"r", v.Right}, {"t", v.Top}, {"b", v.Bottom}} {
			r.CreateAttr(q.k, strconv.Itoa(int(q.v*100000)))
		}
	}
	return nil
}
func addNotes(p *opc.Package, slide, text string) (string, error) {
	name := p.UnusedName("ppt/notesSlides/notesSlide", ".xml")
	d, err := opc.ParseXML([]byte(`<p:notes` + ns + `><p:cSld><p:spTree>` + group + noteShape(text, "Arial") + `</p:spTree></p:cSld>` + override + `</p:notes>`))
	if err != nil {
		return "", err
	}
	if err = p.SetXML(name, d); err != nil {
		return "", err
	}
	if err = p.EnsureContentType(name, "application/vnd.openxmlformats-officedocument.presentationml.notesSlide+xml"); err != nil {
		return "", err
	}
	target := opc.Relative(slide, name)
	if _, err = p.AddRel(slide, rns+"/notesSlide", target, false); err != nil {
		return "", err
	}
	target = opc.Relative(name, slide)
	if _, err = p.AddRel(name, rns+"/slide", target, false); err != nil {
		return "", err
	}
	// Reuse the deck's existing notes master, if present. Notes slides may omit it.
	for _, n := range p.Names() {
		if p.ContentType(n) == "application/vnd.openxmlformats-officedocument.presentationml.notesMaster+xml" {
			target = opc.Relative(name, n)
			_, err = p.AddRel(name, rns+"/notesMaster", target, false)
			break
		}
	}
	return name, err
}
