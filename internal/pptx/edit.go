package pptx

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/beevik/etree"
	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
)

// EditSpec makes bounded edits by stable shape ID, never reconstructing a deck.
type EditSpec struct {
	Edits []Edit `json:"edits"`
}
type Edit struct {
	EndRow         int             `json:"end_row,omitempty"`
	EndColumn      int             `json:"end_column,omitempty"`
	Paragraphs     []Paragraph     `json:"paragraphs,omitempty"`
	ParagraphStyle *ParagraphStyle `json:"paragraph_style,omitempty"`
	Hyperlink      *string         `json:"hyperlink,omitempty"`
	Underline      *bool           `json:"underline,omitempty"`
	Mask           string          `json:"mask,omitempty"`
	DrawingStyle   *DrawingStyle   `json:"drawing_style,omitempty"`
	Part           string          `json:"part,omitempty"`
	Layout         *Layout         `json:"layout,omitempty"`
	Slide          int             `json:"slide"`
	ShapeID        string          `json:"shape_id"`
	Operation      string          `json:"operation"`
	Paragraph      int             `json:"paragraph"`
	Run            int             `json:"run"`
	Match          string          `json:"match"`
	Occurrence     int             `json:"occurrence"`
	Row            int             `json:"row"`
	Column         int             `json:"column"`
	Text           string          `json:"text"`
	X              *float64        `json:"x,omitempty"`
	Y              *float64        `json:"y,omitempty"`
	W              *float64        `json:"w,omitempty"`
	H              *float64        `json:"h,omitempty"`
	Rotation       *float64        `json:"rotation,omitempty"`
	FlipH          *bool           `json:"flip_h,omitempty"`
	FlipV          *bool           `json:"flip_v,omitempty"`
	Font           string          `json:"font,omitempty"`
	FontSize       *float64        `json:"font_size,omitempty"`
	Bold           *bool           `json:"bold,omitempty"`
	Italic         *bool           `json:"italic,omitempty"`
	Color          string          `json:"color,omitempty"`
	Fill           string          `json:"fill,omitempty"`
	Align          string          `json:"align,omitempty"`
	Path           string          `json:"path,omitempty"`
	Crop           *Crop           `json:"crop,omitempty"`
}

