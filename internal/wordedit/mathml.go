package wordedit

import (
	"errors"
	"strings"

	"github.com/beevik/etree"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
)

func M(local string) *etree.Element { return etree.NewElement("m:" + local) }

func mVal(local, val string) *etree.Element {
	e := M(local)
	e.CreateAttr("m:val", val)
	return e
}

// mathml converts the supported MathML subset to OMML elements.
func mathml(src *etree.Element) ([]*etree.Element, error) {
	tag := src.Tag
	kids := src.ChildElements()
	slot := func(name string, nodes ...*etree.Element) (*etree.Element, error) {
		s := M(name)
		for _, n := range nodes {
			out, err := mathml(n)
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
		for _, c := range kids {
			o, err := mathml(c)
			if err != nil {
				return nil, err
			}
			out = append(out, o...)
		}
		return out, nil
	case "mi", "mn", "mo", "mtext":
		if len(kids) == 0 {
			r := M("r")
			t := M("t")
			t.SetText(src.Text())
			t.CreateAttr("xml:space", "preserve")
			r.AddChild(t)
			return []*etree.Element{r}, nil
		}
	case "mfenced":
		d := M("d")
		props := M("dPr")
		for _, kv := range [][3]string{{"begChr", "open", "("}, {"endChr", "close", ")"}, {"sepChr", "separators", ","}} {
			v := src.SelectAttrValue(kv[1], kv[2])
			if len([]rune(v)) > 1 {
				return nil, errors.New("fences and separators must be single characters")
			}
			props.AddChild(mVal(kv[0], v))
		}
		d.AddChild(props)
		for _, c := range kids {
			s, err := slot("e", c)
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
		m := M("m")
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
			mr := M("mr")
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
			rad := M("rad")
			props := M("radPr")
			props.AddChild(mVal("degHide", "1"))
			rad.AddChild(props)
			rad.AddChild(M("deg"))
			s, err := slot("e", kids...)
			if err != nil {
				return nil, err
			}
			rad.AddChild(s)
			return []*etree.Element{rad}, nil
		}
	case "mroot":
		if len(kids) == 2 {
			rad := M("rad")
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
	recipes := map[string][]string{"mfrac": {"f", "num", "den"}, "msup": {"sSup", "e", "sup"}, "msub": {"sSub", "e", "sub"}, "msubsup": {"sSubSup", "e", "sub", "sup"}}
	rec, ok := recipes[tag]
	if !ok || len(kids) != len(rec)-1 {
		return nil, errors.New("unsupported MathML element/arity: " + tag)
	}
	out := M(rec[0])
	for i, name := range rec[1:] {
		s, err := slot(name, kids[i])
		if err != nil {
			return nil, err
		}
		out.AddChild(s)
	}
	return []*etree.Element{out}, nil
}

func equation(d *Doc, spec map[string]any) error {
	src := str(spec, "mathml")
	if strings.TrimSpace(src) == "" {
		return errors.New("mathml is required")
	}
	x, err := opc.ParseXML([]byte(src))
	if err != nil {
		return errors.New("invalid MathML: " + err.Error())
	}
	nodes, err := mathml(x.Root())
	if err != nil {
		return err
	}
	om := M("oMath")
	for _, n := range nodes {
		om.AddChild(n)
	}
	p := W("p")
	p.AddChild(om)
	appendBody(d, p)
	d.Touch(d.Main)
	return nil
}
