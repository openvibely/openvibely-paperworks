// Package mathml converts a deliberately bounded MathML subset to Office Math Markup Language.
package mathml

import (
	"errors"

	"github.com/beevik/etree"
)

func element(local string) *etree.Element { return etree.NewElement("m:" + local) }

func valueElement(local, value string) *etree.Element {
	e := element(local)
	e.CreateAttr("m:val", value)
	return e
}

// ToOMML converts a MathML expression tree to one native OMML equation. It
// rejects elements and arities outside the subset represented below.
func ToOMML(src *etree.Element) (*etree.Element, error) {
	if src == nil {
		return nil, errors.New("MathML expression is empty")
	}
	nodes, err := convert(src)
	if err != nil {
		return nil, err
	}
	if len(nodes) == 0 {
		return nil, errors.New("MathML expression is empty")
	}
	out := element("oMath")
	for _, node := range nodes {
		out.AddChild(node)
	}
	return out, nil
}

// convert maps supported MathML elements to OMML. Keep this subset explicit:
// accepting unknown elements as text would silently change equation meaning.
func convert(src *etree.Element) ([]*etree.Element, error) {
	tag := src.Tag
	kids := src.ChildElements()
	slot := func(name string, nodes ...*etree.Element) (*etree.Element, error) {
		s := element(name)
		for _, n := range nodes {
			out, err := convert(n)
			if err != nil {
				return nil, err
			}
			for _, o := range out {
				s.AddChild(o)
			}
		}
		return s, nil
	}
	switch tag {
	case "math", "mrow":
		var out []*etree.Element
		for _, child := range kids {
			nodes, err := convert(child)
			if err != nil {
				return nil, err
			}
			out = append(out, nodes...)
		}
		return out, nil
	case "mi", "mn", "mo", "mtext":
		if len(kids) == 0 {
			r := element("r")
			t := element("t")
			t.SetText(src.Text())
			t.CreateAttr("xml:space", "preserve")
			r.AddChild(t)
			return []*etree.Element{r}, nil
		}
	case "mfenced":
		d := element("d")
		props := element("dPr")
		for _, attr := range [][3]string{{"begChr", "open", "("}, {"endChr", "close", ")"}, {"sepChr", "separators", ","}} {
			v := src.SelectAttrValue(attr[1], attr[2])
			if len([]rune(v)) > 1 {
				return nil, errors.New("fences and separators must be single characters")
			}
			props.AddChild(valueElement(attr[0], v))
		}
		d.AddChild(props)
		for _, child := range kids {
			s, err := slot("e", child)
			if err != nil {
				return nil, err
			}
			d.AddChild(s)
		}
		return []*etree.Element{d}, nil
	case "mtable":
		if len(kids) == 0 {
			return nil, errors.New("matrix needs at least one row")
		}
		m := element("m")
		cols := -1
		for _, row := range kids {
			if row.Tag != "mtr" {
				return nil, errors.New("expected MathML matrix row")
			}
			cells := row.ChildElements()
			if cols < 0 {
				cols = len(cells)
			}
			if cols == 0 || len(cells) != cols {
				return nil, errors.New("matrix rows must have equal nonzero width")
			}
			mr := element("mr")
			for _, cell := range cells {
				if cell.Tag != "mtd" {
					return nil, errors.New("expected MathML matrix cell")
				}
				s, err := slot("e", cell.ChildElements()...)
				if err != nil {
					return nil, err
				}
				mr.AddChild(s)
			}
			m.AddChild(mr)
		}
		return []*etree.Element{m}, nil
	case "msqrt":
		if len(kids) > 0 {
			rad := element("rad")
			props := element("radPr")
			props.AddChild(valueElement("degHide", "1"))
			rad.AddChild(props)
			rad.AddChild(element("deg"))
			s, err := slot("e", kids...)
			if err != nil {
				return nil, err
			}
			rad.AddChild(s)
			return []*etree.Element{rad}, nil
		}
	case "mroot":
		if len(kids) == 2 {
			rad := element("rad")
			deg, err := slot("deg", kids[1])
			if err != nil {
				return nil, err
			}
			e, err := slot("e", kids[0])
			if err != nil {
				return nil, err
			}
			rad.AddChild(deg)
			rad.AddChild(e)
			return []*etree.Element{rad}, nil
		}
	}
	recipes := map[string][]string{
		"mfrac":   {"f", "num", "den"},
		"msup":    {"sSup", "e", "sup"},
		"msub":    {"sSub", "e", "sub"},
		"msubsup": {"sSubSup", "e", "sub", "sup"},
	}
	recipe, ok := recipes[tag]
	if !ok || len(kids) != len(recipe)-1 {
		return nil, errors.New("unsupported MathML element/arity: " + tag)
	}
	out := element(recipe[0])
	for i, name := range recipe[1:] {
		s, err := slot(name, kids[i])
		if err != nil {
			return nil, err
		}
		out.AddChild(s)
	}
	return []*etree.Element{out}, nil
}
