package pptx

import (
	"fmt"
	"github.com/beevik/etree"
	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"net/url"
	"strconv"
	"unicode/utf8"
)

type ParagraphStyle struct {
	Align      string   `json:"align,omitempty"`
	Before     *float64 `json:"before,omitempty"`
	After      *float64 `json:"after,omitempty"`
	Indent     *float64 `json:"indent,omitempty"`
	MarginLeft *float64 `json:"margin_left,omitempty"`
	Level      *int     `json:"level,omitempty"`
	Bullet     *bool    `json:"bullet,omitempty"`
	BulletChar string   `json:"bullet_char,omitempty"`
}

func styleParagraph(p *etree.Element, s *ParagraphStyle) error {
	if s == nil {
		return fmt.Errorf("paragraph_style required")
	}
	pr := office.Child(p, ans, "pPr")
	if pr == nil {
		pr = etree.NewElement("a:pPr")
		p.InsertChildAt(0, pr)
	}
	if s.Align != "" {
		switch s.Align {
		case "l", "r", "ctr", "just":
		default:
			return fmt.Errorf("invalid paragraph alignment")
		}
		pr.CreateAttr("algn", s.Align)
	}
	if s.Level != nil {
		if *s.Level < 0 || *s.Level > 8 {
			return fmt.Errorf("level must be 0..8")
		}
		pr.CreateAttr("lvl", strconv.Itoa(*s.Level))
	}
	for _, v := range []struct {
		tag   string
		value *float64
	}{{"indent", s.Indent}, {"marL", s.MarginLeft}} {
		if v.value != nil {
			if !finite(*v.value) || *v.value < -1000 || *v.value > 1000 {
				return fmt.Errorf("invalid indent/margin")
			}
			pr.CreateAttr(v.tag, strconv.FormatInt(int64(*v.value*12700), 10))
		}
	}
	insert := func(e *etree.Element) {
		order := []string{"lnSpc", "spcBef", "spcAft", "buClrTx", "buClr", "buSzTx", "buSzPct", "buSzPts", "buFontTx", "buFont", "buNone", "buAutoNum", "buChar", "buBlip", "tabLst", "defRPr", "extLst"}
		rank := func(tag string) int {
			for i, v := range order {
				if tag == v {
					return i
				}
			}
			return len(order)
		}
		for _, c := range pr.ChildElements() {
			if rank(c.Tag) > rank(e.Tag) {
				pr.InsertChildAt(c.Index(), e)
				return
			}
		}
		pr.AddChild(e)
	}
	for _, v := range []struct {
		tag   string
		value *float64
	}{{"spcBef", s.Before}, {"spcAft", s.After}} {
		if v.value != nil {
			if !finite(*v.value) || *v.value < 0 || *v.value > 1000 {
				return fmt.Errorf("spacing must be 0..1000 points")
			}
			if old := office.Child(pr, ans, v.tag); old != nil {
				pr.RemoveChild(old)
			}
			e := etree.NewElement("a:" + v.tag)
			e.CreateElement("a:spcPts").CreateAttr("val", strconv.Itoa(int(*v.value*100)))
			insert(e)
		}
	}
	if s.Bullet != nil || s.BulletChar != "" {
		for _, tag := range []string{"buNone", "buAutoNum", "buChar", "buBlip"} {
			if old := office.Child(pr, ans, tag); old != nil {
				pr.RemoveChild(old)
			}
		}
		if s.Bullet != nil && !*s.Bullet {
			if s.BulletChar != "" {
				return fmt.Errorf("bullet_char conflicts with disabled bullet")
			}
			insert(etree.NewElement("a:buNone"))
		} else {
			char := s.BulletChar
			if char == "" {
				char = "•"
			}
			if utf8.RuneCountInString(char) != 1 {
				return fmt.Errorf("bullet_char must be one character")
			}
			e := etree.NewElement("a:buChar")
			e.CreateAttr("char", char)
			insert(e)
		}
	}
	return nil
}
func setHyperlink(pkg *opc.Package, part string, pr *etree.Element, link string) error {
	for _, h := range office.Children(pr, ans, "hlinkClick") {
		pr.RemoveChild(h)
	}
	if link == "" {
		return nil
	}
	u, err := url.Parse(link)
	if err != nil {
		return err
	}
	if u.Scheme != "https" && u.Scheme != "http" && u.Scheme != "mailto" {
		return fmt.Errorf("hyperlink requires http, https or mailto URL")
	}
	id, err := pkg.AddRel(part, rns+"/hyperlink", link, true)
	if err != nil {
		return err
	}
	h := etree.NewElement("a:hlinkClick")
	h.CreateAttr("r:id", id)
	if ext := office.Child(pr, ans, "extLst"); ext != nil {
		pr.InsertChildAt(ext.Index(), h)
	} else {
		pr.AddChild(h)
	}
	return nil
}
