package pptx

import (
	"fmt"
	"github.com/beevik/etree"
	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
)

// LayoutNode describes either a shape or a container. Unspecified dimensions
// fill the available space; row/column children share it according to Weight.
// Fixed dimensions, gaps, and padding are measured in inches.
type LayoutNode struct {
	FitText    bool          `json:"fit_text,omitempty"`
	ColumnSpan int           `json:"column_span,omitempty"`
	RowSpan    int           `json:"row_span,omitempty"`
	ShapeID    string        `json:"shape_id,omitempty"`
	Mode       string        `json:"mode,omitempty"`
	Children   []*LayoutNode `json:"children,omitempty"`
	Width      *float64      `json:"width,omitempty"`
	Height     *float64      `json:"height,omitempty"`
	Weight     float64       `json:"weight,omitempty"`
	Gap        float64       `json:"gap,omitempty"`
	Padding    float64       `json:"padding,omitempty"`
	Columns    int           `json:"columns,omitempty"`
}

type layoutPlacement struct {
	shape      *etree.Element
	x, y, w, h float64
}

func layoutTree(pkg *opc.Package, part string, root *etree.Element, l *Layout) error {
	if l.Root.ColumnSpan != 0 || l.Root.RowSpan != 0 {
		return fmt.Errorf("spans require a grid parent")
	}
	if len(l.ShapeIDs) > 0 || l.Mode != "" || l.Gap != 0 || l.Columns != 0 {
		return fmt.Errorf("root cannot be combined with flat layout fields")
	}
	if !finite(l.X) || !finite(l.Y) || !finite(l.Width) || !finite(l.Height) || l.Width <= 0 || l.Height <= 0 {
		return fmt.Errorf("invalid layout frame")
	}
	shapes := map[string]*etree.Element{}
	for _, e := range office.All(root, pns, "cNvPr") {
		id := e.SelectAttrValue("id", "")
		if _, ok := shapes[id]; ok {
			return fmt.Errorf("ambiguous shape id %s", id)
		}
		shapes[id] = e.Parent().Parent()
	}
	seen := map[string]bool{}
	nodes := 0
	var placements []layoutPlacement
	var visit func(*LayoutNode, float64, float64, float64, float64, int) error
	visit = func(n *LayoutNode, x, y, w, h float64, depth int) error {
		nodes++
		if n == nil || depth > 32 || nodes > 10000 {
			return fmt.Errorf("layout exceeds node/depth limits or contains null node")
		}
		if !finite(n.Gap) || !finite(n.Padding) || !finite(n.Weight) || n.Gap < 0 || n.Padding < 0 || n.Weight < 0 {
			return fmt.Errorf("invalid layout spacing/weight")
		}
		for _, d := range []*float64{n.Width, n.Height} {
			if d != nil && (!finite(*d) || *d <= 0) {
				return fmt.Errorf("fixed dimensions must be positive")
			}
		}
		if n.ColumnSpan < 0 || n.RowSpan < 0 {
			return fmt.Errorf("spans must be positive")
		}
		if n.FitText && (n.ShapeID == "" || n.Height != nil) {
			return fmt.Errorf("fit_text requires a shape without fixed height")
		}
		if n.Width != nil {
			if *n.Width > w+1e-9 {
				return fmt.Errorf("fixed width exceeds available space")
			}
			w = *n.Width
		}
		if n.Height != nil {
			if *n.Height > h+1e-9 {
				return fmt.Errorf("fixed height exceeds available space")
			}
			h = *n.Height
		}
		if !finite(x) || !finite(y) || !finite(w) || !finite(h) || w <= 0 || h <= 0 {
			return fmt.Errorf("layout has no usable space")
		}
		if n.ShapeID != "" {
			if len(n.Children) > 0 || n.Mode != "" || n.Gap != 0 || n.Padding != 0 || n.Columns != 0 {
				return fmt.Errorf("shape leaf cannot also be a container")
			}
			shape := shapes[n.ShapeID]
			if shape == nil || seen[n.ShapeID] {
				return fmt.Errorf("missing or duplicate layout shape %s", n.ShapeID)
			}
			// Nested group coordinates require a separate transform; do not pretend
			// slide-space coordinates can safely be assigned to grouped children.
			if shape.Parent() == nil || shape.Parent().Tag != "spTree" {
				return fmt.Errorf("layout requires top-level slide shapes")
			}
			if n.FitText {
				measured, err := office.SlideTextHeight(shape, w)
				if err != nil {
					return fmt.Errorf("shape %s: %w", n.ShapeID, err)
				}
				if measured > h+1e-9 {
					return fmt.Errorf("shape %s text exceeds layout height", n.ShapeID)
				}
				h = measured
			}
			seen[n.ShapeID] = true
			placements = append(placements, layoutPlacement{shape, x, y, w, h})
			return nil
		}
		if len(n.Children) == 0 {
			return fmt.Errorf("layout container needs children")
		}
		x += n.Padding
		y += n.Padding
		w -= 2 * n.Padding
		h -= 2 * n.Padding
		if w <= 0 || h <= 0 {
			return fmt.Errorf("padding exceeds layout dimensions")
		}
		if n.Mode == "grid" {
			cells, rows, err := gridCells(n)
			if err != nil {
				return err
			}
			cw := (w - float64(n.Columns-1)*n.Gap) / float64(n.Columns)
			ch := (h - float64(rows-1)*n.Gap) / float64(rows)
			for i, c := range n.Children {
				g := cells[i]
				if err := visit(c, x+float64(g.col)*(cw+n.Gap), y+float64(g.row)*(ch+n.Gap), float64(g.cols)*cw+float64(g.cols-1)*n.Gap, float64(g.rows)*ch+float64(g.rows-1)*n.Gap, depth+1); err != nil {
					return err
				}
			}
			return nil
		}
		if n.Mode != "row" && n.Mode != "column" {
			return fmt.Errorf("container mode must be row, column or grid")
		}
		if n.Columns != 0 {
			return fmt.Errorf("columns applies only to grid")
		}
		available := w
		if n.Mode == "column" {
			available = h
		}
		available -= float64(len(n.Children)-1) * n.Gap
		totalWeight := 0.0
		measuredHeights := map[*LayoutNode]float64{}
		for _, c := range n.Children {
			if c == nil {
				return fmt.Errorf("null layout child")
			}
			if c.ColumnSpan != 0 || c.RowSpan != 0 {
				return fmt.Errorf("spans require a grid parent")
			}
			fixed := c.Width
			if n.Mode == "column" {
				fixed = c.Height
				if c.FitText {
					if c.Height != nil || c.ShapeID == "" {
						return fmt.Errorf("fit_text requires a shape without fixed height")
					}
					shape := shapes[c.ShapeID]
					if shape == nil {
						return fmt.Errorf("missing shape %s", c.ShapeID)
					}
					width := w
					if c.Width != nil {
						width = *c.Width
					}
					v, err := office.SlideTextHeight(shape, width)
					if err != nil {
						return err
					}
					measuredHeights[c] = v
					fixed = &v
				}
			}
			if fixed != nil {
				if !finite(*fixed) || *fixed <= 0 {
					return fmt.Errorf("invalid fixed dimension")
				}
				available -= *fixed
			} else {
				weight := c.Weight
				if weight == 0 {
					weight = 1
				}
				if !finite(weight) || weight < 0 {
					return fmt.Errorf("invalid weight")
				}
				totalWeight += weight
			}
		}
		if available < -1e-9 || !finite(available) || !finite(totalWeight) || totalWeight > 0 && available <= 0 {
			return fmt.Errorf("children and gaps exceed available space")
		}
		for _, c := range n.Children {
			fixed := c.Width
			if n.Mode == "column" {
				fixed = c.Height
				if v, ok := measuredHeights[c]; ok {
					fixed = &v
				}
			}
			size := 0.0
			if fixed != nil {
				size = *fixed
			} else {
				weight := c.Weight
				if weight == 0 {
					weight = 1
				}
				size = available * weight / totalWeight
			}
			cw, ch := w, h
			if n.Mode == "row" {
				cw = size
			} else {
				ch = size
			}
			if err := visit(c, x, y, cw, ch, depth+1); err != nil {
				return err
			}
			if n.Mode == "row" {
				x += size + n.Gap
			} else {
				y += size + n.Gap
			}
		}
		return nil
	}
	if err := visit(l.Root, l.X, l.Y, l.Width, l.Height, 0); err != nil {
		return err
	}
	for _, p := range placements {
		if err := editObject(pkg, part, p.shape, Edit{Operation: "geometry", X: &p.x, Y: &p.y, W: &p.w, H: &p.h}); err != nil {
			return err
		}
	}
	return nil
}