func EditFile(ctx context.Context, input, output string, spec EditSpec) ([]string, error) {
	if len(spec.Edits) < 1 || len(spec.Edits) > 10000 {
		return nil, fmt.Errorf("edits must contain 1..10000 operations")
	}
	p, err := opc.Open(input)
	if err != nil {
		return nil, err
	}
	if _, err = office.PackageReportOf(p); err != nil {
		return nil, err
	}
	slides, err := office.PresentationSlides(p)
	if err != nil {
		return nil, err
	}
	before := map[string]string{}
	for _, part := range p.Parts {
		before[part.Name] = opc.Hash(part.Data)
	}
	dirty := map[string]*etree.Document{}
	var changed []string
	load := func(name string) (*etree.Document, error) {
		if d := dirty[name]; d != nil {
			return d, nil
		}
		d, e := p.XML(name)
		if e != nil {
			return nil, e
		}
		dirty[name] = d
		changed = append(changed, name)
		return d, nil
	}
	for _, op := range spec.Edits {
		if err = ctx.Err(); err != nil {
			return nil, err
		}
		if op.Part == "" && (op.Slide < 1 || op.Slide > len(slides)) {
			return nil, fmt.Errorf("slide index must be 1..%d", len(slides))
		}
		if len(op.Text) > 1<<20 {
			return nil, fmt.Errorf("text exceeds 1 MiB")
		}
		part := op.Part
		if part == "" {
			part = slides[op.Slide-1]
		} else {
			switch p.ContentType(part) {
			case "application/vnd.openxmlformats-officedocument.presentationml.slideMaster+xml", "application/vnd.openxmlformats-officedocument.presentationml.slideLayout+xml":
			default:
				return nil, fmt.Errorf("part must identify an existing slide master or layout")
			}
			if op.Operation == "notes" {
				return nil, fmt.Errorf("notes apply to slides only")
			}
		}
		if op.Operation == "notes" {
			rels, e := p.Rels(part)
			if e != nil {
				return nil, e
			}
			notes := ""
			for _, r := range rels {
				if r.Type == rns+"/notesSlide" && r.TargetMode != "External" {
					if notes != "" {
						return nil, fmt.Errorf("ambiguous notes relationship")
					}
					notes = opc.Resolve(part, r.Target)
				}
			}
			if notes == "" {
				notes, e = addNotes(p, part, op.Text)
				if e != nil {
					return nil, e
				}
			}
			d, e := load(notes)
			if e != nil {
				return nil, e
			}
			var bodies []*etree.Element
			for _, sp := range office.All(d.Root(), office.NSP, "sp") {
				nv := office.Child(sp, office.NSP, "nvSpPr")
				ph := office.Child(office.Child(nv, office.NSP, "nvPr"), office.NSP, "ph")
				if ph != nil && ph.SelectAttrValue("type", "") == "body" {
					body := office.Child(sp, office.NSP, "txBody")
					if body != nil {
						bodies = append(bodies, body)
					}
				}
			}
			if len(bodies) != 1 {
				return nil, fmt.Errorf("notes must have exactly one body placeholder")
			}
			if op.Paragraphs != nil {
				if op.Text != "" {
					return nil, fmt.Errorf("notes text and paragraphs are mutually exclusive")
				}
				if err := replaceRichNotes(bodies[0], op.Paragraphs, "Arial"); err != nil {
					return nil, err
				}
			} else {
				replaceBody(bodies[0], op.Text)
			}
			continue
		}
		d, e := load(part)
		if e != nil {
			return nil, e
		}
		if op.Operation == "layout" {
			if err := layoutObjects(p, part, d.Root(), op.Layout); err != nil {
				return nil, err
			}
			continue
		}
		var matches []*etree.Element
		for _, prop := range office.All(d.Root(), office.NSP, "cNvPr") {
			if prop.SelectAttrValue("id", "") == op.ShapeID {
				matches = append(matches, prop)
			}
		}
		if len(matches) != 1 {
			return nil, fmt.Errorf("shape_id must identify exactly one shape")
		}
		shape := matches[0].Parent().Parent()
		switch op.Operation {
		case "table-merge", "table-unmerge":
			if err := mergeTableCells(shape, op); err != nil {
				return nil, err
			}
		case "table-row-insert", "table-row-delete", "table-column-insert", "table-column-delete", "table-cell-format":
			if err := tableObject(shape, op); err != nil {
				return nil, err
			}
		case "drawing-style":
			if err := styleDrawingInPackage(p, part, shape, op.DrawingStyle, ""); err != nil {
				return nil, err
			}
		case "image-mask", "geometry", "delete", "front", "back", "run-format", "paragraph-format", "fill", "replace-image", "crop":
			if e := editObject(p, part, shape, op); e != nil {
				return nil, e
			}
		case "text", "run-text", "replace-text":
			body := office.Child(shape, office.NSP, "txBody")
			if body == nil {
				return nil, fmt.Errorf("shape has no editable text body")
			}
			paras := office.Children(body, office.NSA, "p")
			if op.Paragraph < 0 || op.Paragraph >= len(paras) {
				return nil, fmt.Errorf("paragraph index out of range")
			}
			if strings.ContainsAny(op.Text, "\r\n") {
				return nil, fmt.Errorf("text edits target one paragraph; newline not permitted")
			}
			if op.Operation == "replace-text" {
				if err := replaceRunMatch(ctx, paras[op.Paragraph], op.Match, op.Text, op.Occurrence); err != nil {
					return nil, err
				}
			} else if op.Operation == "run-text" {
				runs := office.Children(paras[op.Paragraph], office.NSA, "r")
				if op.Run < 0 || op.Run >= len(runs) {
					return nil, fmt.Errorf("run index out of range")
				}
				texts := office.Children(runs[op.Run], office.NSA, "t")
				if len(texts) != 1 {
					return nil, fmt.Errorf("run must contain exactly one text element")
				}
				texts[0].SetText(op.Text)
			} else {
				replaceParagraph(paras[op.Paragraph], op.Text)
			}
		case "table-cell":
			tables := office.All(shape, office.NSA, "tbl")
			if len(tables) != 1 {
				return nil, fmt.Errorf("shape must contain exactly one table")
			}
			rows := office.Children(tables[0], office.NSA, "tr")
			if op.Row < 0 || op.Row >= len(rows) {
				return nil, fmt.Errorf("row index out of range")
			}
			cells := office.Children(rows[op.Row], office.NSA, "tc")
			if op.Column < 0 || op.Column >= len(cells) {
				return nil, fmt.Errorf("column index out of range")
			}
			if mergedContinuation(cells[op.Column]) {
				return nil, fmt.Errorf("edit the top-left merge origin instead of a covered cell")
			}
			body := office.Child(cells[op.Column], office.NSA, "txBody")
			if body == nil {
				return nil, fmt.Errorf("cell has no text body")
			}
			replaceBody(body, op.Text)
		default:
			return nil, fmt.Errorf("unsupported slide edit operation %q", op.Operation)
		}
	}
	for _, part := range changed {
		if err = p.SetXML(part, dirty[part]); err != nil {
			return nil, err
		}
	}
	if err = p.SaveNew(output); err != nil {
		return nil, err
	}
	changed = nil
	for _, part := range p.Parts {
		if before[part.Name] != opc.Hash(part.Data) {
			changed = append(changed, part.Name)
		}
	}
	sort.Strings(changed)
	return changed, nil
}
func replaceParagraph(p *etree.Element, text string) {
	// Explicit replacement of this paragraph's content retains paragraph and first
	// run styling; fields in the replaced paragraph become literal text.
	var style *etree.Element
	for _, r := range office.Children(p, office.NSA, "r") {
		if pr := office.Child(r, office.NSA, "rPr"); pr != nil {
			style = pr.Copy()
			break
		}
	}
	for _, e := range p.ChildElements() {
		if !office.Is(e, office.NSA, "pPr") && !office.Is(e, office.NSA, "endParaRPr") {
			p.RemoveChild(e)
		}
	}
	r := etree.NewElement("a:r")
	ensureDrawingPrefix(r, p)
	if style != nil {
		r.AddChild(style)
	}
	r.CreateElement("a:t").SetText(text)
	if end := office.Child(p, office.NSA, "endParaRPr"); end != nil {
		p.InsertChildAt(end.Index(), r)
	} else {
		p.AddChild(r)
	}
}
func replaceBody(body *etree.Element, text string) {
	paras := office.Children(body, office.NSA, "p")
	var prototype *etree.Element
	if len(paras) > 0 {
		prototype = paras[0].Copy()
	}
	for _, p := range paras {
		body.RemoveChild(p)
	}
	for _, line := range strings.Split(text, "\n") {
		var p *etree.Element
		if prototype != nil {
			p = prototype.Copy()
		} else {
			p = etree.NewElement("a:p")
			p.CreateAttr("xmlns:a", ans)
		}
		body.AddChild(p)
		replaceParagraph(p, line)
	}
}

