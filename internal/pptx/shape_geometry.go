package pptx

import (
	"fmt"
	"github.com/beevik/etree"
	"github.com/openvibely/openvibely-paperworks/internal/office"
	"regexp"
	"strings"
)

type ShapeAdjustment struct {
	Name    string `json:"name"`
	Formula string `json:"formula"`
}
type ShapeGeometry struct {
	Preset       string            `json:"preset,omitempty"`
	CornerRadius *float64          `json:"corner_radius_points,omitempty"`
	Adjustments  []ShapeAdjustment `json:"adjustments,omitempty"`
}

var guideName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
var guideArg = regexp.MustCompile(`^-?[0-9]+$|^[A-Za-z_][A-Za-z0-9_]*$`)

func setShapeGeometry(shape *etree.Element, s *ShapeGeometry) error {
	if s == nil || !office.Is(shape, pns, "sp") {
		return fmt.Errorf("shape-geometry requires a shape and shape_geometry")
	}
	if s.CornerRadius != nil && s.Adjustments != nil {
		return fmt.Errorf("radius and adjustments are mutually exclusive")
	}
	seen := map[string]bool{}
	arities := map[string]int{"val": 1, "abs": 1, "sqrt": 1, "+-": 3, "*/": 3, "+/": 3, "?:": 3, "at2": 2, "cat2": 3, "cos": 2, "max": 2, "min": 2, "mod": 3, "pin": 3, "sat2": 3, "sin": 2, "tan": 2}
	for _, a := range s.Adjustments {
		parts := strings.Fields(a.Formula)
		if !guideName.MatchString(a.Name) || seen[a.Name] || len(parts) == 0 || arities[parts[0]] == 0 || len(parts) != arities[parts[0]]+1 {
			return fmt.Errorf("invalid shape adjustment %q", a.Name)
		}
		for _, v := range parts[1:] {
			if !guideArg.MatchString(v) {
				return fmt.Errorf("invalid adjustment formula")
			}
		}
		seen[a.Name] = true
	}
	pr := office.Child(shape, pns, "spPr")
	preset := s.Preset
	if preset == "" {
		if g := office.Child(pr, ans, "prstGeom"); g != nil {
			preset = g.SelectAttrValue("prst", "")
		}
	}
	if err := setPresetGeometry(shape, preset, s.CornerRadius); err != nil {
		return err
	}
	if s.Adjustments != nil {
		geom := office.Child(pr, ans, "prstGeom")
		if old := office.Child(geom, ans, "avLst"); old != nil {
			geom.RemoveChild(old)
		}
		av := etree.NewElement("a:avLst")
		av.CreateAttr("xmlns:a", ans)
		geom.InsertChildAt(0, av)
		for _, a := range s.Adjustments {
			g := av.CreateElement("a:gd")
			g.CreateAttr("name", a.Name)
			g.CreateAttr("fmla", a.Formula)
		}
	}
	return nil
}