// gridCells packs spans in row-major order without overlaps. Bounds cap work
// before allocating an occupancy grid for untrusted JSON.
type gridCell struct{ row, col, rows, cols int }

func gridCells(n *LayoutNode) ([]gridCell, int, error) {
	if n.Columns < 1 || n.Columns > 10000 || len(n.Children) > 10000 {
		return nil, 0, fmt.Errorf("grid columns/children out of range")
	}
	occupied := map[int]bool{}
	out := make([]gridCell, 0, len(n.Children))
	rows, cursor := 0, 0
	for _, c := range n.Children {
		if c == nil {
			return nil, 0, fmt.Errorf("null grid child")
		}
		rs, cs := c.RowSpan, c.ColumnSpan
		if rs == 0 {
			rs = 1
		}
		if cs == 0 {
			cs = 1
		}
		if rs < 1 || cs < 1 || cs > n.Columns || rs > 10000 || rs*cs > 10000 {
			return nil, 0, fmt.Errorf("invalid grid span")
		}
		placed := false
		for pos := cursor; pos < 10000; pos++ {
			r, col := pos/n.Columns, pos%n.Columns
			if col+cs > n.Columns || (r+rs)*n.Columns > 10000 {
				continue
			}
			free := true
			for y := r; y < r+rs; y++ {
				for x := col; x < col+cs; x++ {
					if occupied[y*n.Columns+x] {
						free = false
					}
				}
			}
			if !free {
				continue
			}
			for y := r; y < r+rs; y++ {
				for x := col; x < col+cs; x++ {
					occupied[y*n.Columns+x] = true
				}
			}
			out = append(out, gridCell{r, col, rs, cs})
			rows = max(rows, r+rs)
			cursor = pos + cs
			placed = true
			break
		}
		if !placed {
			return nil, 0, fmt.Errorf("grid exceeds 10000 cells")
		}
	}
	return out, rows, nil
}
