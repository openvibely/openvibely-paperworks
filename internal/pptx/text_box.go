package pptx

import (
	"fmt"
	"github.com/beevik/etree"
	"github.com/openvibely/openvibely-paperworks/internal/office"
	"strconv"
)

// TextInsets are in points; nil sides retain their existing values.
type TextInsets struct {
	Top    *float64 `json:"top,omitempty"`
	Right  *float64 `json:"right,omitempty"`
	Bottom *float64 `json:"bottom,omitempty"`
	Left   *float64 `json:"left,omitempty"`
}
type TextBoxStyle struct {
	Insets        *TextInsets `json:"insets,omitempty"`
	VerticalAlign string      `json:"vertical_align,omitempty"` // top, middle, bottom
	Wrap          string      `json:"wrap,omitempty"`           // square or none
	AutoFit       string      `json:"auto_fit,omitempty"`       // none, shrinkText, resizeShapeToFitText
}

func styleTextBox(shape *etree.Element, style *TextBoxStyle) error {
	if style == nil {
		return fmt.Errorf("text_box required")
	}
	body := office.Child(shape, pns, "txBody")
	if body == nil {
		return fmt.Errorf("shape has no editable text body")
	}
	pr := office.Child(body, ans, "bodyPr")
	if pr == nil {
		pr = etree.NewElement("a:bodyPr")
		body.InsertChildAt(0, pr)
	}
	if style.VerticalAlign != "" {
		v := map[string]string{"top": "t", "middle": "ctr", "bottom": "b"}[style.VerticalAlign]
		if v == "" {
			return fmt.Errorf("vertical_align must be top, middle or bottom")
		}
		pr.CreateAttr("anchor", v)
	}
	if style.Wrap != "" {
		if style.Wrap != "square" && style.Wrap != "none" {
			return fmt.Errorf("wrap must be square or none")
		}
		pr.CreateAttr("wrap", style.Wrap)
	}
	if i := style.Insets; i != nil {
		for _, v := range []struct {
			name  string
			value *float64
		}{{"tIns", i.Top}, {"rIns", i.Right}, {"bIns", i.Bottom}, {"lIns", i.Left}} {
			if v.value == nil {
				continue
			}
			if !finite(*v.value) || *v.value < 0 || *v.value > 1000 {
				return fmt.Errorf("text insets must be 0..1000 points")
			}
			pr.CreateAttr(v.name, strconv.FormatInt(int64(*v.value*12700), 10))
		}
	}
	if style.AutoFit != "" {
		tag := map[string]string{"none": "noAutofit", "shrinkText": "normAutofit", "resizeShapeToFitText": "spAutoFit"}[style.AutoFit]
		if tag == "" {
			return fmt.Errorf("auto_fit must be none, shrinkText or resizeShapeToFitText")
		}
		for _, name := range []string{"noAutofit", "normAutofit", "spAutoFit"} {
			for _, old := range office.Children(pr, ans, name) {
				pr.RemoveChild(old)
			}
		}
		e := etree.NewElement("a:" + tag)
		inserted := false
		for _, c := range pr.ChildElements() {
			if c.Tag == "scene3d" || c.Tag == "sp3d" || c.Tag == "flatTx" || c.Tag == "extLst" {
				pr.InsertChildAt(c.Index(), e)
				inserted = true
				break
			}
		}
		if !inserted {
			pr.AddChild(e)
		}
	}
	return nil
}
