package pptx

import (
	"fmt"
	"github.com/beevik/etree"
	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"math"
	"strconv"
)

// AutoLayout preserves object sizes and arranges them in a frame, in inches.
type AutoLayout struct {
	ShapeIDs     []string `json:"shape_ids"`
	Direction    string   `json:"direction,omitempty"` // horizontal (default), vertical
	Align        string   `json:"align,omitempty"`     // center (default) or one of eight compass positions
	X            float64  `json:"x,omitempty"`
	Y            float64  `json:"y,omitempty"`
	Width        float64  `json:"width,omitempty"`
	Height       float64  `json:"height,omitempty"`
	FrameShapeID string   `json:"frame_shape_id,omitempty"`
	Gap          *float64 `json:"gap,omitempty"`
	AutoGap      bool     `json:"auto_gap,omitempty"`
	PaddingX     float64  `json:"padding_x,omitempty"`
	PaddingY     float64  `json:"padding_y,omitempty"`
}
type drawingRect struct{ x, y, w, h float64 }

func explicitDrawingRect(shape *etree.Element) (drawingRect, error) {
	pr := office.Child(shape, pns, "spPr")
	if pr == nil {
		pr = office.Child(shape, pns, "grpSpPr")
	}
	xf := office.Child(pr, ans, "xfrm")
	if xf == nil {
		xf = office.Child(shape, pns, "xfrm")
	}
	off, ext := office.Child(xf, ans, "off"), office.Child(xf, ans, "ext")
	r := drawingRect{}
	for _, v := range []struct {
		e    *etree.Element
		key  string
		dest *float64
	}{{off, "x", &r.x}, {off, "y", &r.y}, {ext, "cx", &r.w}, {ext, "cy", &r.h}} {
		if v.e == nil {
			return r, fmt.Errorf("auto-layout requires explicit object geometry")
		}
		n, err := strconv.ParseFloat(v.e.SelectAttrValue(v.key, ""), 64)
		if err != nil || !finite(n) {
			return r, fmt.Errorf("invalid object geometry")
		}
		*v.dest = n / 914400
	}
	if r.w <= 0 || r.h <= 0 {
		return r, fmt.Errorf("auto-layout requires positive object sizes")
	}
	return r, nil
}
func autoLayoutObjects(pkg *opc.Package, part string, root *etree.Element, l *AutoLayout) error {
	if l == nil || len(l.ShapeIDs) == 0 || len(l.ShapeIDs) > 10000 {
		return fmt.Errorf("auto_layout needs 1..10000 shape_ids")
	}
	vertical := l.Direction == "vertical"
	if l.Direction != "" && l.Direction != "horizontal" && !vertical {
		return fmt.Errorf("direction must be horizontal or vertical")
	}
	align := l.Align
	if align == "" {
		align = "center"
	}
	factors := map[string][2]float64{"center": {.5, .5}, "topLeft": {0, 0}, "topCenter": {.5, 0}, "topRight": {1, 0}, "left": {0, .5}, "right": {1, .5}, "bottomLeft": {0, 1}, "bottomCenter": {.5, 1}, "bottomRight": {1, 1}}
	factor, ok := factors[align]
	if !ok {
		return fmt.Errorf("invalid auto-layout alignment")
	}
	for _, v := range []float64{l.X, l.Y, l.Width, l.Height, l.PaddingX, l.PaddingY} {
		if !finite(v) {
			return fmt.Errorf("invalid auto-layout geometry")
		}
	}
	if l.PaddingX < 0 || l.PaddingY < 0 {
		return fmt.Errorf("padding must be nonnegative")
	}
	gap := 0.0
	if l.Gap != nil {
		gap = *l.Gap
		if l.AutoGap {
			return fmt.Errorf("gap and auto_gap are mutually exclusive")
		}
		if !finite(gap) || gap < 0 {
			return fmt.Errorf("gap must be finite and nonnegative")
		}
	}
	shapes := map[string]*etree.Element{}
	for _, pr := range office.All(root, pns, "cNvPr") {
		id := pr.SelectAttrValue("id", "")
		if _, exists := shapes[id]; exists {
			return fmt.Errorf("ambiguous shape id %s", id)
		}
		shapes[id] = pr.Parent().Parent()
	}
	frame := drawingRect{l.X, l.Y, l.Width, l.Height}
	if l.FrameShapeID != "" {
		if l.X != 0 || l.Y != 0 || l.Width != 0 || l.Height != 0 {
			return fmt.Errorf("frame_shape_id cannot be combined with frame coordinates")
		}
		shape := shapes[l.FrameShapeID]
		if shape == nil || shape.Parent() == nil || shape.Parent().Tag != "spTree" {
			return fmt.Errorf("frame_shape_id must identify a top-level shape")
		}
		var err error
		frame, err = explicitDrawingRect(shape)
		if err != nil {
			return err
		}
	} else if l.Width == 0 && l.Height == 0 {
		if l.X != 0 || l.Y != 0 {
			return fmt.Errorf("explicit frame requires width and height")
		}
		main, err := presentationPart(pkg)
		if err != nil {
			return err
		}
		d, err := pkg.XML(main)
		if err != nil {
			return err
		}
		sz := office.Child(d.Root(), pns, "sldSz")
		if sz == nil {
			return fmt.Errorf("missing slide size")
		}
		for _, v := range []struct {
			key  string
			dest *float64
		}{{"cx", &frame.w}, {"cy", &frame.h}} {
			n, err := strconv.ParseFloat(sz.SelectAttrValue(v.key, ""), 64)
			if err != nil || !finite(n) {
				return fmt.Errorf("invalid slide size")
			}
			*v.dest = n / 914400
		}
	}
	frame.x += l.PaddingX
	frame.y += l.PaddingY
	frame.w -= 2 * l.PaddingX
	frame.h -= 2 * l.PaddingY
	if frame.w <= 0 || frame.h <= 0 {
		return fmt.Errorf("padding leaves no usable frame")
	}
	var rects []drawingRect
	seen := map[string]bool{}
	total := 0.0
	cross := 0.0
	for _, id := range l.ShapeIDs {
		shape := shapes[id]
		if shape == nil || seen[id] || id == l.FrameShapeID {
			return fmt.Errorf("missing, duplicate or frame shape in layout")
		}
		seen[id] = true
		if shape.Parent() == nil || shape.Parent().Tag != "spTree" {
			return fmt.Errorf("auto-layout requires top-level shapes")
		}
		r, err := explicitDrawingRect(shape)
		if err != nil {
			return err
		}
		rects = append(rects, r)
		if vertical {
			total += r.h
			cross = math.Max(cross, r.w)
		} else {
			total += r.w
			cross = math.Max(cross, r.h)
		}
	}
	available, crossAvailable := frame.w, frame.h
	if vertical {
		available, crossAvailable = frame.h, frame.w
	}
	if total > available+1e-8 || cross > crossAvailable+1e-8 {
		return fmt.Errorf("objects exceed auto-layout frame")
	}
	if l.AutoGap && len(rects) > 1 {
		gap = math.Max(0, (available-total)/float64(len(rects)-1))
	}
	total += float64(len(rects)-1) * gap
	if total > available+1e-8 {
		return fmt.Errorf("gaps exceed auto-layout frame")
	}
	offset := (available - total) * factor[0]
	if vertical {
		offset = (available - total) * factor[1]
	}
	for i, r := range rects {
		x, y := frame.x+offset, frame.y+(frame.h-r.h)*factor[1]
		if vertical {
			x = frame.x + (frame.w-r.w)*factor[0]
			y = frame.y + offset
			offset += r.h + gap
		} else {
			offset += r.w + gap
		}
		if err := editObject(pkg, part, shapes[l.ShapeIDs[i]], Edit{Operation: "geometry", X: &x, Y: &y}); err != nil {
			return err
		}
	}
	return nil
}
