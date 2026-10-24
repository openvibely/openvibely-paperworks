package pptx

import (
	"fmt"
	"github.com/beevik/etree"
	"github.com/openvibely/openvibely-paperworks/internal/office"
	"math"
	"strconv"
)

var connectorPresets = map[string]string{"straight": "line", "elbow": "bentConnector3", "elbow2": "bentConnector2", "elbow3": "bentConnector3", "elbow4": "bentConnector4", "elbow5": "bentConnector5", "curved": "curvedConnector3"}

// Endpoints omitted during editing retain their original attachments.
func editConnector(root, shape *etree.Element, c *Connector) error {
	if c == nil || !office.Is(shape, pns, "cxnSp") {
		return fmt.Errorf("connector requires a connector shape and specification")
	}
	props := office.All(shape, pns, "cNvCxnSpPr")
	if len(props) != 1 {
		return fmt.Errorf("connector properties missing")
	}
	if c.Kind != "" {
		preset := connectorPresets[c.Kind]
		if preset == "" {
			return fmt.Errorf("unsupported connector kind %q", c.Kind)
		}
		pr := office.Child(shape, pns, "spPr")
		geom := office.Child(pr, ans, "prstGeom")
		if geom == nil {
			return fmt.Errorf("connector preset geometry missing")
		}
		if geom.SelectAttrValue("prst", "") != preset {
			for _, child := range geom.ChildElements() {
				geom.RemoveChild(child)
			}
			geom.CreateElement("a:avLst")
		}
		geom.CreateAttr("prst", preset)
	}
	find := func(id string) (*etree.Element, error) {
		var found *etree.Element
		for _, pr := range office.All(root, pns, "cNvPr") {
			if pr.SelectAttrValue("id", "") == id {
				if found != nil {
					return nil, fmt.Errorf("ambiguous endpoint %s", id)
				}
				found = pr.Parent().Parent()
			}
		}
		if found == nil || found == shape || found.Parent() != shape.Parent() {
			return nil, fmt.Errorf("endpoint must identify another shape in the same drawing tree")
		}
		return found, nil
	}
	for _, v := range []struct {
		tag        string
		end, other *Connection
	}{{"stCxn", c.Start, c.End}, {"endCxn", c.End, c.Start}} {
		if v.end == nil {
			continue
		}
		target, err := find(v.end.ShapeID)
		if err != nil {
			return err
		}
		site := v.end.Site
		if v.end.Side != "" {
			side := v.end.Side
			if side == "auto" {
				other := v.other
				if other == nil {
					tag := "stCxn"
					if v.tag == tag {
						tag = "endCxn"
					}
					old := office.Child(props[0], ans, tag)
					if old != nil {
						other = &Connection{ShapeID: old.SelectAttrValue("id", "")}
					}
				}
				if other == nil {
					return fmt.Errorf("automatic side selection needs both endpoints")
				}
				peer, err := find(other.ShapeID)
				if err != nil {
					return err
				}
				a, err := explicitDrawingRect(target)
				if err != nil {
					return err
				}
				b, err := explicitDrawingRect(peer)
				if err != nil {
					return err
				}
				dx, dy := b.x+b.w/2-a.x-a.w/2, b.y+b.h/2-a.y-a.h/2
				if math.Abs(dx) >= math.Abs(dy) {
					side = "right"
					if dx < 0 {
						side = "left"
					}
				} else {
					side = "bottom"
					if dy < 0 {
						side = "top"
					}
				}
			}
			pr := office.Child(target, pns, "spPr")
			xf := office.Child(pr, ans, "xfrm")
			if xf != nil && (xf.SelectAttrValue("rot", "0") != "0" || xf.SelectAttrValue("flipH", "0") == "1" || xf.SelectAttrValue("flipV", "0") == "1") {
				return fmt.Errorf("side selection on transformed shapes requires explicit site")
			}
			geom := office.Child(pr, ans, "prstGeom")
			if geom == nil {
				return fmt.Errorf("side selection requires preset geometry")
			}
			switch geom.SelectAttrValue("prst", "") {
			case "rect", "roundRect", "diamond", "ellipse":
			default:
				return fmt.Errorf("side selection for this geometry requires explicit site")
			}
			var ok bool
			site, ok = map[string]int{"top": 0, "left": 1, "bottom": 2, "right": 3}[side]
			if !ok {
				return fmt.Errorf("invalid connector side %q", side)
			}
			// DrawingML ellipse has eight sites: cardinal points are 0,2,4,6.
			// See Apache POI presetShapeDefinitions.xml, ellipse/cxnLst.
			if geom.SelectAttrValue("prst", "") == "ellipse" {
				site *= 2
			}
		}
		if site < 0 || site > 255 {
			return fmt.Errorf("invalid connector site")
		}
		for _, old := range office.Children(props[0], ans, v.tag) {
			props[0].RemoveChild(old)
		}
		ref := etree.NewElement("a:" + v.tag)
		ref.CreateAttr("id", v.end.ShapeID)
		ref.CreateAttr("idx", strconv.Itoa(site))
		if v.tag == "stCxn" {
			props[0].InsertChildAt(0, ref)
		} else if ext := office.Child(props[0], ans, "extLst"); ext != nil {
			props[0].InsertChildAt(ext.Index(), ref)
		} else {
			props[0].AddChild(ref)
		}
	}
	return nil
}
