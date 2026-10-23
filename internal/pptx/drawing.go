package pptx

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/beevik/etree"
	"github.com/openvibely/openvibely-paperworks/internal/office"
)

// Drawing coordinates inside a custom path are normalized to a 100000 square.
// Its enclosing shape controls physical size and position in inches.
type PathCommand struct {
	Operation string       `json:"operation"`
	Points    [][2]float64 `json:"points,omitempty"`
}
type Connection struct {
	ShapeID string `json:"shape_id"`
	Site    int    `json:"site"`
}
type Connector struct {
	Kind  string      `json:"kind"`
	Start *Connection `json:"start,omitempty"`
	End   *Connection `json:"end,omitempty"`
}
type GradientStop struct {
	Position float64  `json:"position"`
	Color    string   `json:"color"`
	Opacity  *float64 `json:"opacity,omitempty"`
}
type Gradient struct {
	Path  string         `json:"path,omitempty"` // circle, rect, shape; omitted means linear
	Focus *GradientFocus `json:"focus,omitempty"`

	Angle float64        `json:"angle"`
	Stops []GradientStop `json:"stops"`
}
type GradientFocus struct {
	Left   float64 `json:"left"`
	Top    float64 `json:"top"`
	Right  float64 `json:"right"`
	Bottom float64 `json:"bottom"`
}
type Outline struct {
	HeadWidth  string `json:"head_width,omitempty"`
	HeadLength string `json:"head_length,omitempty"`
	TailWidth  string `json:"tail_width,omitempty"`
	TailLength string `json:"tail_length,omitempty"`

	Color string  `json:"color"`
	Width float64 `json:"width"`
	Dash  string  `json:"dash,omitempty"`
	Head  string  `json:"head,omitempty"`
	Tail  string  `json:"tail,omitempty"`
}
type Shadow struct {
	Color     string   `json:"color"`
	Opacity   *float64 `json:"opacity,omitempty"`
	Blur      float64  `json:"blur"`
	Distance  float64  `json:"distance"`
	Direction float64  `json:"direction"`
}
type DrawingStyle struct {
	Pattern  *PatternFill `json:"pattern,omitempty"`
	Image    *ImageFill   `json:"image,omitempty"`
	Fill     string       `json:"fill,omitempty"`
	Opacity  *float64     `json:"opacity,omitempty"`
	Gradient *Gradient    `json:"gradient,omitempty"`
	Outline  *Outline     `json:"outline,omitempty"`
	Shadow   *Shadow      `json:"shadow,omitempty"`
}

