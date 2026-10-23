package pptx

import (
	"fmt"
	"github.com/beevik/etree"
	"github.com/openvibely/openvibely-paperworks/internal/office"
	"strconv"
	"strings"
)

// TextPaint applies native glyph fills and outlines, independently of box fills.
type TextPaint struct {
	Fill    *DrawingStyle `json:"fill,omitempty"`
	Outline *TextOutline  `json:"outline,omitempty"`
}
type TextOutline struct {
	Width float64       `json:"width"` // points; zero removes the outline
	Dash  string        `json:"dash,omitempty"`
	Fill  *DrawingStyle `json:"fill,omitempty"`
}

func paintFill(s *DrawingStyle) (*etree.Element, error) {
	if s == nil || s.Image != nil || s.Shadow != nil || s.Outline != nil {
		return nil, fmt.Errorf("text paint needs a solid, gradient or pattern fill")
	}
	d := etree.NewDocument()
	if err := d.ReadFromString("<p:sp" + ns + "><p:spPr/></p:sp>"); err != nil {
		return nil, err
	}
	if err := styleDrawingInPackage(nil, "", d.Root(), s, ""); err != nil {
		return nil, err
	}
	for _, c := range office.Child(d.Root(), pns, "spPr").ChildElements() {
		if strings.HasSuffix(c.Tag, "Fill") {
			fill := c.Copy()
			fill.CreateAttr("xmlns:a", ans)
			return fill, nil
		}
	}
	return nil, fmt.Errorf("text paint requires a fill")
}
func paintTextProperties(pr *etree.Element, s *TextPaint) error {
	if s == nil {
		return fmt.Errorf("text_paint required")
	}
	if s.Fill != nil {
		fill, err := paintFill(s.Fill)
		if err != nil {
			return err
		}
		for _, c := range pr.ChildElements() {
			if strings.HasSuffix(c.Tag, "Fill") {
				pr.RemoveChild(c)
			}
		}
		index := 0
		if line := office.Child(pr, ans, "ln"); line != nil {
			index = line.Index() + 1
		}
		pr.InsertChildAt(index, fill)
	}
	if s.Outline != nil {
		o := s.Outline
		if !finite(o.Width) || o.Width < 0 || o.Width > 100 {
			return fmt.Errorf("text outline width must be 0..100 points")
		}
		line := etree.NewElement("a:ln")
		line.CreateAttr("xmlns:a", ans)
		line.CreateAttr("w", strconv.FormatInt(int64(o.Width*12700), 10))
		if o.Width == 0 {
			line.CreateElement("a:noFill")
		} else {
			fill, err := paintFill(o.Fill)
			if err != nil {
				return err
			}
			line.AddChild(fill)
		}
		if o.Dash != "" {
			switch o.Dash {
			case "solid", "dot", "dash", "lgDash", "dashDot", "lgDashDot", "lgDashDotDot", "sysDash", "sysDot", "sysDashDot", "sysDashDotDot":
			default:
				return fmt.Errorf("invalid text outline dash")
			}
			line.CreateElement("a:prstDash").CreateAttr("val", o.Dash)
		}
		for _, old := range office.Children(pr, ans, "ln") {
			pr.RemoveChild(old)
		}
		pr.InsertChildAt(0, line)
	}
	return nil
}
func paintTextBody(shape *etree.Element, s *TextPaint) error {
	body := office.Child(shape, pns, "txBody")
	if body == nil {
		return fmt.Errorf("shape has no text body")
	}
	body.CreateAttr("xmlns:a", ans)
	for _, p := range office.Children(body, ans, "p") {
		pp := office.Child(p, ans, "pPr")
		if pp == nil {
			pp = etree.NewElement("a:pPr")
			p.InsertChildAt(0, pp)
		}
		def := office.Child(pp, ans, "defRPr")
		if def == nil {
			def = pp.CreateElement("a:defRPr")
		}
		if err := paintTextProperties(def, s); err != nil {
			return err
		}
		for _, r := range p.ChildElements() {
			if r.Tag == "r" || r.Tag == "fld" || r.Tag == "br" {
				pr := office.Child(r, ans, "rPr")
				if pr == nil {
					pr = etree.NewElement("a:rPr")
					r.InsertChildAt(0, pr)
				}
				if err := paintTextProperties(pr, s); err != nil {
					return err
				}
			}
		}
		if end := office.Child(p, ans, "endParaRPr"); end != nil {
			if err := paintTextProperties(end, s); err != nil {
				return err
			}
		}
	}
	return nil
}