// ensureDrawingPrefix preserves an existing in-scope DrawingML binding instead
// of serializing a redundant declaration on every replacement run. A detached
// paragraph or a different binding still needs its own declaration.
func ensureDrawingPrefix(element, parent *etree.Element) {
	for p := parent; p != nil; p = p.Parent() {
		for _, attr := range p.Attr {
			if attr.Space == "xmlns" && attr.Key == "a" {
				if attr.Value != ans {
					element.CreateAttr("xmlns:a", ans)
				}
				return
			}
		}
	}
	element.CreateAttr("xmlns:a", ans)
}

// replaceRunMatch only searches consecutive ordinary runs. Fields and breaks
// are boundaries, never flattened into editable literal text.
func replaceRunMatch(ctx context.Context, p *etree.Element, match, text string, occurrence int) error {
	if match == "" || len(match) > 1<<20 || strings.ContainsAny(match, "\r\n") || occurrence < 0 {
		return fmt.Errorf("match must be nonempty single-line text up to 1 MiB and occurrence nonnegative")
	}
	var nodes []*etree.Element
	var joined strings.Builder
	seen := 0
	apply := func() bool {
		value := joined.String()
		for offset := 0; offset <= len(value); {
			i := strings.Index(value[offset:], match)
			if i < 0 {
				break
			}
			start := offset + i
			end := start + len(match)
			if seen == occurrence {
				pos := 0
				inserted := false
				for _, n := range nodes {
					old := n.Text()
					next := pos + len(old)
					if pos < end && next > start {
						lo, hi := max(0, start-pos), min(len(old), end-pos)
						replacement := ""
						if !inserted {
							replacement = text
							inserted = true
						}
						n.SetText(old[:lo] + replacement + old[hi:])
					}
					pos = next
				}
				return true
			}
			seen++
			offset = end
		}
		return false
	}
	for _, child := range p.ChildElements() {
		if err := ctx.Err(); err != nil {
			return err
		}
		if office.Is(child, office.NSA, "r") {
			ts := office.Children(child, office.NSA, "t")
			if len(ts) != 1 {
				return fmt.Errorf("run must contain exactly one text element")
			}
			if joined.Len()+len(ts[0].Text()) > 1<<20 {
				return fmt.Errorf("searchable run segment exceeds 1 MiB")
			}
			nodes = append(nodes, ts[0])
			joined.WriteString(ts[0].Text())
		} else {
			if apply() {
				return nil
			}
			nodes = nil
			joined.Reset()
		}
	}
	if apply() {
		return nil
	}
	return fmt.Errorf("match occurrence not found in consecutive ordinary runs")
}