func drawingElement(id int, e Element) (string, error) {
	d := etree.NewDocument()
	kind := "sp"
	nvKind := "Sp"
	if e.Type == "connector" {
		kind = "cxnSp"
		nvKind = "CxnSp"
	}
	if err := d.ReadFromString("<p:" + kind + ns + ">" + nv(id, e, nvKind) + "<p:spPr>" + xfrm(e, "a") + "</p:spPr></p:" + kind + ">"); err != nil {
		return "", err
	}
	shape := d.Root()
	pr := office.Child(shape, pns, "spPr")
	if e.Type == "connector" {
		c := e.Connector
		if c == nil {
			return "", fmt.Errorf("connector specification required")
		}
		presets := map[string]string{"straight": "line", "elbow": "bentConnector3", "curved": "curvedConnector3"}
		preset := presets[c.Kind]
		if preset == "" {
			return "", fmt.Errorf("connector kind must be straight, elbow or curved")
		}
		geom := pr.CreateElement("a:prstGeom")
		geom.CreateAttr("prst", preset)
		geom.CreateElement("a:avLst")
		nonvisual := office.All(shape, pns, "cNvCxnSpPr")[0]
		for _, v := range []struct {
			name string
			c    *Connection
		}{{"stCxn", c.Start}, {"endCxn", c.End}} {
			if v.c != nil {
				if v.c.ShapeID == "" || v.c.Site < 0 || v.c.Site > 255 {
					return "", fmt.Errorf("invalid connector endpoint")
				}
				r := nonvisual.CreateElement("a:" + v.name)
				r.CreateAttr("id", v.c.ShapeID)
				r.CreateAttr("idx", strconv.Itoa(v.c.Site))
			}
		}
		line := pr.CreateElement("a:ln")
		line.CreateAttr("w", "12700")
		line.CreateElement("a:solidFill").CreateElement("a:srgbClr").CreateAttr("val", "000000")
	} else {
		if len(e.Commands) == 0 || len(e.Commands) > 10000 {
			return "", fmt.Errorf("path requires 1..10000 commands")
		}
		geom := pr.CreateElement("a:custGeom")
		for _, tag := range []string{"avLst", "gdLst", "ahLst", "cxnLst"} {
			geom.CreateElement("a:" + tag)
		}
		r := geom.CreateElement("a:rect")
		for k, v := range map[string]string{"l": "0", "t": "0", "r": "r", "b": "b"} {
			r.CreateAttr(k, v)
		}
		path := geom.CreateElement("a:pathLst").CreateElement("a:path")
		path.CreateAttr("w", "100000")
		path.CreateAttr("h", "100000")
		tags := map[string]string{"move": "moveTo", "line": "lnTo", "quadratic": "quadBezTo", "cubic": "cubicBezTo", "close": "close"}
		counts := map[string]int{"move": 1, "line": 1, "quadratic": 2, "cubic": 3, "close": 0}
		for i, c := range e.Commands {
			tag := tags[c.Operation]
			if tag == "" || len(c.Points) != counts[c.Operation] || i == 0 && c.Operation != "move" {
				return "", fmt.Errorf("invalid path command %d", i)
			}
			cmd := path.CreateElement("a:" + tag)
			for _, p := range c.Points {
				if !finite(p[0]) || !finite(p[1]) || p[0] < 0 || p[0] > 100000 || p[1] < 0 || p[1] > 100000 {
					return "", fmt.Errorf("path points must be 0..100000")
				}
				pt := cmd.CreateElement("a:pt")
				pt.CreateAttr("x", strconv.FormatInt(int64(p[0]), 10))
				pt.CreateAttr("y", strconv.FormatInt(int64(p[1]), 10))
			}
		}
		if e.Fill != "" {
			pr.CreateElement("a:solidFill").CreateElement("a:srgbClr").CreateAttr("val", e.Fill)
		} else {
			pr.CreateElement("a:noFill")
		}
	}
	return d.WriteToString()
}
func rgbaElement(parent *etree.Element, hex string, alpha *float64) error {
	if _, err := color(hex, ""); err != nil {
		return err
	}
	if hex == "" {
		return fmt.Errorf("color required")
	}
	c := parent.CreateElement("a:srgbClr")
	c.CreateAttr("val", hex)
	if alpha != nil {
		if !finite(*alpha) || *alpha < 0 || *alpha > 1 {
			return fmt.Errorf("opacity must be 0..1")
		}
		c.CreateElement("a:alpha").CreateAttr("val", strconv.Itoa(int(*alpha*100000)))
	}
	return nil
}
func insertDrawingProperty(pr, e *etree.Element) {
	order := []string{"xfrm", "prstGeom", "custGeom", "noFill", "solidFill", "gradFill", "blipFill", "pattFill", "grpFill", "ln", "effectLst", "effectDag", "scene3d", "sp3d", "extLst"}
	rank := func(s string) int {
		for i, v := range order {
			if s == v {
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
func styleDrawing(shape *etree.Element, s *DrawingStyle) error {
	if s == nil {
		return fmt.Errorf("drawing_style required")
	}
	if s.Pattern != nil || s.Image != nil {
		return fmt.Errorf("pattern/image fill requires package-aware styling")
	}
	pr := office.Child(shape, pns, "spPr")
	if pr == nil {
		return fmt.Errorf("object has no drawing properties")
	}
	if s.Fill != "" && s.Gradient != nil {
		return fmt.Errorf("fill and gradient are mutually exclusive")
	}
	if s.Fill != "" || s.Gradient != nil {
		for _, c := range pr.ChildElements() {
			if strings.HasSuffix(c.Tag, "Fill") {
				pr.RemoveChild(c)
			}
		}
		var fill *etree.Element
		if s.Gradient != nil {
			g := s.Gradient
			if len(g.Stops) < 2 || len(g.Stops) > 100 || !finite(g.Angle) {
				return fmt.Errorf("gradient needs 2..100 stops and finite angle")
			}
			fill = etree.NewElement("a:gradFill")
			stops := fill.CreateElement("a:gsLst")
			last := -1.0
			for _, s := range g.Stops {
				if !finite(s.Position) || s.Position < 0 || s.Position > 1 || s.Position < last {
					return fmt.Errorf("gradient stops must be ascending in 0..1")
				}
				last = s.Position
				stop := stops.CreateElement("a:gs")
				stop.CreateAttr("pos", strconv.Itoa(int(s.Position*100000)))
				if err := rgbaElement(stop, s.Color, s.Opacity); err != nil {
					return err
				}
			}
			angle := int64(g.Angle*60000) % 21600000
			if angle < 0 {
				angle += 21600000
			}
			if g.Path == "" {
				if g.Focus != nil {
					return fmt.Errorf("gradient focus requires a path gradient")
				}
				fill.CreateElement("a:lin").CreateAttr("ang", strconv.FormatInt(angle, 10))
			} else {
				switch g.Path {
				case "circle", "rect", "shape":
				default:
					return fmt.Errorf("invalid gradient path")
				}
				if g.Angle != 0 {
					return fmt.Errorf("path gradient does not accept angle")
				}
				focus := g.Focus
				if focus == nil {
					focus = &GradientFocus{Left: .5, Top: .5, Right: .5, Bottom: .5}
				}
				path := fill.CreateElement("a:path")
				path.CreateAttr("path", g.Path)
				rect := path.CreateElement("a:fillToRect")
				for _, v := range []struct {
					k string
					v float64
				}{{"l", focus.Left}, {"t", focus.Top}, {"r", focus.Right}, {"b", focus.Bottom}} {
					if !finite(v.v) || v.v < 0 || v.v > 1 {
						return fmt.Errorf("gradient focus insets must be in 0..1")
					}
					rect.CreateAttr(v.k, strconv.Itoa(int(v.v*100000)))
				}
				if focus.Left+focus.Right > 1 || focus.Top+focus.Bottom > 1 {
					return fmt.Errorf("gradient focus rectangle is inverted")
				}
			}
		} else {
			fill = etree.NewElement("a:solidFill")
			if err := rgbaElement(fill, s.Fill, s.Opacity); err != nil {
				return err
			}
		}
		insertDrawingProperty(pr, fill)
	}
	if s.Outline != nil {
		l := s.Outline
		if !finite(l.Width) || l.Width < 0 || l.Width > 100 {
			return fmt.Errorf("outline width must be 0..100 points")
		}
		if old := office.Child(pr, ans, "ln"); old != nil {
			pr.RemoveChild(old)
		}
		line := etree.NewElement("a:ln")
		line.CreateAttr("w", strconv.FormatInt(int64(l.Width*12700), 10))
		if err := rgbaElement(line.CreateElement("a:solidFill"), l.Color, nil); err != nil {
			return err
		}
		if l.Dash != "" {
			switch l.Dash {
			case "solid", "dot", "dash", "lgDash", "dashDot", "lgDashDot", "lgDashDotDot", "sysDash", "sysDot", "sysDashDot", "sysDashDotDot":
			default:
				return fmt.Errorf("invalid line dash")
			}
			line.CreateElement("a:prstDash").CreateAttr("val", l.Dash)
		}
		for _, v := range []struct{ tag, value, width, length string }{{"headEnd", l.Head, l.HeadWidth, l.HeadLength}, {"tailEnd", l.Tail, l.TailWidth, l.TailLength}} {
			if v.value == "" && (v.width != "" || v.length != "") {
				return fmt.Errorf("arrow sizing requires an explicit arrow type")
			}
			for _, size := range []string{v.width, v.length} {
				if size != "" && size != "sm" && size != "med" && size != "lg" {
					return fmt.Errorf("arrow size must be sm, med or lg")
				}
			}
			if v.value != "" {
				switch v.value {
				case "none", "triangle", "stealth", "diamond", "oval", "arrow":
				default:
					return fmt.Errorf("invalid arrow type")
				}
				end := line.CreateElement("a:" + v.tag)
				end.CreateAttr("type", v.value)
				if v.width != "" {
					end.CreateAttr("w", v.width)
				}
				if v.length != "" {
					end.CreateAttr("len", v.length)
				}
			}
		}
		insertDrawingProperty(pr, line)
	}
	if s.Shadow != nil {
		v := s.Shadow
		if !finite(v.Blur) || !finite(v.Distance) || !finite(v.Direction) || v.Blur < 0 || v.Distance < 0 || v.Blur > 100 || v.Distance > 100 {
			return fmt.Errorf("invalid shadow")
		}
		for _, tag := range []string{"effectLst", "effectDag"} {
			if c := office.Child(pr, ans, tag); c != nil {
				pr.RemoveChild(c)
			}
		}
		effects := etree.NewElement("a:effectLst")
		shadow := effects.CreateElement("a:outerShdw")
		shadow.CreateAttr("blurRad", strconv.FormatInt(int64(v.Blur*12700), 10))
		shadow.CreateAttr("dist", strconv.FormatInt(int64(v.Distance*12700), 10))
		angle := int64(v.Direction*60000) % 21600000
		if angle < 0 {
			angle += 21600000
		}
		shadow.CreateAttr("dir", strconv.FormatInt(angle, 10))
		shadow.CreateAttr("rotWithShape", "0")
		if err := rgbaElement(shadow, v.Color, v.Opacity); err != nil {
			return err
		}
		insertDrawingProperty(pr, effects)
	}
	return nil
}
